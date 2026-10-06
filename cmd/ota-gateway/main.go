package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const challengeTTL = time.Hour

var (
	udidRE      = regexp.MustCompile("^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{8}-[0-9a-fA-F]{16})$")
	challengeRE = regexp.MustCompile("^[0-9a-f]{48}$")
)

type pendingDevice struct {
	ID          string    `json:"id"`
	UDID        string    `json:"udid"`
	Product     string    `json:"product"`
	Version     string    `json:"version"`
	CollectedAt time.Time `json:"collected_at"`
}

type gateway struct {
	publicURL          string
	data               string
	token              string
	profileSigningCert string
	profileSigningKey  string
	profileSigningCA   string

	mu         sync.Mutex
	challenges map[string]time.Time
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func readToken(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\x00") {
		return "", fmt.Errorf("invalid sync token")
	}
	return token, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (g *gateway) authorized(r *http.Request) bool {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(g.token)) == 1
}

func (g *gateway) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func regularReadableFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func (g *gateway) profileSigningConfigured() bool {
	return regularReadableFile(g.profileSigningCert) && regularReadableFile(g.profileSigningKey)
}

func (g *gateway) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"service":        "ils-adhoc-ota",
		"profile_signed": g.profileSigningConfigured(),
	})
}

func (g *gateway) enrollPage(w http.ResponseWriter, r *http.Request) {
	collected := r.URL.Query().Get("collected") == "1"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	title := "登记这台 iPhone / iPad"
	body := "<p>ILS 只收集 UDID、设备型号和 iOS 版本，用于管理员登记 Ad Hoc 测试设备。不会安装 MDM、根证书或授予额外设备权限。</p>" +
		"<a class=\"button\" href=\"/enroll.mobileconfig\">下载设备登记描述文件</a>" +
		"<small>下载后请在“设置”中安装已下载的描述文件。登记链接 1 小时内有效，并且只能成功使用一次。</small>"
	if collected {
		title = "设备登记已完成"
		body = "<div class=\"ok\">设备信息已经安全提交到 ILS OTA Gateway。</div>" +
			"<p>这个描述文件只用于一次性读取 UDID、设备型号和系统版本。登记数据已经保存，后续同步到 ILS、Apple Developer 注册和 Ad Hoc 安装都不依赖它继续留在手机上。</p>" +
			"<p><strong>现在可以删除“ILS 设备信息收集”描述文件。</strong></p>" +
			"<small>路径：设置 → 通用 → VPN 与设备管理 → ILS 设备信息收集 → 移除已下载的描述文件。删除不会取消已经完成的设备登记。</small>"
	}

	page := "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
		"<title>ILS 设备登记</title><style>body{font-family:-apple-system,BlinkMacSystemFont,\"Segoe UI\",sans-serif;background:#f5f7fa;color:#182736;margin:0}" +
		"main{max-width:560px;margin:0 auto;padding:48px 22px}.card{background:#fff;border:1px solid #dde4ea;border-radius:16px;padding:26px;box-shadow:0 8px 30px #14253812}" +
		"h1{font-size:24px;margin:0 0 10px}p{line-height:1.65;color:#536577}strong{color:#182736}a.button{display:inline-block;margin-top:10px;padding:12px 16px;border-radius:10px;background:#182736;color:#fff;text-decoration:none;font-weight:650}" +
		"small{display:block;margin-top:18px;color:#7a8794;line-height:1.55}.ok{padding:12px 14px;border-radius:10px;background:#eef7f1;color:#246e52;margin-bottom:16px}</style></head>" +
		"<body><main><div class=\"card\"><h1>" + title + "</h1>" + body + "</div></main></body></html>"
	_, _ = io.WriteString(w, page)
}

func (g *gateway) challengeDir() string {
	return filepath.Join(g.data, "challenges")
}

func (g *gateway) challengePath(challenge string) string {
	return filepath.Join(g.challengeDir(), challenge+".challenge")
}

func (g *gateway) loadChallengeLocked(challenge string) (time.Time, bool) {
	if !challengeRE.MatchString(challenge) {
		return time.Time{}, false
	}
	now := time.Now()
	if expires, ok := g.challenges[challenge]; ok {
		if now.Before(expires) {
			return expires, true
		}
		delete(g.challenges, challenge)
		_ = os.Remove(g.challengePath(challenge))
		return time.Time{}, false
	}
	raw, err := os.ReadFile(g.challengePath(challenge))
	if err != nil {
		return time.Time{}, false
	}
	expires, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(raw)))
	if err != nil || !now.Before(expires) {
		_ = os.Remove(g.challengePath(challenge))
		return time.Time{}, false
	}
	if g.challenges == nil {
		g.challenges = map[string]time.Time{}
	}
	g.challenges[challenge] = expires
	return expires, true
}

