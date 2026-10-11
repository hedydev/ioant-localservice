package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func normalizeSimulatorReleaseProfile(profile ReleaseProfile) (ReleaseProfile, error) {
	if strings.TrimSpace(profile.Platform) != "ios" || strings.TrimSpace(profile.Lane) != "ios-simulator" {
		return normalizeReleaseProfile(profile)
	}
	if strings.TrimSpace(profile.ResultContract) != "ils-result-v1" {
		return profile, fmt.Errorf("iOS Simulator Profile 必须使用 ils-result-v1")
	}
	profile.AppleTeamID = ""
	profile.Lane = "ios-adhoc"
	normalized, err := normalizeReleaseProfile(profile)
	if err != nil {
		return normalized, err
	}
	normalized.Lane = "ios-simulator"
	if normalized.Architecture != "arm64" {
		return normalized, fmt.Errorf("iOS Simulator Profile 当前仅支持 Apple Silicon arm64")
	}
	return normalized, nil
}

// releaseProfilesLocalV2 extends the current local-checkout profile API with an
// artifact-only ios-simulator lane without changing the legacy release runner.
func (a *App) releaseProfilesLocalV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.releaseProfilesLocal(w, r)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		fail(w, http.StatusBadRequest, "Release Profile 格式无效")
		return
	}
	var probe struct {
		Platform string `json:"platform"`
		Lane     string `json:"lane"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		fail(w, http.StatusBadRequest, "Release Profile 格式无效")
		return
	}
	if strings.TrimSpace(probe.Platform) != "ios" || strings.TrimSpace(probe.Lane) != "ios-simulator" {
		r.Body = io.NopCloser(bytes.NewReader(raw))
		a.releaseProfilesLocal(w, r)
		return
	}
	if !a.authorized(w, r) {
		return
	}
	project := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(project)
	a.mu.RUnlock()
	if !exists {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	if _, err = a.readSource(project); err != nil {
		fail(w, http.StatusConflict, "请先关联本地项目目录")
		return
	}
	var input releaseProfileLocal
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "Release Profile 格式无效")
		return
	}
	profile, err := normalizeSimulatorReleaseProfile(input.ReleaseProfile)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	buildType, err := normalizeBuildType(input.BuildType)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != "" {
		fail(w, http.StatusConflict, "构建期间不能修改 Release Profile")
		return
	}
	path := a.releaseProfilePath(project, profile.ID)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err == nil {
		err = atomicJSON(path, profile)
	}
	if err == nil {
		metaPath := a.releaseProfileMetaPath(project, profile.ID)
		if err = os.MkdirAll(filepath.Dir(metaPath), 0700); err == nil {
			err = atomicJSON(metaPath, map[string]string{"build_type": buildType})
		}
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存 Release Profile 失败")
		return
	}
	respond(w, http.StatusOK, releaseProfileLocal{ReleaseProfile: profile, BuildType: buildType})
}

// startPackageOnlyBuild dispatches the existing macOS Internal Test lane and
// the iOS Simulator lane through their package-only runners.
func (a *App) startPackageOnlyBuild(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		fail(w, http.StatusBadRequest, "请选择 Internal Test Profile")
		return
	}
	var input struct {
		Profile string `json:"profile"`
	}
	if json.Unmarshal(raw, &input) != nil || strings.TrimSpace(input.Profile) == "" {
		fail(w, http.StatusBadRequest, "请选择 Internal Test Profile")
		return
	}
	profile, readErr := a.readReleaseProfile(r.PathValue("project"), input.Profile)
	if readErr == nil && profile.Platform == "ios" && profile.Lane == "ios-simulator" {
		a.startSimulatorInternalTestBuild(w, r, input.Profile)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	a.startInternalTestBuild(w, r)
}

func (a *App) startSimulatorInternalTestBuild(w http.ResponseWriter, r *http.Request, profileID string) {
	if !a.authorized(w, r) {
		return
	}
	project := r.PathValue("project")
	source, err := a.readSource(project)
	if err != nil {
		fail(w, http.StatusConflict, "请先关联项目目录")
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
	profile, err := a.readReleaseProfile(project, profileID)
	if err != nil {
		fail(w, http.StatusNotFound, "ILS Release Profile 不存在")
		return
	}
	profile, err = normalizeSimulatorReleaseProfile(profile)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if profile.Platform != "ios" || profile.Lane != "ios-simulator" {
		fail(w, http.StatusBadRequest, "只有 ios-simulator Profile 可以使用 Simulator Internal Test 模式")
		return
	}

	buildType := a.readProfileBuildType(project, profile.ID)
	snapshot := snapshotFromSource(info, buildType)
	a.refreshProjectIcons(project, info.Path)

	a.buildMu.Lock()
	if a.activeBuild != "" {
		a.buildMu.Unlock()
		fail(w, http.StatusConflict, "已有构建正在执行，请稍后再构建 Simulator 测试包")
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
		fail(w, http.StatusInternalServerError, "创建 Simulator 构建任务失败")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	a.activeBuild = job.ID
	a.cancelBuild = cancel
	a.buildReceipts = map[string][]string{job.ID: {}}
	a.buildMu.Unlock()

	go a.runSimulatorInternalTestBuild(ctx, cancel, job, source, profile, snapshot)
	respond(w, http.StatusAccepted, buildJobLocalView{
		BuildJob: job, Branch: snapshot.Branch, Head: snapshot.Head,
		Upstream: snapshot.Upstream, Remote: snapshot.Remote,
		Dirty: snapshot.Dirty, BuildType: snapshot.BuildType,
	})
}

func (a *App) runSimulatorInternalTestBuild(ctx context.Context, cancel context.CancelFunc, job BuildJob, source BuildSource, profile ReleaseProfile, snapshot buildSourceSnapshot) {
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
	commandFailure := func(commandErr error) {
		log.Flush()
		finish("failed", buildFailureMessage(file.Name(), commandErr, ctx, "iOS Simulator 构建"))
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
		"ILS_PLATFORM=ios", "ILS_CHANNEL="+profile.Channel, "ILS_ARCHITECTURE="+profile.Architecture,
		"ILS_VARIANT="+profile.Variant, "ILS_LANE=ios-simulator", "ILS_PUBLISH_MODE=artifact-only",
	)
	fmt.Fprintf(log, "ILS iOS Simulator Internal Test\nBranch: %s\nHEAD: %s\nDirty: %t\nBuild type: %s\nNo signing team, provisioning profile, App Store Connect upload, or Release record is required.\n", snapshot.Branch, snapshot.Head, snapshot.Dirty, snapshot.BuildType)

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
		commandFailure(err)
		return
	}

	result, err := readBuildResult(output)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	if result.Status != "succeeded" || result.Platform != "ios" || result.Lane != "ios-simulator" {
		finish("failed", "Simulator 结果必须是 succeeded + ios + ios-simulator")
		return
	}
	if strings.TrimSpace(result.BundleID) == "" {
		finish("failed", "Simulator 结果必须提供 bundle_id")
		return
	}
	artifact, err := outputArtifact(output, result.Artifact)
	if err != nil {
		finish("failed", err.Error())
		return
	}
	info, err := os.Stat(artifact)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		finish("failed", "Simulator 测试包不存在或为空")
		return
	}
	if strings.ToLower(filepath.Ext(artifact)) != ".zip" {
		finish("failed", "iOS Simulator 测试包必须是包含 .app 的 ZIP")
		return
	}
	job.Result = &result
	job.Platform = result.Platform
	job.Lane = result.Lane
	job.Stage = "complete"
	job.StageState = "succeeded"
	job.Progress = nil
	job.Message = "iOS Simulator 测试包已生成，可下载或安装到当前已启动 Simulator"
	finish("succeeded", "")
}

func (a *App) simulatorInternalTestRecords(project string) []InternalTestRecord {
	out := []InternalTestRecord{}
	paths, _ := filepath.Glob(filepath.Join(a.data, "builds", "*", "job.json"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var job BuildJob
		if json.Unmarshal(raw, &job) != nil || job.ProjectID != project || job.Status != "succeeded" || job.Lane != "ios-simulator" || job.Result == nil {
			continue
		}
		result := job.Result
		if result.Status != "succeeded" || result.Platform != "ios" || result.Lane != "ios-simulator" || strings.TrimSpace(result.Artifact) == "" {
			continue
		}
		output := filepath.Join(a.data, "builds", job.ID, "output")
		artifact, err := outputArtifact(output, result.Artifact)
		if err != nil {
			continue
		}
		info, err := os.Stat(artifact)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			continue
		}
		architecture := strings.TrimSpace(result.Architecture)
		if architecture == "" {
			architecture = strings.TrimSpace(job.ReleaseArchitecture)
		}
		out = append(out, InternalTestRecord{
			ID: job.ID, ProjectID: job.ProjectID, Kind: "internal-test", Platform: "ios", Lane: "ios-simulator",
			Version: strings.TrimSpace(result.Version), Build: strings.TrimSpace(result.Build), Architecture: architecture,
			Channel: strings.TrimSpace(job.ReleaseChannel), Variant: strings.TrimSpace(job.ReleaseVariant), Notes: strings.TrimSpace(job.ReleaseNotes),
			Filename: filepath.Base(artifact), Size: info.Size(), CreatedAt: job.CreatedAt, FinishedAt: job.FinishedAt,
			DownloadURL: "/api/builds/" + job.ID + "/artifact",
		})
	}
	return out
}

func (a *App) listPackageOnlyRecords(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(project)
	a.mu.RUnlock()
	if !exists {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	out := append(a.internalTestRecords(project), a.simulatorInternalTestRecords(project)...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 100 {
		out = out[:100]
	}
	respond(w, http.StatusOK, out)
}

func (a *App) downloadPackageOnlyArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("job")
	if !jobRE.MatchString(id) {
		fail(w, http.StatusNotFound, "任务不存在")
		return
	}
	raw, err := os.ReadFile(a.buildJobPath(id))
	if err != nil {
		a.downloadInternalBuildArtifact(w, r)
		return
	}
	var job BuildJob
	if json.Unmarshal(raw, &job) != nil || job.Lane != "ios-simulator" {
		a.downloadInternalBuildArtifact(w, r)
		return
	}
	if !a.authorized(w, r) {
		return
	}
	if job.Status != "succeeded" || job.Result == nil {
		fail(w, http.StatusConflict, "Simulator 测试包尚未可用")
		return
	}
	output := filepath.Join(a.data, "builds", id, "output")
	artifact, err := outputArtifact(output, job.Result.Artifact)
	if err != nil || strings.ToLower(filepath.Ext(artifact)) != ".zip" {
		fail(w, http.StatusNotFound, "Simulator 测试包不存在")
		return
	}
	file, err := os.Open(artifact)
	if err != nil {
		fail(w, http.StatusNotFound, "Simulator 测试包不存在")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		fail(w, http.StatusNotFound, "Simulator 测试包不存在")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(artifact)}))
	http.ServeContent(w, r, filepath.Base(artifact), info.ModTime(), file)
}

func findSimulatorApp(root string) (string, error) {
	found := ""
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != root && entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
			if found != "" {
				return fmt.Errorf("Simulator ZIP 中包含多个 .app")
			}
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("Simulator ZIP 中没有 .app")
	}
	return found, nil
}

func shortCommandOutput(raw []byte) string {
	value := strings.TrimSpace(string(raw))
	if len(value) > 1200 {
		value = value[len(value)-1200:]
	}
	return value
}

func (a *App) installSimulatorBuild(w http.ResponseWriter, r *http.Request) {
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
	if json.Unmarshal(raw, &job) != nil || job.Status != "succeeded" || job.Lane != "ios-simulator" || job.Result == nil {
		fail(w, http.StatusConflict, "Simulator 测试包尚未可用")
		return
	}
	output := filepath.Join(a.data, "builds", id, "output")
	artifact, err := outputArtifact(output, job.Result.Artifact)
	if err != nil || strings.ToLower(filepath.Ext(artifact)) != ".zip" {
		fail(w, http.StatusNotFound, "Simulator 测试包不存在")
		return
	}
	xcrun, err := exec.LookPath("xcrun")
	if err != nil {
		fail(w, http.StatusConflict, "未找到 Xcode xcrun；请先安装并选择 Xcode")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if booted, bootErr := exec.CommandContext(ctx, xcrun, "simctl", "getenv", "booted", "HOME").CombinedOutput(); bootErr != nil {
		detail := shortCommandOutput(booted)
		if detail != "" {
			fail(w, http.StatusConflict, "没有可用的已启动 iOS Simulator："+detail)
		} else {
			fail(w, http.StatusConflict, "没有可用的已启动 iOS Simulator；请先打开 Simulator 并启动一台设备")
		}
		return
	}
	temp, err := os.MkdirTemp("", "ils-ios-simulator-*")
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法创建 Simulator 临时目录")
		return
	}
	defer os.RemoveAll(temp)
	if extracted, extractErr := exec.CommandContext(ctx, "/usr/bin/ditto", "-x", "-k", artifact, temp).CombinedOutput(); extractErr != nil {
		fail(w, http.StatusInternalServerError, "无法解压 Simulator 测试包："+shortCommandOutput(extracted))
		return
	}
	appPath, err := findSimulatorApp(temp)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	infoPlist := filepath.Join(appPath, "Info.plist")
	bundleRaw, bundleErr := exec.CommandContext(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", infoPlist).CombinedOutput()
	if bundleErr != nil {
		fail(w, http.StatusConflict, "无法读取 Simulator App Bundle ID")
		return
	}
	bundleID := strings.TrimSpace(string(bundleRaw))
	if expected := strings.TrimSpace(job.Result.BundleID); expected == "" || bundleID != expected {
		fail(w, http.StatusConflict, "Simulator App Bundle ID 与构建结果不一致")
		return
	}
	_, _ = exec.CommandContext(ctx, xcrun, "simctl", "terminate", "booted", bundleID).CombinedOutput()
	if installed, installErr := exec.CommandContext(ctx, xcrun, "simctl", "install", "booted", appPath).CombinedOutput(); installErr != nil {
		fail(w, http.StatusInternalServerError, "安装到 Simulator 失败："+shortCommandOutput(installed))
		return
	}
	launched, launchErr := exec.CommandContext(ctx, xcrun, "simctl", "launch", "booted", bundleID).CombinedOutput()
	if launchErr != nil {
		respond(w, http.StatusOK, map[string]any{
			"installed": true, "launched": false, "bundle_id": bundleID,
			"warning": "测试包已安装，但自动启动失败：" + shortCommandOutput(launched),
		})
		return
	}
	respond(w, http.StatusOK, map[string]any{"installed": true, "launched": true, "bundle_id": bundleID})
}
