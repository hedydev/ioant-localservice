package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNodeBinaryAssignments(t *testing.T) {
	raw := strings.Join([]string{
		"# generated locally",
		"export NODE_BINARY=\"/Users/old/.nvm/versions/node/v20/bin/node\"",
		"NODE_BINARY=$(command -v node)",
	}, "\n")
	assignments := parseNodeBinaryAssignments(raw)
	if len(assignments) != 2 {
		t.Fatalf("assignments = %d, want 2", len(assignments))
	}
	if !assignments[0].Literal || assignments[0].Value != "/Users/old/.nvm/versions/node/v20/bin/node" {
		t.Fatalf("first assignment = %#v", assignments[0])
	}
	if assignments[1].Literal || assignments[1].Value != "" {
		t.Fatalf("dynamic assignment = %#v", assignments[1])
	}
}

func TestFindIOSXcodeEnvFilesSkipsGeneratedTrees(t *testing.T) {
	root := t.TempDir()
	wanted := filepath.Join(root, "apps", "mobile", "ios", ".xcode.env.local")
	skipped := filepath.Join(root, "node_modules", "pkg", "ios", ".xcode.env.local")
	for _, path := range []string{wanted, skipped} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("export NODE_BINARY=/missing/node\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := findIOSXcodeEnvFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != wanted {
		t.Fatalf("files = %#v, want [%q]", files, wanted)
	}
}

func TestRepairNodeBinaryAssignmentPreservesOtherLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".xcode.env.local")
	original := "# local only\nexport NODE_BINARY=\"/old/node\"\nOTHER=value\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	assignments, err := readNodeBinaryAssignments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 {
		t.Fatalf("assignments = %d, want 1", len(assignments))
	}
	if err := repairNodeBinaryAssignment(path, assignments[0].Line); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	want := "# local only\nexport NODE_BINARY=\"$(command -v node)\"\nOTHER=value\n"
	if got != want {
		t.Fatalf("repaired file = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestGitPathStateRecognizesIgnoredLocalEnv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init", "-q")
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, raw)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("**/.xcode.env.local\n"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "apps", "mobile", "ios", ".xcode.env.local")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("export NODE_BINARY=/missing/node\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tracked, ignored, err := gitPathState(context.Background(), root, path)
	if err != nil {
		t.Fatal(err)
	}
	if tracked || !ignored {
		t.Fatalf("tracked=%t ignored=%t, want false/true", tracked, ignored)
	}
}

func TestPreparePortableIOSNodeEnvironmentRepairsIgnoredLocalFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init", "-q")
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, raw)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("**/.xcode.env.local\n"), 0644); err != nil {
		t.Fatal(err)
	}
	localEnv := filepath.Join(root, "apps", "mobile", "ios", ".xcode.env.local")
	if err := os.MkdirAll(filepath.Dir(localEnv), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localEnv, []byte("export NODE_BINARY=\"/Users/old/.nvm/node\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(bin, "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\necho v99.1.0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	env, report, err := preparePortableIOSNodeEnvironment(
		context.Background(), root, "ios", "expo", &log, []string{"PATH=" + bin},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := envValue(env, "NODE_BINARY"); got != node {
		t.Fatalf("NODE_BINARY = %q, want %q", got, node)
	}
	if got := envValue(env, "ILS_NODE_BINARY"); got != node {
		t.Fatalf("ILS_NODE_BINARY = %q, want %q", got, node)
	}
	if report.NodeVersion != "v99.1.0" || len(report.Repaired) != 1 || report.Repaired[0] != "apps/mobile/ios/.xcode.env.local" {
		t.Fatalf("report = %#v", report)
	}
	raw, err := os.ReadFile(localEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `export NODE_BINARY="$(command -v node)"`) {
		t.Fatalf("local env was not repaired: %s", raw)
	}
	if !strings.Contains(log.String(), "Auto-repaired ignored local Xcode Node env") {
		t.Fatalf("repair evidence missing from log: %s", log.String())
	}
}

func TestReplaceEnvValueUsesSingleEffectiveValue(t *testing.T) {
	env := []string{"PATH=/bin", "NODE_BINARY=/old", "A=1", "NODE_BINARY=/older"}
	env = replaceEnvValue(env, "NODE_BINARY", "/new")
	if got := envValue(env, "NODE_BINARY"); got != "/new" {
		t.Fatalf("NODE_BINARY = %q, want /new", got)
	}
	count := 0
	for _, item := range env {
		if strings.HasPrefix(item, "NODE_BINARY=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("NODE_BINARY entries = %d, want 1", count)
	}
}

func TestPortableIOSNodePreflightEnabled(t *testing.T) {
	if !portableIOSNodePreflightEnabled("ios", "expo") {
		t.Fatal("ios + expo should enable preflight")
	}
	if portableIOSNodePreflightEnabled("macos", "expo") || portableIOSNodePreflightEnabled("ios", "native") {
		t.Fatal("preflight enabled for unrelated platform/build type")
	}
}
