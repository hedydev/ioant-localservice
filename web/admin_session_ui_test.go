package web

import (
	"strings"
	"testing"
)

func TestAdminSessionCacheSurvivesTransientOutage(t *testing.T) {
	raw, err := Files.ReadFile("js/core.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	for _, required := range []string{
		"ils.admin-token",
		"response.status===401||response.status===403",
		"ILS admin session restore deferred:",
		"writeCachedAdminToken(authenticated?token:'')",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("core.js missing admin-session guard %q", required)
		}
	}

	start := strings.Index(js, "export async function restoreAdminSession()")
	end := strings.Index(js[start:], "export function initCore()")
	if start < 0 || end < 0 {
		t.Fatal("restoreAdminSession function not found")
	}
	restore := js[start : start+end]
	catchIndex := strings.Index(restore, "catch(error)")
	if catchIndex < 0 {
		t.Fatal("restoreAdminSession catch block not found")
	}
	catchBlock := restore[catchIndex:]
	if strings.Contains(catchBlock, "setAdmin(false)") {
		t.Fatal("transient restore failure must not clear cached admin state")
	}
}
