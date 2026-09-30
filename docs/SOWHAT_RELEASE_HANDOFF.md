# Sowhat → ILS release handoff

This document records the current local integration between Sowhat and ILS. It is machine-specific operational guidance, not the reusable release standard.

Reusable rules are owned by:

- Hero Skills `standards/apple-release-build-contract.md`
- Hero Skills `skills/publish-localservice-builds/SKILL.md`
- ILS `docs/RELEASE_PROFILES.md`

## Current integration baseline

| Item | Value |
| --- | --- |
| Sowhat checkout | `/Users/ted/Documents/workspace/ioant-sowhat` |
| ILS checkout | `/Users/ted/Documents/workspace/ioant-localservice` |
| ILS local origin | `http://127.0.0.1:8787` |
| ILS project ID | `sowhat` |
| Expected source branch | `main` |
| Sowhat standard-entrypoint commit | `aea8710f7aebab6fd81675f0c4a28e4e641f9d19` or newer |
| ILS result-contract commit | `02c75cbc3455f7494336da112f33a2919b1744b6` or newer |
| Bundle ID | `com.ioant.sowhat` |
| Known Apple Team on this Mac | `64RS366WKG` |
| ILS administrator token file | `/Users/ted/Documents/workspace/ioant-localservice/.localservice/admin-token` |

The token path may be used by ILS itself and legacy compatibility tooling. Never print or commit the token contents.

## Preferred Sowhat release entrypoints

Sowhat now implements the shared Apple release contract directly:

```text
scripts/ils-build-macos.sh
├─ --test
└─ --release

scripts/ils-build-ios.sh
├─ --adhoc
└─ --testflight
```

These scripts are build/package/submission-only. They do not:

- switch, stash, reset, clean, commit or push Git;
- install or launch Sowhat;
- read the ILS administrator token;
- call the ILS release API.

They write machine-readable results under the ILS task output directory.

## Build identity

The standard Sowhat wrappers use:

```text
version = MARKETING_VERSION, unless ILS_VERSION is supplied
build   = git rev-list --count HEAD, unless ILS_BUILD_NUMBER is supplied
```

The build number is injected through Xcode build settings. The release process does not edit `project.pbxproj` merely to increment a build number.

The actual signed/archive output is then checked for the requested Bundle ID, version and build.

## Recommended ILS Release Profiles

Create these profiles after the Sowhat project directory is linked to `main`.

### macOS test DMG

```json
{
  "id": "macos-test",
  "name": "macOS Test DMG",
  "platform": "macos",
  "architecture": "arm64",
  "channel": "dev",
  "variant": "desktop-dmg",
  "lane": "macos-test",
  "result_contract": "ils-result-v1",
  "build_command": "bash scripts/ils-build-macos.sh --test",
  "package_command": "",
  "artifact": "",
  "version_command": "",
  "build_number_command": "",
  "notes": "Sowhat Apple Silicon internal test DMG"
}
```

This lane creates a Developer-ID-signed arm64 DMG without notarization. Another Mac may require Finder **Open** or **Privacy & Security → Open Anyway**.

### macOS release DMG

```json
{
  "id": "macos-release",
  "name": "macOS Release DMG",
  "platform": "macos",
  "architecture": "arm64",
  "channel": "stable",
  "variant": "desktop-dmg",
  "lane": "macos-release",
  "result_contract": "ils-result-v1",
  "build_command": "bash scripts/ils-build-macos.sh --release",
  "package_command": "",
  "artifact": "",
  "version_command": "",
  "build_number_command": "",
  "notes": "Sowhat notarized Apple Silicon release DMG"
}
```

This lane additionally requires `SOWHAT_NOTARY_PROFILE` in the ILS service environment.

### iOS Ad Hoc

