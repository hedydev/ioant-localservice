package web

import (
	"strings"
	"testing"
)

func TestDeletionUsesModernConfirmationDialogAndSuccessNotice(t *testing.T) {
	coreRaw, err := Files.ReadFile("js/core.js")
	if err != nil {
		t.Fatal(err)
	}
	core := string(coreRaw)
	for _, required := range []string{
		"export function confirmAction",
		"action-confirm-dialog",
		"/confirm-dialog.css",
	} {
		if !strings.Contains(core, required) {
			t.Fatalf("core.js missing confirmation UI contract %q", required)
		}
	}

	cssRaw, err := Files.ReadFile("confirm-dialog.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssRaw)
	for _, required := range []string{
		"dialog.action-confirm-dialog",
		"backdrop-filter",
		"action-confirm-primary",
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("confirm-dialog.css missing modern dialog style %q", required)
		}
	}

	for _, path := range []string{"js/build-jobs.js", "js/releases.js"} {
		raw, err := Files.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		js := string(raw)
		if !strings.Contains(js, "confirmAction(") {
			t.Fatalf("%s must use shared modern confirmation dialog", path)
		}
		if strings.Contains(js, "confirm(") {
			t.Fatalf("%s must not use the browser confirm dialog", path)
		}
		if !strings.Contains(js, "'success'") {
			t.Fatalf("%s must show a success notice after deletion", path)
		}
	}
}
