package service

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// startInternalTestBuild runs macos-test profiles as package-only jobs. Internal
// Test is deliberately not a Release: the project builds one artifact into
// ILS_OUTPUT_DIR, ILS keeps it on the Build Job, and no Release record,
// notarization, App Store submission, or deep bundle validation is performed.
func (a *App) startInternalTestBuild(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	project := r.PathValue("project")
	source, err := a.readSource(project)
	if err != nil {
		fail(w, http.StatusConflict, "请先关联项目目录")
		return
	}
	var input struct {
		Profile string `json:"profile"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || input.Profile == "" {
		fail(w, http.StatusBadRequest, "请选择 Internal Test Profile")
		return
	}
	info, err := inspectSource(source)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if strings.TrimSpace(info.Head) == "" {
		fail(w, http.StatusConflict, "当前 Git 工作目录没有可记录的 HEAD")
		return
	}
	profile, err := a.readReleaseProfile(project, input.Profile)
	if err != nil {
		fail(w, http.StatusNotFound, "ILS Release Profile 不存在")
		return
	}
	profile, err = normalizeReleaseProfile(profile)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if profile.Platform != "macos" || profile.Lane != "macos-test" {
		fail(w, http.StatusBadRequest, "只有 macos-test Profile 可以使用 Internal Test 打包模式")
		return
	}
	if profile.ResultContract != "ils-result-v1" {
		fail(w, http.StatusBadRequest, "Internal Test Profile 必须使用 ils-result-v1")
		return
	}

	buildType := a.readProfileBuildType(project, profile.ID)
	snapshot := snapshotFromSource(info, buildType)
	a.refreshProjectIcons(project, info.Path)

	a.buildMu.Lock()
	if a.activeBuild != "" {
		a.buildMu.Unlock()
		fail(w, http.StatusConflict, "已有构建正在执行，请稍后再打包")
		return
	}
	job := BuildJob{
		ID: randomID(16), ProjectID: project, Mode: "profile", ProfileID: profile.ID,
		Title: profile.Name, Platform: profile.Platform, Lane: profile.Lane,
		ReleaseVariant: profile.Variant, ReleaseChannel: profile.Channel,
		ReleaseArchitecture: profile.Architecture, ReleaseNotes: profile.Notes,
		Status: "running", Stage: "preflight", Commit: snapshot.Head,
		CreatedAt: time.Now().UTC(), ReleaseIDs: []string{},
	}
	if err = os.MkdirAll(filepath.Dir(a.buildJobPath(job.ID)), 0700); err == nil {
		a.snapshotBuildIcon(job.ProjectID, job.Platform, job.ID)
		err = atomicJSON(a.buildJobPath(job.ID), job)
	}
	if err == nil {
		err = atomicJSON(a.buildSourceSnapshotPath(job.ID), snapshot)
	}
	if err != nil {
		a.buildMu.Unlock()
		fail(w, http.StatusInternalServerError, "创建 Internal Test 构建任务失败")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	a.activeBuild = job.ID
	a.cancelBuild = cancel
	a.buildReceipts = map[string][]string{job.ID: {}}
	a.buildMu.Unlock()

	go a.runInternalTestBuild(ctx, cancel, job, source, profile, snapshot)
	respond(w, http.StatusAccepted, buildJobLocalView{
		BuildJob: job, Branch: snapshot.Branch, Head: snapshot.Head,
		Upstream: snapshot.Upstream, Remote: snapshot.Remote,
		Dirty: snapshot.Dirty, BuildType: snapshot.BuildType,
	})
}

func (a *App) runInternalTestBuild(ctx context.Context, cancel context.CancelFunc, job BuildJob, source BuildSource, profile ReleaseProfile, snapshot buildSourceSnapshot) {
	defer func() {
		cancel()
		a.buildMu.Lock()
		a.activeBuild = ""
		a.cancelBuild = nil
		a.buildMu.Unlock()
	}()
	finish := func(status, message string) {
		job.Status = status
		job.Error = message
		now := time.Now().UTC()
		job.FinishedAt = &now
		_ = a.writeBuild(job)
	}

	file, err := os.OpenFile(filepath.Join(a.data, "builds", job.ID, "build.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		finish("failed", "无法创建日志")
		return
	}
	defer file.Close()
	log := &cappedLog{file: file, secret: a.token}
	defer log.Flush()
	commandFailure := func(label string, commandErr error) {
		log.Flush()
		finish("failed", buildFailureMessage(file.Name(), commandErr, ctx, label))
	}

	output := filepath.Join(a.data, "builds", job.ID, "output")
	if err = os.MkdirAll(output, 0700); err != nil {
		finish("failed", "无法创建产物目录")
		return
	}
	dirtyValue := "0"
	if snapshot.Dirty {
		dirtyValue = "1"
	}
	tokenFile := filepath.Join(a.data, "admin-token")
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"LOCALSERVICE_URL="+a.buildOrigin, "LOCALSERVICE_TOKEN_FILE="+tokenFile, "LOCALSERVICE_ROOT="+a.buildRoot,
		"LOCALSERVICE_PROJECT_ID="+job.ProjectID, "LOCALSERVICE_JOB_ID="+job.ID, "LOCALSERVICE_OUTPUT_DIR="+output, "LOCALSERVICE_GIT_COMMIT="+job.Commit,
		"ILS_URL="+a.buildOrigin, "ILS_TOKEN_FILE="+tokenFile, "ILS_ROOT="+a.buildRoot, "ILS_PROJECT_ID="+job.ProjectID,
		"ILS_JOB_ID="+job.ID, "ILS_OUTPUT_DIR="+output, "ILS_GIT_COMMIT="+job.Commit,
		"ILS_GIT_BRANCH="+snapshot.Branch, "ILS_GIT_DIRTY="+dirtyValue, "ILS_GIT_UPSTREAM="+snapshot.Upstream, "ILS_GIT_REMOTE="+snapshot.Remote,
		"ILS_BUILD_TYPE="+snapshot.BuildType,
		"ILS_PLATFORM="+profile.Platform, "ILS_CHANNEL="+profile.Channel, "ILS_ARCHITECTURE="+profile.Architecture,
		"ILS_VARIANT="+profile.Variant, "ILS_LANE="+profile.Lane, "ILS_PUBLISH_MODE=artifact-only",
	)
	fmt.Fprintf(log, "ILS Internal Test package-only build\nBranch: %s\nHEAD: %s\nDirty: %t\nBuild type: %s\nNo Release record will be created. No notarization or deep bundle validation is performed by ILS.\n", snapshot.Branch, snapshot.Head, snapshot.Dirty, snapshot.BuildType)

	var progressMu sync.Mutex
	jobLog := &eventLog{dst: log, secret: a.token, onEvent: func(event BuildEvent) {
		progressMu.Lock()
		defer progressMu.Unlock()
		job.Stage = event.Stage
		job.StageState = event.State
		job.Progress = event.Progress
		job.Message = event.Message
		_ = a.writeBuild(job)
	}}

	job.Stage = "build"
	job.StageState = "running"
	_ = a.writeBuild(job)
	fmt.Fprintf(log, "Single entrypoint: %s\n", profile.BuildCommand)
	if err = runProcess(ctx, source.Path, jobLog, env, "/bin/bash", "-lc", profile.BuildCommand); err != nil {
		commandFailure("Internal Test 打包", err)
		return
	}

	// Internal Test intentionally avoids the normal release validation/publish
	// pipeline. The only required checks are that the script returned a result
	// for this macos-test lane and that its non-empty artifact stays inside the
	// job output directory so it can be downloaded safely.
	result, err := readBuildResult(output)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	if result.Status != "succeeded" || result.Platform != "macos" || result.Lane != "macos-test" {
		finish("failed", "Internal Test 结果必须是 succeeded + macos + macos-test")
		return
	}
	artifact, err := outputArtifact(output, result.Artifact)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	info, err := os.Stat(artifact)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		finish("failed", "Internal Test 打包产物不存在或为空")
		return
	}
	ext := strings.ToLower(filepath.Ext(artifact))
	if ext != ".dmg" && ext != ".pkg" && ext != ".zip" {
		finish("failed", "Internal Test macOS 产物必须是 DMG、PKG 或 ZIP")
		return
	}
	job.Result = &result
	job.Platform = result.Platform
	job.Lane = result.Lane
	job.Stage = "complete"
	job.StageState = "succeeded"
	job.Progress = nil
	job.Message = "Internal Test 安装包已生成，可直接下载"
	finish("succeeded", "")
}

func (a *App) downloadInternalBuildArtifact(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("job")
	if !jobRE.MatchString(id) {
		fail(w, http.StatusNotFound, "任务不存在")
		return
	}
	raw, err := os.ReadFile(a.buildJobPath(id))
	if err != nil {
		fail(w, http.StatusNotFound, "任务不存在")
		return
	}
	var job BuildJob
	if json.Unmarshal(raw, &job) != nil || job.Status != "succeeded" || job.Lane != "macos-test" || job.Result == nil {
		fail(w, http.StatusConflict, "Internal Test 安装包尚未可用")
		return
	}
	output := filepath.Join(a.data, "builds", id, "output")
	artifact, err := outputArtifact(output, job.Result.Artifact)
	if err != nil {
		fail(w, http.StatusNotFound, "Internal Test 安装包不存在")
		return
	}
	file, err := os.Open(artifact)
	if err != nil {
		fail(w, http.StatusNotFound, "Internal Test 安装包不存在")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		fail(w, http.StatusNotFound, "Internal Test 安装包不存在")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(artifact)}))
	http.ServeContent(w, r, filepath.Base(artifact), info.ModTime(), file)
}
