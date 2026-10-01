package service

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const appStoreConnectOrigin = "https://api.appstoreconnect.apple.com"

var ascKeyIDRE = regexp.MustCompile(`^[A-Z0-9]{10}$`)

type AppStoreConnectConfig struct {
	KeyID          string `json:"key_id"`
	IssuerID       string `json:"issuer_id,omitempty"`
	PrivateKeyPath string `json:"private_key_path"`
}

type AppStoreConnectConfigStatus struct {
	Configured     bool       `json:"configured"`
	KeyID          string     `json:"key_id,omitempty"`
	IssuerID       string     `json:"issuer_id,omitempty"`
	KeyType        string     `json:"key_type,omitempty"`
	PrivateKeyPath string     `json:"private_key_path,omitempty"`
	PrivateKeyMode string     `json:"private_key_mode,omitempty"`
	Connected      bool       `json:"connected"`
	Refreshing     bool       `json:"refreshing"`
	LastCheckAt    *time.Time `json:"last_check_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

type TestFlightReleaseInfo struct {
	AppleBuildID       string     `json:"apple_build_id,omitempty"`
	ProcessingState    string     `json:"processing_state,omitempty"`
	InternalBuildState string     `json:"internal_build_state,omitempty"`
	ExternalBuildState string     `json:"external_build_state,omitempty"`
	PublicLink         string     `json:"public_link,omitempty"`
	LastCheckedAt      *time.Time `json:"last_checked_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
}

type ascResource struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Attributes json.RawMessage `json:"attributes"`
}

type ascListResponse struct {
	Data     []ascResource `json:"data"`
	Included []ascResource `json:"included,omitempty"`
}

type ascBuildAttributes struct {
	Version         string `json:"version"`
	ProcessingState string `json:"processingState"`
	Expired         bool   `json:"expired"`
}

type ascBetaDetailAttributes struct {
	InternalBuildState string `json:"internalBuildState"`
	ExternalBuildState string `json:"externalBuildState"`
}

type ascBetaGroupAttributes struct {
	Name              string `json:"name"`
	IsInternalGroup   bool   `json:"isInternalGroup"`
	PublicLinkEnabled bool   `json:"publicLinkEnabled"`
	PublicLink        string `json:"publicLink"`
}

