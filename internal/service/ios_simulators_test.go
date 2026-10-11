package service

import "testing"

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
