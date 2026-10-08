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
	for _, required := range []string{
		"data-delete-build-job",
		"method:'DELETE'",
		"正式 Release / 已发布安装包不会被删除",
		"build-record-deleted",
	} {
		if !strings.Contains(buildJS, required) {
			t.Fatalf("build-jobs.js missing deletion contract %q", required)
		}
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
