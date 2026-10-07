package web

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestJavaScriptSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; skipping browser-module syntax check")
	}

	paths, err := filepath.Glob("js/*.js")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "app.js")

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			cmd := exec.Command(node, "--check", path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s has invalid JavaScript syntax: %v\n%s", path, err, output)
			}
		})
	}
}
