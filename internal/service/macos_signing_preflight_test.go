package service

import (
	"strings"
	"testing"
)

func TestParseAppleDevelopmentIdentities(t *testing.T) {
	raw := []byte(`  1) AAA "Apple Development: Ted Example (64RS366WKG)"
  2) BBB "Apple Distribution: Ted Example (64RS366WKG)"
  3) CCC "Developer ID Application: Ted Example (64RS366WKG)"
  4) DDD "Apple Development: Other Example (ABCDEFGHIJ)"
     4 valid identities found
`)
	got := parseAppleDevelopmentIdentities(raw)
	if len(got) != 2 {
		t.Fatalf("development identities = %d, want 2: %#v", len(got), got)
	}
	if got[0].TeamID != "64RS366WKG" || !strings.HasPrefix(got[0].Identity, "Apple Development:") {
		t.Fatalf("first identity = %#v", got[0])
	}
	if got[1].TeamID != "ABCDEFGHIJ" || !strings.HasPrefix(got[1].Identity, "Apple Development:") {
		t.Fatalf("second identity = %#v", got[1])
	}
}

func TestSelectMacOSDevelopmentSigningPreferredTeam(t *testing.T) {
	identities := []macOSDevelopmentSigning{
		{TeamID: "64RS366WKG", Identity: "Apple Development: Ted Example (64RS366WKG)"},
		{TeamID: "ABCDEFGHIJ", Identity: "Apple Development: Other Example (ABCDEFGHIJ)"},
	}
	got, err := selectMacOSDevelopmentSigning(identities, "64rs366wkg")
	if err != nil {
		t.Fatal(err)
	}
	if got.TeamID != "64RS366WKG" {
		t.Fatalf("team = %q", got.TeamID)
	}
}

func TestSelectMacOSDevelopmentSigningRejectsMissingPreferredTeam(t *testing.T) {
	_, err := selectMacOSDevelopmentSigning([]macOSDevelopmentSigning{
		{TeamID: "ABCDEFGHIJ", Identity: "Apple Development: Other Example (ABCDEFGHIJ)"},
	}, "64RS366WKG")
	if err == nil || !strings.Contains(err.Error(), "Team 64RS366WKG") {
		t.Fatalf("error = %v", err)
	}
}

func TestSelectMacOSDevelopmentSigningRejectsAmbiguousTeam(t *testing.T) {
	_, err := selectMacOSDevelopmentSigning([]macOSDevelopmentSigning{
		{TeamID: "64RS366WKG", Identity: "Apple Development: Ted Example (64RS366WKG)"},
		{TeamID: "ABCDEFGHIJ", Identity: "Apple Development: Other Example (ABCDEFGHIJ)"},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "多个 Apple Development Team") {
		t.Fatalf("error = %v", err)
	}
}

func TestSelectMacOSDevelopmentSigningRejectsMissingCertificate(t *testing.T) {
	_, err := selectMacOSDevelopmentSigning(nil, "")
	if err == nil || !strings.Contains(err.Error(), "Apple Development 证书") {
		t.Fatalf("error = %v", err)
	}
}

func TestMacOSPackageOnlyLaneIsInternalTestOnly(t *testing.T) {
	if !isMacOSPackageOnlyLane("macos-test") {
		t.Fatal("macos-test must be package-only")
	}
	for _, lane := range []string{"macos-release", "macos-portable-adhoc", "ios-adhoc"} {
		if isMacOSPackageOnlyLane(lane) {
			t.Fatalf("lane %q must not use macOS Internal Test package-only semantics", lane)
		}
	}
}
