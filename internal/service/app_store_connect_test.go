package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestASCKey(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "AuthKey_ABCDEF1234.p8")
	raw := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path, key
}

func decodeJWTSegment(t *testing.T, value string, target any) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func verifyASCJWT(t *testing.T, token string, key *ecdsa.PrivateKey) (map[string]any, map[string]any) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d, want 3", len(parts))
	}
	var header, claims map[string]any
	decodeJWTSegment(t, parts[0], &header)
	decodeJWTSegment(t, parts[1], &claims)
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(signature) != 64 {
		t.Fatalf("signature length = %d, want 64", len(signature))
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
		t.Fatal("JWT signature verification failed")
	}
	return header, claims
}

func TestASCJWTTeamAndIndividualKeys(t *testing.T) {
	path, key := writeTestASCKey(t)
	now := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)

	team := AppStoreConnectConfig{KeyID: "ABCDEF1234", IssuerID: "11111111-2222-3333-4444-555555555555", PrivateKeyPath: path}
	token, err := ascJWT(team, now)
	if err != nil {
		t.Fatal(err)
	}
	header, claims := verifyASCJWT(t, token, key)
	if header["alg"] != "ES256" || header["kid"] != team.KeyID {
		t.Fatalf("unexpected header: %#v", header)
	}
	if claims["iss"] != team.IssuerID || claims["aud"] != "appstoreconnect-v1" {
		t.Fatalf("unexpected team claims: %#v", claims)
	}
	if _, exists := claims["sub"]; exists {
		t.Fatalf("team token unexpectedly contains sub: %#v", claims)
	}

	individual := AppStoreConnectConfig{KeyID: "ABCDEF1234", PrivateKeyPath: path}
	token, err = ascJWT(individual, now)
	if err != nil {
		t.Fatal(err)
	}
	_, claims = verifyASCJWT(t, token, key)
	if claims["sub"] != "user" {
		t.Fatalf("individual token sub = %#v", claims["sub"])
	}
	if _, exists := claims["iss"]; exists {
		t.Fatalf("individual token unexpectedly contains iss: %#v", claims)
	}
}

func TestNormalizeAppStoreConnectConfig(t *testing.T) {
	path, _ := writeTestASCKey(t)
	cfg, err := normalizeAppStoreConnectConfig(AppStoreConnectConfig{
		KeyID:          "abcdef1234",
		IssuerID:       " issuer ",
		PrivateKeyPath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KeyID != "ABCDEF1234" || cfg.IssuerID != "issuer" {
		t.Fatalf("normalized config = %#v", cfg)
	}
}

func TestBuildUploadAttributesAcceptsNestedAndLegacyState(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantState    string
		wantErrors   int
		wantWarnings int
		wantInfos    int
	}{
		{
			name: "current nested state",
			raw: `{"cfBundleShortVersionString":"0.1.0","cfBundleVersion":"228","platform":"IOS","state":{"state":"PROCESSING","errors":[],"warnings":[{"code":"WARN"}],"infos":[{"message":"Processing"}]}}`,
			wantState: "PROCESSING",
			wantWarnings: 1,
			wantInfos: 1,
		},
		{
			name: "failed nested state",
			raw: `{"cfBundleShortVersionString":"0.1.0","cfBundleVersion":"229","platform":"IOS","state":{"state":"FAILED","errors":[{"code":"ERR1"},{"code":"ERR2"}],"warnings":[],"infos":[]}}`,
			wantState: "FAILED",
			wantErrors: 2,
		},
		{
			name: "legacy string state",
			raw: `{"cfBundleShortVersionString":"0.1.0","cfBundleVersion":"224","platform":"IOS","state":"COMPLETE"}`,
			wantState: "COMPLETE",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var attrs ascBuildUploadAttributes
			if err := json.Unmarshal([]byte(tc.raw), &attrs); err != nil {
				t.Fatal(err)
			}
			if attrs.State.State != tc.wantState ||
				len(attrs.State.Errors) != tc.wantErrors ||
				len(attrs.State.Warnings) != tc.wantWarnings ||
				len(attrs.State.Infos) != tc.wantInfos {
				t.Fatalf("parsed state = %#v", attrs.State)
			}
		})
	}

	var invalid ascBuildUploadAttributes
	if err := json.Unmarshal([]byte(`{"state":{"warnings":[]}}`), &invalid); err == nil {
		t.Fatal("expected nested state without state value to fail")
	}
}

