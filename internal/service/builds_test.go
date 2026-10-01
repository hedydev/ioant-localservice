package service

import (
	"bytes"
	"testing"
)

func TestEventLogParsesXcodeProgress(t *testing.T) {
	var dst bytes.Buffer
	var events []BuildEvent
	log := eventLog{
		dst:    &dst,
		secret: "top-secret",
		onEvent: func(ev BuildEvent) {
			events = append(events, ev)
		},
	}

	input := "ILS_EVENT {\"stage\":\"upload\",\"state\":\"started\",\"message\":\"Uploading\"}\n" +
		"2026-10-01 11:07:00.000 xcodebuild[1:2] Progress 37%: Uploading package…\n"
	if _, err := log.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	got := events[1]
	if got.Stage != "upload" || got.State != "running" {
		t.Fatalf("stage/state = %s/%s", got.Stage, got.State)
	}
	if got.Progress == nil || *got.Progress != 37 {
		t.Fatalf("progress = %v, want 37", got.Progress)
	}
	if got.Message != "Uploading package…" {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestEventLogIgnoresXcodeProgressWithoutXcodeStage(t *testing.T) {
	var dst bytes.Buffer
	var events []BuildEvent
	log := eventLog{
		dst: &dst,
		onEvent: func(ev BuildEvent) {
			events = append(events, ev)
		},
	}

	input := "ILS_EVENT {\"stage\":\"preflight\",\"state\":\"started\"}\n" +
		"Progress 91%: not an Xcode transfer stage\n"
	if _, err := log.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}


func TestNormalizeReleaseProfileTestFlightLink(t *testing.T) {
	base := ReleaseProfile{
		ID:             "ios-testflight",
		Name:           "iOS TestFlight",
		Platform:       "ios",
		Architecture:   "arm64",
		Channel:        "beta",
		Variant:        "default",
		Lane:           "ios-testflight",
		ResultContract: "ils-result-v1",
		BuildCommand:   "bash scripts/ils-build-ios.sh --testflight",
	}

	valid := base
	valid.TestFlightURL = "https://testflight.apple.com/join/AbCd1234"
	got, err := normalizeReleaseProfile(valid)
	if err != nil {
		t.Fatalf("normalize valid TestFlight URL: %v", err)
	}
	if got.TestFlightURL != valid.TestFlightURL {
		t.Fatalf("TestFlightURL = %q, want %q", got.TestFlightURL, valid.TestFlightURL)
	}

	badHost := base
	badHost.TestFlightURL = "https://example.com/join/AbCd1234"
	if _, err := normalizeReleaseProfile(badHost); err == nil {
		t.Fatal("expected non-Apple TestFlight URL to fail")
	}

	wrongLane := base
	wrongLane.Lane = "ios-adhoc"
	wrongLane.TestFlightURL = valid.TestFlightURL
	if _, err := normalizeReleaseProfile(wrongLane); err == nil {
		t.Fatal("expected TestFlight URL on non-TestFlight lane to fail")
	}
}
