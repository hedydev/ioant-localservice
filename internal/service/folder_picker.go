package service

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// The browser cannot obtain an absolute host path from a directory upload.
// Open the picker on the service Mac instead; never upload directory contents.
func (a *App) selectFolder(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	if runtime.GOOS != "darwin" {
		fail(w, 409, "原生文件夹选择仅支持 macOS，请手动填写路径")
		return
	}
	if !a.folderPickerMu.TryLock() {
		fail(w, 409, "Mac 上已有文件夹选择窗口，请先完成或取消选择")
		return
	}
	defer a.folderPickerMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	// Fixed AppleScript; no request parameters are interpolated into executable code.
	script := `tell application "Finder"
 activate
 try
  set chosenFolder to choose folder with prompt "选择 Localservice 要关联的 Git 项目文件夹"
  return POSIX path of chosenFolder
 on error number -128
  return ""
 end try
end tell`
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).Output()
	if err != nil {
		if ctx.Err() != nil {
			fail(w, 408, "文件夹选择已超时，请重试或手动输入路径")
			return
		}
		fail(w, 409, "无法打开 Mac 文件夹窗口。请确认 Mac 已登录桌面，并允许服务控制 Finder（系统设置 → 隐私与安全性 → 自动化），也可手动填写路径。")
		return
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		respond(w, 200, map[string]any{"cancelled": true})
		return
	}
	root, err := canonicalRepo(path)
	if err != nil {
		fail(w, 400, "所选文件夹不是可用的 Git 项目："+err.Error())
		return
	}
	respond(w, 200, map[string]any{"cancelled": false, "path": root})
}
