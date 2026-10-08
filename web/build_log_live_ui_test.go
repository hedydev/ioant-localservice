package web

import (
	"strings"
	"testing"
)

func TestLiveBuildLogUsesLightweightTailAndDownload(t *testing.T) {
	appRaw, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appRaw), "./js/build-log-live.js") {
		t.Fatal("app bootstrap does not load live build log module")
	}

	jsRaw, err := Files.ReadFile("js/build-log-live.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsRaw)
	for _, required := range []string{
		"setInterval(()=>{ void refreshActiveLog(); },1000)",
		"/log?full=1",
		"下载日志",
		"仅显示最新 ",
		"不自动滚动",
		"new Blob([data.log||'']",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("build-log-live.js missing %q", required)
		}
	}
}
