package service

import (
	"crypto/x509"
	"strings"
	"testing"
)

func TestParseCodeSigningIdentityRefs(t *testing.T) {
	raw := []byte(`  1) 6CEF17A0296FD73CF917262BA1B4B3BD665CC3E8 "Apple Development: ao bing (CC9PUAE5N3)"
  2) 0123456789ABCDEF0123456789ABCDEF01234567 "Apple Distribution: Bing Ao (64RS366WKG)"
     2 valid identities found
`)
	got := parseCodeSigningIdentityRefs(raw)
	if len(got) != 2 {
		t.Fatalf("identity refs = %d, want 2: %#v", len(got), got)
	}
	if got[0].Hash != "6CEF17A0296FD73CF917262BA1B4B3BD665CC3E8" || got[0].Identity != "Apple Development: ao bing (CC9PUAE5N3)" {
		t.Fatalf("first identity = %#v", got[0])
	}
}

func TestAppleCertificateTeamIDUsesOrganizationalUnit(t *testing.T) {
	cert := &x509.Certificate{}
	cert.Subject.OrganizationalUnit = []string{"64RS366WKG"}
	if got := appleCertificateTeamID(cert); got != "64RS366WKG" {
		t.Fatalf("team = %q", got)
	}
}

func TestAppleDevelopmentSigningsUseCertificateTeamNotCommonNameSuffix(t *testing.T) {
	got := appleDevelopmentSignings([]codeSigningIdentity{
		{
			Hash:     "6CEF17A0296FD73CF917262BA1B4B3BD665CC3E8",
			Identity: "Apple Development: ao bing (CC9PUAE5N3)",
			TeamID:   "64RS366WKG",
		},
	})
	if len(got) != 1 {
		t.Fatalf("development identities = %d, want 1: %#v", len(got), got)
	}
	if got[0].TeamID != "64RS366WKG" {
		t.Fatalf("team = %q, want certificate OU team", got[0].TeamID)
	}
	if got[0].Identity != "Apple Development: ao bing (CC9PUAE5N3)" {
		t.Fatalf("identity = %q", got[0].Identity)
	}
}

func TestSelectMacOSDevelopmentSigningPreferredTeam(t *testing.T) {
	identities := []macOSDevelopmentSigning{
		{TeamID: "64RS366WKG", Identity: "Apple Development: ao bing (CC9PUAE5N3)"},
		{TeamID: "ABCDEFGHIJ", Identity: "Apple Development: Other Example (OTHER12345)"},
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
		{TeamID: "ABCDEFGHIJ", Identity: "Apple Development: Other Example (OTHER12345)"},
	}, "64RS366WKG")
	if err == nil || !strings.Contains(err.Error(), "Team 64RS366WKG") {
		t.Fatalf("error = %v", err)
	}
}

func TestSelectMacOSDevelopmentSigningRejectsAmbiguousTeam(t *testing.T) {
	_, err := selectMacOSDevelopmentSigning([]macOSDevelopmentSigning{
		{TeamID: "64RS366WKG", Identity: "Apple Development: ao bing (CC9PUAE5N3)"},
		{TeamID: "ABCDEFGHIJ", Identity: "Apple Development: Other Example (OTHER12345)"},
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
