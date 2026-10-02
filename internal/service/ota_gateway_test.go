package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOTAManifestUsesGatewayArtifactURL(t *testing.T) {
	release := Release{
		ProjectID: "demo",
		Version:   "1.2.3",
		Build:     7,
		BundleID:  "com.example.demo",
		IOS:       &IOSInfo{Build: "7"},
	}
	artifact := "https://ota.example.com/releases/demo/abcdef/app.ipa"
	got := otaManifestXML(release, artifact)
	for _, want := range []string{artifact, "com.example.demo", "<string>7</string>", "demo 1.2.3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("manifest missing %q: %s", want, got)
		}
	}
}

func TestOTAInstallURL(t *testing.T) {
	manifest := "https://ota.example.com/releases/demo/abc/manifest.plist"
	got := otaInstallURL(manifest)
	if !strings.HasPrefix(got, "itms-services://?action=download-manifest&url=") {
		t.Fatalf("unexpected install URL: %q", got)
	}
	if !strings.Contains(got, "https%3A%2F%2Fota.example.com") {
		t.Fatalf("manifest URL was not escaped: %q", got)
	}
}

func TestSaveDeviceRecordKeepsGatewaySource(t *testing.T) {
	data := t.TempDir()
	a := &App{data: data}
	d := Device{
		UDID:        "00000000-0000000000000000",
		Product:     "iPhone",
		Version:     "26.0",
		CollectedAt: time.Now().UTC(),
		Status:      "pending_apple_registration",
		Source:      "public_ota_gateway",
	}
	created, err := a.saveDeviceRecord(d)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first device save should report created")
	}
	files, err := filepath.Glob(filepath.Join(data, "devices", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("device files = %v, err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var got Device
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "public_ota_gateway" || got.UDID != d.UDID {
		t.Fatalf("stored device = %#v", got)
	}
	created, err = a.saveDeviceRecord(d)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("same UDID should update the same registry record")
	}
}
