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
	"strings"
	"time"
)

var otaSSHHostRE = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9.-]*$")
var otaSSHUserRE = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_-]*$")
var otaRemoteRootRE = regexp.MustCompile("^/[A-Za-z0-9._/-]+$")
var otaGatewayDeviceIDRE = regexp.MustCompile("^[a-f0-9]{64}$")

type OTAGatewayConfig struct {
	PublicURL     string `json:"public_url"`
	SSHHost       string `json:"ssh_host"`
	SSHUser       string `json:"ssh_user"`
	SSHKeyPath    string `json:"ssh_key_path"`
	RemoteRoot    string `json:"remote_root"`
	SyncTokenFile string `json:"sync_token_file"`
}

type OTAGatewayStatus struct {
	Configured    bool       `json:"configured"`
	PublicURL     string     `json:"public_url,omitempty"`
	EnrollmentURL string     `json:"enrollment_url,omitempty"`
	SSHHost       string     `json:"ssh_host,omitempty"`
	SSHUser       string     `json:"ssh_user,omitempty"`
	RemoteRoot    string     `json:"remote_root,omitempty"`
	Connected     bool       `json:"connected"`
	Syncing       bool       `json:"syncing"`
	LastCheckAt   *time.Time `json:"last_check_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type OTAReleaseInfo struct {
	Status      string     `json:"status"`
	PublicURL   string     `json:"public_url,omitempty"`
	ManifestURL string     `json:"manifest_url,omitempty"`
	ArtifactURL string     `json:"artifact_url,omitempty"`
	SyncedAt    *time.Time `json:"synced_at,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
}

type gatewayPendingDevice struct {
	ID          string    `json:"id"`
	UDID        string    `json:"udid"`
	Product     string    `json:"product"`
	Version     string    `json:"version"`
	CollectedAt time.Time `json:"collected_at"`
}

type OTASyncReport struct {
	Pending   int `json:"pending"`
	Imported  int `json:"imported"`
	Acked     int `json:"acked"`
	Artifacts int `json:"artifacts,omitempty"`
	Failed    int `json:"failed,omitempty"`
}

func (a *App) otaGatewayConfigPath() string {
	return filepath.Join(a.data, "ota-gateway.json")
}

func normalizeOTAGatewayConfig(cfg OTAGatewayConfig) (OTAGatewayConfig, error) {
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	cfg.SSHHost = strings.TrimSpace(cfg.SSHHost)
	cfg.SSHUser = strings.TrimSpace(cfg.SSHUser)
	cfg.SSHKeyPath = strings.TrimSpace(cfg.SSHKeyPath)
	cfg.RemoteRoot = strings.TrimRight(strings.TrimSpace(cfg.RemoteRoot), "/")
	cfg.SyncTokenFile = strings.TrimSpace(cfg.SyncTokenFile)

	u, e := url.Parse(cfg.PublicURL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return cfg, fmt.Errorf("OTA Gateway public_url must be an HTTPS origin")
	}
	if !otaSSHHostRE.MatchString(cfg.SSHHost) || !otaSSHUserRE.MatchString(cfg.SSHUser) {
		return cfg, fmt.Errorf("OTA Gateway SSH host or user is invalid")
	}
	if cfg.RemoteRoot == "" || !otaRemoteRootRE.MatchString(cfg.RemoteRoot) {
		return cfg, fmt.Errorf("OTA Gateway remote_root must be a simple absolute path")
	}
	if !filepath.IsAbs(cfg.SSHKeyPath) || !filepath.IsAbs(cfg.SyncTokenFile) {
		return cfg, fmt.Errorf("OTA Gateway SSH key and sync token must use absolute paths")
	}
	key, e := filepath.EvalSymlinks(cfg.SSHKeyPath)
	if e != nil {
		return cfg, fmt.Errorf("OTA Gateway SSH key was not found")
	}
	info, e := os.Stat(key)
	if e != nil || !info.Mode().IsRegular() {
		return cfg, fmt.Errorf("OTA Gateway SSH key is invalid")
	}
	cfg.SSHKeyPath = key
	tokenFile, e := filepath.EvalSymlinks(cfg.SyncTokenFile)
	if e != nil {
		return cfg, fmt.Errorf("OTA Gateway sync token was not found")
	}
	cfg.SyncTokenFile = tokenFile
	if _, e = readOTAGatewayToken(tokenFile); e != nil {
		return cfg, e
	}
	return cfg, nil
}

