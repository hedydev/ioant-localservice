package web

import (
	"strings"
	"testing"
)

func TestAppStoreConnectTeamBindingUI(t *testing.T) {
	ascRaw, err := Files.ReadFile("js/app-store-connect.js")
	if err != nil {
		t.Fatal(err)
	}
	ascJS := string(ascRaw)
	for _, required := range []string{
		"/api/app-store-connect/team-binding",
		"asc-team-id",
		"Team API Key 必须绑定这台 Mac 上对应的 Apple Team",
		"asc-team-binding-changed",
	} {
		if !strings.Contains(ascJS, required) {
			t.Fatalf("app-store-connect.js missing Team binding contract %q", required)
		}
	}

	servicesRaw, err := Files.ReadFile("js/services.js")
	if err != nil {
		t.Fatal(err)
	}
	servicesJS := string(servicesRaw)
	for _, required := range []string{
		"/api/app-store-connect/team-binding",
		"ascTeamBinding.team_id",
		"设备注册已禁用",
		"当前设备 Team 与 App Store Connect API Key 绑定 Team 不一致",
	} {
		if !strings.Contains(servicesJS, required) {
			t.Fatalf("services.js missing Team binding contract %q", required)
		}
	}
}
