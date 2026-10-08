package service

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestWriteRollingBuildLogKeepsNewestOutput(t *testing.T) {
	path := t.TempDir() + "/build.log"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	written := 0
	writeRollingBuildLog(f, &written, "EARLIEST-LINE\n")
	writeRollingBuildLog(f, &written, strings.Repeat("compile output that is intentionally noisy\n", 70000))
	writeRollingBuildLog(f, &written, "LATEST-ERROR: missing module Foo\n")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if len(data) > buildLogLimit {
		t.Fatalf("rolling log size = %d, want <= %d", len(data), buildLogLimit)
	}
	if strings.Contains(text, "EARLIEST-LINE") {
		t.Fatal("oldest output should have been evicted")
	}
	if !strings.Contains(text, buildLogTrimMarker) {
		t.Fatal("rolling log should explain that older output was omitted")
	}
	if !strings.Contains(text, "LATEST-ERROR: missing module Foo") {
		t.Fatal("newest output must be retained")
	}
}

func TestConcreteBuildFailurePrefersCompilerError(t *testing.T) {
	path := t.TempDir() + "/build.log"
	log := strings.Join([]string{
		"CompileC something",
		"Sources/App.swift:42:17: error: cannot find 'missingSymbol' in scope",
		"** ARCHIVE FAILED **",
		"IWB iOS release failed: see build log",
	}, "\n")
	if err := os.WriteFile(path, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	got := concreteBuildFailure(path)
	if !strings.Contains(got, "cannot find 'missingSymbol' in scope") {
		t.Fatalf("failure summary = %q", got)
	}
	message := buildFailureMessage(path, nil, context.Background(), "ILS Build Command")
	if !strings.Contains(message, "cannot find 'missingSymbol' in scope") {
		t.Fatalf("failure message = %q", message)
	}
}
