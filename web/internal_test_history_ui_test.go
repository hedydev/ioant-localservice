package web

import (
	"strings"
	"testing"
)

func TestInternalTestHistoryAppearsInOverviewAndHistory(t *testing.T) {
	projectsRaw, err := Files.ReadFile("js/projects.js")
	if err != nil {
		t.Fatal(err)
	}
	projects := string(projectsRaw)
	for _, required := range []string{
		"/internal-test-records",
		"state.internalTests",
		"Promise.allSettled",
	} {
		if !strings.Contains(projects, required) {
			t.Fatalf("projects.js missing Internal Test history behavior %q", required)
		}
	}

	releasesRaw, err := Files.ReadFile("js/releases.js")
	if err != nil {
		t.Fatal(err)
	}
	releases := string(releasesRaw)
	for _, required := range []string{
		"...state.internalTests.map",
		"Internal Test",
		"data-history-internal-download",
		"历史记录",
	} {
		if !strings.Contains(releases, required) {
			t.Fatalf("releases.js missing Internal Test history UI %q", required)
		}
	}
}
