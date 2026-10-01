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
