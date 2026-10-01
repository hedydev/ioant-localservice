package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type BuildSource struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}
type ReleaseScript struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
	Ready    bool   `json:"ready"`
	Reason   string `json:"reason,omitempty"`
}
type ReleaseProfile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Platform           string `json:"platform"`
	Architecture       string `json:"architecture"`
	Channel            string `json:"channel"`
	Variant            string `json:"variant"`
	Lane               string `json:"lane,omitempty"`
	ResultContract     string `json:"result_contract,omitempty"`
	AppleTeamID        string `json:"apple_team_id,omitempty"`
	BuildCommand       string `json:"build_command"`
	PackageCommand     string `json:"package_command,omitempty"`
	Artifact           string `json:"artifact,omitempty"`
	VersionCommand     string `json:"version_command,omitempty"`
	BuildNumberCommand string `json:"build_number_command,omitempty"`
	Notes              string `json:"notes,omitempty"`
}
type SourceInfo struct {
	BuildSource
	CurrentBranch string          `json:"current_branch"`
	Head          string          `json:"head"`
	Upstream      string          `json:"upstream"`
	Remote        string          `json:"remote"`
	Dirty         bool            `json:"dirty"`
	Blocker       string          `json:"blocker,omitempty"`
	Scripts       []ReleaseScript `json:"scripts"`
}
type BuildJob struct {
	ID         string       `json:"id"`
	ProjectID  string       `json:"project_id"`
	Mode       string       `json:"mode,omitempty"`
	Script     string       `json:"script,omitempty"`
	ProfileID  string       `json:"profile_id,omitempty"`
	Title      string       `json:"title,omitempty"`
	Status     string       `json:"status"`
	Stage      string       `json:"stage"`
	StageState string       `json:"stage_state,omitempty"`
	Progress   *int         `json:"progress,omitempty"`
	Message    string       `json:"message,omitempty"`
	Error      string       `json:"error,omitempty"`
	Commit     string       `json:"commit,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	ReleaseIDs []string     `json:"release_ids"`
	Result     *BuildResult `json:"result,omitempty"`
}

type BuildEvent struct {
	Stage    string `json:"stage"`
	State    string `json:"state"`
	Message  string `json:"message,omitempty"`
	Progress *int   `json:"progress,omitempty"`
}

type BuildResult struct {
	SchemaVersion     int    `json:"schema_version"`
	Lane              string `json:"lane"`
	Status            string `json:"status"`
	Platform          string `json:"platform"`
	Artifact          string `json:"artifact,omitempty"`
	Version           string `json:"version"`
	Build             string `json:"build"`
	Architecture      string `json:"architecture"`
	BundleID          string `json:"bundle_id,omitempty"`
	Distribution      string `json:"distribution"`
	SHA256            string `json:"sha256,omitempty"`
	Archive           string `json:"archive,omitempty"`
	SubmissionResult  string `json:"submission_result,omitempty"`
}

var releaseScriptRE = regexp.MustCompile(`^(scripts/)?release[A-Za-z0-9_-]*\.sh$`)

func gitRead(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	b, e := cmd.Output()
	if e != nil {
		return "", fmt.Errorf("Git 检查失败：%s", strings.Join(args, " "))
	}
	return strings.TrimSpace(string(b)), nil
}
func canonicalRepo(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("请输入 Mac 上的绝对路径")
	}
	p, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", fmt.Errorf("目录不存在")
	}
	top, e := gitRead(p, "rev-parse", "--show-toplevel")
	if e != nil {
		return "", fmt.Errorf("该目录不是 Git 工作目录")
	}
	return filepath.EvalSymlinks(top)
}
func safeTracked(root, path string, max int64) ([]byte, error) {
	full := filepath.Join(root, filepath.FromSlash(path))
	real, e := filepath.EvalSymlinks(full)
	if e != nil || real != full {
		return nil, fmt.Errorf("文件不存在或使用符号链接")
	}
	if _, e = gitRead(root, "ls-files", "--error-unmatch", "--", path); e != nil {
		return nil, fmt.Errorf("文件尚未纳入 Git")
	}
	f, e := os.Open(full)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("不是普通文件")
	}
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, fmt.Errorf("文件过大")
	}
	return b, nil
}
func inspectSource(s BuildSource) (SourceInfo, error) {
	info := SourceInfo{BuildSource: s, Scripts: []ReleaseScript{}}
	root, e := canonicalRepo(s.Path)
	if e != nil {
		return info, e
	}
	info.Path = root
	info.CurrentBranch, _ = gitRead(root, "symbolic-ref", "--short", "HEAD")
	info.Head, _ = gitRead(root, "rev-parse", "HEAD")
	info.Upstream, _ = gitRead(root, "rev-parse", "--abbrev-ref", "@{upstream}")
	remote, _ := gitRead(root, "config", "--get", "branch."+info.CurrentBranch+".remote")
	if remote != "" {
		info.Remote, _ = gitRead(root, "remote", "get-url", remote)
	}
	if u, e := url.Parse(info.Remote); e == nil && u.User != nil {
		u.User = nil
		u.RawQuery = ""
		info.Remote = u.String()
	}
	status, e := gitRead(root, "status", "--porcelain", "--untracked-files=normal")
	if e != nil {
		return info, e
	}
	info.Dirty = status != ""
	switch {
	case info.CurrentBranch == "":
		info.Blocker = "当前是 detached HEAD，请在项目目录切回配置的分支"
	case info.CurrentBranch != s.Branch:
		info.Blocker = "当前分支不是配置的 " + s.Branch + "；服务不会自动切换分支"
	case info.Dirty:
		info.Blocker = "工作目录有未提交或未跟踪文件，请先由项目开发者处理"
	case info.Upstream == "":
		info.Blocker = "当前分支尚未配置 upstream"
	}
	paths, e := gitRead(root, "ls-files", "-z")
	if e != nil {
		return info, e
	}
	for _, path := range strings.Split(paths, "\x00") {
		if !releaseScriptRE.MatchString(path) {
			continue
		}
		script := ReleaseScript{Path: path, Title: filepath.Base(path)}
		if _, e = safeTracked(root, path, 1<<20); e != nil {
			script.Reason = e.Error()
		} else {
			b, e := safeTracked(root, strings.TrimSuffix(path, ".sh")+".md", 64<<10)
			if e != nil || strings.TrimSpace(string(b)) == "" {
				script.Reason = "缺少可读取、已纳入 Git 的同名 Markdown 说明"
			} else {
				script.Markdown = string(b)
				script.Ready = true
				for _, line := range strings.Split(string(b), "\n") {
					if strings.HasPrefix(line, "# ") {
						script.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
						break
					}
				}
			}
		}
		info.Scripts = append(info.Scripts, script)
	}
	sort.Slice(info.Scripts, func(i, j int) bool { return info.Scripts[i].Path < info.Scripts[j].Path })
	return info, nil
}
func (a *App) sourcePath(id string) string { return filepath.Join(a.data, "build-sources", id+".json") }
func (a *App) readSource(id string) (BuildSource, error) {
	var s BuildSource
	if !slugRE.MatchString(id) {
		return s, fmt.Errorf("项目标识无效")
	}
	b, e := os.ReadFile(a.sourcePath(id))
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func (a *App) releaseProfilePath(project, id string) string {
	return filepath.Join(a.data, "release-profiles", project, id+".json")
}
func (a *App) readReleaseProfile(project, id string) (ReleaseProfile, error) {
	var p ReleaseProfile
	if !slugRE.MatchString(project) || !slugRE.MatchString(id) {
		return p, fmt.Errorf("Release Profile 标识无效")
	}
	b, e := os.ReadFile(a.releaseProfilePath(project, id))
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(b, &p)
	return p, e
}
func normalizeReleaseProfile(p ReleaseProfile) (ReleaseProfile, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Platform = strings.TrimSpace(p.Platform)
	p.Architecture = strings.TrimSpace(p.Architecture)
	p.Channel = strings.TrimSpace(p.Channel)
	p.Variant = strings.TrimSpace(p.Variant)
	p.Lane = strings.TrimSpace(p.Lane)
	p.ResultContract = strings.TrimSpace(p.ResultContract)
	p.AppleTeamID = strings.ToUpper(strings.TrimSpace(p.AppleTeamID))
	p.BuildCommand = strings.TrimSpace(p.BuildCommand)
	p.PackageCommand = strings.TrimSpace(p.PackageCommand)
	p.Artifact = strings.TrimSpace(p.Artifact)
	p.VersionCommand = strings.TrimSpace(p.VersionCommand)
	p.BuildNumberCommand = strings.TrimSpace(p.BuildNumberCommand)
	if p.Channel == "" { p.Channel = "dev" }
	if p.Variant == "" { p.Variant = "default" }
	if p.Lane == "" {
		if p.Platform == "ios" { p.Lane = "ios-adhoc" } else { p.Lane = "macos-test" }
	}
	if p.Architecture == "" {
		if p.Platform == "ios" { p.Architecture = "arm64" } else { p.Architecture = "universal" }
	}
	if !slugRE.MatchString(p.ID) || p.Name == "" || len(p.Name) > 120 {
		return p, fmt.Errorf("Profile ID 或名称无效")
	}
	if !oneOf(p.Platform, "ios", "macos") || !oneOf(p.Channel, "dev", "beta", "stable") || !oneOf(p.Architecture, "arm64", "x86_64", "universal") || (p.Platform == "ios" && p.Architecture != "arm64") {
		return p, fmt.Errorf("平台、架构或渠道无效")
	}
	if !slugRE.MatchString(p.Variant) { return p, fmt.Errorf("variant 必须是小写字母、数字或连字符") }
	if p.Platform == "ios" && !oneOf(p.Lane, "ios-adhoc", "ios-testflight") {
		return p, fmt.Errorf("iOS lane 必须是 ios-adhoc 或 ios-testflight")
	}
	if p.Platform == "macos" && !oneOf(p.Lane, "macos-test", "macos-release") {
		return p, fmt.Errorf("macOS lane 必须是 macos-test 或 macos-release")
	}
	if p.ResultContract != "" && p.ResultContract != "ils-result-v1" {
		return p, fmt.Errorf("result_contract 仅支持 ils-result-v1")
	}
	if p.AppleTeamID != "" && !appleTeamIDRE.MatchString(p.AppleTeamID) {
		return p, fmt.Errorf("Apple Team ID 必须是 10 位大写字母或数字")
	}
	if p.Lane == "ios-testflight" && p.ResultContract != "ils-result-v1" {
		return p, fmt.Errorf("TestFlight Profile 必须使用 ils-result-v1")
	}
	if p.BuildCommand == "" || len(p.BuildCommand) > 32768 || len(p.PackageCommand) > 32768 {
		return p, fmt.Errorf("需要有效的构建命令")
	}
	if p.ResultContract == "" {
		if p.Artifact == "" || len(p.Artifact) > 2048 {
			return p, fmt.Errorf("兼容 Profile 需要有效的产物路径")
		}
		if strings.ContainsRune(p.Artifact, 0) || filepath.IsAbs(p.Artifact) {
			return p, fmt.Errorf("产物路径必须相对项目目录，或使用 ILS_OUTPUT_DIR")
		}
		cleanArtifact := filepath.Clean(p.Artifact)
		if strings.HasPrefix(cleanArtifact, ".."+string(filepath.Separator)) || cleanArtifact == ".." {
			return p, fmt.Errorf("产物路径不能逃离项目目录；需要外部产物时使用 ILS_OUTPUT_DIR")
		}
		if p.Platform == "macos" && (p.VersionCommand == "" || p.BuildNumberCommand == "") {
			return p, fmt.Errorf("兼容 macOS Profile 需要 version command 与 build number command")
		}
	}
	if len(p.VersionCommand) > 8192 || len(p.BuildNumberCommand) > 8192 || len(p.Notes) > 16000 {
		return p, fmt.Errorf("Profile 字段过长")
	}
	return p, nil
}
func (a *App) listReleaseProfiles(project string) ([]ReleaseProfile, error) {
	paths, e := filepath.Glob(filepath.Join(a.data, "release-profiles", project, "*.json"))
	if e != nil { return nil, e }
	out := []ReleaseProfile{}
	for _, path := range paths {
		b, e := os.ReadFile(path)
		if e != nil { return nil, e }
		var p ReleaseProfile
		if e = json.Unmarshal(b, &p); e != nil { return nil, e }
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (a *App) releaseProfiles(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) { return }
	project := r.PathValue("project")
	a.mu.RLock(); exists := a.projectExists(project); a.mu.RUnlock()
	if !exists { fail(w, 404, "项目不存在"); return }
	if r.Method == "GET" {
		profiles, e := a.listReleaseProfiles(project)
		if e != nil { fail(w, 500, "读取 Release Profiles 失败"); return }
		respond(w, 200, profiles); return
	}
	if _, e := a.readSource(project); e != nil { fail(w, 409, "请先关联本地项目目录"); return }
	var p ReleaseProfile
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)); dec.DisallowUnknownFields()
	if e := dec.Decode(&p); e != nil { fail(w, 400, "Release Profile 格式无效"); return }
	p, e := normalizeReleaseProfile(p)
	if e != nil { fail(w, 400, e.Error()); return }
	a.buildMu.Lock(); defer a.buildMu.Unlock()
	if a.activeBuild != "" { fail(w, 409, "构建期间不能修改 Release Profile"); return }
	path := a.releaseProfilePath(project, p.ID)
	if e = os.MkdirAll(filepath.Dir(path), 0700); e == nil { e = atomicJSON(path, p) }
	if e != nil { fail(w, 500, "保存 Release Profile 失败"); return }
	respond(w, 200, p)
}
func (a *App) deleteReleaseProfile(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) { return }
	project, id := r.PathValue("project"), r.PathValue("profile")
	if !slugRE.MatchString(project) || !slugRE.MatchString(id) { fail(w, 404, "Release Profile 不存在"); return }
	a.buildMu.Lock(); defer a.buildMu.Unlock()
	if a.activeBuild != "" { fail(w, 409, "构建期间不能删除 Release Profile"); return }
	if e := os.Remove(a.releaseProfilePath(project, id)); e != nil {
		if os.IsNotExist(e) { fail(w, 404, "Release Profile 不存在") } else { fail(w, 500, "删除 Release Profile 失败") }
		return
	}
	respond(w, 200, map[string]bool{"deleted": true})
}
func (a *App) buildSource(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(id)
	a.mu.RUnlock()
	if !exists {
		fail(w, 404, "项目不存在")
		return
	}
	if r.Method == "GET" {
		s, e := a.readSource(id)
		if os.IsNotExist(e) {
			respond(w, 200, map[string]any{"configured": false})
			return
		}
		if e != nil {
			fail(w, 500, "读取项目目录失败")
			return
		}
		info, e := inspectSource(s)
		if e != nil {
			fail(w, 409, e.Error())
			return
		}
		respond(w, 200, info)
		return
	}
	var s BuildSource
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&s) != nil {
		fail(w, 400, "配置无效")
		return
	}
	if s.Branch == "" {
		s.Branch = "main"
	}
	if len(s.Branch) > 200 || strings.HasPrefix(s.Branch, "-") {
		fail(w, 400, "分支无效")
		return
	}
	info, e := inspectSource(s)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	s.Path = info.Path
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != "" {
		fail(w, 409, "构建期间不能更改项目目录")
		return
	}
	if e = os.MkdirAll(filepath.Dir(a.sourcePath(id)), 0700); e == nil {
		e = atomicJSON(a.sourcePath(id), s)
	}
	if e != nil {
		fail(w, 500, "保存项目目录失败")
		return
	}
	respond(w, 200, info)
}
func atomicJSON(path string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".metadata-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func (a *App) buildJobPath(id string) string { return filepath.Join(a.data, "builds", id, "job.json") }
func (a *App) writeBuild(j BuildJob) error {
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	return atomicJSON(a.buildJobPath(j.ID), j)
}
func (a *App) startBuild(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) { return }
	id := r.PathValue("project")
	source, e := a.readSource(id)
	if e != nil { fail(w, 409, "请先关联项目目录"); return }
	var input struct { Script string `json:"script"`; Profile string `json:"profile"` }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || (input.Script == "") == (input.Profile == "") {
		fail(w, 400, "请选择一个项目脚本或 ILS Release Profile"); return
	}
	info, e := inspectSource(source)
	if e != nil { fail(w, 409, e.Error()); return }
	if info.Blocker != "" { fail(w, 409, info.Blocker); return }
	var profile *ReleaseProfile
	mode, title := "script", input.Script
	if input.Profile != "" {
		p, e := a.readReleaseProfile(id, input.Profile)
		if e != nil { fail(w, 404, "ILS Release Profile 不存在"); return }
		p, e = normalizeReleaseProfile(p)
		if e != nil { fail(w, 409, e.Error()); return }
		profile = &p; mode, title = "profile", p.Name
	} else {
		if !releaseScriptRE.MatchString(input.Script) { fail(w, 400, "请选择已识别的发布脚本"); return }
		found := false
		for _, x := range info.Scripts { if x.Path == input.Script && x.Ready { found = true } }
		if !found { fail(w, 409, "脚本或 Markdown 说明不可用"); return }
	}
	a.buildMu.Lock()
	if a.activeBuild != "" { a.buildMu.Unlock(); fail(w, 409, "已有构建正在执行，请稍后发布"); return }
	j := BuildJob{ID: randomID(16), ProjectID: id, Mode: mode, Script: input.Script, ProfileID: input.Profile, Title: title, Status: "running", Stage: "pull", CreatedAt: time.Now().UTC(), ReleaseIDs: []string{}}
	if e = os.MkdirAll(filepath.Dir(a.buildJobPath(j.ID)), 0700); e == nil { e = atomicJSON(a.buildJobPath(j.ID), j) }
	if e != nil { a.buildMu.Unlock(); fail(w, 500, "创建构建任务失败"); return }
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	a.activeBuild = j.ID; a.cancelBuild = cancel; a.buildReceipts = map[string][]string{j.ID: {}}
	a.buildMu.Unlock()
	go a.runBuild(ctx, cancel, j, source, profile)
	respond(w, 202, j)
}
// Child scripts may publish more than one artifact. Track accepted uploads,
// including idempotent retries, rather than trusting a successful shell exit.
func (a *App) recordBuildPublication(job, project, release string) {
	if job == "" {
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != job {
		return
	}
	b, e := os.ReadFile(a.buildJobPath(job))
	if e != nil {
		return
	}
	var j BuildJob
	if json.Unmarshal(b, &j) != nil || j.ProjectID != project {
		return
	}
	for _, id := range a.buildReceipts[job] {
		if id == release {
			return
		}
	}
	a.buildReceipts[job] = append(a.buildReceipts[job], release)
	j.ReleaseIDs = append([]string{}, a.buildReceipts[job]...)
	_ = atomicJSON(a.buildJobPath(job), j)
}
func (a *App) buildJobs(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	out := []BuildJob{}
	paths, _ := filepath.Glob(filepath.Join(a.data, "builds", "*", "job.json"))
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		var j BuildJob
		if json.Unmarshal(b, &j) != nil || j.ProjectID != r.PathValue("project") {
			continue
		}
		if j.Status == "running" && j.ID != a.activeBuild {
			j.Status = "failed"
			j.Error = "服务重启导致任务中断，请检查日志和已发布产物后重试"
		}
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 30 {
		out = out[:30]
	}
	respond(w, 200, out)
}
func (a *App) buildLog(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("job")
	if !jobRE.MatchString(id) {
		fail(w, 404, "任务不存在")
		return
	}
	b, e := os.ReadFile(filepath.Join(a.data, "builds", id, "build.log"))
	if e != nil {
		fail(w, 404, "日志尚未生成")
		return
	}
	respond(w, 200, map[string]string{"log": strings.ReplaceAll(string(b), a.token, "[REDACTED]")})
}


var buildEventStages = map[string]bool{
	"preflight": true, "pull": true, "build": true, "archive": true, "validate": true,
	"export": true, "package": true, "notarize": true, "upload": true,
	"submitted": true, "processing": true, "complete": true,
}
var buildEventStates = map[string]bool{
	"queued": true, "started": true, "running": true, "succeeded": true, "failed": true,
}

type eventLog struct {
	dst     io.Writer
	mu      sync.Mutex
	pending string
	secret  string
	onEvent func(BuildEvent)
}

func (l *eventLog) Write(p []byte) (int, error) {
	if _, e := l.dst.Write(p); e != nil {
		return 0, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending += string(p)
	for {
		i := strings.IndexByte(l.pending, '\n')
		if i < 0 {
			if len(l.pending) > 64<<10 {
				l.pending = l.pending[len(l.pending)-(64<<10):]
			}
			break
		}
		line := strings.TrimSpace(l.pending[:i])
		l.pending = l.pending[i+1:]
		if !strings.HasPrefix(line, "ILS_EVENT ") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "ILS_EVENT "))
		if len(raw) == 0 || len(raw) > 16<<10 {
			continue
		}
		var ev BuildEvent
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.DisallowUnknownFields()
		if dec.Decode(&ev) != nil || !buildEventStages[ev.Stage] || !buildEventStates[ev.State] {
			continue
		}
		if ev.Progress != nil && (*ev.Progress < 0 || *ev.Progress > 100) {
			continue
		}
		ev.Message = strings.ReplaceAll(ev.Message, l.secret, "[REDACTED]")
		if len(ev.Message) > 1000 {
			ev.Message = ev.Message[:1000]
		}
		if l.onEvent != nil {
			l.onEvent(ev)
		}
	}
	return len(p), nil
}

func readBuildResult(output string) (BuildResult, error) {
	var result BuildResult
	path := filepath.Join(output, "ils-result.json")
	b, e := os.ReadFile(path)
	if e != nil {
		return result, fmt.Errorf("缺少 ILS_OUTPUT_DIR/ils-result.json")
	}
	if len(b) > 64<<10 {
		return result, fmt.Errorf("ils-result.json 超过 64 KiB")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&result); e != nil {
		return result, fmt.Errorf("ils-result.json 格式无效：%v", e)
	}
	if result.SchemaVersion != 1 {
		return result, fmt.Errorf("ils-result.json schema_version 必须为 1")
	}
	return result, nil
}

func outputArtifact(output, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("ils-result.json 缺少 artifact")
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(output, filepath.FromSlash(candidate))
	}
	outputReal, e := filepath.EvalSymlinks(output)
	if e != nil {
		return "", fmt.Errorf("无法解析 ILS_OUTPUT_DIR")
	}
	real, e := filepath.EvalSymlinks(candidate)
	if e != nil {
		return "", fmt.Errorf("找不到结果产物")
	}
	rel, e := filepath.Rel(outputReal, real)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("结果产物必须位于 ILS_OUTPUT_DIR 内")
	}
	info, e := os.Stat(real)
	if e != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("结果产物不是普通文件")
	}
	return real, nil
}

func fileSHA256(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func validateContractResult(profile ReleaseProfile, result BuildResult, output string) (string, error) {
	if result.Platform != profile.Platform || result.Architecture != profile.Architecture || result.Lane != profile.Lane {
		return "", fmt.Errorf("ils-result.json 与 Profile 的 platform / architecture / lane 不一致")
	}
	if !validVersion(result.Version) {
		return "", fmt.Errorf("ils-result.json version 不是有效 SemVer")
	}
	if _, e := parseBuild(result.Build); e != nil {
		return "", fmt.Errorf("ils-result.json build 必须是正整数")
	}
	if profile.Lane == "ios-testflight" {
		if result.Status != "submitted" || result.Distribution != "app-store-connect" || result.SubmissionResult != "upload-succeeded" {
			return "", fmt.Errorf("TestFlight 结果必须明确为 submitted + app-store-connect + upload-succeeded")
		}
		if result.Artifact != "" {
			return "", fmt.Errorf("TestFlight 结果不应伪造本地发布 artifact")
		}
		return "", nil
	}
	if result.Status != "succeeded" {
		return "", fmt.Errorf("本地安装包结果 status 必须为 succeeded")
	}
	artifact, e := outputArtifact(output, result.Artifact)
	if e != nil {
		return "", e
	}
	ext := strings.ToLower(filepath.Ext(artifact))
	if (profile.Platform == "ios" && ext != ".ipa") || (profile.Platform == "macos" && !oneOf(ext, ".dmg", ".pkg", ".zip")) {
		return "", fmt.Errorf("ils-result.json 产物类型与平台不匹配")
	}
	if result.SHA256 != "" {
		actual, e := fileSHA256(artifact)
		if e != nil {
			return "", fmt.Errorf("无法计算结果产物 SHA-256")
		}
		if !strings.EqualFold(result.SHA256, actual) {
			return "", fmt.Errorf("ils-result.json SHA-256 与实际产物不一致")
		}
	}
	if profile.Platform == "ios" {
		ipa, e := inspectIPA(artifact)
		if e != nil {
			return "", fmt.Errorf("IPA 检查失败：%v", e)
		}
		if ipa.Version != result.Version || ipa.Build != result.Build {
			return "", fmt.Errorf("ils-result.json version/build 与 IPA 不一致")
		}
		if result.BundleID != "" && ipa.BundleID != result.BundleID {
			return "", fmt.Errorf("ils-result.json bundle_id 与 IPA 不一致")
		}
	}
	return artifact, nil
}

// Bound disk use while continuing to drain process output.
type cappedLog struct {
	mu      sync.Mutex
	file    *os.File
	written int
	secret  string
	pending string
}

func (l *cappedLog) emit(p string) {
	left := (2 << 20) - l.written
	if left <= 0 {
		return
	}
	if len(p) > left {
		p = p[:left]
	}
	_, _ = l.file.WriteString(p)
	l.written += len(p)
	if l.written == 2<<20 {
		_, _ = l.file.WriteString("\n[日志达到 2 MiB 上限，后续输出省略]\n")
	}
}
func (l *cappedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(p)
	l.pending = strings.ReplaceAll(l.pending+string(p), l.secret, "[REDACTED]")
	keep := len(l.secret) - 1
	if len(l.pending) > keep {
		cut := len(l.pending) - keep
		l.emit(l.pending[:cut])
		l.pending = l.pending[cut:]
	}
	return n, nil
}
func (l *cappedLog) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.emit(strings.ReplaceAll(l.pending, l.secret, "[REDACTED]"))
	l.pending = ""
}
func runProcess(ctx context.Context, root string, log io.Writer, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 3 * time.Second
	return cmd.Run()
}
type smallCapture struct { data []byte; truncated bool }
func (w *smallCapture) Write(p []byte) (int, error) {
	n := len(p); const limit = 16 << 10
	if len(w.data) < limit {
		keep := limit-len(w.data)
		if len(p)>keep { w.data=append(w.data,p[:keep]...); w.truncated=true } else { w.data=append(w.data,p...) }
	} else if len(p)>0 { w.truncated=true }
	return n,nil
}
func runValueCommand(ctx context.Context, root string, log io.Writer, env []string, command string) (string, error) {
	var out smallCapture
	cmd := exec.CommandContext(ctx, "/bin/bash", "-lc", command)
	cmd.Dir=root; cmd.Env=env; cmd.Stdout=&out; cmd.Stderr=log
	cmd.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}
	cmd.Cancel=func() error { if cmd.Process!=nil { return syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL) }; return nil }
	cmd.WaitDelay=3*time.Second
	if e:=cmd.Run(); e!=nil { return "",e }
	if out.truncated { return "",fmt.Errorf("命令输出超过 16 KiB") }
	value:=strings.TrimSpace(string(out.data))
	if value=="" || strings.ContainsAny(value,"\r\n") { return "",fmt.Errorf("命令必须只输出一个值") }
	return value,nil
}
func resolveProfileArtifact(root, output, pattern string) (string,error) {
	replacer:=strings.NewReplacer("${ILS_OUTPUT_DIR}",output,"$ILS_OUTPUT_DIR",output,"${LOCALSERVICE_OUTPUT_DIR}",output,"$LOCALSERVICE_OUTPUT_DIR",output)
	pattern=replacer.Replace(pattern)
	if strings.Contains(pattern,"$") { return "",fmt.Errorf("产物路径包含不支持的环境变量") }
	if !filepath.IsAbs(pattern) { pattern=filepath.Join(root,filepath.FromSlash(pattern)) }
	matches,e:=filepath.Glob(pattern)
	if e!=nil { return "",fmt.Errorf("产物路径无效") }
	if len(matches)!=1 { return "",fmt.Errorf("产物路径必须唯一匹配一个文件，当前匹配 %d 个",len(matches)) }
	real,e:=filepath.EvalSymlinks(matches[0]); if e!=nil { return "",fmt.Errorf("找不到构建产物") }
	info,e:=os.Stat(real); if e!=nil || !info.Mode().IsRegular() { return "",fmt.Errorf("构建产物不是普通文件") }
	return real,nil
}
func (a *App) runBuild(ctx context.Context, cancel context.CancelFunc, j BuildJob, source BuildSource, profile *ReleaseProfile) {
	defer func(){ cancel(); a.buildMu.Lock(); a.activeBuild=""; a.cancelBuild=nil; a.buildMu.Unlock() }()
	finish:=func(status,message string){
		a.buildMu.Lock(); j.ReleaseIDs=append([]string{},a.buildReceipts[j.ID]...); a.buildMu.Unlock()
		j.Status=status; j.Error=message; now:=time.Now().UTC(); j.FinishedAt=&now; _=a.writeBuild(j)
	}
	f,e:=os.OpenFile(filepath.Join(a.data,"builds",j.ID,"build.log"),os.O_CREATE|os.O_WRONLY,0600)
	if e!=nil { finish("failed","无法创建日志"); return }
	defer f.Close()
	log:=&cappedLog{file:f,secret:a.token}; defer log.Flush()
	info,e:=inspectSource(source)
	if e!=nil || info.Blocker!="" { finish("failed","执行前 Git 状态发生变化，请重新扫描"); return }
	env:=append(os.Environ(),"GIT_TERMINAL_PROMPT=0")
	fmt.Fprintln(log,"ILS: git pull --ff-only (hooks and autostash disabled)")
	pullCtx,pullCancel:=context.WithTimeout(ctx,3*time.Minute)
	e=runProcess(pullCtx,source.Path,log,env,"git","-c","core.hooksPath=/dev/null","-c","rebase.autoStash=false","-c","merge.autoStash=false","pull","--ff-only")
	pullCancel()
	if e!=nil { finish("failed","git pull 失败，未执行构建；详情见日志"); return }
	info,e=inspectSource(source)
	if e!=nil || info.Blocker!="" { finish("failed","拉取后的 Git 状态不满足发布条件，请重新扫描"); return }
	upstreamHead,e:=gitRead(source.Path,"rev-parse","@{upstream}")
	if e!=nil || info.Head!=upstreamHead { finish("failed","本地 HEAD 与 upstream 不一致，请先由开发者处理"); return }
	if profile==nil {
		ready:=false; for _,x:=range info.Scripts { if x.Path==j.Script && x.Ready { ready=true } }
		if !ready { finish("failed","拉取后脚本已删除或说明不完整，请重新扫描"); return }
	}
	j.Commit=info.Head
	output:=filepath.Join(a.data,"builds",j.ID,"output")
	if e=os.MkdirAll(output,0700); e!=nil { finish("failed","无法创建产物目录"); return }
	tokenFile:=filepath.Join(a.data,"admin-token")
	env=append(env,
		"LOCALSERVICE_URL="+a.buildOrigin,"LOCALSERVICE_TOKEN_FILE="+tokenFile,"LOCALSERVICE_ROOT="+a.buildRoot,
		"LOCALSERVICE_PROJECT_ID="+j.ProjectID,"LOCALSERVICE_JOB_ID="+j.ID,"LOCALSERVICE_OUTPUT_DIR="+output,"LOCALSERVICE_GIT_COMMIT="+j.Commit,
		"ILS_URL="+a.buildOrigin,"ILS_TOKEN_FILE="+tokenFile,"ILS_ROOT="+a.buildRoot,"ILS_PROJECT_ID="+j.ProjectID,
		"ILS_JOB_ID="+j.ID,"ILS_OUTPUT_DIR="+output,"ILS_GIT_COMMIT="+j.Commit)
	var progressMu sync.Mutex
	jobLog:=&eventLog{dst:log,secret:a.token,onEvent:func(ev BuildEvent){
		progressMu.Lock()
		defer progressMu.Unlock()
		j.Stage=ev.Stage
		j.StageState=ev.State
		j.Progress=ev.Progress
		j.Message=ev.Message
		_=a.writeBuild(j)
	}}
	if profile!=nil {
		env=append(env,
			"ILS_PLATFORM="+profile.Platform,
			"ILS_CHANNEL="+profile.Channel,
			"ILS_ARCHITECTURE="+profile.Architecture,
			"ILS_VARIANT="+profile.Variant,
			"ILS_LANE="+profile.Lane)
		if profile.AppleTeamID!="" {
			env=append(env,"ILS_APPLE_TEAM_ID="+profile.AppleTeamID)
		}
	}
	if profile==nil {
		j.Stage="script"; _=a.writeBuild(j)
		fmt.Fprintf(log,"Source %s\nRun project script: /bin/bash %s\n",j.Commit,j.Script)
		if e=runProcess(ctx,source.Path,jobLog,env,"/bin/bash",j.Script); e!=nil { finish("failed","项目发布脚本失败或任务超时；已上传的包会保留，请查看日志"); return }
	} else {
		j.Stage="build"; _=a.writeBuild(j)
		fmt.Fprintf(log,"Source %s\nILS Release Profile: %s (%s)\n",j.Commit,profile.Name,profile.ID)
		if e=runProcess(ctx,source.Path,jobLog,env,"/bin/bash","-lc",profile.BuildCommand); e!=nil { finish("failed","ILS Build Command 失败或任务超时；详情见日志"); return }
		if profile.PackageCommand!="" {
			j.Stage="package"; _=a.writeBuild(j)
			if e=runProcess(ctx,source.Path,jobLog,env,"/bin/bash","-lc",profile.PackageCommand); e!=nil { finish("failed","ILS Package Command 失败或任务超时；详情见日志"); return }
		}
		var artifact,version,build string
		if profile.ResultContract=="ils-result-v1" {
			j.Stage="validate"; j.StageState="running"; j.Progress=nil; _=a.writeBuild(j)
			result,resultErr:=readBuildResult(output)
			if resultErr!=nil { finish("failed",resultErr.Error()); return }
			artifact,resultErr=validateContractResult(*profile,result,output)
			if resultErr!=nil { finish("failed",resultErr.Error()); return }
			j.Result=&result
			version,build=result.Version,result.Build
			_=a.writeBuild(j)
			if profile.Lane=="ios-testflight" {
				j.Stage="complete"; j.StageState="succeeded"; j.Progress=nil
				if j.Message=="" { j.Message="App Store Connect 已接受上传；等待 Apple 处理" }
				finish("succeeded","")
				return
			}
		} else {
			artifact,e=resolveProfileArtifact(source.Path,output,profile.Artifact)
			if e!=nil { finish("failed",e.Error()); return }
			ext:=strings.ToLower(filepath.Ext(artifact))
			if (profile.Platform=="ios"&&ext!=".ipa") || (profile.Platform=="macos"&&!oneOf(ext,".dmg",".pkg",".zip")) { finish("failed","Profile 产物类型与平台不匹配"); return }
			if profile.Platform=="ios" {
				ipa,inspectErr:=inspectIPA(artifact); if inspectErr!=nil { finish("failed","IPA 检查失败："+inspectErr.Error()); return }
				version,build=ipa.Version,ipa.Build
			} else {
				j.Stage="metadata"; _=a.writeBuild(j)
				version,e=runValueCommand(ctx,source.Path,log,env,profile.VersionCommand); if e!=nil { finish("failed","无法读取 macOS version："+e.Error()); return }
				build,e=runValueCommand(ctx,source.Path,log,env,profile.BuildNumberCommand); if e!=nil { finish("failed","无法读取 macOS build number："+e.Error()); return }
			}
			if !validVersion(version) { finish("failed","Profile 解析出的 version 不是有效 SemVer"); return }
			if _,e=parseBuild(build); e!=nil { finish("failed","Profile 解析出的 build number 必须是正整数"); return }
		}
		j.Stage="publish"; j.StageState="running"; j.Progress=nil; _=a.writeBuild(j)
		push:=filepath.Join(a.buildRoot,"scripts","push.sh")
		if stat,statErr:=os.Stat(push); statErr!=nil || !stat.Mode().IsRegular() { finish("failed","ILS scripts/push.sh 不可用"); return }
		profileEnv:=append(env,"RELEASE_NOTES="+profile.Notes)
		fmt.Fprintf(log,"Publish %s version %s build %s\n",filepath.Base(artifact),version,build)
		if e=runProcess(ctx,source.Path,jobLog,profileEnv,"/bin/bash",push,j.ProjectID,version,build,profile.Platform,artifact,profile.Channel,profile.Architecture,profile.Variant); e!=nil { finish("failed","ILS 发布失败；详情见日志"); return }
	}
	a.buildMu.Lock(); count:=len(a.buildReceipts[j.ID]); a.buildMu.Unlock()
	if count==0 { finish("failed","构建命令退出成功，但没有收到与本任务关联的发布记录"); return }
	j.Stage="complete"; j.StageState="succeeded"; j.Progress=nil; finish("succeeded","")
}
func (a *App) ConfigureBuildRunner(origin, root string) error {
	u, e := url.Parse(origin)
	if e != nil || !oneOf(u.Scheme, "http", "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("build-origin 必须为构建进程可访问的服务 origin")
	}
	root, e = filepath.Abs(root)
	if e != nil {
		return e
	}
	a.buildOrigin = origin
	a.buildRoot = root
	return nil
}
func (a *App) StopBuild() {
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.cancelBuild != nil {
		a.cancelBuild()
	}
}