type ascErrorResponse struct {
	Errors []struct {
		Status string `json:"status"`
		Code   string `json:"code"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

type ascResolver struct {
	cfg    AppStoreConnectConfig
	appIDs map[string]string
	preIDs map[string]string
}

type testFlightLookup struct {
	Status        string
	StatusMessage string
	OpenURL       string
	Info          TestFlightReleaseInfo
}

func (a *App) appStoreConnectConfigPath() string {
	return filepath.Join(a.data, "app-store-connect.json")
}

func normalizeAppStoreConnectConfig(cfg AppStoreConnectConfig) (AppStoreConnectConfig, error) {
	cfg.KeyID = strings.ToUpper(strings.TrimSpace(cfg.KeyID))
	cfg.IssuerID = strings.TrimSpace(cfg.IssuerID)
	cfg.PrivateKeyPath = strings.TrimSpace(cfg.PrivateKeyPath)
	if !ascKeyIDRE.MatchString(cfg.KeyID) {
		return cfg, fmt.Errorf("Key ID 必须是 10 位大写字母或数字")
	}
	if len(cfg.IssuerID) > 128 {
		return cfg, fmt.Errorf("Issuer ID 过长")
	}
	if !filepath.IsAbs(cfg.PrivateKeyPath) {
		return cfg, fmt.Errorf("请选择这台 Mac 上的 App Store Connect .p8 私钥文件")
	}
	real, e := filepath.EvalSymlinks(cfg.PrivateKeyPath)
	if e != nil {
		return cfg, fmt.Errorf("找不到 App Store Connect 私钥文件")
	}
	cfg.PrivateKeyPath = real
	if strings.ToLower(filepath.Ext(real)) != ".p8" {
		return cfg, fmt.Errorf("App Store Connect 私钥必须是 .p8 文件")
	}
	if _, e = readASCPrivateKey(real); e != nil {
		return cfg, e
	}
	return cfg, nil
}

func readASCPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, fmt.Errorf("无法读取 App Store Connect 私钥")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<10 {
		return nil, fmt.Errorf("App Store Connect 私钥文件无效")
	}
	raw, e := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if e != nil || len(raw) > 64<<10 {
		return nil, fmt.Errorf("无法读取 App Store Connect 私钥")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("App Store Connect 私钥不是有效 PEM")
	}
	if anyKey, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes); parseErr == nil {
		if key, ok := anyKey.(*ecdsa.PrivateKey); ok && key.Curve.Params().Name == "P-256" {
			return key, nil
		}
	}
	if key, parseErr := x509.ParseECPrivateKey(block.Bytes); parseErr == nil && key.Curve.Params().Name == "P-256" {
		return key, nil
	}
	return nil, fmt.Errorf("App Store Connect 私钥必须是 ES256 / P-256 私钥")
}

func ascBase64JSON(value any) (string, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ascJWT(cfg AppStoreConnectConfig, now time.Time) (string, error) {
	key, e := readASCPrivateKey(cfg.PrivateKeyPath)
	if e != nil {
		return "", e
	}
	header, e := ascBase64JSON(map[string]any{
		"alg": "ES256",
		"kid": cfg.KeyID,
		"typ": "JWT",
	})
	if e != nil {
		return "", e
	}
	payload := map[string]any{
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
	}
	if cfg.IssuerID != "" {
		payload["iss"] = cfg.IssuerID
	} else {
		payload["sub"] = "user"
	}
	body, e := ascBase64JSON(payload)
	if e != nil {
		return "", e
	}
	unsigned := header + "." + body
	digest := sha256.Sum256([]byte(unsigned))
	r, s, e := ecdsa.Sign(cryptorand.Reader, key, digest[:])
	if e != nil {
		return "", fmt.Errorf("无法签署 App Store Connect JWT")
	}
	signature := make([]byte, 64)
	rb, sb := r.Bytes(), s.Bytes()
	copy(signature[32-len(rb):32], rb)
	copy(signature[64-len(sb):], sb)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func ascAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var document ascErrorResponse
	if json.Unmarshal(raw, &document) == nil && len(document.Errors) > 0 {
		item := document.Errors[0]
		message := strings.TrimSpace(item.Detail)
		if message == "" {
			message = strings.TrimSpace(item.Title)
		}
		if message == "" {
			message = strings.TrimSpace(item.Code)
		}
		if message != "" {
			return fmt.Errorf("App Store Connect 返回 %s：%s", resp.Status, message)
		}
	}
	return fmt.Errorf("App Store Connect 返回 %s", resp.Status)
}

func ascGET(ctx context.Context, cfg AppStoreConnectConfig, path string, query url.Values, target any) error {
	token, e := ascJWT(cfg, time.Now().UTC())
	if e != nil {
		return e
	}
	u := appStoreConnectOrigin + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, e := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if e != nil {
		return fmt.Errorf("无法连接 App Store Connect：%v", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ascAPIError(resp)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if e = dec.Decode(target); e != nil {
		return fmt.Errorf("App Store Connect 响应格式无效")
	}
	return nil
}

func (a *App) readAppStoreConnectConfig() (AppStoreConnectConfig, error) {
	var cfg AppStoreConnectConfig
	raw, e := os.ReadFile(a.appStoreConnectConfigPath())
	if e != nil {
		return cfg, e
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return cfg, fmt.Errorf("App Store Connect 配置损坏")
	}
	return normalizeAppStoreConnectConfig(cfg)
}

func (a *App) recordASCCheck(err error) {
	a.ascMu.Lock()
	defer a.ascMu.Unlock()
	now := time.Now().UTC()
	a.ascLastCheckAt = &now
	a.ascConnected = err == nil
	if err == nil {
		a.ascLastError = ""
	} else {
		a.ascLastError = err.Error()
	}
}

func (a *App) appStoreConnectStatus() AppStoreConnectConfigStatus {
	status := AppStoreConnectConfigStatus{}
	cfg, e := a.readAppStoreConnectConfig()
	if e == nil {
		status.Configured = true
		status.KeyID = cfg.KeyID
		status.IssuerID = cfg.IssuerID
		status.PrivateKeyPath = cfg.PrivateKeyPath
		if cfg.IssuerID == "" {
			status.KeyType = "individual"
		} else {
			status.KeyType = "team"
		}
		if info, statErr := os.Stat(cfg.PrivateKeyPath); statErr == nil {
			status.PrivateKeyMode = fmt.Sprintf("%#o", info.Mode().Perm())
		}
	}
	a.ascMu.Lock()
	status.Connected = a.ascConnected
	status.Refreshing = a.ascRefreshing
	if a.ascLastCheckAt != nil {
		copy := *a.ascLastCheckAt
		status.LastCheckAt = &copy
	}
	status.LastError = a.ascLastError
	a.ascMu.Unlock()
	return status
}

func checkAppStoreConnect(ctx context.Context, cfg AppStoreConnectConfig) error {
	query := url.Values{}
	query.Set("limit", "1")
	query.Set("fields[apps]", "name,bundleId")
	var response ascListResponse
	return ascGET(ctx, cfg, "/v1/apps", query, &response)
}

func (a *App) appStoreConnectConfig(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		respond(w, 200, a.appStoreConnectStatus())
		return
	case http.MethodDelete:
		if e := os.Remove(a.appStoreConnectConfigPath()); e != nil && !os.IsNotExist(e) {
			fail(w, 500, "删除 App Store Connect 配置失败")
			return
		}
		a.ascMu.Lock()
		a.ascConnected = false
		a.ascLastError = ""
		a.ascLastCheckAt = nil
		a.ascLastRefresh = time.Time{}
		a.ascMu.Unlock()
		respond(w, 200, a.appStoreConnectStatus())
		return
	}
	var cfg AppStoreConnectConfig
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&cfg); e != nil {
		fail(w, 400, "App Store Connect 配置格式无效")
		return
	}
	cfg, e := normalizeAppStoreConnectConfig(cfg)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	if e = atomicJSON(a.appStoreConnectConfigPath(), cfg); e != nil {
		fail(w, 500, "保存 App Store Connect 配置失败")
		return
	}
	_ = os.Chmod(a.appStoreConnectConfigPath(), 0600)
	a.ascMu.Lock()
	a.ascLastRefresh = time.Time{}
	a.ascMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	e = checkAppStoreConnect(ctx, cfg)
	a.recordASCCheck(e)
	respond(w, 200, a.appStoreConnectStatus())
}

func (a *App) appStoreConnectCheck(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	cfg, e := a.readAppStoreConnectConfig()
	if e != nil {
		fail(w, 409, "请先配置 App Store Connect API Key")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	e = checkAppStoreConnect(ctx, cfg)
	a.recordASCCheck(e)
	respond(w, 200, a.appStoreConnectStatus())
}

func (a *App) selectAppStoreConnectKey(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	if runtime.GOOS != "darwin" {
		fail(w, 409, "原生文件选择仅支持 macOS，请手动填写 .p8 路径")
		return
	}
	if !a.folderPickerMu.TryLock() {
		fail(w, 409, "Mac 上已有文件选择窗口，请先完成或取消选择")
		return
	}
	defer a.folderPickerMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	script := `tell application "Finder"
 activate
 try
  set chosenFile to choose file with prompt "选择 App Store Connect API 私钥（.p8）"
  return POSIX path of chosenFile
 on error number -128
  return ""
 end try
end tell`
	out, e := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).Output()
	if e != nil {
		if ctx.Err() != nil {
			fail(w, 408, "文件选择已超时，请重试")
			return
		}
		fail(w, 409, "无法打开 Mac 文件选择窗口，请确认服务可控制 Finder，也可以手动填写 .p8 绝对路径")
		return
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		respond(w, 200, map[string]any{"cancelled": true})
		return
	}
	real, e := filepath.EvalSymlinks(path)
	if e != nil || strings.ToLower(filepath.Ext(real)) != ".p8" {
		fail(w, 400, "请选择有效的 .p8 私钥")
		return
	}
	if _, e = readASCPrivateKey(real); e != nil {
		fail(w, 400, e.Error())
		return
	}
	respond(w, 200, map[string]any{"cancelled": false, "path": real})
}

func (r *ascResolver) appID(ctx context.Context, bundleID string) (string, error) {
	if id := r.appIDs[bundleID]; id != "" {
		return id, nil
	}
	query := url.Values{}
	query.Set("filter[bundleId]", bundleID)
	query.Set("fields[apps]", "bundleId,name")
	query.Set("limit", "2")
	var response ascListResponse
	if e := ascGET(ctx, r.cfg, "/v1/apps", query, &response); e != nil {
		return "", e
	}
	if len(response.Data) == 0 {
		return "", nil
	}
	if len(response.Data) > 1 {
		return "", fmt.Errorf("App Store Connect 中 bundle_id %s 匹配多个 App", bundleID)
	}
	r.appIDs[bundleID] = response.Data[0].ID
	return response.Data[0].ID, nil
}

func (r *ascResolver) preReleaseVersionID(ctx context.Context, appID, version string) (string, error) {
	key := appID + "|" + version
	if id := r.preIDs[key]; id != "" {
		return id, nil
	}
	query := url.Values{}
	query.Set("filter[app]", appID)
	query.Set("filter[version]", version)
	query.Set("filter[platform]", "IOS")
	query.Set("fields[preReleaseVersions]", "version,platform")
	query.Set("limit", "2")
	var response ascListResponse
	if e := ascGET(ctx, r.cfg, "/v1/preReleaseVersions", query, &response); e != nil {
		return "", e
	}
	if len(response.Data) == 0 {
		return "", nil
	}
	if len(response.Data) > 1 {
		return "", fmt.Errorf("App Store Connect 中版本 %s 匹配多个 iOS prerelease version", version)
	}
	r.preIDs[key] = response.Data[0].ID
	return response.Data[0].ID, nil
}

func betaAvailable(state string) bool {
	return state == "READY_FOR_BETA_TESTING" || state == "IN_BETA_TESTING"
}

func testFlightState(processing, internal, external string, expired bool) (string, string) {
	if expired || internal == "EXPIRED" || external == "EXPIRED" {
		return "unavailable", "TestFlight 构建已过期"
	}
	switch processing {
	case "FAILED", "INVALID":
		return "unavailable", "Apple Processing 失败：" + processing
	case "PROCESSING", "":
		return "processing", "Apple 正在 Processing"
	}
	if betaAvailable(internal) || betaAvailable(external) {
		if betaAvailable(internal) && external != "" && !betaAvailable(external) {
			return "available", "内部 TestFlight 已可测试；外部测试状态：" + external
		}
		return "available", "TestFlight 已可测试"
	}
	switch {
	case internal == "MISSING_EXPORT_COMPLIANCE" || external == "MISSING_EXPORT_COMPLIANCE":
		return "processing", "等待补充出口合规信息"
	case internal == "IN_EXPORT_COMPLIANCE_REVIEW" || external == "IN_EXPORT_COMPLIANCE_REVIEW":
		return "processing", "Apple 正在审核出口合规信息"
	case internal == "PROCESSING_EXCEPTION" || external == "PROCESSING_EXCEPTION":
		return "unavailable", "TestFlight Processing Exception"
	case external == "BETA_REJECTED":
		return "unavailable", "外部 TestFlight Beta Review 被拒绝"
	case external == "WAITING_FOR_BETA_REVIEW":
		return "processing", "等待 TestFlight Beta Review"
	case external == "IN_BETA_REVIEW":
		return "processing", "TestFlight Beta Review 进行中"
	case external == "READY_FOR_BETA_SUBMISSION":
		return "processing", "Apple 已处理完成；等待提交外部 TestFlight Beta Review"
	case external == "BETA_APPROVED":
		return "processing", "外部 TestFlight Beta Review 已批准；等待可测试状态"
	default:
		return "processing", "Apple 已完成二进制处理；等待 TestFlight 可测试状态"
	}
}

func (r *ascResolver) testFlight(ctx context.Context, release Release) (testFlightLookup, error) {
	now := time.Now().UTC()
	info := TestFlightReleaseInfo{LastCheckedAt: &now}
	result := testFlightLookup{
		Status:        "submitted",
		StatusMessage: "已提交到 App Store Connect；等待 Apple 构建记录",
		OpenURL:       release.OpenURL,
		Info:          info,
	}
	appID, e := r.appID(ctx, release.BundleID)
	if e != nil {
		return result, e
	}
	if appID == "" {
		result.StatusMessage = "App Store Connect 未找到此 Bundle ID；请检查 App Record 与 API Key 权限"
		return result, nil
	}
	preID, e := r.preReleaseVersionID(ctx, appID, release.Version)
	if e != nil {
		return result, e
	}
	if preID == "" {
		result.StatusMessage = "已找到 App，但 Apple 尚未暴露这个 iOS 版本；可能仍在同步"
		return result, nil
	}
	query := url.Values{}
	query.Set("filter[preReleaseVersion]", preID)
	query.Set("filter[version]", fmt.Sprintf("%d", release.Build))
	query.Set("fields[builds]", "version,processingState,expired")
	query.Set("fields[buildBetaDetails]", "internalBuildState,externalBuildState")
	query.Set("fields[betaGroups]", "name,isInternalGroup,publicLinkEnabled,publicLink")
	query.Set("include", "buildBetaDetail,betaGroups")
	query.Set("limit", "2")
	query.Set("limit[betaGroups]", "50")
	var response ascListResponse
	if e = ascGET(ctx, r.cfg, "/v1/builds", query, &response); e != nil {
		return result, e
	}
	if len(response.Data) == 0 {
		result.StatusMessage = fmt.Sprintf("已找到版本 %s，但 Apple 尚未暴露 build %d；可能仍在同步", release.Version, release.Build)
		return result, nil
	}
	if len(response.Data) > 1 {
		return result, fmt.Errorf("App Store Connect 中版本 %s build %d 匹配多个构建", release.Version, release.Build)
	}
	build := response.Data[0]
	var buildAttributes ascBuildAttributes
	if json.Unmarshal(build.Attributes, &buildAttributes) != nil {
		return result, fmt.Errorf("App Store Connect build attributes 无效")
	}
	info.AppleBuildID = build.ID
	info.ProcessingState = buildAttributes.ProcessingState

	var detail ascBetaDetailAttributes
	groups := []ascBetaGroupAttributes{}
	for _, included := range response.Included {
		switch included.Type {
		case "buildBetaDetails":
			_ = json.Unmarshal(included.Attributes, &detail)
		case "betaGroups":
			var group ascBetaGroupAttributes
			if json.Unmarshal(included.Attributes, &group) == nil {
				groups = append(groups, group)
			}
		}
	}
	info.InternalBuildState = detail.InternalBuildState
	info.ExternalBuildState = detail.ExternalBuildState
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].IsInternalGroup != groups[j].IsInternalGroup {
			return !groups[i].IsInternalGroup
		}
		return groups[i].Name < groups[j].Name
	})
	for _, group := range groups {
		if group.PublicLinkEnabled && strings.HasPrefix(group.PublicLink, "https://testflight.apple.com/") {
			info.PublicLink = group.PublicLink
			result.OpenURL = group.PublicLink
			break
		}
	}
	result.Status, result.StatusMessage = testFlightState(
		buildAttributes.ProcessingState,
		detail.InternalBuildState,
		detail.ExternalBuildState,
		buildAttributes.Expired,
	)
	result.Info = info
	return result, nil
}

func (a *App) syncBuildJobFromTestFlightRelease(release Release) {
	if release.BuildJobID == "" || !jobRE.MatchString(release.BuildJobID) {
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	raw, e := os.ReadFile(a.buildJobPath(release.BuildJobID))
	if e != nil {
		return
	}
	var job BuildJob
	if json.Unmarshal(raw, &job) != nil || job.Lane != "ios-testflight" || job.Status == "running" {
		return
	}
	switch release.Status {
	case "submitted":
		job.Stage = "submitted"
		job.StageState = "succeeded"
	case "processing":
		job.Stage = "processing"
		job.StageState = "running"
	case "available":
		job.Stage = "available"
		job.StageState = "succeeded"
	case "unavailable":
		job.Stage = "available"
		job.StageState = "failed"
	}
	job.Message = release.StatusMessage
	job.Progress = nil
	_ = atomicJSON(a.buildJobPath(job.ID), job)
}

func (a *App) refreshTestFlightReleases(ctx context.Context, cfg AppStoreConnectConfig) (int, error) {
	a.mu.RLock()
	candidates := make([]Release, 0)
	for _, release := range a.state.Releases {
		if release.Delivery != "testflight" || release.BundleID == "" {
			continue
		}
		if (release.Status == "available" || release.Status == "unavailable") &&
			release.TestFlight != nil && release.TestFlight.LastCheckedAt != nil &&
			time.Since(*release.TestFlight.LastCheckedAt) < 15*time.Minute {
			continue
		}
		candidates = append(candidates, release)
	}
	a.mu.RUnlock()
	sort.SliceStable(candidates, func(i, j int) bool {
		iActive := candidates[i].Status == "submitted" || candidates[i].Status == "processing"
		jActive := candidates[j].Status == "submitted" || candidates[j].Status == "processing"
		if iActive != jActive {
			return iActive
		}
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	resolver := &ascResolver{
		cfg:    cfg,
		appIDs: map[string]string{},
		preIDs: map[string]string{},
	}
	lookups := map[string]testFlightLookup{}
	var firstLookupErr error
	for _, release := range candidates {
		if ctx.Err() != nil {
			return len(lookups), ctx.Err()
		}
		lookup, e := resolver.testFlight(ctx, release)
		if e != nil {
			if firstLookupErr == nil {
				firstLookupErr = e
			}
			now := time.Now().UTC()
			lookup = testFlightLookup{
				Status:        release.Status,
				StatusMessage: release.StatusMessage,
				OpenURL:       release.OpenURL,
				Info:          TestFlightReleaseInfo{LastCheckedAt: &now, LastError: e.Error()},
			}
			if release.TestFlight != nil {
				lookup.Info.AppleBuildID = release.TestFlight.AppleBuildID
				lookup.Info.ProcessingState = release.TestFlight.ProcessingState
				lookup.Info.InternalBuildState = release.TestFlight.InternalBuildState
				lookup.Info.ExternalBuildState = release.TestFlight.ExternalBuildState
				lookup.Info.PublicLink = release.TestFlight.PublicLink
			}
		}
		lookups[release.ID] = lookup
	}

	a.mu.Lock()
	next := state{
		Projects: append([]Project{}, a.state.Projects...),
		Releases: append([]Release{}, a.state.Releases...),
	}
	updated := []Release{}
	for i := range next.Releases {
		lookup, ok := lookups[next.Releases[i].ID]
		if !ok {
			continue
		}
		next.Releases[i].Status = lookup.Status
		next.Releases[i].StatusMessage = lookup.StatusMessage
		if lookup.OpenURL != "" {
			next.Releases[i].OpenURL = lookup.OpenURL
		}
		next.Releases[i].TestFlight = &lookup.Info
		updated = append(updated, next.Releases[i])
	}
	if e := a.save(next); e != nil {
		a.mu.Unlock()
		return 0, fmt.Errorf("保存 TestFlight 状态失败")
	}
	a.state = next
	a.mu.Unlock()

	for _, release := range updated {
		a.syncBuildJobFromTestFlightRelease(release)
	}
	return len(updated), firstLookupErr
}

func (a *App) recordASCRefresh(err error) {
	a.ascMu.Lock()
	defer a.ascMu.Unlock()
	now := time.Now().UTC()
	a.ascLastCheckAt = &now
	a.ascConnected = err == nil
	a.ascRefreshing = false
	if err == nil {
		a.ascLastError = ""
	} else {
		a.ascLastError = err.Error()
	}
}

func (a *App) scheduleTestFlightRefresh() {
	cfg, e := a.readAppStoreConnectConfig()
	if e != nil {
		return
	}
	a.ascMu.Lock()
	if a.ascRefreshing || (!a.ascLastRefresh.IsZero() && time.Since(a.ascLastRefresh) < 60*time.Second) {
		a.ascMu.Unlock()
		return
	}
	a.ascRefreshing = true
	a.ascLastRefresh = time.Now()
	a.ascMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		_, refreshErr := a.refreshTestFlightReleases(ctx, cfg)
		a.recordASCRefresh(refreshErr)
	}()
}

func (a *App) appStoreConnectRefresh(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	cfg, e := a.readAppStoreConnectConfig()
	if e != nil {
		fail(w, 409, "请先配置 App Store Connect API Key")
		return
	}
	a.ascMu.Lock()
	if a.ascRefreshing {
		a.ascMu.Unlock()
		fail(w, 409, "TestFlight 状态正在刷新")
		return
	}
	a.ascRefreshing = true
	a.ascLastRefresh = time.Now()
	a.ascMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	count, refreshErr := a.refreshTestFlightReleases(ctx, cfg)
	a.recordASCRefresh(refreshErr)
	response := map[string]any{"updated": count, "status": a.appStoreConnectStatus()}
	if refreshErr != nil {
		response["warning"] = refreshErr.Error()
	}
	respond(w, 200, response)
}

func (a *App) appStoreConnectBuildEnv() []string {
	cfg, e := a.readAppStoreConnectConfig()
	if e != nil || cfg.IssuerID == "" {
		// App Store Connect status queries support Individual API keys, but
		// xcodebuild distribution authentication requires the Team Issuer ID.
		// Leave the child environment untouched so Xcode can use its signed-in account.
		return nil
	}
	return []string{
		"ILS_ASC_KEY_ID=" + cfg.KeyID,
		"ILS_ASC_KEY_PATH=" + cfg.PrivateKeyPath,
		"ILS_ASC_ISSUER_ID=" + cfg.IssuerID,
	}
}

func (a *App) publishTestFlightRelease(job BuildJob, profile ReleaseProfile, result BuildResult) (Release, error) {
	build, e := parseBuild(result.Build)
	if e != nil {
		return Release{}, fmt.Errorf("TestFlight build number 无效")
	}
	release := Release{
		Variant:       profile.Variant,
		ID:            randomID(16),
		ProjectID:     job.ProjectID,
		Version:       result.Version,
		Build:         build,
		Platform:      "ios",
		Architecture:  profile.Architecture,
		Channel:       profile.Channel,
		Notes:         profile.Notes,
		CreatedAt:     time.Now().UTC(),
		BundleID:      result.BundleID,
		Delivery:      "testflight",
		Status:        "submitted",
		StatusMessage: "已提交到 App Store Connect；等待 Apple Processing",
		OpenURL:       profile.TestFlightURL,
		BuildJobID:    job.ID,
		TestFlight:    &TestFlightReleaseInfo{},
	}
	a.snapshotReleaseIcon(release, "")

	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Releases {
		existing := a.state.Releases[i]
		if existing.ProjectID == release.ProjectID &&
			existing.Variant == release.Variant &&
			existing.Version == release.Version &&
			existing.Build == release.Build &&
			existing.Platform == release.Platform &&
			existing.Architecture == release.Architecture &&
			existing.Channel == release.Channel &&
			existing.Delivery == "testflight" {
			existing.BundleID = release.BundleID
			existing.Status = release.Status
			existing.StatusMessage = release.StatusMessage
			existing.BuildJobID = release.BuildJobID
			if release.OpenURL != "" {
				existing.OpenURL = release.OpenURL
			}
			next := state{Projects: a.state.Projects, Releases: append([]Release{}, a.state.Releases...)}
			next.Releases[i] = existing
			if e := a.save(next); e != nil {
				return Release{}, e
			}
			a.state = next
			return a.decorated(existing), nil
		}
	}
	next := state{Projects: a.state.Projects, Releases: append(append([]Release{}, a.state.Releases...), release)}
	if e := a.save(next); e != nil {
		_ = os.Remove(a.releaseIconPath(release.ID))
		return Release{}, e
	}
	a.state = next
	return a.decorated(release), nil
}
