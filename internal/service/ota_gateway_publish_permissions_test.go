package service

import (
	"strings"
	"testing"
)

func TestOTAFinalizeRemoteCommandMakesPublishedFilesReadable(t *testing.T) {
	stage := "/srv/ils-adhoc-ota/releases/demo/release.tmp-abcd"
	remote := "/srv/ils-adhoc-ota/releases/demo/release"
	got := otaFinalizeRemoteCommand(stage, remote)

	permissionStep := "chmod 0644 -- " + stage + "/app.ipa " + stage + "/manifest.plist"
	if !strings.HasPrefix(got, permissionStep+" && ") {
		t.Fatalf("finalize command must publish readable files before promotion: %q", got)
	}
	if !strings.Contains(got, "rm -rf -- "+remote+" && mv -- "+stage+" "+remote) {
		t.Fatalf("finalize command lost atomic release promotion: %q", got)
	}
}
