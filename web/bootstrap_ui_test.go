package web

import (
	"strings"
	"testing"
)

func TestProjectBootstrapIsIndependentFromAdminAndOptionalModules(t *testing.T) {
	appRaw, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(appRaw)
	if strings.Contains(app, "await restoreAdminSession()") {
		t.Fatal("core project bootstrap must not await administrator-session restoration")
	}
	for _, forbidden := range []string{
		"import {initBuildJobs}",
		"import {initAutomation}",
		"import {initAppStoreConnect}",
	} {
		if strings.Contains(app, forbidden) {
			t.Fatalf("optional module must not be a static bootstrap dependency: %q", forbidden)
		}
	}
	for _, required := range []string{
		"initCore();",
		"initProjects();",
		"await import(path)",
		"Promise.all(optionalModules.map(initOptionalModule))",
		"void finishBootstrap();",
		"部分功能模块初始化失败",
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("app.js missing fault-isolated bootstrap guard %q", required)
		}
	}
	projectsIndex := strings.Index(app, "initProjects();")
	finishIndex := strings.Index(app, "void finishBootstrap();")
	if projectsIndex < 0 || finishIndex < 0 || projectsIndex > finishIndex {
		t.Fatal("public project bootstrap must start before optional-module/admin bootstrap")
	}

	projectsRaw, err := Files.ReadFile("js/projects.js")
	if err != nil {
		t.Fatal(err)
	}
	projects := string(projectsRaw)
	projectFetch := strings.Index(projects, "await api('/api/projects'")
	render := strings.Index(projects, "renderProjects();")
	connected := strings.Index(projects, "$('#connection').textContent='服务已连接';")
	releaseFetch := strings.Index(projects, "await api('/api/projects/'+encodeURIComponent(current)+'/releases'")
	if projectFetch < 0 || render < 0 || connected < 0 || releaseFetch < 0 {
		t.Fatal("projects.js bootstrap markers are missing")
	}
	if !(projectFetch < render && render < releaseFetch && connected < releaseFetch) {
		t.Fatal("project list and connection state must render before Release loading")
	}

	coreRaw, err := Files.ReadFile("js/core.js")
	if err != nil {
		t.Fatal(err)
	}
	core := string(coreRaw)
	for _, required := range []string{
		"fetchWithTimeout",
		"authenticatedAtStart&&epoch!==state.authEpoch",
		"fetchWithTimeout('/api/admin/session'",
	} {
		if !strings.Contains(core, required) {
			t.Fatalf("core.js missing non-blocking bootstrap guard %q", required)
		}
	}
}