func (g *gateway) challengeValid(challenge string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.loadChallengeLocked(challenge)
	return ok
}

func (g *gateway) newChallenge() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	dir := g.challengeDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	now := time.Now()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	active := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".challenge") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, readErr := os.ReadFile(path)
		expires, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(raw)))
		if readErr != nil || parseErr != nil || !now.Before(expires) {
			_ = os.Remove(path)
			continue
		}
		active++
	}
	if active >= 256 {
		return "", fmt.Errorf("too many enrollment requests")
	}

	challenge := randomHex(24)
	expires := now.Add(challengeTTL).UTC()
	path := g.challengePath(challenge)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(expires.Format(time.RFC3339Nano)+"\n"), 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if g.challenges == nil {
		g.challenges = map[string]time.Time{}
	}
	g.challenges[challenge] = expires
	return challenge, nil
}

func (g *gateway) signProfile(ctx context.Context, profile []byte) ([]byte, error) {
	if !g.profileSigningConfigured() {
		return profile, nil
	}
	args := []string{
		"cms", "-sign", "-binary",
		"-signer", g.profileSigningCert,
		"-inkey", g.profileSigningKey,
		"-outform", "DER",
		"-nodetach",
		"-md", "sha256",
	}
	if regularReadableFile(g.profileSigningCA) {
		args = append(args, "-certfile", g.profileSigningCA)
	}
	cmd := exec.CommandContext(ctx, "openssl", args...)
	cmd.Stdin = bytes.NewReader(profile)
	signed, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("profile CMS signing failed: %w", err)
	}
	if len(signed) == 0 {
		return nil, fmt.Errorf("profile CMS signing returned empty output")
	}
	return signed, nil
}

func (g *gateway) profile(w http.ResponseWriter, r *http.Request) {
	challenge, err := g.newChallenge()
	if err != nil {
		fail(w, http.StatusTooManyRequests, "登记请求过多，请稍后再试")
		return
	}
	uuidRaw := randomHex(16)
	uuid := uuidRaw[:8] + "-" + uuidRaw[8:12] + "-" + uuidRaw[12:16] + "-" + uuidRaw[16:20] + "-" + uuidRaw[20:]
	callback := html.EscapeString(g.publicURL + "/device/callback/" + challenge)
	profile := []byte(fmt.Sprintf(
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\"><plist version=\"1.0\"><dict>"+
			"<key>PayloadContent</key><dict><key>URL</key><string>%s</string><key>DeviceAttributes</key><array><string>UDID</string><string>PRODUCT</string><string>VERSION</string></array><key>Challenge</key><string>%s</string></dict>"+
			"<key>PayloadOrganization</key><string>ILS</string><key>PayloadDisplayName</key><string>ILS 设备信息收集</string><key>PayloadDescription</key><string>仅收集 UDID、设备型号与系统版本供管理员登记 Ad Hoc 测试设备。不会安装根证书或 MDM。</string>"+
			"<key>PayloadVersion</key><integer>1</integer><key>PayloadUUID</key><string>%s</string><key>PayloadIdentifier</key><string>local.ioant.ils.enrollment.%s</string><key>PayloadType</key><string>Profile Service</string></dict></plist>",
		callback, challenge, uuid, uuid,
	))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	profile, err = g.signProfile(ctx, profile)
	if err != nil {
		log.Printf("profile signing failed: %v", err)
		fail(w, http.StatusInternalServerError, "设备登记描述文件签名失败，请稍后重试")
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="ils-device.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(profile)
}

func parsePlistStrings(raw []byte) (map[string]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	values := map[string]string{}
	var key string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return nil, err
			}
			key = value
		case "string":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return nil, err
			}
			if key != "" {
				values[key] = value
				key = ""
			}
		}
	}
	return values, nil
}

func (g *gateway) consumeChallenge(challenge string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.loadChallengeLocked(challenge); !ok {
		return false
	}
	if err := os.Remove(g.challengePath(challenge)); err != nil {
		return false
	}
	delete(g.challenges, challenge)
	return true
}

