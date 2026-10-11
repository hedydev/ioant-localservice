package service

import "testing"

func TestSimulatorProfileFromProjectManifest(t *testing.T) {
	raw := []byte(`{
  "schema_version": 1,
  "id": "sowhat",
  "profiles": [
    {
      "id": "ios-testflight",
      "name": "Sowhat TestFlight",
      "platform": "ios",
      "architecture": "arm64",
      "channel": "stable",
      "variant": "default",
      "lane": "ios-testflight",
      "result_contract": "ils-result-v1",
      "build_type": "native",
      "build_command": "bash scripts/ils-build-ios.sh --testflight",
      "requires_apple_team": true
    },
    {
      "id": "ios-simulator",
      "name": "Sowhat · iOS Simulator Internal Test",
      "platform": "ios",
      "architecture": "arm64",
      "channel": "dev",
      "variant": "default",
      "lane": "ios-simulator",
      "result_contract": "ils-result-v1",
      "build_type": "native",
      "build_command": "bash scripts/ils-build-ios-simulator.sh"
    }
  ]
}`)

	profile, err := simulatorProfileFromProjectManifest(raw, "sowhat")
	if err != nil {
		t.Fatalf("simulatorProfileFromProjectManifest: %v", err)
	}
	if profile == nil {
		t.Fatal("expected Simulator profile")
	}
	if profile.ID != "ios-simulator" || profile.Lane != "ios-simulator" || profile.Platform != "ios" {
		t.Fatalf("unexpected Simulator profile: %+v", profile)
	}
	if profile.BuildType != "native" {
		t.Fatalf("build type = %q, want native", profile.BuildType)
	}
	if profile.AppleTeamID != "" {
		t.Fatalf("Simulator profile unexpectedly has Apple Team %q", profile.AppleTeamID)
	}
}

func TestSimulatorProfileFromProjectManifestRejectsWrongProject(t *testing.T) {
	raw := []byte(`{"schema_version":1,"id":"other","profiles":[]}`)
	if _, err := simulatorProfileFromProjectManifest(raw, "sowhat"); err == nil {
		t.Fatal("expected project id mismatch")
	}
}

func TestSimulatorProfileFromProjectManifestRejectsDuplicates(t *testing.T) {
	raw := []byte(`{
  "schema_version": 1,
  "id": "sowhat",
  "profiles": [
    {"id":"sim-a","name":"A","platform":"ios","architecture":"arm64","channel":"dev","variant":"default","lane":"ios-simulator","result_contract":"ils-result-v1","build_command":"echo a"},
    {"id":"sim-b","name":"B","platform":"ios","architecture":"arm64","channel":"dev","variant":"default","lane":"ios-simulator","result_contract":"ils-result-v1","build_command":"echo b"}
  ]
}`)
	if _, err := simulatorProfileFromProjectManifest(raw, "sowhat"); err == nil {
		t.Fatal("expected duplicate Simulator profile error")
	}
}
