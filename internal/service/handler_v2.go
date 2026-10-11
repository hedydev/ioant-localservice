package service

import (
	"net/http"
	"net/url"
)

// HandlerV2 is the current ILS HTTP router. Apple Developer device management
// lives under /api/apple-devices so it cannot conflict with the Profile Service
// callback namespace under /api/devices/callback/.
func (a *App) HandlerV2() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/session", func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(w, r) {
			return
		}
		respond(w, http.StatusOK, map[string]any{"authenticated": true, "role": "admin"})
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{
			"status":                 "ok",
			"ota_configured":         a.publicURL != "",
			"ota_gateway_configured": a.otaGatewayConfigured(),
			"public_enrollment_url":  a.otaGatewayEnrollmentURL(),
			"max_upload_bytes":       maxUpload,
		})
	})
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		respond(w, 200, a.state.Projects)
	})
	mux.HandleFunc("POST /api/projects", a.createProject)
	mux.HandleFunc("GET /api/projects/{project}/releases", a.listReleases)
	mux.HandleFunc("GET /api/projects/{project}/internal-test-records", a.listPackageOnlyRecords)
	mux.HandleFunc("POST /api/projects/{project}/releases", a.upload)
	mux.HandleFunc("DELETE /api/releases/{release}", a.deleteReleaseArtifactRecord)
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
	mux.HandleFunc("POST /api/apple-devices/{udid}/register", a.registerDeviceWithApple)
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
	mux.HandleFunc("GET /api/projects/{project}/release-profiles", a.releaseProfilesLocalV2)
	mux.HandleFunc("POST /api/projects/{project}/release-profiles", a.releaseProfilesLocalV2)
	mux.HandleFunc("DELETE /api/projects/{project}/release-profiles/{profile}", a.deleteReleaseProfileLocal)
	mux.HandleFunc("POST /api/projects/{project}/builds", a.startLocalCheckoutBuild)
	mux.HandleFunc("POST /api/projects/{project}/internal-builds", a.startPackageOnlyBuild)
	mux.HandleFunc("GET /api/projects/{project}/builds", a.buildJobsLocal)
	mux.HandleFunc("DELETE /api/builds/{job}", a.deleteBuildRecord)
	mux.HandleFunc("GET /api/builds/{job}/log", a.buildLogView)
	mux.HandleFunc("GET /api/builds/{job}/artifact", a.downloadPackageOnlyArtifact)
	mux.HandleFunc("POST /api/builds/{job}/simulator-install", a.installSimulatorBuild)
	mux.Handle("GET /", http.FileServer(http.FS(a.static)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != "" {
			origin, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || origin.Host != r.Host {
				fail(w, http.StatusForbidden, "不允许跨域修改")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