func (g *gateway) callback(w http.ResponseWriter, r *http.Request) {
	challenge := r.PathValue("challenge")
	if !g.challengeValid(challenge) {
		fail(w, http.StatusGone, "登记链接已过期，请重新下载描述文件")
		return
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		fail(w, http.StatusBadRequest, "设备响应过大")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openssl", "cms", "-verify", "-inform", "DER", "-noverify")
	cmd.Stdin = bytes.NewReader(raw)
	decoded, err := cmd.Output()
	if err != nil {
		fail(w, http.StatusBadRequest, "设备响应 CMS 签名无效")
		return
	}
	fields, err := parsePlistStrings(decoded)
	if err != nil || fields["CHALLENGE"] != challenge || !udidRE.MatchString(fields["UDID"]) {
		fail(w, http.StatusBadRequest, "设备 UDID 或登记凭证无效")
		return
	}
	if !g.consumeChallenge(challenge) {
		fail(w, http.StatusGone, "登记链接已使用或过期")
		return
	}

	hash := sha256.Sum256([]byte(fields["UDID"]))
	id := hex.EncodeToString(hash[:])
	device := pendingDevice{
		ID:          id,
		UDID:        fields["UDID"],
		Product:     fields["PRODUCT"],
		Version:     fields["VERSION"],
		CollectedAt: time.Now().UTC(),
	}
	dir := filepath.Join(g.data, "devices")
	if err = os.MkdirAll(dir, 0700); err != nil {
		fail(w, http.StatusInternalServerError, "设备信息保存失败")
		return
	}
	rawDevice, _ := json.Marshal(device)
	path := filepath.Join(dir, id+".json")
	if err = os.WriteFile(path+".tmp", rawDevice, 0600); err == nil {
		err = os.Rename(path+".tmp", path)
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "设备信息保存失败")
		return
	}
	http.Redirect(w, r, g.publicURL+"/enroll?collected=1", http.StatusMovedPermanently)
}

func (g *gateway) pending(w http.ResponseWriter, r *http.Request) {
	if !g.authorized(r) {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	paths, _ := filepath.Glob(filepath.Join(g.data, "devices", "*.json"))
	devices := []pendingDevice{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var device pendingDevice
		if json.Unmarshal(raw, &device) == nil && device.ID != "" && udidRE.MatchString(device.UDID) {
			devices = append(devices, device)
		}
	}
	writeJSON(w, http.StatusOK, devices)
}

func (g *gateway) ack(w http.ResponseWriter, r *http.Request) {
	if !g.authorized(r) {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	if len(id) != 64 {
		fail(w, http.StatusNotFound, "device not found")
		return
	}
	for _, ch := range id {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			fail(w, http.StatusNotFound, "device not found")
			return
		}
	}
	path := filepath.Join(g.data, "devices", id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fail(w, http.StatusInternalServerError, "failed to acknowledge device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8790", "listen address behind Nginx")
	publicURL := flag.String("public-url", "", "public HTTPS origin")
	data := flag.String("data", "/var/lib/ils-ota-gateway", "private state directory")
	tokenFile := flag.String("sync-token-file", "/etc/ils-ota-gateway/sync-token", "ILS sync bearer token file")
	profileSigningCert := flag.String("profile-signing-cert", "", "PEM certificate used to CMS-sign enrollment profiles")
	profileSigningKey := flag.String("profile-signing-key", "", "PEM private key used to CMS-sign enrollment profiles")
	profileSigningCA := flag.String("profile-signing-chain", "", "optional PEM certificate chain included with the CMS signature")
	flag.Parse()

	*publicURL = strings.TrimRight(strings.TrimSpace(*publicURL), "/")
	host := strings.TrimPrefix(*publicURL, "https://")
	if !strings.HasPrefix(*publicURL, "https://") || host == "" || strings.ContainsAny(host, "/?#@") {
		log.Fatal("public-url must be an HTTPS origin")
	}
	token, err := readToken(*tokenFile)
	if err != nil {
		log.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(*data, "devices"), 0700); err != nil {
		log.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(*data, "challenges"), 0700); err != nil {
		log.Fatal(err)
	}
	g := &gateway{
		publicURL:          *publicURL,
		data:               *data,
		token:              token,
		profileSigningCert: strings.TrimSpace(*profileSigningCert),
		profileSigningKey:  strings.TrimSpace(*profileSigningKey),
		profileSigningCA:   strings.TrimSpace(*profileSigningCA),
		challenges:         map[string]time.Time{},
	}
	if (g.profileSigningCert == "") != (g.profileSigningKey == "") {
		log.Fatal("profile-signing-cert and profile-signing-key must be provided together")
	}
	if g.profileSigningCert != "" && !g.profileSigningConfigured() {
		log.Printf("WARNING: enrollment profile signing files are not readable yet; profiles will remain unsigned until the service is restarted after certificate provisioning")
		g.profileSigningCert = ""
		g.profileSigningKey = ""
		g.profileSigningCA = ""
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_ils/health", g.health)
	mux.HandleFunc("GET /enroll", g.enrollPage)
	mux.HandleFunc("GET /enroll.mobileconfig", g.profile)
	mux.HandleFunc("POST /device/callback/{challenge}", g.callback)
	mux.HandleFunc("GET /api/ils/devices/pending", g.pending)
	mux.HandleFunc("POST /api/ils/devices/{id}/ack", g.ack)

	server := &http.Server{
		Addr:              *listen,
		Handler:           g.securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	log.Printf("ILS OTA Gateway listening on %s (signed enrollment profiles: %t)", *listen, g.profileSigningConfigured())
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
