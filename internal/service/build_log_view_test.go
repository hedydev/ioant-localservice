package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBuildLogTailKeepsNewestLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.log")
	var b strings.Builder
	for i := 1; i <= 450; i++ {
		fmt.Fprintf(&b, "line-%03d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}

	got, truncated, err := readBuildLogTail(path, 300)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("expected truncated tail")
	}
	if strings.Contains(got, "line-001") || strings.Contains(got, "line-100") {
		t.Fatalf("tail retained old lines: %q", got[:min(len(got), 120)])
	}
	if !strings.Contains(got, "line-151") || !strings.Contains(got, "line-450") {
		t.Fatal("tail does not contain expected newest lines")
	}
}

func TestReadBuildLogTailReturnsWholeSmallLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.log")
	want := "first\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(want), 0600); err != nil {
		t.Fatal(err)
	}
	got, truncated, err := readBuildLogTail(path, 300)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("small log should not be truncated")
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
