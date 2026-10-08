package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type releaseProfileLocal struct {
	ReleaseProfile
	BuildType string `json:"build_type,omitempty"`
}

type buildSourceSnapshot struct {
	Branch    string `json:"branch,omitempty"`
	Head      string `json:"head,omitempty"`
	Upstream  string `json:"upstream,omitempty"`
	Remote    string `json:"remote,omitempty"`
	Dirty     bool   `json:"dirty"`
	BuildType string `json:"build_type,omitempty"`
}

type buildJobLocalView struct {
	BuildJob
	Branch    string `json:"branch,omitempty"`
	Head      string `json:"head,omitempty"`
	Upstream  string `json:"upstream,omitempty"`
	Remote    string `json:"remote,omitempty"`
	Dirty     bool   `json:"dirty"`
	BuildType string `json:"build_type,omitempty"`
}

func normalizeBuildType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "native", nil
	}
	switch value {
	case "native", "expo", "hybrid-web-native", "tauri":
		return value, nil
	default:
		return "", fmt.Errorf("构建类型必须是 native、expo、hybrid-web-native 或 tauri")
	}
}

func (a *App) releaseProfileMetaPath(project, id string) string {
	return filepath.Join(a.data, "release-profile-meta", project, id+".json")
}

func (a *App) buildSourceSnapshotPath(job string) string {
	return filepath.Join(a.data, "builds", job, "source.json")
}

func (a *App) readProfileBuildType(project, id string) string {
	var meta struct {
		BuildType string `json:"build_type"`
	}
	raw, err := os.ReadFile(a.releaseProfileMetaPath(project, id))
	if err == nil && json.Unmarshal(raw, &meta) == nil {
		if value, normalizeErr := normalizeBuildType(meta.BuildType); normalizeErr == nil {
			return value
		}
	}
	return "native"
}

func (a *App) releaseProfilesLocal(w http.ResponseWriter, r *http.Request) {
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
	if r.Method == http.MethodGet {
		profiles, err := a.listReleaseProfiles(project)
		if err != nil {
			fail(w, http.StatusInternalServerError, "读取 Release Profiles 失败")
			return
		}
		out := make([]releaseProfileLocal, 0, len(profiles))
		for _, profile := range profiles {
			out = append(out, releaseProfileLocal{ReleaseProfile: profile, BuildType: a.readProfileBuildType(project, profile.ID)})
		}
		respond(w, http.StatusOK, out)
		return
	}
	if _, err := a.readSource(project); err != nil {
		fail(w, http.StatusConflict, "请先关联本地项目目录")
		return
	}
	var input releaseProfileLocal
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "Release Profile 格式无效")
		return
	}
	profile, err := normalizeReleaseProfile(input.ReleaseProfile)
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