func readOTAGatewayToken(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", fmt.Errorf("cannot read OTA Gateway sync token")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 4096 {
		return "", fmt.Errorf("OTA Gateway sync token file is invalid")
	}
	raw, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(raw) > 4096 {
		return "", fmt.Errorf("cannot read OTA Gateway sync token")
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || strings.ContainsRune(token, rune(10)) || strings.ContainsRune(token, rune(13)) || strings.ContainsRune(token, rune(0)) {
		return "", fmt.Errorf("OTA Gateway sync token is invalid")
	}
	return token, nil
}

func (a *App) readOTAGatewayConfig() (OTAGatewayConfig, error) {
	var cfg OTAGatewayConfig
	raw, e := os.ReadFile(a.otaGatewayConfigPath())
	if e != nil {
		return cfg, e
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&cfg); e != nil {
		return cfg, fmt.Errorf("OTA Gateway config is invalid")
	}
	return normalizeOTAGatewayConfig(cfg)
}

func (a *App) otaGatewayConfigured() bool {
	_, e := a.readOTAGatewayConfig()
	return e == nil
}

func (a *App) otaGatewayEnrollmentURL() string {
	cfg, e := a.readOTAGatewayConfig()
	if e != nil {
		return ""
	}
	return cfg.PublicURL + "/enroll"
}

func (a *App) pendingOTAGatewayInfo(v Release) *OTAReleaseInfo {
	cfg, e := a.readOTAGatewayConfig()
	if e != nil || v.Platform != "ios" || v.Delivery == "testflight" || v.IOS == nil || !v.IOS.OTAEligible() {
		return nil
	}
	base := cfg.PublicURL + "/releases/" + v.ProjectID + "/" + v.ID
	return &OTAReleaseInfo{
		Status:      "pending",
		PublicURL:   base,
		ArtifactURL: base + "/app.ipa",
		ManifestURL: base + "/manifest.plist",
	}
}

func (a *App) otaGatewayStatus() OTAGatewayStatus {
	status := OTAGatewayStatus{}
	if cfg, e := a.readOTAGatewayConfig(); e == nil {
		status.Configured = true
		status.PublicURL = cfg.PublicURL
		status.EnrollmentURL = cfg.PublicURL + "/enroll"
		status.SSHHost = cfg.SSHHost
		status.SSHUser = cfg.SSHUser
		status.RemoteRoot = cfg.RemoteRoot
	}
	a.otaMu.Lock()
	status.Connected = a.otaConnected
	status.Syncing = a.otaSyncing
	if a.otaLastCheckAt != nil {
		copy := *a.otaLastCheckAt
		status.LastCheckAt = &copy
	}
	status.LastError = a.otaLastError
	a.otaMu.Unlock()
	return status
}

func (a *App) recordOTAGatewayCheck(err error) {
	a.otaMu.Lock()
	defer a.otaMu.Unlock()
	now := time.Now().UTC()
	a.otaLastCheckAt = &now
	a.otaConnected = err == nil
	if err == nil {
		a.otaLastError = ""
	} else {
		a.otaLastError = err.Error()
	}
}

func (a *App) gatewayRequest(ctx context.Context, cfg OTAGatewayConfig, method, path string, body io.Reader, target any) error {
	token, e := readOTAGatewayToken(cfg.SyncTokenFile)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, method, cfg.PublicURL+path, body)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, e := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if e != nil {
		return fmt.Errorf("cannot connect to OTA Gateway: %v", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		var result map[string]string
		_ = json.Unmarshal(raw, &result)
		message := strings.TrimSpace(result["error"])
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("OTA Gateway returned %s", message)
	}
	if target == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); e != nil {
		return fmt.Errorf("OTA Gateway response is invalid")
	}
	return nil
}

