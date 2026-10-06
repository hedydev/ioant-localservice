package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParsePlistStrings(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><plist><dict><key>CHALLENGE</key><string>abc</string><key>UDID</key><string>00000000-0000000000000000</string><key>PRODUCT</key><string>iPhone</string></dict></plist>`)
	got, err := parsePlistStrings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["CHALLENGE"] != "abc" || got["UDID"] != "00000000-0000000000000000" || got["PRODUCT"] != "iPhone" {
		t.Fatalf("parsed plist = %#v", got)
	}
}

func TestChallengeSurvivesGatewayRestartAndIsSingleUse(t *testing.T) {
	data := t.TempDir()
	g1 := &gateway{data: data, challenges: map[string]time.Time{}}
	challenge, err := g1.newChallenge()
	if err != nil {
		t.Fatal(err)
	}
	if len(challenge) != 48 || strings.Trim(challenge, "0123456789abcdef") != "" {
		t.Fatalf("challenge = %q", challenge)
	}
	if !g1.challengeValid(challenge) {
		t.Fatal("fresh challenge was rejected")
	}

	g2 := &gateway{data: data, challenges: map[string]time.Time{}}
	if !g2.challengeValid(challenge) {
		t.Fatal("persisted challenge did not survive gateway restart")
	}
	if !g2.consumeChallenge(challenge) {
		t.Fatal("persisted challenge could not be consumed")
	}
	if g2.consumeChallenge(challenge) {
		t.Fatal("challenge was accepted twice")
	}
}

func TestExpiredPersistedChallengeIsRejected(t *testing.T) {
	data := t.TempDir()
	g := &gateway{data: data, challenges: map[string]time.Time{}}
	if err := os.MkdirAll(g.challengeDir(), 0700); err != nil {
		t.Fatal(err)
	}
	challenge := strings.Repeat("a", 48)
	expires := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano) + "\n"
	if err := os.WriteFile(g.challengePath(challenge), []byte(expires), 0600); err != nil {
		t.Fatal(err)
	}
	if g.challengeValid(challenge) {
		t.Fatal("expired challenge was accepted")
	}
	if _, err := os.Stat(g.challengePath(challenge)); !os.IsNotExist(err) {
		t.Fatalf("expired challenge file was not removed: %v", err)
	}
}

func TestHealthReportsGatewayProcess(t *testing.T) {
	g := &gateway{}
	req := httptest.NewRequest(http.MethodGet, "/_ils/health", nil)
	res := httptest.NewRecorder()
	g.health(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if got := res.Body.String(); !strings.Contains(got, `"ok":true`) || !strings.Contains(got, `"service":"ils-adhoc-ota"`) || !strings.Contains(got, `"profile_signed":false`) {
		t.Fatalf("health body = %q", got)
	}
}

func TestCollectedEnrollmentPageExplainsProfileCanBeRemoved(t *testing.T) {
	g := &gateway{}
	req := httptest.NewRequest(http.MethodGet, "/enroll?collected=1", nil)
	res := httptest.NewRecorder()
	g.enrollPage(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	body := res.Body.String()
	if !strings.Contains(body, "设备登记已完成") || !strings.Contains(body, "现在可以删除") || !strings.Contains(body, "删除不会取消已经完成的设备登记") {
		t.Fatalf("completion page missing removal guidance: %q", body)
	}
	if strings.Contains(body, `href="/enroll.mobileconfig"`) {
		t.Fatal("completion page unexpectedly offers another enrollment download")
	}
}

func TestSignProfileProducesEmbeddedCMSPayload(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl unavailable")
	}
	if err := exec.Command("openssl", "cms", "-help").Run(); err != nil {
		t.Skip("local openssl does not support cms")
	}
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	key := filepath.Join(dir, "key.pem")
	cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", cert, "-days", "1", "-subj", "/CN=ota.example.test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate signing identity: %v\n%s", err, out)
	}
	g := &gateway{profileSigningCert: cert, profileSigningKey: key}
	payload := []byte(`<?xml version="1.0"?><plist><dict><key>PayloadType</key><string>Profile Service</string></dict></plist>`)
	signed, err := g.signProfile(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(signed, payload) {
		t.Fatal("profile was not wrapped in CMS")
	}
	verify := exec.Command("openssl", "cms", "-verify", "-inform", "DER", "-noverify")
	verify.Stdin = bytes.NewReader(signed)
	decoded, err := verify.Output()
	if err != nil {
		t.Fatalf("verify signed profile: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded payload mismatch\n got: %q\nwant: %q", decoded, payload)
	}
}