func (a *App) deleteReleaseProfileLocal(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	project, id := r.PathValue("project"), r.PathValue("profile")
	if !slugRE.MatchString(project) || !slugRE.MatchString(id) {
		fail(w, http.StatusNotFound, "Release Profile 不存在")
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != "" {
		fail(w, http.StatusConflict, "构建期间不能删除 Release Profile")
		return
	}
	if err := os.Remove(a.releaseProfilePath(project, id)); err != nil {
		if os.IsNotExist(err) {
			fail(w, http.StatusNotFound, "Release Profile 不存在")
		} else {
			fail(w, http.StatusInternalServerError, "删除 Release Profile 失败")
		}
		return
	}
	_ = os.Remove(a.releaseProfileMetaPath(project, id))
	respond(w, http.StatusOK, map[string]bool{"deleted": true})
}

func snapshotFromSource(info SourceInfo, buildType string) buildSourceSnapshot {
	branch := strings.TrimSpace(info.CurrentBranch)
	if branch == "" {
		branch = "detached"
	}
	return buildSourceSnapshot{
		Branch: branch, Head: info.Head, Upstream: info.Upstream, Remote: info.Remote,
		Dirty: info.Dirty, BuildType: buildType,
	}
}

func (a *App) readBuildSourceSnapshot(job string) buildSourceSnapshot {
	var snapshot buildSourceSnapshot
	raw, err := os.ReadFile(a.buildSourceSnapshotPath(job))
	if err == nil {
		_ = json.Unmarshal(raw, &snapshot)
	}
	return snapshot
}

func (a *App) startLocalCheckoutBuild(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("project")
	source, err := a.readSource(id)
	if err != nil {
		fail(w, http.StatusConflict, "请先关联项目目录")
		return
	}
	var input struct {
		Script  string `json:"script"`
		Profile string `json:"profile"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || (input.Script == "") == (input.Profile == "") {
		fail(w, http.StatusBadRequest, "请选择一个项目脚本或 ILS Release Profile")
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
	a.refreshProjectIcons(id, info.Path)

	var profile *ReleaseProfile
	mode, title, buildType := "script", input.Script, "native"
	if input.Profile != "" {
		value, readErr := a.readReleaseProfile(id, input.Profile)
		if readErr != nil {
			fail(w, http.StatusNotFound, "ILS Release Profile 不存在")
			return
		}
		value, readErr = normalizeReleaseProfile(value)
		if readErr != nil {
			fail(w, http.StatusConflict, readErr.Error())
			return
		}
		profile = &value
		mode, title = "profile", value.Name
		buildType = a.readProfileBuildType(id, value.ID)
	} else {
		if !releaseScriptRE.MatchString(input.Script) {
			fail(w, http.StatusBadRequest, "请选择已识别的发布脚本")
			return
		}
		found := false
		for _, item := range info.Scripts {
			if item.Path == input.Script && item.Ready {
				found = true
				break
			}
		}
		if !found {
			fail(w, http.StatusConflict, "脚本或 Markdown 说明不可用")
			return
		}
	}

	snapshot := snapshotFromSource(info, buildType)
	a.buildMu.Lock()
	if a.activeBuild != "" {
		a.buildMu.Unlock()
		fail(w, http.StatusConflict, "已有构建正在执行，请稍后发布")
		return
	}
	job := BuildJob{
		ID: randomID(16), ProjectID: id, Mode: mode, Script: input.Script, ProfileID: input.Profile,
		Title: title, Status: "running", Stage: "preflight", Commit: snapshot.Head,
		CreatedAt: time.Now().UTC(), ReleaseIDs: []string{},
	}
	if profile != nil {
		job.Platform = profile.Platform
		job.Lane = profile.Lane
		job.ReleaseVariant = profile.Variant
		job.ReleaseChannel = profile.Channel
		job.ReleaseArchitecture = profile.Architecture
		job.ReleaseNotes = profile.Notes
		job.TestFlightURL = profile.TestFlightURL
		job.TestFlightGroupName = profile.TestFlightGroupName
		job.TestFlightGroupType = profile.TestFlightGroupType
		job.TestFlightCreateGroup = profile.TestFlightCreateGroup
		job.TestFlightSubmitBetaReview = profile.TestFlightSubmitBetaReview
	}
	if err = os.MkdirAll(filepath.Dir(a.buildJobPath(job.ID)), 0700); err == nil {
		if job.Platform != "" {
			a.snapshotBuildIcon(job.ProjectID, job.Platform, job.ID)
		}
		err = atomicJSON(a.buildJobPath(job.ID), job)
	}
	if err == nil {
		err = atomicJSON(a.buildSourceSnapshotPath(job.ID), snapshot)
	}
	if err != nil {
		a.buildMu.Unlock()
		fail(w, http.StatusInternalServerError, "创建构建任务失败")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	a.activeBuild = job.ID
	a.cancelBuild = cancel
	a.buildReceipts = map[string][]string{job.ID: {}}
	a.buildMu.Unlock()
	go a.runLocalCheckoutBuild(ctx, cancel, job, source, profile, info, snapshot)
	respond(w, http.StatusAccepted, buildJobLocalView{BuildJob: job, Branch: snapshot.Branch, Head: snapshot.Head, Upstream: snapshot.Upstream, Remote: snapshot.Remote, Dirty: snapshot.Dirty, BuildType: snapshot.BuildType})
}

func (a *App) buildJobsLocal(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	out := []buildJobLocalView{}
	paths, _ := filepath.Glob(filepath.Join(a.data, "builds", "*", "job.json"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var job BuildJob
		if json.Unmarshal(raw, &job) != nil || job.ProjectID != r.PathValue("project") {
			continue
		}
		if job.Status == "running" && job.ID != a.activeBuild {
			job.Status = "failed"
			job.Error = "服务重启导致任务中断，请检查日志和已发布产物后重试"
		}
		snapshot := a.readBuildSourceSnapshot(job.ID)
		out = append(out, buildJobLocalView{BuildJob: job, Branch: snapshot.Branch, Head: snapshot.Head, Upstream: snapshot.Upstream, Remote: snapshot.Remote, Dirty: snapshot.Dirty, BuildType: snapshot.BuildType})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 30 {
		out = out[:30]
	}
	respond(w, http.StatusOK, out)
}

func (a *App) runLocalCheckoutBuild(ctx context.Context, cancel context.CancelFunc, job BuildJob, source BuildSource, profile *ReleaseProfile, info SourceInfo, snapshot buildSourceSnapshot) {
	defer func() {
		cancel()
		a.buildMu.Lock()
		a.activeBuild = ""
		a.cancelBuild = nil
		a.buildMu.Unlock()
	}()
	finish := func(status, message string) {
		a.buildMu.Lock()
		job.ReleaseIDs = append([]string{}, a.buildReceipts[job.ID]...)
		a.buildMu.Unlock()
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

	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	fmt.Fprintf(log, "ILS local checkout build\nBranch: %s\nHEAD: %s\nDirty: %t\nUpstream: %s\nRemote: %s\nBuild type: %s\nNo git pull/stash/reset/clean/checkout is performed.\n", snapshot.Branch, snapshot.Head, snapshot.Dirty, snapshot.Upstream, snapshot.Remote, snapshot.BuildType)
	a.refreshProjectIcons(job.ProjectID, source.Path)
	if job.Platform != "" {
		a.snapshotBuildIcon(job.ProjectID, job.Platform, job.ID)
	}
	if profile == nil {
		ready := false
		for _, item := range info.Scripts {
			if item.Path == job.Script && item.Ready {
				ready = true
				break
			}
		}
		if !ready {
			finish("failed", "项目脚本已删除或说明不完整，请重新扫描")
			return
		}
	}

	output := filepath.Join(a.data, "builds", job.ID, "output")
	if err = os.MkdirAll(output, 0700); err != nil {
		finish("failed", "无法创建产物目录")
		return
	}
	tokenFile := filepath.Join(a.data, "admin-token")
	dirtyValue := "0"
	if snapshot.Dirty {
		dirtyValue = "1"
	}
	env = append(env,
		"LOCALSERVICE_URL="+a.buildOrigin, "LOCALSERVICE_TOKEN_FILE="+tokenFile, "LOCALSERVICE_ROOT="+a.buildRoot,
		"LOCALSERVICE_PROJECT_ID="+job.ProjectID, "LOCALSERVICE_JOB_ID="+job.ID, "LOCALSERVICE_OUTPUT_DIR="+output, "LOCALSERVICE_GIT_COMMIT="+job.Commit,
		"ILS_URL="+a.buildOrigin, "ILS_TOKEN_FILE="+tokenFile, "ILS_ROOT="+a.buildRoot, "ILS_PROJECT_ID="+job.ProjectID,
		"ILS_JOB_ID="+job.ID, "ILS_OUTPUT_DIR="+output, "ILS_GIT_COMMIT="+job.Commit,
		"ILS_GIT_BRANCH="+snapshot.Branch, "ILS_GIT_DIRTY="+dirtyValue, "ILS_GIT_UPSTREAM="+snapshot.Upstream, "ILS_GIT_REMOTE="+snapshot.Remote,
		"ILS_BUILD_TYPE="+snapshot.BuildType,
	)
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
	if profile != nil {
		env = append(env,
			"ILS_PLATFORM="+profile.Platform,
			"ILS_CHANNEL="+profile.Channel,
			"ILS_ARCHITECTURE="+profile.Architecture,
			"ILS_VARIANT="+profile.Variant,
			"ILS_LANE="+profile.Lane,
		)
		if profile.AppleTeamID != "" {
			env = append(env, "ILS_APPLE_TEAM_ID="+profile.AppleTeamID)
		}
		if profile.Lane == "ios-testflight" {
			env = append(env, a.appStoreConnectBuildEnv()...)
		}
	}
	if profile != nil && profile.Lane == "ios-testflight" {
		job.Stage = "preflight"
		job.StageState = "running"
		job.Progress = nil
		job.Message = "正在检查 App Store Connect App 记录"
		_ = a.writeBuild(job)
		fmt.Fprintf(log, "ILS TestFlight preflight: verify App Store Connect app for profile %s\n", profile.ID)
		bundleID, checkErr := a.verifyTestFlightAppRecord(ctx, job.ProjectID, profile.ID, source.Path)
		if checkErr != nil {
			job.StageState = "failed"
			job.Message = checkErr.Error()
			_ = a.writeBuild(job)
			fmt.Fprintf(log, "ERROR: %s\n", checkErr.Error())
			finish("failed", checkErr.Error())
			return
		}
		env = append(env, "ILS_BUNDLE_ID="+bundleID)
		job.StageState = "succeeded"
		job.Message = "App Store Connect App 已确认：" + bundleID
		_ = a.writeBuild(job)
		fmt.Fprintf(log, "ILS TestFlight preflight succeeded: App Store Connect contains %s\n", bundleID)
	}

	if profile == nil {
		job.Stage = "script"
		_ = a.writeBuild(job)
		fmt.Fprintf(log, "Run project script: /bin/bash %s\n", job.Script)
		if err = runProcess(ctx, source.Path, jobLog, env, "/bin/bash", job.Script); err != nil {
			finish("failed", "项目发布脚本失败或任务超时；已上传的包会保留，请查看日志")
			return
		}
	} else {
		job.Stage = "preflight"
		_ = a.writeBuild(job)
		fmt.Fprintf(log, "ILS Release Profile: %s (%s)\nSingle entrypoint: %s\n", profile.Name, profile.ID, profile.BuildCommand)
		if err = runProcess(ctx, source.Path, jobLog, env, "/bin/bash", "-lc", profile.BuildCommand); err != nil {
			finish("failed", "ILS Build Command 失败或任务超时；详情见日志")
			return
		}
		if profile.PackageCommand != "" {
			job.Stage = "package"
			_ = a.writeBuild(job)
			if err = runProcess(ctx, source.Path, jobLog, env, "/bin/bash", "-lc", profile.PackageCommand); err != nil {
				finish("failed", "ILS Package Command 失败或任务超时；详情见日志")
				return
			}
		}
		var artifact, version, build string
		if profile.ResultContract == "ils-result-v1" {
			job.Stage = "validate"
			job.StageState = "running"
			job.Progress = nil
			_ = a.writeBuild(job)
			result, resultErr := readBuildResult(output)
			if resultErr != nil {
				finish("failed", resultErr.Error())
				return
			}
			artifact, resultErr = validateContractResult(*profile, result, output)
			if resultErr != nil {
				finish("failed", resultErr.Error())
				return
			}
			job.Result = &result
			job.Platform = result.Platform
			job.Lane = result.Lane
			version, build = result.Version, result.Build
			_ = a.writeBuild(job)
			if profile.Lane == "ios-testflight" {
				release, publishErr := a.publishTestFlightRelease(job, *profile, result)
				if publishErr != nil {
					finish("failed", "TestFlight 已上传，但 ILS 发布记录保存失败："+publishErr.Error())
					return
				}
				a.recordBuildPublication(job.ID, job.ProjectID, release.ID)
				job.Stage = "submitted"
				job.StageState = "succeeded"
				job.Progress = nil
				job.Message = "已发布到 ILS；App Store Connect 已接受上传，等待 Apple Processing"
				finish("succeeded", "")
				return
			}
		} else {
			artifact, err = resolveProfileArtifact(source.Path, output, profile.Artifact)
			if err != nil {
				finish("failed", err.Error())
				return
			}
			ext := strings.ToLower(filepath.Ext(artifact))
			if (profile.Platform == "ios" && ext != ".ipa") || (profile.Platform == "macos" && !oneOf(ext, ".dmg", ".pkg", ".zip")) {
				finish("failed", "Profile 产物类型与平台不匹配")
				return
			}
			if profile.Platform == "ios" {
				ipa, inspectErr := inspectIPA(artifact)
				if inspectErr != nil {
					finish("failed", "IPA 检查失败："+inspectErr.Error())
					return
				}
				version, build = ipa.Version, ipa.Build
			} else {
				job.Stage = "metadata"
				_ = a.writeBuild(job)
				version, err = runValueCommand(ctx, source.Path, log, env, profile.VersionCommand)
				if err != nil {
					finish("failed", "无法读取 macOS version："+err.Error())
					return
				}
				build, err = runValueCommand(ctx, source.Path, log, env, profile.BuildNumberCommand)
				if err != nil {
					finish("failed", "无法读取 macOS build number："+err.Error())
					return
				}
			}
			if !validVersion(version) {
				finish("failed", "Profile 解析出的 version 不是有效 SemVer")
				return
			}
			if _, err = parseBuild(build); err != nil {
				finish("failed", "Profile 解析出的 build number 必须是正整数")
				return
			}
		}
		job.Stage = "publish"
		job.StageState = "running"
		job.Progress = nil
		_ = a.writeBuild(job)
		push := filepath.Join(a.buildRoot, "scripts", "push.sh")
		if stat, statErr := os.Stat(push); statErr != nil || !stat.Mode().IsRegular() {
			finish("failed", "ILS scripts/push.sh 不可用")
			return
		}
		profileEnv := append(env, "RELEASE_NOTES="+profile.Notes)
		fmt.Fprintf(log, "Publish %s version %s build %s\n", filepath.Base(artifact), version, build)
		if err = runProcess(ctx, source.Path, jobLog, profileEnv, "/bin/bash", push, job.ProjectID, version, build, profile.Platform, artifact, profile.Channel, profile.Architecture, profile.Variant); err != nil {
			finish("failed", "ILS 发布失败；详情见日志")
			return
		}
	}
	a.buildMu.Lock()
	count := len(a.buildReceipts[job.ID])
	a.buildMu.Unlock()
	if count == 0 {
		finish("failed", "构建命令退出成功，但没有收到与本任务关联的发布记录")
		return
	}
	job.Stage = "complete"
	job.StageState = "succeeded"
	job.Progress = nil
	finish("succeeded", "")
}