func TestBuildUploadDiagnosticSuffix(t *testing.T) {
	state := ascBuildUploadState{
		State: "FAILED",
		Errors: []json.RawMessage{json.RawMessage(`{"code":"A"}`), json.RawMessage(`{"code":"B"}`)},
		Warnings: []json.RawMessage{json.RawMessage(`{"code":"W"}`)},
		Infos: []json.RawMessage{json.RawMessage(`{"message":"I"}`)},
	}
	if got, want := buildUploadDiagnosticSuffix(state), "（2 个错误，1 个警告，1 条信息）"; got != want {
		t.Fatalf("suffix = %q, want %q", got, want)
	}
}

func TestBuildUploadState(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		wantStatus string
		wantNext   bool
	}{
		{"awaiting upload", "AWAITING_UPLOAD", "submitted", false},
		{"processing", "PROCESSING", "processing", false},
		{"failed", "FAILED", "unavailable", false},
		{"complete", "COMPLETE", "processing", true},
		{"not visible", "", "submitted", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, _, next := buildUploadState(tc.state)
			if status != tc.wantStatus || next != tc.wantNext {
				t.Fatalf("buildUploadState(%q) = %q/%v, want %q/%v", tc.state, status, next, tc.wantStatus, tc.wantNext)
			}
		})
	}
}

func TestTestFlightState(t *testing.T) {
	tests := []struct {
		name       string
		processing string
		internal   string
		external   string
		expired    bool
		want       string
	}{
		{"processing", "PROCESSING", "", "", false, "processing"},
		{"internal ready", "VALID", "READY_FOR_BETA_TESTING", "", false, "available"},
		{"external testing", "VALID", "", "IN_BETA_TESTING", false, "available"},
		{"failed", "FAILED", "", "", false, "unavailable"},
		{"compliance", "VALID", "MISSING_EXPORT_COMPLIANCE", "", false, "processing"},
		{"beta rejected", "VALID", "", "BETA_REJECTED", false, "unavailable"},
		{"expired", "VALID", "IN_BETA_TESTING", "", true, "unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := testFlightState(tc.processing, tc.internal, tc.external, tc.expired)
			if got != tc.want {
				t.Fatalf("state = %q, want %q", got, tc.want)
			}
		})
	}
}


func TestAppStoreConnectBuildEnvOnlyInjectsTeamKeys(t *testing.T) {
	a := &App{data: t.TempDir()}
	path, _ := writeTestASCKey(t)

	individual := AppStoreConnectConfig{KeyID: "ABCDEF1234", PrivateKeyPath: path}
	if err := atomicJSON(a.appStoreConnectConfigPath(), individual); err != nil {
		t.Fatal(err)
	}
	if env := a.appStoreConnectBuildEnv(); len(env) != 0 {
		t.Fatalf("individual key unexpectedly injected into xcodebuild env: %#v", env)
	}

	team := individual
	team.IssuerID = "11111111-2222-3333-4444-555555555555"
	if err := atomicJSON(a.appStoreConnectConfigPath(), team); err != nil {
		t.Fatal(err)
	}
	env := a.appStoreConnectBuildEnv()
	if len(env) != 3 {
		t.Fatalf("team key env length = %d, want 3: %#v", len(env), env)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"ILS_ASC_KEY_ID=ABCDEF1234",
		"ILS_ASC_KEY_PATH=" + realPath,
		"ILS_ASC_ISSUER_ID=" + team.IssuerID,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("team env missing %q: %#v", want, env)
		}
	}
}