func (a *App) checkOTAGateway(ctx context.Context) error {
	cfg, e := a.readOTAGatewayConfig()
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, cfg.PublicURL+"/_ils/health", nil)
	if e != nil {
		return e
	}
	resp, e := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if e != nil {
		return fmt.Errorf("OTA Gateway HTTPS is unreachable: %v", e)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OTA Gateway HTTPS health returned %s", resp.Status)
	}
	var pending []gatewayPendingDevice
	return a.gatewayRequest(ctx, cfg, http.MethodGet, "/api/ils/devices/pending", nil, &pending)
}

func (a *App) otaGatewayConfig(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	respond(w, 200, a.otaGatewayStatus())
}

func (a *App) otaGatewayCheck(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	e := a.checkOTAGateway(ctx)
	a.recordOTAGatewayCheck(e)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	respond(w, 200, a.otaGatewayStatus())
}

func (a *App) syncGatewayDevices(ctx context.Context) (OTASyncReport, error) {
	report := OTASyncReport{}
	cfg, e := a.readOTAGatewayConfig()
	if e != nil {
		return report, e
	}
	var pending []gatewayPendingDevice
	if e = a.gatewayRequest(ctx, cfg, http.MethodGet, "/api/ils/devices/pending", nil, &pending); e != nil {
		return report, e
	}
	report.Pending = len(pending)
	for _, item := range pending {
		if !otaGatewayDeviceIDRE.MatchString(item.ID) || !udidRE.MatchString(item.UDID) {
			report.Failed++
			continue
		}
		d := Device{
			UDID:             item.UDID,
			Product:          item.Product,
			Version:          item.Version,
			CollectedAt:      item.CollectedAt,
			Status:           "pending_apple_registration",
			IdentityVerified: false,
			Source:           "public_ota_gateway",
		}
		if d.CollectedAt.IsZero() {
			d.CollectedAt = time.Now().UTC()
		}
		a.enrollmentMu.Lock()
		created, saveErr := a.saveDeviceRecord(d)
		a.enrollmentMu.Unlock()
		if saveErr != nil {
			report.Failed++
			continue
		}
		if created {
			report.Imported++
		}
		if e = a.gatewayRequest(ctx, cfg, http.MethodPost, "/api/ils/devices/"+item.ID+"/ack", nil, nil); e != nil {
			report.Failed++
			continue
		}
		report.Acked++
	}
	return report, nil
}

