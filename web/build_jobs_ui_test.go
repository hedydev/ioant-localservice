package web

import (
	"strings"
	"testing"
)

func TestBuildJobsPollingUXContract(t *testing.T) {
	jsRaw, err := Files.ReadFile("js/build-jobs.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsRaw)
	for _, required := range []string{
		"container.insertBefore(article,currentAtIndex)",
		"window.scrollTo(pageX,pageY)",
		"text.startsWith(previous)",
		"buildState.followLog=false",
		"fill.animate(",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("build-jobs.js missing UX guard %q", required)
		}
	}
	if strings.Contains(js, "container.appendChild(article);") {
		t.Fatal("build polling still re-appends every existing job card")
	}

	cssRaw, err := Files.ReadFile("build-jobs.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssRaw)
	for _, required := range []string{"overflow-anchor: none", ".job-progress-track", ".job-progress-fill"} {
		if !strings.Contains(css, required) {
			t.Fatalf("build-jobs.css missing %q", required)
		}
	}
}
