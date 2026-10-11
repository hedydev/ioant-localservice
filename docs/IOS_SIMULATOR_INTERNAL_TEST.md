# iOS Simulator Internal Test

## Purpose

ILS supports a package-only `ios-simulator` lane for fast iOS testing on the Mac-hosted Xcode Simulator.

This lane is deliberately separate from Ad Hoc and TestFlight. An Ad Hoc IPA is an `iphoneos` artifact for registered physical devices and is not installable into the iOS Simulator. Simulator builds target `iphonesimulator` and are packaged as a ZIP containing the generated `.app`.

Every project manifest that declares an iOS client must declare exactly one `ios-simulator` Profile. The ILS importer enforces this so new iOS clients cannot accidentally omit the local Simulator test path.

## Profile contract

A Simulator Release Profile uses:

- `platform: ios`
- `lane: ios-simulator`
- `architecture: arm64`
- `result_contract: ils-result-v1`
- one project-owned `build_command`
- no Apple Team requirement

The selected Simulator device is intentionally **not** stored in the project repository. It is machine-local ILS state because Simulator UDIDs belong to one Mac/Xcode installation.

In the Profile editor, ILS reads `xcrun simctl list devices available -j` and exposes the installed iOS Simulator devices through a dropdown. A Profile may either keep automatic selection or remember one preferred local device.

At build time ILS resolves the device and passes these environment values to the project script:

```text
ILS_SIMULATOR_UDID
ILS_SIMULATOR_NAME
ILS_SIMULATOR_RUNTIME
```

Projects should use `ILS_SIMULATOR_UDID` for the Xcode destination when present, for example:

```text
platform=iOS Simulator,id=<ILS_SIMULATOR_UDID>
```

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
  -> selected Simulator Profile
  -> selected local Simulator device
  -> project Simulator build script
  -> iphonesimulator arm64 .app
  -> ZIP under ILS_OUTPUT_DIR
  -> ils-result.json
  -> Build Job complete
```

There is no formal ILS Release for this lane. The successful Build Job itself is the source of truth and is exposed in both Overview and Version History as a Simulator Internal Test record.

## Device selection and build history

ILS stores two separate local selections:

1. **Profile preferred device** — the default Simulator used when starting a new build.
2. **Build Job run device** — the Simulator most recently used to run that completed artifact.

The Build Job records the device selected when the build starts. This gives the build log a concrete target while keeping the resulting arm64 `iphonesimulator` package reusable on another compatible local Simulator.

After a build succeeds, changing the Simulator dropdown does **not** rebuild the app. The same ZIP can be installed and launched on another available Simulator.

## Install and launch

For a successful Simulator Build Job, ILS exposes:

```text
POST /api/builds/{job}/simulator-install
```

Optional request body:

```json
{
  "simulator_device_id": "SIMULATOR-UDID"
}
```

ILS then:

1. resolves the requested local Simulator by exact UDID;
2. boots it when necessary;
3. waits for `simctl bootstatus` to report it ready;
4. extracts the Build Job ZIP to a temporary directory;
5. requires exactly one `.app` bundle;
6. verifies its `CFBundleIdentifier` against `ils-result.json`;
7. installs with `xcrun simctl install <udid> <app>`;
8. launches with `xcrun simctl launch <udid> <bundle-id>`;
9. remembers that device as the Build Job's current run target.

The Build Jobs page, Overview, and Version History all expose the same controls:

```text
[Download ZIP]  [Simulator device ▼]  [Run]
```

A user can therefore build once, choose another Simulator later, and run the same build without recompiling.

## Local APIs

Authenticated administrator APIs used by the UI:

```text
GET  /api/local/ios-simulators
GET  /api/projects/{project}/simulator-profile-devices
POST /api/projects/{project}/release-profiles/{profile}/simulator-device
GET  /api/projects/{project}/simulator-build-devices
POST /api/builds/{job}/simulator-install
```

## Explicit non-goals

The Simulator lane does not:

- use or transform an Ad Hoc IPA;
- require an Apple Team;
- sign with a Distribution or Development certificate;
- use a provisioning profile;
- register a physical device;
- create or update an App Store Connect App record;
- upload to TestFlight;
- create a formal ILS Release or public OTA artifact;
- store Mac-specific Simulator UDIDs in the project repository.

Ad Hoc and TestFlight remain independent physical-device / Apple distribution lanes.
