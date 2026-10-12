package service

import "testing"

func TestProfilesFromProjectManifest(t *testing.T) {
	raw := []byte(`{
  "schema_version": 1,
  "id": "netscale",
  "profiles": [
    {
      "id": "macos-test",
      "name": "NetScale macOS Internal Test",
      "platform": "macos",
      "architecture": "arm64",
      "channel": "dev",
      "variant": "default",
      "lane": "macos-test",
      "result_contract": "ils-result-v1",
      "build_type": "hybrid-web-native",
      "build_command": "bash scripts/ils-build-macos.sh",
      "requires_apple_team": true
    },
    {
      "id": "ios-simulator",
      "name": "NetScale iOS Simulator",
      "platform": "ios",
      "architecture": "arm64",
      "channel": "dev",
      "variant": "default",
      "lane": "ios-simulator",
      "result_contract": "ils-result-v1",
      "build_type": "hybrid-web-native",
      "build_command": "bash scripts/ils-build-ios-simulator.sh"
    },
    {
      "id": "ios-adhoc",
      "name": "NetScale iOS Ad Hoc",
      "platform": "ios",
      "architecture": "arm64",
      "channel": "stable",
      "variant": "default",
      "lane": "ios-adhoc",
      "result_contract": "ils-result-v1",
      "build_type": "hybrid-web-native",
      "build_command": "bash scripts/ils-build-ios.sh --adhoc",
      "requires_apple_team": true
    },
    {
      "id": "ios-testflight",
      "name": "NetScale iOS TestFlight",
      "platform": "ios",
      "architecture": "arm64",
      "channel": "stable",
      "variant": "default",
      "lane": "ios-testflight",
      "result_contract": "ils-result-v1",
      "build_type": "hybrid-web-native",
      "build_command": "bash scripts/ils-build-ios.sh --testflight",
      "requires_apple_team": true
    }
  ]
}`)

	profiles, err := profilesFromProjectManifest(raw, "netscale")
	if err != nil {
		t.Fatalf("profilesFromProjectManifest: %v", err)
	}
	if len(profiles) != 4 {
		t.Fatalf("profiles = %d, want 4", len(profiles))
	}
	want := map[string]string{
		"macos-test":    "macos-test",
		"ios-simulator": "ios-simulator",
		"ios-adhoc":     "ios-adhoc",
		"ios-testflight": "ios-testflight",
	}
	for _, profile := range profiles {
		if want[profile.ID] != profile.Lane {
			t.Fatalf("profile %s lane = %q, want %q", profile.ID, profile.Lane, want[profile.ID])
		}
		if profile.BuildType != "hybrid-web-native" {
			t.Fatalf("profile %s build type = %q", profile.ID, profile.BuildType)
		}
	}

	simulator, err := simulatorProfileFromProjectManifest(raw, "netscale")
	if err != nil {
		t.Fatalf("simulatorProfileFromProjectManifest: %v", err)
	}
	if simulator == nil || simulator.ID != "ios-simulator" || simulator.Lane != "ios-simulator" {
		t.Fatalf("unexpected Simulator profile: %+v", simulator)
	}
	if simulator.AppleTeamID != "" {
		t.Fatalf("Simulator profile unexpectedly has Apple Team %q", simulator.AppleTeamID)
	}
}

func TestProfilesFromProjectManifestRejectsWrongProject(t *testing.T) {
	raw := []byte(`{"schema_version":1,"id":"other","profiles":[]}`)
	if _, err := profilesFromProjectManifest(raw, "netscale"); err == nil {
		t.Fatal("expected project id mismatch")
	}
}

func TestProfilesFromProjectManifestRejectsDuplicateIDs(t *testing.T) {
	raw := []byte(`{
  "schema_version": 1,
  "id": "netscale",
  "profiles": [
    {"id":"same","name":"A","platform":"macos","architecture":"arm64","channel":"dev","variant":"default","lane":"macos-test","result_contract":"ils-result-v1","build_command":"echo a"},
    {"id":"same","name":"B","platform":"ios","architecture":"arm64","channel":"stable","variant":"default","lane":"ios-adhoc","result_contract":"ils-result-v1","build_command":"echo b"}
  ]
}`)
	if _, err := profilesFromProjectManifest(raw, "netscale"); err == nil {
		t.Fatal("expected duplicate profile id error")
	}
}

func TestProfilesFromProjectManifestRejectsDuplicateSimulators(t *testing.T) {
	raw := []byte(`{
  "schema_version": 1,
  "id": "netscale",
  "profiles": [
    {"id":"sim-a","name":"A","platform":"ios","architecture":"arm64","channel":"dev","variant":"default","lane":"ios-simulator","result_contract":"ils-result-v1","build_command":"echo a"},
    {"id":"sim-b","name":"B","platform":"ios","architecture":"arm64","channel":"dev","variant":"default","lane":"ios-simulator","result_contract":"ils-result-v1","build_command":"echo b"}
  ]
}`)
	if _, err := profilesFromProjectManifest(raw, "netscale"); err == nil {
		t.Fatal("expected duplicate Simulator profile error")
	}
}
