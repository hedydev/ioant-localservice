package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validSimulatorProfile() ReleaseProfile {
	return ReleaseProfile{
		ID:             "ios-simulator",
		Name:           "iOS Simulator Internal Test",
		Platform:       "ios",
		Architecture:   "arm64",
		Channel:        "dev",
		Variant:        "default",
		Lane:           "ios-simulator",
		ResultContract: "ils-result-v1",
		BuildCommand:   "bash scripts/ils-build-ios-simulator.sh",
	}
}

func TestNormalizeSimulatorReleaseProfile(t *testing.T) {
	profile, err := normalizeSimulatorReleaseProfile(validSimulatorProfile())
	if err != nil {
		t.Fatalf("normalizeSimulatorReleaseProfile: %v", err)
	}
	if profile.Platform != "ios" || profile.Lane != "ios-simulator" || profile.Architecture != "arm64" {
		t.Fatalf("unexpected normalized profile: %+v", profile)
	}
	if profile.AppleTeamID != "" {
		t.Fatalf("Simulator profile unexpectedly requires Apple Team %q", profile.AppleTeamID)
	}
}

func TestNormalizeSimulatorReleaseProfileRequiresContract(t *testing.T) {
	profile := validSimulatorProfile()
	profile.ResultContract = ""
	if _, err := normalizeSimulatorReleaseProfile(profile); err == nil || !strings.Contains(err.Error(), "ils-result-v1") {
		t.Fatalf("expected ils-result-v1 error, got %v", err)
	}
}

func TestNormalizeSimulatorReleaseProfileRejectsNonArm64(t *testing.T) {
	profile := validSimulatorProfile()
	profile.Architecture = "x86_64"
	if _, err := normalizeSimulatorReleaseProfile(profile); err == nil {
		t.Fatal("expected non-arm64 Simulator profile to fail")
	}
}

func TestSimulatorInternalTestRecordsExposeSucceededZip(t *testing.T) {
	data := t.TempDir()
	a := &App{data: data}
	jobID := strings.Repeat("a", 32)
	output := filepath.Join(data, "builds", jobID, "output")
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(output, "Sowhat-0.1.0-42-iOS-Simulator-arm64.zip")
	if err := os.WriteFile(artifact, []byte("simulator-package"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := BuildJob{
		ID: jobID, ProjectID: "sowhat", Status: "succeeded", Platform: "ios", Lane: "ios-simulator",
		ReleaseChannel: "dev", ReleaseVariant: "default", ReleaseArchitecture: "arm64",
		CreatedAt: now, FinishedAt: &now,
		Result: &BuildResult{
			SchemaVersion: 1, Lane: "ios-simulator", Status: "succeeded", Platform: "ios",
			Artifact: artifact, Version: "0.1.0", Build: "42", Architecture: "arm64",
			BundleID: "com.ioant.sowhat", Distribution: "simulator",
		},
	}
	if err := atomicJSON(a.buildJobPath(jobID), job); err != nil {
		t.Fatal(err)
	}

	records := a.simulatorInternalTestRecords("sowhat")
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.ID != jobID || record.Platform != "ios" || record.Lane != "ios-simulator" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Filename != filepath.Base(artifact) || record.DownloadURL != "/api/builds/"+jobID+"/artifact" {
		t.Fatalf("unexpected artifact view: %+v", record)
	}
}
