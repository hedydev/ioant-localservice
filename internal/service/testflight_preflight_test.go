package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectTestFlightBundleID(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, ".ils")
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "schema_version": 1,
  "id": "demo",
  "name": "Demo",
  "app_store_connect": {
    "profiles": {
      "ios-testflight": {
        "bundle_id": "com.ioant.demo.mobile"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(manifestDir, "project.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	bundleID, err := projectTestFlightBundleID(root, "ios-testflight")
	if err != nil {
		t.Fatal(err)
	}
	if bundleID != "com.ioant.demo.mobile" {
		t.Fatalf("bundleID = %q", bundleID)
	}
	missing, err := projectTestFlightBundleID(root, "other-profile")
	if err != nil {
		t.Fatal(err)
	}
	if missing != "" {
		t.Fatalf("missing profile bundleID = %q", missing)
	}
}

func TestPreviousTestFlightBundleIDUsesNewestRelease(t *testing.T) {
	older := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	a := &App{state: state{Releases: []Release{
		{ProjectID: "demo", Delivery: "testflight", BundleID: "com.ioant.demo.old", CreatedAt: older},
		{ProjectID: "other", Delivery: "testflight", BundleID: "com.ioant.other", CreatedAt: newer},
		{ProjectID: "demo", Delivery: "testflight", BundleID: "com.ioant.demo.mobile", CreatedAt: newer},
	}}}
	if got := a.previousTestFlightBundleID("demo"); got != "com.ioant.demo.mobile" {
		t.Fatalf("bundleID = %q", got)
	}
}
