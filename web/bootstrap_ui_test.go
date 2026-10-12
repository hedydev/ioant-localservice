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
	// Administrator restoration may be awaited inside finishBootstrap because
	// finishBootstrap itself is launched without awaiting it. What must never
	// return is a top-level project bootstrap that blocks initProjects on the
	// remembered administrator session.
	if strings.Contains(app, "initCore();\nawait restoreAdminSession()") || strings.Contains(app, "initProjects();\nawait restoreAdminSession()") {
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
		"await restoreAdminSession();",
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
	historyLoad := strings.Index(projects, "await loadProjectReleases(current);")
	if projectFetch < 0 || render < 0 || connected < 0 || historyLoad < 0 {
		t.Fatal("projects.js bootstrap markers are missing")
	}
	if !(projectFetch < render && render < historyLoad && connected < historyLoad) {
		t.Fatal("project list and connection state must render before Release/Internal Test history loading")
	}
	for _, required := range []string{
		"Promise.allSettled([",
		"api('/api/projects/'+encodeURIComponent(project)+'/releases'",
		"api('/api/projects/'+encodeURIComponent(project)+'/internal-test-records'",
	} {
		if !strings.Contains(projects, required) {
			t.Fatalf("projects.js missing fault-isolated history loader %q", required)
		}
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
