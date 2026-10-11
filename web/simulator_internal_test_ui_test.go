package web

import (
	"strings"
	"testing"
)

func TestSimulatorInternalTestUIContract(t *testing.T) {
	profilesRaw, err := Files.ReadFile("js/build-profiles.js")
	if err != nil {
		t.Fatal(err)
	}
	profiles := string(profilesRaw)
	for _, required := range []string{
		"ios-simulator",
		"iOS · Simulator Internal Test",
		"Simulator Internal Test",
		"不需要 Apple Team",
		"profile.lane!=='ios-simulator'",
	} {
		if !strings.Contains(profiles, required) {
			t.Fatalf("build-profiles.js missing Simulator contract %q", required)
		}
	}

	actionsRaw, err := Files.ReadFile("js/build-actions.js")
	if err != nil {
		t.Fatal(err)
	}
	actions := string(actionsRaw)
	for _, required := range []string{
		"profile.platform==='ios'&&profile.lane==='ios-simulator'",
		"/internal-builds",
		"不会创建 Release",
	} {
		if !strings.Contains(actions, required) {
			t.Fatalf("build-actions.js missing Simulator package-only routing %q", required)
		}
	}

	jobsRaw, err := Files.ReadFile("js/build-jobs.js")
	if err != nil {
		t.Fatal(err)
	}
	jobs := string(jobsRaw)
	for _, required := range []string{
		"data-install-simulator",
		"/simulator-install",
		"'ios-simulator':['preflight','build','package','complete']",
		"Simulator 测试包已生成",
	} {
		if !strings.Contains(jobs, required) {
			t.Fatalf("build-jobs.js missing Simulator build action %q", required)
		}
	}

	releasesRaw, err := Files.ReadFile("js/releases.js")
	if err != nil {
		t.Fatal(err)
	}
	releases := string(releasesRaw)
	for _, required := range []string{
		"data-history-simulator-install",
		"iOS Simulator",
		"record.lane==='ios-simulator'",
		"/simulator-install",
	} {
		if !strings.Contains(releases, required) {
			t.Fatalf("releases.js missing Simulator history action %q", required)
		}
	}
}
