package web

import (
	"strings"
	"testing"
)

func TestGlobalSettingsOwnServiceConfiguration(t *testing.T) {
	settingsRaw, err := Files.ReadFile("js/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	settings := string(settingsRaw)
	for _, required := range []string{
		"全局设置",
		"这些配置不属于任何单个项目",
		"global-settings-button",
		"dataset.globalView='settings'",
		"relocateGlobalConfiguration",
		"#asc-status",
		"App Store Connect / Apple Developer",
		"data-go-view=\"services\"",
	} {
		if !strings.Contains(settings, required) {
			t.Fatalf("settings.js missing global settings contract %q", required)
		}
	}

	ascRaw, err := Files.ReadFile("js/app-store-connect.js")
	if err != nil {
		t.Fatal(err)
	}
	asc := string(ascRaw)
	for _, required := range []string{
		"state.view==='settings'",
		"event.detail.view==='settings'",
		"state.view!=='settings'",
	} {
		if !strings.Contains(asc, required) {
			t.Fatalf("app-store-connect.js missing global settings lifecycle %q", required)
		}
	}
	if strings.Contains(asc, "state.view==='ios'") || strings.Contains(asc, "event.detail.view==='ios'") || strings.Contains(asc, "state.view!=='ios'") {
		t.Fatal("App Store Connect configuration must not be scoped to the project iOS view")
	}

	navigationRaw, err := Files.ReadFile("js/navigation.js")
	if err != nil {
		t.Fatal(err)
	}
	navigation := string(navigationRaw)
	if !strings.Contains(navigation, "#global-services-nav [data-global-view]") {
		t.Fatal("navigation.js must highlight all global views, including settings")
	}
}
