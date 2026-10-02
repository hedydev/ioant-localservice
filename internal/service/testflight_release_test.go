package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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


func TestReconcileTestFlightBuildJobsBackfillsMissingRelease(t *testing.T) {
	data := t.TempDir()
	a := &App{
		data: data,
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	profile := ReleaseProfile{
		ID:             "ios-testflight",
		Name:           "iOS TestFlight",
		Platform:       "ios",
		Architecture:   "arm64",
		Channel:        "beta",
		Variant:        "default",
		Lane:           "ios-testflight",
		ResultContract: "ils-result-v1",
		BuildCommand:   "true",
	}
	if err := os.MkdirAll(filepath.Dir(a.releaseProfilePath("demo", profile.ID)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(a.releaseProfilePath("demo", profile.ID), profile); err != nil {
		t.Fatal(err)
	}

	finished := time.Date(2026, 10, 2, 0, 26, 0, 0, time.UTC)
	job := BuildJob{
		ID:         "11111111111111111111111111111111",
		ProjectID:  "demo",
		Mode:       "profile",
		ProfileID:  profile.ID,
		Title:      profile.Name,
		Platform:   "ios",
		Lane:       "ios-testflight",
		Status:     "succeeded",
		Stage:      "submitted",
		StageState: "succeeded",
		CreatedAt:  finished.Add(-10 * time.Minute),
		FinishedAt: &finished,
		Result: &BuildResult{
			SchemaVersion:    1,
			Lane:             "ios-testflight",
			Status:           "submitted",
			Platform:         "ios",
			Version:          "0.1.0",
			Build:            "228",
			Architecture:     "arm64",
			BundleID:         "com.ioant.sowhat",
			Distribution:     "app-store-connect",
			SubmissionResult: "upload-succeeded",
		},
	}
	path := a.buildJobPath(job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, job); err != nil {
		t.Fatal(err)
	}

	report, err := a.reconcileTestFlightBuildJobs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciled != 1 || len(a.state.Releases) != 1 {
		t.Fatalf("reconciled=%d releases=%d, want 1/1", report.Reconciled, len(a.state.Releases))
	}
	release := a.state.Releases[0]
	if release.Delivery != "testflight" || release.Version != "0.1.0" || release.Build != 228 || release.BundleID != "com.ioant.sowhat" {
		t.Fatalf("recovered release = %#v", release)
	}
	if !release.CreatedAt.Equal(finished) {
		t.Fatalf("created_at = %s, want %s", release.CreatedAt, finished)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored BuildJob
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.ReleaseIDs) != 1 || stored.ReleaseIDs[0] != release.ID {
		t.Fatalf("release_ids = %#v, want [%s]", stored.ReleaseIDs, release.ID)
	}

	report, err = a.reconcileTestFlightBuildJobs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciled != 0 || len(a.state.Releases) != 1 {
		t.Fatalf("second reconcile reconciled=%d releases=%d, want 0/1", report.Reconciled, len(a.state.Releases))
	}
}

func TestReconcileTestFlightBuildJobsUsesPersistedProfileSnapshot(t *testing.T) {
	data := t.TempDir()
	a := &App{
		data: data,
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	job := BuildJob{
		ID:                  "22222222222222222222222222222222",
		ProjectID:           "demo",
		Mode:                "profile",
		ProfileID:           "deleted-profile",
		Title:               "Deleted TestFlight Profile",
		Platform:            "ios",
		Lane:                "ios-testflight",
		ReleaseVariant:      "enterprise-beta",
		ReleaseChannel:      "stable",
		ReleaseArchitecture: "arm64",
		ReleaseNotes:        "snapshot notes",
		TestFlightURL:       "https://testflight.apple.com/join/AbCd1234",
		Status:              "succeeded",
		Stage:               "submitted",
		StageState:          "succeeded",
		CreatedAt:           time.Now().UTC(),
		Result: &BuildResult{
			SchemaVersion:    1,
			Lane:             "ios-testflight",
			Status:           "submitted",
			Platform:         "ios",
			Version:          "2.0.0",
			Build:            "9",
			Architecture:     "arm64",
			BundleID:         "com.example.snapshot",
			Distribution:     "app-store-connect",
			SubmissionResult: "upload-succeeded",
		},
	}
	path := a.buildJobPath(job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, job); err != nil {
		t.Fatal(err)
	}

	report, err := a.reconcileTestFlightBuildJobs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciled != 1 || len(a.state.Releases) != 1 {
		t.Fatalf("reconciled=%d releases=%d, want 1/1", report.Reconciled, len(a.state.Releases))
	}
	release := a.state.Releases[0]
	if release.Variant != job.ReleaseVariant || release.Channel != job.ReleaseChannel ||
		release.Architecture != job.ReleaseArchitecture || release.Notes != job.ReleaseNotes {
		t.Fatalf("snapshot metadata not preserved: %#v", release)
	}
	if release.OpenURL != job.TestFlightURL {
		t.Fatalf("open_url=%q, want %q", release.OpenURL, job.TestFlightURL)
	}
}

func TestReconcileTestFlightBuildJobsRejectsUntrustedUploadResult(t *testing.T) {
	data := t.TempDir()
	a := &App{
		data: data,
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	job := BuildJob{
		ID:         "33333333333333333333333333333333",
		ProjectID:  "demo",
		Platform:   "ios",
		Lane:       "ios-testflight",
		Status:     "succeeded",
		Stage:      "submitted",
		StageState: "succeeded",
		Result: &BuildResult{
			SchemaVersion: 1,
			Lane:          "ios-testflight",
			Status:        "submitted",
			Platform:      "ios",
			Version:       "1.0.0",
			Build:         "1",
			Architecture:  "arm64",
			BundleID:      "com.example.invalid",
			Distribution:  "app-store-connect",
		},
	}
	path := a.buildJobPath(job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, job); err != nil {
		t.Fatal(err)
	}

	report, err := a.reconcileTestFlightBuildJobs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciled != 0 || len(a.state.Releases) != 0 {
		t.Fatalf("untrusted upload was recovered: reconciled=%d releases=%d", report.Reconciled, len(a.state.Releases))
	}
}



func TestReconcileTestFlightBuildJobsAcceptsLegacyMissingTopLevelLaneAndSchema(t *testing.T) {
	data := t.TempDir()
	a := &App{
		data: data,
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	job := BuildJob{
		ID:         "66666666666666666666666666666666",
		ProjectID:  "demo",
		Mode:       "profile",
		ProfileID:  "ios-testflight",
		Title:      "Legacy TestFlight",
		Platform:   "ios",
		Status:     "succeeded",
		Stage:      "submitted",
		StageState: "succeeded",
		CreatedAt:  time.Now().UTC(),
		Result: &BuildResult{
			Lane:             "ios-testflight",
			Status:           "submitted",
			Platform:         "ios",
			Version:          "0.1.0",
			Build:            "224",
			Architecture:     "arm64",
			BundleID:     "com.ioant.sowhat",
			Distribution: "app-store-connect",
		},
	}
	path := a.buildJobPath(job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, job); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(filepath.Dir(path), "build.log"),
		[]byte("ILS_EVENT {\"stage\":\"upload\",\"state\":\"succeeded\",\"message\":\"Upload accepted\"}\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	report, err := a.reconcileTestFlightBuildJobs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 1 || report.Eligible != 1 || report.Reconciled != 1 {
		t.Fatalf("legacy report = %#v", report)
	}
	if len(a.state.Releases) != 1 || a.state.Releases[0].Build != 224 {
		t.Fatalf("legacy release was not recovered: %#v", a.state.Releases)
	}
}

func TestReconcileReportExplainsMissingUploadEvidence(t *testing.T) {
	data := t.TempDir()
	a := &App{
		data: data,
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	job := BuildJob{
		ID:        "77777777777777777777777777777777",
		ProjectID: "demo",
		Lane:      "ios-testflight",
		Result: &BuildResult{
			SchemaVersion: 1,
			Lane:          "ios-testflight",
			Status:        "submitted",
			Platform:      "ios",
			Version:       "1.0.0",
			Build:         "10",
			Architecture:  "arm64",
			BundleID:      "com.example.missing-evidence",
			Distribution:  "app-store-connect",
		},
	}
	path := a.buildJobPath(job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, job); err != nil {
		t.Fatal(err)
	}

	report, err := a.reconcileTestFlightBuildJobs()
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciled != 0 || report.Skipped["missing_upload_succeeded_evidence"] != 1 {
		t.Fatalf("unexpected reconciliation report: %#v", report)
	}
}

func TestPublishTestFlightReleaseDedupesByAppleBuildIdentity(t *testing.T) {
	a := &App{
		data: t.TempDir(),
		state: state{
			Projects: []Project{{ID: "demo", Name: "Demo"}},
			Releases: []Release{},
		},
	}
	result := BuildResult{
		SchemaVersion:    1,
		Lane:             "ios-testflight",
		Status:           "submitted",
		Platform:         "ios",
		Version:          "3.0.0",
		Build:            "12",
		Architecture:     "arm64",
		BundleID:         "com.example.identity",
		Distribution:     "app-store-connect",
		SubmissionResult: "upload-succeeded",
	}
	first, err := a.publishTestFlightRelease(
		BuildJob{ID: "44444444444444444444444444444444", ProjectID: "demo"},
		ReleaseProfile{
			Variant: "default", Channel: "beta", Architecture: "arm64",
			TestFlightURL: "https://testflight.apple.com/join/First123",
		},
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.publishTestFlightRelease(
		BuildJob{ID: "55555555555555555555555555555555", ProjectID: "demo"},
		ReleaseProfile{
			Variant: "renamed-variant", Channel: "stable", Architecture: "arm64",
			TestFlightURL: "https://testflight.apple.com/join/Second12",
		},
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(a.state.Releases) != 1 {
		t.Fatalf("same Apple build created duplicates: first=%s second=%s count=%d", first.ID, second.ID, len(a.state.Releases))
	}
	if a.state.Releases[0].BuildJobID != "55555555555555555555555555555555" {
		t.Fatalf("latest retry was not retained as build_job_id: %q", a.state.Releases[0].BuildJobID)
	}
}
