package web

import (
	"strings"
	"testing"
)

func TestSettingsThemeIntegration(t *testing.T) {
	appRaw, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(appRaw)
	for _, required := range []string{
		"localStorage.getItem('ils.theme')",
		"theme.css?v=20261013",
		"./js/settings.js",
		"initSettings",
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("app.js missing theme bootstrap %q", required)
		}
	}

	routerRaw, err := Files.ReadFile("js/router.js")
	if err != nil {
		t.Fatal(err)
	}
	router := string(routerRaw)
	for _, required := range []string{
		"globalViews=new Set(['services','settings'])",
		"if(globalViews.has(view))return '#/'+view",
	} {
		if !strings.Contains(router, required) {
			t.Fatalf("router.js missing settings route guard %q", required)
		}
	}

	settingsRaw, err := Files.ReadFile("js/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	settings := string(settingsRaw)
	for _, required := range []string{
		"new Set(['light','dark'])",
		"data-theme-choice=\"light\"",
		"data-theme-choice=\"dark\"",
		"localStorage.setItem(THEME_KEY,value)",
		"activateView('settings')",
		"已切换为深色主题",
	} {
		if !strings.Contains(settings, required) {
			t.Fatalf("settings.js missing theme behavior %q", required)
		}
	}

	themeRaw, err := Files.ReadFile("theme.css")
	if err != nil {
		t.Fatal(err)
	}
	theme := string(themeRaw)
	for _, required := range []string{
		`:root[data-theme="light"]`,
		`:root[data-theme="dark"]`,
		`.theme-choice-grid`,
		`:root[data-theme="dark"] dialog`,
		`:root[data-theme="dark"] .build-job`,
	} {
		if !strings.Contains(theme, required) {
			t.Fatalf("theme.css missing theme presentation %q", required)
		}
	}
}
