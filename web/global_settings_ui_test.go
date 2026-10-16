package web

import (
	"strings"
	"testing"
)

func TestAppleDeveloperOwnsGlobalServiceConfiguration(t *testing.T) {
	appleRaw, err := Files.ReadFile("js/apple-global.js")
	if err != nil {
		t.Fatal(err)
	}
	apple := string(appleRaw)
	for _, required := range []string{
		"global-apple-button",
		"dataset.globalView='apple'",
		"relocateGlobalConfiguration",
		"#asc-status",
		"App Store Connect / Apple Developer",
		"所有项目共享",
	} {
		if !strings.Contains(apple, required) {
			t.Fatalf("apple-global.js missing Apple services contract %q", required)
		}
	}

	settingsRaw, err := Files.ReadFile("js/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	settings := string(settingsRaw)
	if strings.Contains(settings, "#asc-status") || strings.Contains(settings, "relocateGlobalConfiguration") || strings.Contains(settings, "global-settings-button") {
		t.Fatal("browser settings must not own App Store Connect or a left-side global settings entry")
	}
	for _, required := range []string{"<h2>设置</h2>", "服务级 Apple / OTA 配置使用左侧独立菜单"} {
		if !strings.Contains(settings, required) {
			t.Fatalf("settings.js missing separated settings contract %q", required)
		}
	}

	ascRaw, err := Files.ReadFile("js/app-store-connect.js")
	if err != nil {
		t.Fatal(err)
	}
	asc := string(ascRaw)
	for _, required := range []string{
		"state.view==='apple'",
		"event.detail.view==='apple'",
		"state.view!=='apple'",
	} {
		if !strings.Contains(asc, required) {
			t.Fatalf("app-store-connect.js missing Apple global lifecycle %q", required)
		}
	}
	for _, forbidden := range []string{"state.view==='ios'", "event.detail.view==='ios'", "state.view!=='ios'", "state.view==='settings'", "event.detail.view==='settings'", "state.view!=='settings'"} {
		if strings.Contains(asc, forbidden) {
			t.Fatalf("App Store Connect must not be scoped to project iOS or browser settings: %q", forbidden)
		}
	}

	navigationRaw, err := Files.ReadFile("js/navigation.js")
	if err != nil {
		t.Fatal(err)
	}
	navigation := string(navigationRaw)
	if !strings.Contains(navigation, "#global-services-nav [data-global-view]") || !strings.Contains(navigation, "'apple'") {
		t.Fatal("navigation.js must highlight and protect the Apple global view")
	}

	appRaw, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appRaw), "./js/apple-global.js") {
		t.Fatal("app.js must initialize the dedicated Apple global module")
	}
}
