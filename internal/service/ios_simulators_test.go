package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIOSRuntimeLabel(t *testing.T) {
	if got := iosRuntimeLabel("com.apple.CoreSimulator.SimRuntime.iOS-26-1"); got != "iOS 26.1" {
		t.Fatalf("iosRuntimeLabel = %q, want %q", got, "iOS 26.1")
	}
}

func TestCompareRuntimeDesc(t *testing.T) {
	newer := "com.apple.CoreSimulator.SimRuntime.iOS-26-1"
	older := "com.apple.CoreSimulator.SimRuntime.iOS-18-6"
	if got := compareRuntimeDesc(newer, older); got >= 0 {
		t.Fatalf("compareRuntimeDesc(newer, older) = %d, want < 0", got)
	}
	if got := compareRuntimeDesc(older, newer); got <= 0 {
		t.Fatalf("compareRuntimeDesc(older, newer) = %d, want > 0", got)
	}
	if got := compareRuntimeDesc(newer, newer); got != 0 {
		t.Fatalf("compareRuntimeDesc(equal, equal) = %d, want 0", got)
	}
}

func TestSimulatorFrontendPathPrefersSimulatorApp(t *testing.T) {
	xcode := filepath.Join(t.TempDir(), "Xcode.app")
	developerDir := filepath.Join(xcode, "Contents", "Developer")
	simulatorPath := filepath.Join(developerDir, "Applications", "Simulator.app")
	deviceHubPath := filepath.Join(xcode, "Contents", "Applications", "DeviceHub.app")
	if err := os.MkdirAll(simulatorPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deviceHubPath, 0755); err != nil {
		t.Fatal(err)
	}
	kind, path := simulatorFrontendPath(developerDir)
	if kind != "simulator" || path != simulatorPath {
		t.Fatalf("simulatorFrontendPath = (%q, %q), want (%q, %q)", kind, path, "simulator", simulatorPath)
	}
}

func TestSimulatorFrontendPathUsesXcode27DeviceHub(t *testing.T) {
	xcode := filepath.Join(t.TempDir(), "Xcode.app")
	developerDir := filepath.Join(xcode, "Contents", "Developer")
	deviceHubPath := filepath.Join(xcode, "Contents", "Applications", "DeviceHub.app")
	if err := os.MkdirAll(deviceHubPath, 0755); err != nil {
		t.Fatal(err)
	}
	kind, path := simulatorFrontendPath(developerDir)
	if kind != "device-hub" || path != deviceHubPath {
		t.Fatalf("simulatorFrontendPath = (%q, %q), want (%q, %q)", kind, path, "device-hub", deviceHubPath)
	}
}

func TestSimulatorFrontendOpenArgs(t *testing.T) {
	const deviceID = "11111111-2222-3333-4444-555555555555"
	simulatorPath := "/Applications/Xcode.app/Contents/Developer/Applications/Simulator.app"
	if got, want := simulatorFrontendOpenArgs("simulator", simulatorPath, deviceID), []string{"-a", simulatorPath, "--args", "-CurrentDeviceUDID", deviceID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Simulator open args = %#v, want %#v", got, want)
	}
	deviceHubPath := "/Applications/Xcode.app/Contents/Applications/DeviceHub.app"
	if got, want := simulatorFrontendOpenArgs("device-hub", deviceHubPath, deviceID), []string{"-a", deviceHubPath, "devices://device/open?id=" + deviceID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Device Hub open args = %#v, want %#v", got, want)
	}
}
