package service

import (
	"context"
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
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Script     string     `json:"script"`
	Status     string     `json:"status"`
	Stage      string     `json:"stage"`
	Error      string     `json:"error,omitempty"`
	Commit     string     `json:"commit,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ReleaseIDs []string   `json:"release_ids"`
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
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("project")
	s, e := a.readSource(id)
	if e != nil {
		fail(w, 409, "请先关联项目目录")
		return
	}
	var input struct {
		Script string `json:"script"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || !releaseScriptRE.MatchString(input.Script) {
		fail(w, 400, "请选择已识别的发布脚本")
		return
	}
	info, e := inspectSource(s)
	if e != nil {
		fail(w, 409, e.Error())
		return
	}
	if info.Blocker != "" {
		fail(w, 409, info.Blocker)
		return
	}
	found := false
	for _, x := range info.Scripts {
		if x.Path == input.Script && x.Ready {
			found = true
		}
	}
	if !found {
		fail(w, 409, "脚本或 Markdown 说明不可用")
		return
	}
	a.buildMu.Lock()
	if a.activeBuild != "" {
		a.buildMu.Unlock()
		fail(w, 409, "已有构建正在执行，请稍后发布")
		return
	}
	j := BuildJob{ID: randomID(16), ProjectID: id, Script: input.Script, Status: "running", Stage: "pull", CreatedAt: time.Now().UTC(), ReleaseIDs: []string{}}
	if e = os.MkdirAll(filepath.Dir(a.buildJobPath(j.ID)), 0700); e == nil {
		e = atomicJSON(a.buildJobPath(j.ID), j)
	}
	if e != nil {
		a.buildMu.Unlock()
		fail(w, 500, "创建构建任务失败")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	a.activeBuild = j.ID
	a.cancelBuild = cancel
	a.buildReceipts = map[string][]string{j.ID: {}}
	a.buildMu.Unlock()
	go a.runBuild(ctx, cancel, j, s)
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
func (a *App) runBuild(ctx context.Context, cancel context.CancelFunc, j BuildJob, s BuildSource) {
	defer func() { cancel(); a.buildMu.Lock(); a.activeBuild = ""; a.cancelBuild = nil; a.buildMu.Unlock() }()
	finish := func(status, message string) {
		a.buildMu.Lock()
		j.ReleaseIDs = append([]string{}, a.buildReceipts[j.ID]...)
		a.buildMu.Unlock()
		j.Status = status
		j.Error = message
		now := time.Now().UTC()
		j.FinishedAt = &now
		_ = a.writeBuild(j)
	}
	f, e := os.OpenFile(filepath.Join(a.data, "builds", j.ID, "build.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		finish("failed", "无法创建日志")
		return
	}
	defer f.Close()
	log := &cappedLog{file: f, secret: a.token}
	defer log.Flush()
	// Recheck immediately before mutating this local working directory.
	info, e := inspectSource(s)
	if e != nil || info.Blocker != "" {
		finish("failed", "执行前 Git 状态发生变化，请重新扫描")
		return
	}
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	fmt.Fprintln(log, "执行 git pull --ff-only（禁用 Git hooks / autostash）")
	pullCtx, pullCancel := context.WithTimeout(ctx, 3*time.Minute)
	e = runProcess(pullCtx, s.Path, log, env, "git", "-c", "core.hooksPath=/dev/null", "-c", "rebase.autoStash=false", "-c", "merge.autoStash=false", "pull", "--ff-only")
	pullCancel()
	if e != nil {
		finish("failed", "git pull 失败，未执行发布脚本；详情见日志")
		return
	}
	info, e = inspectSource(s)
	if e != nil || info.Blocker != "" {
		finish("failed", "拉取后的 Git 状态不满足发布条件，请重新扫描")
		return
	}
	upstreamHead, e := gitRead(s.Path, "rev-parse", "@{upstream}")
	if e != nil || info.Head != upstreamHead {
		finish("failed", "本地 HEAD 与 upstream 不一致，请先由开发者处理")
		return
	}
	scriptReady := false
	for _, x := range info.Scripts {
		if x.Path == j.Script && x.Ready {
			scriptReady = true
		}
	}
	if !scriptReady {
		finish("failed", "拉取后脚本已删除或说明不完整，请重新扫描")
		return
	}
	j.Commit = info.Head
	j.Stage = "script"
	_ = a.writeBuild(j)
	output := filepath.Join(a.data, "builds", j.ID, "output")
	if e = os.MkdirAll(output, 0700); e != nil {
		finish("failed", "无法创建产物目录")
		return
	}
	env = append(env, "LOCALSERVICE_URL="+a.buildOrigin, "LOCALSERVICE_TOKEN_FILE="+filepath.Join(a.data, "admin-token"), "LOCALSERVICE_ROOT="+a.buildRoot, "LOCALSERVICE_PROJECT_ID="+j.ProjectID, "LOCALSERVICE_JOB_ID="+j.ID, "LOCALSERVICE_OUTPUT_DIR="+output, "LOCALSERVICE_GIT_COMMIT="+j.Commit)
	fmt.Fprintf(log, "源码 %s\n执行 /bin/bash %s\n", j.Commit, j.Script)
	e = runProcess(ctx, s.Path, log, env, "/bin/bash", j.Script)
	if e != nil {
		finish("failed", "脚本失败或任务超时；已上传的包会保留，请查看日志")
		return
	}
	a.buildMu.Lock()
	count := len(a.buildReceipts[j.ID])
	a.buildMu.Unlock()
	if count == 0 {
		finish("failed", "脚本退出成功，但未收到带本任务 job_id 的发布记录；请按规范调用 push.sh")
		return
	}
	j.Stage = "complete"
	finish("succeeded", "")
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
