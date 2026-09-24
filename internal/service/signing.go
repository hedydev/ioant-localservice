package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// This trusted local configuration is never accepted from an HTTP caller.
// Xcode retains account credentials; browsers may only trigger known archives.
type SigningConfig struct {
	Projects map[string]SigningTarget `json:"projects"`
}
type SigningTarget struct {
	Archive                  string `json:"archive"`
	TeamID                   string `json:"team_id"`
	Method                   string `json:"method"`
	Channel                  string `json:"channel"`
	AllowProvisioningUpdates bool   `json:"allow_provisioning_updates"`
}
type SigningJob struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	ReleaseID string    `json:"release_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

var jobRE = regexp.MustCompile(`^[a-f0-9]{32}$`)
var teamRE = regexp.MustCompile(`^[A-Z0-9]{10}$`)

func (a *App) writeJob(j SigningJob) error {
	a.signingMu.Lock()
	defer a.signingMu.Unlock()
	raw, e := json.Marshal(j)
	if e != nil {
		return e
	}
	path := filepath.Join(a.data, "signing", j.ID, "job.json")
	if e = os.WriteFile(path+".tmp", raw, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func (a *App) startSigning(w http.ResponseWriter, r *http.Request) {
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
	raw, e := os.ReadFile(filepath.Join(a.data, "signing.json"))
	if e != nil {
		fail(w, 409, "请先在 Mac 配置 .localservice/signing.json，指定项目归档与签名团队。参见 README 的签名服务配置。")
		return
	}
	var config SigningConfig
	if e = json.Unmarshal(raw, &config); e != nil {
		fail(w, 500, "签名配置格式无效")
		return
	}
	target, ok := config.Projects[id]
	if !ok {
		fail(w, 409, "此项目尚未配置签名归档")
		return
	}
	if target.Method == "" {
		target.Method = "release-testing"
	}
	if target.Channel == "" {
		target.Channel = "dev"
	}
	if !filepath.IsAbs(target.Archive) || filepath.Ext(target.Archive) != ".xcarchive" || !teamRE.MatchString(target.TeamID) || !oneOf(target.Method, "release-testing", "debugging") || !oneOf(target.Channel, "dev", "beta", "stable") {
		fail(w, 500, "归档路径、签名团队、导出方式或渠道配置无效")
		return
	}
	if info, e := os.Stat(target.Archive); e != nil || !info.IsDir() {
		fail(w, 409, "Mac 上找不到配置的 xcarchive，请先构建项目归档")
		return
	}
	a.signingMu.Lock()
	if a.signingBusy {
		a.signingMu.Unlock()
		fail(w, 409, "已有签名任务运行，请稍后再试")
		return
	}
	a.signingBusy = true
	a.signingMu.Unlock()
	j := SigningJob{ID: randomID(16), ProjectID: id, Status: "running", CreatedAt: time.Now().UTC()}
	if e = os.MkdirAll(filepath.Join(a.data, "signing", j.ID), 0700); e == nil {
		e = a.writeJob(j)
	}
	if e != nil {
		a.signingMu.Lock()
		a.signingBusy = false
		a.signingMu.Unlock()
		fail(w, 500, "无法保存签名任务")
		return
	}
	go a.runSigning(j, target)
	respond(w, 202, j)
}
func (a *App) getSigning(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("job")
	if !jobRE.MatchString(id) {
		fail(w, 404, "任务不存在")
		return
	}
	a.signingMu.Lock()
	defer a.signingMu.Unlock()
	raw, e := os.ReadFile(filepath.Join(a.data, "signing", id, "job.json"))
	if e != nil {
		fail(w, 404, "任务不存在")
		return
	}
	var job SigningJob
	if json.Unmarshal(raw, &job) != nil {
		fail(w, 500, "任务状态损坏")
		return
	}
	if job.Status == "running" && !a.signingBusy {
		job.Status = "failed"
		job.Error = "服务重启，任务已中断，请重新发起签名"
	}
	respond(w, 200, job)
}
func (a *App) runSigning(j SigningJob, target SigningTarget) {
	defer func() { a.signingMu.Lock(); a.signingBusy = false; a.signingMu.Unlock() }()
	dir := filepath.Join(a.data, "signing", j.ID)
	failed := func(message string) { j.Status = "failed"; j.Error = message; _ = a.writeJob(j) }
	options := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>method</key><string>%s</string><key>teamID</key><string>%s</string><key>signingStyle</key><string>automatic</string><key>manageAppVersionAndBuildNumber</key><false/></dict></plist>`, target.Method, target.TeamID)
	optionsPath := filepath.Join(dir, "ExportOptions.plist")
	if e := os.WriteFile(optionsPath, []byte(options), 0600); e != nil {
		failed("无法写入导出配置")
		return
	}
	log, e := os.OpenFile(filepath.Join(dir, "xcodebuild.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		failed("无法创建签名日志")
		return
	}
	defer log.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	args := []string{"-exportArchive", "-archivePath", target.Archive, "-exportPath", filepath.Join(dir, "export"), "-exportOptionsPlist", optionsPath}
	if target.AllowProvisioningUpdates {
		args = append(args, "-allowProvisioningUpdates")
	}
	command := exec.CommandContext(ctx, "/usr/bin/xcodebuild", args...)
	command.Stdout = log
	command.Stderr = log
	if e = command.Run(); e != nil {
		failed("Xcode 导出失败：请在 Mac 查看 .localservice/signing/" + j.ID + "/xcodebuild.log，检查账号、证书和描述文件")
		return
	}
	files, _ := filepath.Glob(filepath.Join(dir, "export", "*.ipa"))
	if len(files) != 1 {
		failed("Xcode 未生成唯一 IPA")
		return
	}
	info, e := inspectIPA(files[0])
	if e != nil {
		failed(e.Error())
		return
	}
	build, e := parseBuild(info.Build)
	if e != nil || !validVersion(info.Version) {
		failed("IPA 版本须为 x.y.z，构建号须为正整数")
		return
	}
	// Reuse the same streaming upload validation and idempotency as external CI.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var writeErr error
		defer func() {
			if writeErr == nil {
				writeErr = mw.Close()
			}
			_ = pw.CloseWithError(writeErr)
		}()
		for key, value := range map[string]string{"version": info.Version, "build": strconv.FormatInt(build, 10), "platform": "ios", "architecture": "arm64", "channel": target.Channel, "notes": "由 Mac 签名服务导出"} {
			if writeErr = mw.WriteField(key, value); writeErr != nil {
				return
			}
		}
		f, e := os.Open(files[0])
		if e != nil {
			writeErr = e
			return
		}
		defer f.Close()
		part, e := mw.CreateFormFile("file", filepath.Base(files[0]))
		if e != nil {
			writeErr = e
			return
		}
		_, writeErr = io.Copy(part, f)
	}()
	req := httptest.NewRequest("POST", "/api/projects/"+j.ProjectID+"/releases", pr)
	req.SetPathValue("project", j.ProjectID)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+a.token)
	rec := httptest.NewRecorder()
	a.upload(rec, req)
	_ = pr.Close()
	if rec.Code != 200 && rec.Code != 201 {
		var result map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		failed("签名完成，但发布失败：" + result["error"])
		return
	}
	var release Release
	_ = json.Unmarshal(rec.Body.Bytes(), &release)
	j.Status = "succeeded"
	j.ReleaseID = release.ID
	_ = a.writeJob(j)
}