func (a *App) otaGatewaySyncDevices(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	a.otaMu.Lock()
	if a.otaSyncing {
		a.otaMu.Unlock()
		fail(w, 409, "OTA Gateway is already syncing")
		return
	}
	a.otaSyncing = true
	a.otaMu.Unlock()
	defer func() {
		a.otaMu.Lock()
		a.otaSyncing = false
		a.otaMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	report, e := a.syncGatewayDevices(ctx)
	a.recordOTAGatewayCheck(e)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	respond(w, 200, report)
}

func otaManifestXML(v Release, artifactURL string) string {
	build := fmt.Sprintf("%d", v.Build)
	if v.IOS != nil && strings.TrimSpace(v.IOS.Build) != "" {
		build = v.IOS.Build
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>items</key><array><dict><key>assets</key><array><dict><key>kind</key><string>software-package</string><key>url</key><string>%s</string></dict></array><key>metadata</key><dict><key>bundle-identifier</key><string>%s</string><key>bundle-version</key><string>%s</string><key>kind</key><string>software</string><key>title</key><string>%s</string></dict></dict></array></dict></plist>`,
		escapeXML(artifactURL), escapeXML(v.BundleID), escapeXML(build), escapeXML(v.ProjectID+" "+v.Version))
}

func otaInstallURL(manifestURL string) string {
	if manifestURL == "" {
		return ""
	}
	return "itms-services://?action=download-manifest&url=" + url.QueryEscape(manifestURL)
}

func (a *App) updateReleaseOTA(id string, info *OTAReleaseInfo) (Release, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := state{Projects: a.state.Projects, Releases: append([]Release{}, a.state.Releases...)}
	for i := range next.Releases {
		if next.Releases[i].ID != id {
			continue
		}
		next.Releases[i].OTA = info
		if e := a.save(next); e != nil {
			return Release{}, e
		}
		a.state = next
		return next.Releases[i], nil
	}
	return Release{}, fmt.Errorf("Release not found")
}

type limitedStringWriter struct {
	dst   *strings.Builder
	limit int
}

func (w *limitedStringWriter) Write(p []byte) (int, error) {
	n := len(p)
	remaining := w.limit - w.dst.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.dst.Write(p)
	}
	return n, nil
}

func runOTACommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stdout = io.Discard
	cmd.Stderr = &limitedStringWriter{dst: &stderr, limit: 8 << 10}
	if e := cmd.Run(); e != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = e.Error()
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func otaSSHOptions(cfg OTAGatewayConfig) []string {
	return []string{
		"-i", cfg.SSHKeyPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=12",
		"-o", "ServerAliveInterval=20",
		"-o", "ServerAliveCountMax=3",
	}
}

func otaFinalizeRemoteCommand(stageDir, remoteDir string) string {
	return "chmod 0644 -- " + stageDir + "/app.ipa " + stageDir + "/manifest.plist && rm -rf -- " + remoteDir + " && mv -- " + stageDir + " " + remoteDir
}

func (a *App) syncReleaseToOTAGateway(ctx context.Context, id string) (Release, error) {
	a.otaArtifactMu.Lock()
	defer a.otaArtifactMu.Unlock()

	cfg, e := a.readOTAGatewayConfig()
	if e != nil {
		return Release{}, e
	}
	release, ok := a.getRelease(id)
	if !ok {
		return Release{}, fmt.Errorf("Release not found")
	}
	if release.Delivery == "testflight" || release.Platform != "ios" || release.IOS == nil || !release.IOS.OTAEligible() {
		return Release{}, fmt.Errorf("only eligible Ad Hoc or Enterprise iOS Releases can sync to the OTA Gateway")
	}
	artifact := filepath.Join(a.data, "artifacts", release.ID)
	info, e := os.Stat(artifact)
	if e != nil || !info.Mode().IsRegular() {
		return Release{}, fmt.Errorf("Release IPA file was not found")
	}

	publicDir := cfg.PublicURL + "/releases/" + release.ProjectID + "/" + release.ID
	artifactURL := publicDir + "/app.ipa"
	manifestURL := publicDir + "/manifest.plist"
	pending := &OTAReleaseInfo{
		Status:      "syncing",
		PublicURL:   publicDir,
		ArtifactURL: artifactURL,
		ManifestURL: manifestURL,
	}
	_, _ = a.updateReleaseOTA(release.ID, pending)

	failed := func(message string) (Release, error) {
		stateInfo := *pending
		stateInfo.Status = "failed"
		stateInfo.LastError = message
		updated, updateErr := a.updateReleaseOTA(release.ID, &stateInfo)
		if updateErr != nil {
			return Release{}, fmt.Errorf("%s; failed to persist OTA state: %v", message, updateErr)
		}
		return updated, fmt.Errorf("%s", message)
	}

	manifestFile, e := os.CreateTemp(a.data, "ota-manifest-*.plist")
	if e != nil {
		return failed("cannot create OTA manifest")
	}
	manifestPath := manifestFile.Name()
	defer os.Remove(manifestPath)
	if e = manifestFile.Chmod(0600); e == nil {
		_, e = io.WriteString(manifestFile, otaManifestXML(release, artifactURL))
	}
	if e == nil {
		e = manifestFile.Sync()
	}
	closeErr := manifestFile.Close()
	if e != nil || closeErr != nil {
		return failed("cannot write OTA manifest")
	}

	remoteDir := cfg.RemoteRoot + "/releases/" + release.ProjectID + "/" + release.ID
	stageDir := remoteDir + ".tmp-" + randomID(4)
	target := cfg.SSHUser + "@" + cfg.SSHHost
	sshOptions := otaSSHOptions(cfg)

	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		args := append(append([]string{}, sshOptions...), target, "rm -rf -- "+stageDir)
		_ = runOTACommand(cleanupCtx, "ssh", args...)
	}

	sshArgs := append(append([]string{}, sshOptions...), target, "rm -rf -- "+stageDir+" && mkdir -p -- "+stageDir)
	if e = runOTACommand(ctx, "ssh", sshArgs...); e != nil {
		return failed("cannot create OTA Gateway release directory")
	}
	scpOptions := append([]string{}, sshOptions...)
	if e = runOTACommand(ctx, "scp", append(scpOptions, artifact, target+":"+stageDir+"/app.ipa")...); e != nil {
		cleanup()
		return failed("failed to upload IPA to OTA Gateway")
	}
	if e = runOTACommand(ctx, "scp", append(scpOptions, manifestPath, target+":"+stageDir+"/manifest.plist")...); e != nil {
		cleanup()
		return failed("failed to upload OTA manifest")
	}
	finalizeArgs := append(append([]string{}, sshOptions...), target, otaFinalizeRemoteCommand(stageDir, remoteDir))
	if e = runOTACommand(ctx, "ssh", finalizeArgs...); e != nil {
		cleanup()
		return failed("failed to finalize OTA Gateway release directory")
	}

	client := &http.Client{Timeout: 20 * time.Second}
	for _, public := range []string{manifestURL, artifactURL} {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodHead, public, nil)
		if reqErr != nil {
			return failed("cannot verify OTA public URL")
		}
		resp, reqErr := client.Do(req)
		if reqErr != nil {
			return failed("OTA Gateway public file is unreachable")
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return failed("OTA Gateway public file verification failed")
		}
	}

	now := time.Now().UTC()
	synced := &OTAReleaseInfo{
		Status:      "synced",
		PublicURL:   publicDir,
		ArtifactURL: artifactURL,
		ManifestURL: manifestURL,
		SyncedAt:    &now,
	}
	updated, e := a.updateReleaseOTA(release.ID, synced)
	if e != nil {
		return Release{}, e
	}
	return a.decorated(updated), nil
}

func (a *App) scheduleOTAGatewaySync(id string) {
	if !a.otaGatewayConfigured() {
		return
	}
	release, ok := a.getRelease(id)
	if !ok || release.Platform != "ios" || release.Delivery == "testflight" || release.IOS == nil || !release.IOS.OTAEligible() {
		return
	}
	if release.OTA != nil && release.OTA.Status == "synced" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		_, _ = a.syncReleaseToOTAGateway(ctx, id)
	}()
}

func (a *App) reconcileOTAGatewayArtifacts() {
	if !a.otaGatewayConfigured() {
		return
	}
	a.mu.RLock()
	ids := []string{}
	for _, release := range a.state.Releases {
		if release.Platform != "ios" || release.Delivery == "testflight" || release.IOS == nil || !release.IOS.OTAEligible() {
			continue
		}
		if release.OTA == nil || oneOf(release.OTA.Status, "pending", "syncing", "failed") {
			ids = append(ids, release.ID)
		}
	}
	a.mu.RUnlock()
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		_, _ = a.syncReleaseToOTAGateway(ctx, id)
		cancel()
	}
}

func (a *App) otaGatewaySyncArtifacts(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	if !a.otaGatewayConfigured() {
		fail(w, 409, "OTA Gateway is not configured")
		return
	}
	a.mu.RLock()
	ids := []string{}
	for _, release := range a.state.Releases {
		if release.Platform == "ios" && release.Delivery != "testflight" && release.IOS != nil && release.IOS.OTAEligible() &&
			(release.OTA == nil || release.OTA.Status != "synced") {
			ids = append(ids, release.ID)
		}
	}
	a.mu.RUnlock()
	report := OTASyncReport{}
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
		_, e := a.syncReleaseToOTAGateway(ctx, id)
		cancel()
		if e != nil {
			report.Failed++
		} else {
			report.Artifacts++
		}
	}
	respond(w, 200, report)
}

func (a *App) otaGatewaySyncRelease(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	release, e := a.syncReleaseToOTAGateway(ctx, r.PathValue("release"))
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	respond(w, 200, release)
}
