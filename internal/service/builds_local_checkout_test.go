package service

import "testing"

func TestNormalizeBuildType(t *testing.T) {
	tests := map[string]string{
		"":                  "native",
		"native":            "native",
		" Expo ":            "expo",
		"hybrid-web-native": "hybrid-web-native",
		"TAURI":             "tauri",
	}
	for input, want := range tests {
		got, err := normalizeBuildType(input)
		if err != nil {
			t.Fatalf("normalizeBuildType(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeBuildType(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeBuildType("flutter"); err == nil {
		t.Fatal("unsupported build type should fail")
	}
}

func TestSnapshotFromSourcePreservesLocalGitProvenance(t *testing.T) {
	info := SourceInfo{
		CurrentBranch: "feature/local-release",
		Head:          "0123456789abcdef0123456789abcdef01234567",
		Upstream:      "origin/feature/local-release",
		Remote:        "git@github.com:hedydev/example.git",
		Dirty:         true,
	}
	got := snapshotFromSource(info, "expo")
	if got.Branch != info.CurrentBranch || got.Head != info.Head || got.Upstream != info.Upstream || got.Remote != info.Remote {
		t.Fatalf("snapshot lost Git provenance: %#v", got)
	}
	if !got.Dirty {
		t.Fatal("snapshot should preserve dirty=true")
	}
	if got.BuildType != "expo" {
		t.Fatalf("build type = %q, want expo", got.BuildType)
	}
}

func TestSnapshotFromSourceAllowsDetachedHead(t *testing.T) {
	got := snapshotFromSource(SourceInfo{Head: "abc123"}, "native")
	if got.Branch != "detached" {
		t.Fatalf("branch = %q, want detached", got.Branch)
	}
	if got.Head != "abc123" {
		t.Fatalf("head = %q, want abc123", got.Head)
	}
}
