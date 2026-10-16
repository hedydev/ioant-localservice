package web

import (
	"strings"
	"testing"
)

func TestDeletionControlsStayWired(t *testing.T) {
	buildRaw, err := Files.ReadFile("js/build-jobs.js")
	if err != nil {
		t.Fatal(err)
	}
	buildJS := string(buildRaw)
	if !strings.Contains(buildJS, "data-delete-build-job") {
		t.Fatal("build-jobs.js must render build deletion controls")
	}

	cascadeRaw, err := Files.ReadFile("js/build-deletion-ui.js")
	if err != nil {
		t.Fatal(err)
	}
	cascadeJS := string(cascadeRaw)
	for _, required := range []string{
		"data-delete-build-job",
		"method:'DELETE'",
		"deleted_release_ids",
		"版本历史",
		"OTA Gateway",
		"TestFlight",
		"data-refreshed",
		"build-record-deleted",
	} {
		if !strings.Contains(cascadeJS, required) {
			t.Fatalf("build-deletion-ui.js missing cascade contract %q", required)
		}
	}

	appRaw, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appRaw), "./js/build-deletion-ui.js") {
		t.Fatal("app.js must initialize build deletion cascade UI")
	}

	releasesRaw, err := Files.ReadFile("js/releases.js")
	if err != nil {
		t.Fatal(err)
	}
	releasesJS := string(releasesRaw)
	for _, required := range []string{
		"data-delete-history-build",
		"data-delete-release",
		"/api/releases/",
		"/api/builds/",
		"method:'DELETE'",
		"OTA Gateway",
	} {
		if !strings.Contains(releasesJS, required) {
			t.Fatalf("releases.js missing deletion contract %q", required)
		}
	}

	releaseUIRaw, err := Files.ReadFile("js/release-ui.js")
	if err != nil {
		t.Fatal(err)
	}
	releaseUI := string(releaseUIRaw)
	if !strings.Contains(releaseUI, "allowDelete&&release.delivery!=='testflight'") {
		t.Fatal("release-ui.js must not expose local artifact deletion for TestFlight")
	}
}