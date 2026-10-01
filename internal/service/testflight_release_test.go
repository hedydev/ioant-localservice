package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishTestFlightReleaseCreatesMetadataRelease(t *testing.T) {
	a := &App{
		data: t.TempDir(),
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	job := BuildJob{ID: "0123456789abcdef0123456789abcdef", ProjectID: "demo"}
	profile := ReleaseProfile{
		ID:            "ios-testflight",
		Name:          "iOS TestFlight",
		Platform:      "ios",
		Architecture:  "arm64",
		Channel:       "beta",
		Variant:       "default",
		Lane:          "ios-testflight",
		TestFlightURL: "https://testflight.apple.com/join/AbCd1234",
	}
	result := BuildResult{
		Lane:         "ios-testflight",
		Status:       "submitted",
		Platform:     "ios",
		Version:      "1.2.3",
		Build:        "45",
		Architecture: "arm64",
		BundleID:     "com.example.demo",
		Distribution: "app-store-connect",
	}

	release, err := a.publishTestFlightRelease(job, profile, result)
	if err != nil {
		t.Fatal(err)
	}
	if release.Delivery != "testflight" || release.Status != "submitted" {
		t.Fatalf("release delivery/status = %q/%q", release.Delivery, release.Status)
	}
	if release.DownloadURL != "" || release.Filename != "" || release.SHA256 != "" {
		t.Fatalf("TestFlight release unexpectedly has local artifact metadata: %#v", release)
	}
	if release.OpenURL != profile.TestFlightURL {
		t.Fatalf("open_url = %q, want %q", release.OpenURL, profile.TestFlightURL)
	}
	if release.TestFlight == nil || release.TestFlight.FallbackURL != profile.TestFlightURL {
		t.Fatalf("fallback URL was not retained separately: %#v", release.TestFlight)
	}
	if len(a.state.Releases) != 1 {
		t.Fatalf("release count = %d, want 1", len(a.state.Releases))
	}

	appleURL := "https://testflight.apple.com/join/ZyXw9876"
	a.state.Releases[0].Status = "available"
	a.state.Releases[0].StatusMessage = "TestFlight 已可测试"
	a.state.Releases[0].OpenURL = appleURL
	a.state.Releases[0].TestFlight = &TestFlightReleaseInfo{
		PublicLink:  appleURL,
		FallbackURL: profile.TestFlightURL,
	}

	again, err := a.publishTestFlightRelease(job, profile, result)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != release.ID || len(a.state.Releases) != 1 {
		t.Fatalf("idempotent TestFlight publish created duplicate: first=%s second=%s count=%d", release.ID, again.ID, len(a.state.Releases))
	}
	if again.Status != "available" || again.OpenURL != appleURL {
		t.Fatalf("idempotent publish rewrote Apple state/link: %#v", again)
	}
	if again.TestFlight == nil || again.TestFlight.FallbackURL != profile.TestFlightURL {
		t.Fatalf("idempotent publish lost fallback URL: %#v", again.TestFlight)
	}
}

func TestTestFlightFallbackURLTracksSource(t *testing.T) {
	fallback := "https://testflight.apple.com/join/Fallback1"
	public := "https://testflight.apple.com/join/Public123"
	tests := []struct {
		name    string
		release Release
		want    string
	}{
		{
			name:    "legacy open url is fallback",
			release: Release{OpenURL: fallback, Delivery: "testflight"},
			want:    fallback,
		},
		{
			name: "apple public link without stored fallback",
			release: Release{
				OpenURL:    public,
				Delivery:   "testflight",
				TestFlight: &TestFlightReleaseInfo{PublicLink: public},
			},
			want: "",
		},
		{
			name: "stored fallback survives apple public link",
			release: Release{
				OpenURL:  public,
				Delivery: "testflight",
				TestFlight: &TestFlightReleaseInfo{
					PublicLink:  public,
					FallbackURL: fallback,
				},
			},
			want: fallback,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := testFlightFallbackURL(tc.release); got != tc.want {
				t.Fatalf("fallback = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSyncBuildJobFromTestFlightRelease(t *testing.T) {
	a := &App{data: t.TempDir()}
	job := BuildJob{
		ID:         "0123456789abcdef0123456789abcdef",
		ProjectID:  "demo",
		Lane:       "ios-testflight",
		Status:     "succeeded",
		Stage:      "submitted",
		StageState: "succeeded",
	}
	path := a.buildJobPath(job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, job); err != nil {
		t.Fatal(err)
	}

	a.syncBuildJobFromTestFlightRelease(Release{
		BuildJobID:    job.ID,
		Status:        "unavailable",
		StatusMessage: "Apple Processing 失败",
	})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got BuildJob
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Stage != "available" || got.StageState != "failed" || got.Status != "succeeded" {
		t.Fatalf("synced job = stage %q state %q status %q", got.Stage, got.StageState, got.Status)
	}
	if got.Message != "Apple Processing 失败" {
		t.Fatalf("message = %q", got.Message)
	}
}
