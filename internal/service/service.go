package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxUpload = int64(4 << 30)

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
type Release struct {
	Variant       string                 `json:"variant"`
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"project_id"`
	Version       string                 `json:"version"`
	Build         int64                  `json:"build"`
	Platform      string                 `json:"platform"`
	Architecture  string                 `json:"architecture"`
	Channel       string                 `json:"channel"`
	Notes         string                 `json:"notes"`
	Filename      string                 `json:"filename,omitempty"`
	Size          int64                  `json:"size,omitempty"`
	SHA256        string                 `json:"sha256,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	BundleID      string                 `json:"bundle_id,omitempty"`
	IOS           *IOSInfo               `json:"ios,omitempty"`
	Delivery      string                 `json:"delivery,omitempty"`
	Status        string                 `json:"status,omitempty"`
	StatusMessage string                 `json:"status_message,omitempty"`
	OpenURL       string                 `json:"open_url,omitempty"`
	BuildJobID    string                 `json:"build_job_id,omitempty"`
	TestFlight    *TestFlightReleaseInfo `json:"testflight,omitempty"`
	OTA           *OTAReleaseInfo        `json:"ota,omitempty"`
	DownloadURL   string                 `json:"download_url,omitempty"`
	InstallURL    string                 `json:"install_url,omitempty"`
	AppIconURL    string                 `json:"app_icon_url,omitempty"`
}
type state struct {
	Projects []Project `json:"projects"`
	Releases []Release `json:"releases"`
}
type App struct {
	folderPickerMu sync.Mutex
	ascMu          sync.Mutex
	ascRefreshing  bool
	ascConnected   bool
	ascLastRefresh time.Time
	ascLastCheckAt *time.Time
	ascLastError   string
	buildMu        sync.Mutex
	activeBuild    string
	cancelBuild    context.CancelFunc
	buildReceipts  map[string][]string
	buildOrigin    string
	buildRoot      string
	mu             sync.RWMutex
	data           string
	publicURL      string
	token          string
	state          state
	static         fs.FS
	signingMu      sync.Mutex
	signingBusy    bool
	enrollmentMu   sync.Mutex
	enrollments    map[string]time.Time
	otaMu          sync.Mutex
	otaConnected   bool
	otaSyncing     bool
	otaLastCheckAt *time.Time
	otaLastError   string
	otaArtifactMu  sync.Mutex
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func New(data, publicURL string, static fs.FS) (*App, error) {
	var pathErr error
	data, pathErr = filepath.Abs(data)
	if pathErr != nil {
		return nil, pathErr
	}
	publicURL = strings.TrimRight(publicURL, "/")
	if publicURL != "" {
		u, e := url.Parse(publicURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return nil, fmt.Errorf("public-url 必须是 HTTPS origin，例如 https://builds.local:8787")
		}
	}
	if e := os.MkdirAll(filepath.Join(data, "artifacts"), 0700); e != nil {
		return nil, e
	}
	a := &App{data: data, publicURL: publicURL, static: static, state: state{Projects: []Project{}, Releases: []Release{}}}
	tokenPath := filepath.Join(data, "admin-token")
	raw, e := os.ReadFile(tokenPath)
	if errors.Is(e, os.ErrNotExist) {
		raw = []byte(randomID(32))
		e = os.WriteFile(tokenPath, raw, 0600)
	}
	if e != nil {
		return nil, e
	}
	a.token = strings.TrimSpace(string(raw))
	if len(a.token) < 32 {
		return nil, fmt.Errorf("admin-token 太短")
	}
	raw, e = os.ReadFile(filepath.Join(data, "state.json"))
	if e == nil {
		if e = json.Unmarshal(raw, &a.state); e != nil {
			return nil, e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	for i := range a.state.Releases {
		if a.state.Releases[i].Variant == "" {
			a.state.Releases[i].Variant = "default"
		}
		if a.state.Releases[i].Delivery == "" {
			a.state.Releases[i].Delivery = "artifact"
		}
		if a.state.Releases[i].Status == "" {
			a.state.Releases[i].Status = "published"
		}
	}
	// Remove only unfinished upload files from a previous interrupted run.
	pending, _ := filepath.Glob(filepath.Join(data, "upload-*"))
	for _, p := range pending {
		_ = os.Remove(p)
	}
	// Historical TestFlight repair is best-effort. A damaged legacy job must
	// not prevent ILS from starting; the manual ASC refresh surfaces the same
	// reconciliation error together with privacy-safe skip diagnostics.
	_, _ = a.reconcileTestFlightBuildJobs()
	go a.reconcileOTAGatewayArtifacts()
	return a, nil
}
func randomID(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (a *App) save(s state) error {
	raw, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(a.data, "state-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(raw); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(a.data, "state.json"))
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func (a *App) authorized(w http.ResponseWriter, r *http.Request) bool {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
		fail(w, 401, "请输入正确的发布密钥")
		return false
	}
	return true
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/session", func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(w, r) {
			return
		}
		respond(w, http.StatusOK, map[string]any{"authenticated": true, "role": "admin"})
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{
			"status":                    "ok",
			"ota_configured":            a.publicURL != "",
			"ota_gateway_configured":    a.otaGatewayConfigured(),
			"public_enrollment_url":     a.otaGatewayEnrollmentURL(),
			"max_upload_bytes":          maxUpload,
		})
	})
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		respond(w, 200, a.state.Projects)
	})
	mux.HandleFunc("POST /api/projects", a.createProject)
	mux.HandleFunc("GET /api/projects/{project}/releases", a.listReleases)
	mux.HandleFunc("POST /api/projects/{project}/releases", a.upload)
	mux.HandleFunc("GET /api/projects/{project}/icon", a.projectIcon)
	mux.HandleFunc("GET /api/builds/{job}/icon", a.buildIcon)
	mux.HandleFunc("GET /api/releases/{release}/icon", a.releaseIcon)
	mux.HandleFunc("GET /api/projects/{project}/updates", a.updates)
	mux.HandleFunc("GET /api/releases/{release}/download", a.download)
	mux.HandleFunc("GET /api/releases/{release}/manifest.plist", a.manifest)
	mux.HandleFunc("POST /api/projects/{project}/signing", a.startSigning)
	mux.HandleFunc("GET /api/signing/{job}", a.getSigning)
	mux.HandleFunc("GET /api/devices/enroll.mobileconfig", a.enrollmentProfile)
	mux.HandleFunc("POST /api/devices/callback/{challenge}", a.enrollmentCallback)
	mux.HandleFunc("GET /api/devices", a.listDevices)
	mux.HandleFunc("GET /api/ota-gateway/config", a.otaGatewayConfig)
	mux.HandleFunc("POST /api/ota-gateway/check", a.otaGatewayCheck)
	mux.HandleFunc("POST /api/ota-gateway/sync-devices", a.otaGatewaySyncDevices)
	mux.HandleFunc("POST /api/ota-gateway/sync-artifacts", a.otaGatewaySyncArtifacts)
	mux.HandleFunc("POST /api/releases/{release}/ota-sync", a.otaGatewaySyncRelease)
	mux.HandleFunc("GET /api/projects/{project}/build-source", a.buildSource)
	mux.HandleFunc("POST /api/local/select-folder", a.selectFolder)
	mux.HandleFunc("GET /api/local/apple-signing-teams", a.appleSigningTeams)
	mux.HandleFunc("POST /api/local/select-app-store-connect-key", a.selectAppStoreConnectKey)
	mux.HandleFunc("GET /api/app-store-connect/config", a.appStoreConnectConfig)
	mux.HandleFunc("POST /api/app-store-connect/config", a.appStoreConnectConfig)
	mux.HandleFunc("DELETE /api/app-store-connect/config", a.appStoreConnectConfig)
	mux.HandleFunc("POST /api/app-store-connect/check", a.appStoreConnectCheck)
	mux.HandleFunc("POST /api/app-store-connect/refresh", a.appStoreConnectRefresh)
	mux.HandleFunc("POST /api/projects/{project}/build-source", a.buildSource)
	mux.HandleFunc("GET /api/projects/{project}/release-profiles", a.releaseProfiles)
	mux.HandleFunc("POST /api/projects/{project}/release-profiles", a.releaseProfiles)
	mux.HandleFunc("DELETE /api/projects/{project}/release-profiles/{profile}", a.deleteReleaseProfile)
	mux.HandleFunc("POST /api/projects/{project}/builds", a.startBuild)
	mux.HandleFunc("GET /api/projects/{project}/builds", a.buildJobs)
	mux.HandleFunc("GET /api/builds/{job}/log", a.buildLog)
	mux.Handle("GET /", http.FileServer(http.FS(a.static)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == "POST" && r.Header.Get("Origin") != "" {
			origin, e := url.Parse(r.Header.Get("Origin"))
			if e != nil || origin.Host != r.Host {
				fail(w, 403, "不允许跨域发布")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *App) createProject(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	var p Project
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&p); e != nil {
		fail(w, 400, "项目格式无效")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if !slugRE.MatchString(p.ID) || p.Name == "" || len(p.Name) > 120 {
		fail(w, 400, "项目标识须为小写字母、数字或连字符，名称不能为空")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.state.Projects {
		if x.ID == p.ID {
			fail(w, 409, "项目标识已存在")
			return
		}
	}
	p.CreatedAt = time.Now().UTC()
	next := state{Projects: append(append([]Project{}, a.state.Projects...), p), Releases: a.state.Releases}
	if e := a.save(next); e != nil {
		fail(w, 500, "项目保存失败")
		return
	}
	a.state = next
	respond(w, 201, p)
}
func (a *App) projectExists(id string) bool {
	for _, p := range a.state.Projects {
		if p.ID == id {
			return true
		}
	}
	return false
}
func (a *App) decorated(v Release) Release {
	if v.Delivery == "" {
		v.Delivery = "artifact"
	}
	if v.Status == "" {
		v.Status = "published"
	}
	if v.Delivery == "artifact" {
		v.DownloadURL = "/api/releases/" + v.ID + "/download"
	}
	if _, e := os.Stat(a.releaseIconPath(v.ID)); os.IsNotExist(e) {
		artifact := ""
		if v.Delivery == "artifact" {
			artifact = filepath.Join(a.data, "artifacts", v.ID)
		}
		a.snapshotReleaseIcon(v, artifact)
	}
	if _, e := os.Stat(a.releaseIconPath(v.ID)); e == nil {
		v.AppIconURL = "/api/releases/" + v.ID + "/icon"
	}
	if v.Delivery == "artifact" && v.IOS != nil && v.IOS.OTAEligible() {
		if v.OTA != nil && v.OTA.Status == "synced" && v.OTA.ManifestURL != "" {
			v.InstallURL = otaInstallURL(v.OTA.ManifestURL)
		} else if a.publicURL != "" {
			v.InstallURL = "itms-services://?action=download-manifest&url=" + url.QueryEscape(a.publicURL+"/api/releases/"+v.ID+"/manifest.plist")
		}
	}
	return v
}
func (a *App) releases(id string) []Release {
	out := []Release{}
	for _, v := range a.state.Releases {
		if v.ProjectID == id {
			out = append(out, a.decorated(v))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		c := compareRelease(out[i], out[j])
		if c == 0 {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return c > 0
	})
	return out
}
func (a *App) listReleases(w http.ResponseWriter, r *http.Request) {
	a.scheduleTestFlightRefresh()
	a.mu.RLock()
	defer a.mu.RUnlock()
	id := r.PathValue("project")
	if !a.projectExists(id) {
		fail(w, 404, "项目不存在")
		return
	}
	respond(w, 200, a.releases(id))
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(id)
	a.mu.RUnlock()
	if !exists {
		fail(w, 404, "请先创建项目")
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(30 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	reader, e := r.MultipartReader()
	if e != nil {
		fail(w, 400, "请使用 multipart/form-data 上传")
		return
	}
	fields := map[string]string{}
	var temp string
	var filename string
	var size int64
	var digest string
	defer func() {
		if temp != "" {
			_ = os.Remove(temp)
		}
	}()
	for count := 0; ; count++ {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil || count > 16 {
			fail(w, 400, "上传数据无效或过大")
			return
		}
		name := part.FormName()
		if name == "file" {
			if temp != "" || part.FileName() == "" {
				fail(w, 400, "仅支持一个安装包")
				return
			}
			filename = part.FileName()
			if len(filename) > 200 || strings.ContainsAny(filename, "\r\n\\") {
				fail(w, 400, "安装包文件名无效")
				return
			}
			f, err := os.CreateTemp(a.data, "upload-*")
			if err != nil {
				fail(w, 500, "无法创建上传文件")
				return
			}
			temp = f.Name()
			hash := sha256.New()
			size, err = io.Copy(io.MultiWriter(f, hash), io.LimitReader(part, maxUpload+1))
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err != nil || closeErr != nil || size > maxUpload {
				fail(w, 413, "上传失败或安装包超过 4 GB")
				return
			}
			digest = hex.EncodeToString(hash.Sum(nil))
		} else {
			if _, exists := fields[name]; exists {
				fail(w, 400, "重复字段")
				return
			}
			b, err := io.ReadAll(io.LimitReader(part, 16385))
			if err != nil || len(b) > 16384 {
				fail(w, 400, "字段过长")
				return
			}
			fields[name] = string(b)
		}
	}
	build, e := parseBuild(fields["build"])
	v := Release{ID: randomID(16), ProjectID: id, Version: fields["version"], Build: build, Platform: fields["platform"], Architecture: fields["architecture"], Channel: fields["channel"], Notes: fields["notes"], Filename: filename, Size: size, SHA256: digest, CreatedAt: time.Now().UTC(), BundleID: fields["bundle_id"], Delivery: "artifact", Status: "published"}
	v.Variant = fields["variant"]
	if v.Variant == "" {
		v.Variant = "default"
	}
	if !slugRE.MatchString(v.Variant) {
		fail(w, 400, "variant 必须是小写字母、数字或连字符")
		return
	}
	if v.Channel == "" {
		v.Channel = "dev"
	}
	if v.Architecture == "" {
		if v.Platform == "ios" {
			v.Architecture = "arm64"
		} else {
			v.Architecture = "universal"
		}
	}
	if e != nil || !validVersion(v.Version) {
		fail(w, 400, "version 须为语义版本（如 1.2.0），build 须为正整数")
		return
	}
	if !oneOf(v.Platform, "ios", "macos") || !oneOf(v.Channel, "dev", "beta", "stable") || !oneOf(v.Architecture, "arm64", "x86_64", "universal") || (v.Platform == "ios" && v.Architecture != "arm64") {
		fail(w, 400, "平台、架构或渠道无效")
		return
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if size == 0 || temp == "" || (v.Platform == "ios" && ext != ".ipa") || (v.Platform == "macos" && !oneOf(ext, ".dmg", ".pkg", ".zip")) {
		fail(w, 400, "iOS 请上传 IPA；macOS 请上传 DMG、PKG 或 ZIP")
		return
	}
	if v.Platform == "ios" {
		info, err := inspectIPA(temp)
		if err != nil {
			fail(w, 400, "IPA 检查失败："+err.Error())
			return
		}
		v.IOS = info
		if v.BundleID != "" && v.BundleID != info.BundleID {
			fail(w, 400, "bundle_id 与 IPA 不一致")
			return
		}
		v.BundleID = info.BundleID
		if info.Version != v.Version || info.Build != strconv.FormatInt(v.Build, 10) {
			fail(w, 400, "version / build 与 IPA 内的版本不一致")
			return
		}
		v.OTA = a.pendingOTAGatewayInfo(v)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.state.Releases {
		if x.Variant == v.Variant && x.ProjectID == id && x.Version == v.Version && x.Build == v.Build && x.Platform == v.Platform && x.Channel == v.Channel && x.Architecture == v.Architecture && (x.Delivery == "" || x.Delivery == "artifact") {
			if x.SHA256 == v.SHA256 {
				if _, iconErr := os.Stat(a.releaseIconPath(x.ID)); os.IsNotExist(iconErr) {
					a.snapshotReleaseIcon(x, temp)
				}
				a.recordBuildPublication(fields["job_id"], id, x.ID)
				go a.scheduleOTAGatewaySync(x.ID)
				respond(w, 200, a.decorated(x))
				return
			}
			fail(w, 409, "这个版本与构建号已有不同安装包，请递增 build")
			return
		}
	}
	dest := filepath.Join(a.data, "artifacts", v.ID)
	if e := os.Rename(temp, dest); e != nil {
		fail(w, 500, "安装包保存失败")
		return
	}
	temp = ""
	a.snapshotReleaseIcon(v, dest)
	next := state{Projects: a.state.Projects, Releases: append(append([]Release{}, a.state.Releases...), v)}
	if e := a.save(next); e != nil {
		_ = os.Remove(dest)
		_ = os.Remove(a.releaseIconPath(v.ID))
		fail(w, 500, "版本保存失败")
		return
	}
	a.state = next
	a.recordBuildPublication(fields["job_id"], id, v.ID)
	go a.scheduleOTAGatewaySync(v.ID)
	respond(w, 201, a.decorated(v))
}
func oneOf(s string, choices ...string) bool {
	for _, c := range choices {
		if c == s {
			return true
		}
	}
	return false
}
func (a *App) updates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	variant := q.Get("variant")
	if variant == "" {
		variant = "default"
	}
	if !slugRE.MatchString(variant) {
		fail(w, 400, "variant 无效")
		return
	}
	version := q.Get("current_version")
	build, e := parseBuild(q.Get("current_build"))
	platform := q.Get("platform")
	channel := q.Get("channel")
	arch := q.Get("architecture")
	if channel == "" {
		channel = "dev"
	}
	if arch == "" {
		if platform == "ios" {
			arch = "arm64"
		} else {
			arch = "universal"
		}
	}
	if !validVersion(version) || e != nil || !oneOf(platform, "ios", "macos") || !oneOf(channel, "dev", "beta", "stable") || !oneOf(arch, "arm64", "x86_64", "universal") || (platform == "ios" && arch != "arm64") {
		fail(w, 400, "需要有效的 platform、current_version、current_build，以及可选的 channel / architecture")
		return
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	id := r.PathValue("project")
	if !a.projectExists(id) {
		fail(w, 404, "项目不存在")
		return
	}
	var latest *Release
	for _, v := range a.releases(id) {
		if v.Variant != variant || v.Platform != platform || v.Channel != channel || (v.Architecture != arch && v.Architecture != "universal") {
			continue
		}
		if v.Delivery == "testflight" && v.Status != "available" {
			continue
		}
		copy := v
		latest = &copy
		break
	}
	update := latest != nil && compareRelease(*latest, Release{Version: version, Build: build}) > 0
	respond(w, 200, map[string]any{"update_available": update, "latest": latest, "current_version": version, "current_build": build})
}
func (a *App) getRelease(id string) (Release, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, v := range a.state.Releases {
		if v.ID == id {
			return v, true
		}
	}
	return Release{}, false
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	v, ok := a.getRelease(r.PathValue("release"))
	if !ok {
		fail(w, 404, "版本不存在")
		return
	}
	if v.Delivery == "testflight" {
		fail(w, 409, "这个发布通过 TestFlight 分发，没有本地 IPA 下载")
		return
	}
	f, e := os.Open(filepath.Join(a.data, "artifacts", v.ID))
	if e != nil {
		fail(w, 404, "安装包文件不存在")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="artifact`+filepath.Ext(v.Filename)+`"; filename*=UTF-8''`+url.PathEscape(v.Filename))
	w.Header().Set("ETag", `"`+v.SHA256+`"`)
	http.ServeContent(w, r, v.Filename, v.CreatedAt, f)
}
func escapeXML(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func (a *App) manifest(w http.ResponseWriter, r *http.Request) {
	v, ok := a.getRelease(r.PathValue("release"))
	if !ok {
		fail(w, 404, "版本不存在")
		return
	}
	if v.Delivery == "testflight" {
		fail(w, 409, "TestFlight 发布不使用 Ad Hoc OTA manifest")
		return
	}
	if a.publicURL == "" || v.IOS == nil || !v.IOS.OTAEligible() {
		fail(w, 409, "需要 HTTPS 地址及有效的设备分发描述文件；开发签名请通过 Xcode / Configurator 安装")
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = io.WriteString(w, otaManifestXML(v, a.publicURL+"/api/releases/"+v.ID+"/download"))
}