```json
{
  "id": "ios-adhoc",
  "name": "iOS Ad Hoc",
  "platform": "ios",
  "architecture": "arm64",
  "channel": "dev",
  "variant": "default",
  "lane": "ios-adhoc",
  "result_contract": "ils-result-v1",
  "build_command": "bash scripts/ils-build-ios.sh --adhoc",
  "package_command": "",
  "artifact": "",
  "version_command": "",
  "build_number_command": "",
  "notes": "Sowhat registered-device Ad Hoc IPA"
}
```

The script performs:

```text
Release archive
→ validate archive
→ release-testing export
→ validate IPA + embedded.mobileprovision + ProvisionedDevices
→ write ils-result.json
→ ILS validates again and publishes IPA
```

A published IPA can be downloaded over HTTP. Safari OTA installation additionally requires ILS to use an HTTPS public origin trusted by the iPhone/iPad, and the device UDID must be present in the Ad Hoc provisioning profile.

### iOS TestFlight

```json
{
  "id": "ios-testflight",
  "name": "iOS TestFlight",
  "platform": "ios",
  "architecture": "arm64",
  "channel": "beta",
  "variant": "default",
  "lane": "ios-testflight",
  "result_contract": "ils-result-v1",
  "build_command": "bash scripts/ils-build-ios.sh --testflight",
  "package_command": "",
  "artifact": "",
  "version_command": "",
  "build_number_command": "",
  "notes": "Sowhat TestFlight submission"
}
```

The script performs:

```text
Release archive
→ validate archive
→ app-store-connect upload
→ write submitted result
```

ILS stores the task, stage, live log and upload result. TestFlight has no fake local IPA release record.

An upload accepted by App Store Connect is reported as **submitted/upload succeeded**. It is not reported as **available** unless an Apple status source later proves processing has finished.

## App Store Connect authentication

The TestFlight entrypoint can use the Apple account already configured in Xcode.

For unattended API-key authentication, start ILS with these variables available to its process:

```text
ILS_ASC_KEY_PATH
ILS_ASC_KEY_ID
ILS_ASC_ISSUER_ID
```

The key file must stay outside Git. Do not log the private-key contents.

For signing-team selection, the standard iOS script accepts:

```text
ILS_APPLE_TEAM_ID
```

or the Sowhat-specific compatibility variable:

```text
SOWHAT_APPLE_TEAM_ID
```

If neither is supplied, the script only auto-selects when exactly one signing team can be derived from the local keychain.

## ILS progress contract

Sowhat standard scripts emit lines such as:

```text
ILS_EVENT {"stage":"archive","state":"started","message":"Creating signed iPhoneOS Release archive"}
ILS_EVENT {"stage":"upload","state":"started","message":"Uploading Sowhat archive to App Store Connect / TestFlight"}
ILS_EVENT {"stage":"submitted","state":"succeeded","message":"TestFlight upload submitted; Apple processing is not yet confirmed"}
```

ILS parses valid events into the build job while preserving normal stdout/stderr as the full log.

Numeric percentages are shown only when the underlying build/upload tool supplies trustworthy progress. No elapsed-time-based fake percentage is used.

## First validation sequence

Do not treat the existence of the profiles or scripts as release evidence. Validate the real Mac environment in this order:

1. Update/rebuild/restart ILS from its current main.
2. Update Sowhat main and confirm the checkout is clean and synchronized with its upstream.
3. Link the Sowhat checkout to ILS with expected branch `main`.
4. Save the `macos-test`, `ios-adhoc`, and `ios-testflight` profiles.
5. Run **macOS Test DMG** first and confirm ILS creates a real release record and downloadable DMG.
6. Run **iOS Ad Hoc** and confirm the real signed IPA appears as an ILS release.
7. Install/run on the intended registered iPhone/iPad as a separate user validation step.
8. Run **iOS TestFlight** only when an actual App Store Connect submission is intended. Confirm ILS shows upload feedback, then confirm processing/availability separately in App Store Connect.

A build/export/upload success is not physical-device runtime acceptance.
