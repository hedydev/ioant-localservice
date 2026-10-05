package main

import (
	"net/http"
	"net/http/httptest"
	"os"
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
	if got := res.Body.String(); !strings.Contains(got, `"ok":true`) || !strings.Contains(got, `"service":"ils-adhoc-ota"`) {
		t.Fatalf("health body = %q", got)
	}
}
