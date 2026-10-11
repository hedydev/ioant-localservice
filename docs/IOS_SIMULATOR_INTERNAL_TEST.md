# iOS Simulator Internal Test

## Purpose

ILS supports a package-only `ios-simulator` lane for fast iOS testing on the Mac-hosted Xcode Simulator.

This lane is deliberately separate from Ad Hoc and TestFlight. An Ad Hoc IPA is an `iphoneos` artifact for registered physical devices and is not installable into the iOS Simulator. Simulator builds target `iphonesimulator` and are packaged as a ZIP containing the generated `.app`.

## Contract

A Simulator Release Profile uses:

- `platform: ios`
- `lane: ios-simulator`
- `architecture: arm64`
- `result_contract: ils-result-v1`
- one project-owned `build_command`

The project script must write `$ILS_OUTPUT_DIR/ils-result.json` with:

```json
{
  "schema_version": 1,
  "lane": "ios-simulator",
  "status": "succeeded",
  "platform": "ios",
  "artifact": "/absolute/path/inside/ILS_OUTPUT_DIR/App-Simulator.zip",
  "version": "1.2.3",
  "build": "123",
  "architecture": "arm64",
  "bundle_id": "com.example.app",
  "distribution": "simulator"
}
```

ILS requires the artifact to be a non-empty ZIP inside that Build Job's output directory.

## Pipeline

```text
current local checkout
  -> project Simulator build script
  -> iphonesimulator arm64 .app
  -> ZIP under ILS_OUTPUT_DIR
  -> ils-result.json
  -> Build Job complete
```

There is no formal ILS Release for this lane. The successful Build Job itself is the source of truth and is exposed in Overview / Version History as a Simulator Internal Test record.

## Install and launch

For a successful Simulator Build Job, ILS exposes `POST /api/builds/{job}/simulator-install` to an authenticated administrator.

ILS:

1. verifies that an iOS Simulator device is already booted;
2. extracts the Build Job ZIP to a temporary directory;
3. requires exactly one `.app` bundle;
4. verifies its `CFBundleIdentifier` against `ils-result.json`;
5. installs with `xcrun simctl install booted`;
6. launches with `xcrun simctl launch booted`.

The web UI provides both **Download ZIP** and **Install and launch in Simulator** actions. Successful install/launch uses the normal top success notification.

## Explicit non-goals

The Simulator lane does not:

- use or transform an Ad Hoc IPA;
- require an Apple Team;
- sign with a Distribution or Development certificate;
- use a provisioning profile;
- register a physical device;
- create or update an App Store Connect App record;
- upload to TestFlight;
- create a formal ILS Release or public OTA artifact.

Ad Hoc and TestFlight remain independent physical-device / Apple distribution lanes.
