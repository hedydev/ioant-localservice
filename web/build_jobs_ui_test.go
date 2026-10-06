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
		"const scrollTop=element.scrollTop",
		"element.scrollTop=Math.min(scrollTop,maxScroll)",
		"buildState.followLog=false",
		"pipeline-flow",
		"fill.animate(",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("build-jobs.js missing UX guard %q", required)
		}
	}
	for _, forbidden := range []string{
		"container.appendChild(article);",
		"element.scrollTop=element.scrollHeight",
		"buildState.followLog=true",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("build polling contains forbidden auto-scroll/reappend behavior %q", forbidden)
		}
	}

	cssRaw, err := Files.ReadFile("build-jobs.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssRaw)
	for _, required := range []string{
		"overflow-anchor: none",
		".job-progress-track",
		".job-progress-shine",
		"ils-build-progress-indeterminate",
		"ils-build-stage-pulse",
		"ils-build-running-edge",
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("build-jobs.css missing %q", required)
		}
	}
}
