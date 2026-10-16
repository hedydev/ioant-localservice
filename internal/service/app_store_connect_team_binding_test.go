package service

import (
	"strings"
	"testing"
)

func TestAppStoreConnectTeamBindingRequiresCurrentMatchingTeam(t *testing.T) {
	a := &App{data: t.TempDir()}
	cfg := AppStoreConnectConfig{
		KeyID:    "ABCDEF1234",
		IssuerID: "issuer-a",
	}

	if err := a.validateAppStoreConnectTeamSelection(cfg, "64RS366WKG"); err == nil || !strings.Contains(err.Error(), "绑定 Apple Team") {
		t.Fatalf("missing binding error = %v", err)
	}

	binding, err := a.writeAppStoreConnectTeamBinding(cfg, "64RS366WKG")
	if err != nil {
		t.Fatal(err)
	}
	if binding.TeamID != "64RS366WKG" || binding.KeyID != cfg.KeyID || binding.IssuerID != cfg.IssuerID {
		t.Fatalf("binding = %+v", binding)
	}
	if err = a.validateAppStoreConnectTeamSelection(cfg, "64RS366WKG"); err != nil {
		t.Fatalf("matching team rejected: %v", err)
	}
	if err = a.validateAppStoreConnectTeamSelection(cfg, "ABCDEFGHIJ"); err == nil || !strings.Contains(err.Error(), "不能注册") {
		t.Fatalf("mismatched team error = %v", err)
	}

	changed := cfg
	changed.IssuerID = "issuer-b"
	if err = a.validateAppStoreConnectTeamSelection(changed, "64RS366WKG"); err == nil || !strings.Contains(err.Error(), "已变更") {
		t.Fatalf("stale key error = %v", err)
	}
}

func TestNormalizeAppleTeamID(t *testing.T) {
	teamID, err := normalizeAppleTeamID(" 64rs366wkg ")
	if err != nil {
		t.Fatal(err)
	}
	if teamID != "64RS366WKG" {
		t.Fatalf("teamID = %q", teamID)
	}
	if _, err = normalizeAppleTeamID("short"); err == nil {
		t.Fatal("invalid Team ID was accepted")
	}
}
