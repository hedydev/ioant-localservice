package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

type Device struct {
	UDID             string    `json:"udid"`
	Product          string    `json:"product"`
	Version          string    `json:"version"`
	CollectedAt      time.Time `json:"collected_at"`
	Status           string    `json:"status"`
	IdentityVerified bool      `json:"identity_verified"`
	Source           string    `json:"source,omitempty"`
}

var udidRE = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{8}-[0-9a-fA-F]{16})$`)

func (a *App) enrollmentProfile(w http.ResponseWriter, r *http.Request) {
	if a.publicURL == "" {
		fail(w, 409, "设备登记需要先配置 iPhone 信任的 HTTPS 地址（public-url）")
		return
	}
	a.enrollmentMu.Lock()
	defer a.enrollmentMu.Unlock()
	if a.enrollments == nil {
		a.enrollments = map[string]time.Time{}
	}
	for k, expires := range a.enrollments {
		if time.Now().After(expires) {
			delete(a.enrollments, k)
		}
	}
	if len(a.enrollments) >= 128 {
		fail(w, 429, "登记请求过多，请稍后再试")
		return
	}
	challenge := randomID(24)
	a.enrollments[challenge] = time.Now().Add(15 * time.Minute)
	uuid := randomID(16)
	uuid = uuid[:8] + "-" + uuid[8:12] + "-" + uuid[12:16] + "-" + uuid[16:20] + "-" + uuid[20:]
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="localservice-device.mobileconfig"`)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>PayloadContent</key><dict><key>URL</key><string>%s</string><key>DeviceAttributes</key><array><string>UDID</string><string>PRODUCT</string><string>VERSION</string></array><key>Challenge</key><string>%s</string></dict><key>PayloadOrganization</key><string>Localservice</string><key>PayloadDisplayName</key><string>Localservice 设备信息收集</string><key>PayloadDescription</key><string>仅收集 UDID、设备型号与系统版本供管理员登记测试设备。不会安装根证书、MDM 或授予应用签名权限。</string><key>PayloadVersion</key><integer>1</integer><key>PayloadUUID</key><string>%s</string><key>PayloadIdentifier</key><string>local.ioant.localservice.enrollment.%s</string><key>PayloadType</key><string>Profile Service</string></dict></plist>`, escapeXML(a.publicURL+"/api/devices/callback/"+challenge), challenge, uuid, uuid)
}
func (a *App) enrollmentCallback(w http.ResponseWriter, r *http.Request) {
	challenge := r.PathValue("challenge")
	a.enrollmentMu.Lock()
	expires, exists := a.enrollments[challenge]
	a.enrollmentMu.Unlock()
	if !exists || time.Now().After(expires) {
		fail(w, 410, "登记链接已过期，请重新下载描述文件")
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		fail(w, 400, "设备响应过大")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Verify CMS content integrity, but do NOT claim Apple device identity: this
	// intentionally does not pin Apple's device CA. Records need admin review.
	cmd := exec.CommandContext(ctx, "/usr/bin/openssl", "cms", "-verify", "-inform", "DER", "-noverify")
	cmd.Stdin = bytes.NewReader(raw)
	decoded, e := cmd.Output()
	if e != nil {
		fail(w, 400, "设备响应 CMS 签名无效")
		return
	}
	var fields map[string]any
	if e = plistJSON(decoded, &fields); e != nil {
		fail(w, 400, "设备响应格式无效")
		return
	}
	str := func(key string) string { s, _ := fields[key].(string); return s }
	if str("CHALLENGE") != challenge || !udidRE.MatchString(str("UDID")) {
		fail(w, 400, "设备 UDID 或登记凭证无效")
		return
	}
	d := Device{UDID: str("UDID"), Product: str("PRODUCT"), Version: str("VERSION"), CollectedAt: time.Now().UTC(), Status: "pending_apple_registration", IdentityVerified: false, Source: "local_ils"}
	a.enrollmentMu.Lock()
	defer a.enrollmentMu.Unlock()
	if current, ok := a.enrollments[challenge]; !ok || time.Now().After(current) {
		fail(w, 410, "登记链接已使用或过期")
		return
	}
	if _, e = a.saveDeviceRecord(d); e != nil {
		fail(w, 500, "设备信息保存失败")
		return
	}
	delete(a.enrollments, challenge)
	http.Redirect(w, r, a.publicURL+"/?enrollment=collected", http.StatusSeeOther)
}
func (a *App) saveDeviceRecord(d Device) (bool, error) {
	dir := filepath.Join(a.data, "devices")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return false, e
	}
	hash := sha256.Sum256([]byte(d.UDID))
	path := filepath.Join(dir, hex.EncodeToString(hash[:])+".json")
	_, statErr := os.Stat(path)
	created := os.IsNotExist(statErr)
	if d.Source == "" {
		d.Source = "legacy"
	}
	b, e := json.Marshal(d)
	if e != nil {
		return false, e
	}
	if e = os.WriteFile(path+".tmp", b, 0600); e == nil {
		e = os.Rename(path+".tmp", path)
	}
	return created, e
}

func (a *App) listDevices(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	a.enrollmentMu.Lock()
	defer a.enrollmentMu.Unlock()
	paths, _ := filepath.Glob(filepath.Join(a.data, "devices", "*.json"))
	devices := []Device{}
	for _, p := range paths {
		raw, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		var d Device
		if json.Unmarshal(raw, &d) == nil {
			devices = append(devices, d)
		}
	}
	respond(w, 200, devices)
}
