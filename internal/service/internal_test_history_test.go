package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInternalTestRecordsExposeSuccessfulArtifactWithoutRelease(t *testing.T) {
	data := t.TempDir()
	a := &App{data: data}
	created := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	job := BuildJob{
		ID: "0123456789abcdef0123456789abcdef",
		ProjectID: "wincat",
		ProfileID: "macos-test",
		Title: "WinCat macOS Internal Test",
		Platform: "macos",
		Lane: "macos-test",
		ReleaseVariant: "internal",
		ReleaseChannel: "dev",
		ReleaseArchitecture: "arm64",
		Status: "succeeded",
		Stage: "complete",
		CreatedAt: created,
		Result: &BuildResult{
			SchemaVersion: 1,
			Lane: "macos-test",
			Status: "succeeded",
			Platform: "macos",
			Artifact: "WinCat_0.1.0_aarch64.dmg",
			Version: "0.1.0",
			Build: "17",
			Architecture: "arm64",
			Distribution: "internal-test",
		},
	}
	jobDir := filepath.Join(data, "builds", job.ID)
	output := filepath.Join(jobDir, "output")
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, job.Result.Artifact), []byte("dmg-data"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "job.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}

	records := a.internalTestRecords("wincat")
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != "internal-test" || record.Version != "0.1.0" || record.Build != "17" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Filename != "WinCat_0.1.0_aarch64.dmg" || record.Size == 0 {
		t.Fatalf("artifact metadata missing: %+v", record)
	}
	if record.DownloadURL != "/api/builds/"+job.ID+"/artifact" {
		t.Fatalf("download_url = %q", record.DownloadURL)
	}
}

func TestInternalTestRecordsIgnoreFailedJobs(t *testing.T) {
	data := t.TempDir()
	a := &App{data: data}
	job := BuildJob{
		ID: "fedcba9876543210fedcba9876543210",
		ProjectID: "wincat",
		Platform: "macos",
		Lane: "macos-test",
		Status: "failed",
		CreatedAt: time.Now().UTC(),
		Result: &BuildResult{Lane: "macos-test", Status: "failed", Platform: "macos", Artifact: "failed.dmg"},
	}
	jobDir := filepath.Join(data, "builds", job.ID)
	if err := os.MkdirAll(jobDir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "job.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got := a.internalTestRecords("wincat"); len(got) != 0 {
		t.Fatalf("records = %d, want 0", len(got))
	}
}
