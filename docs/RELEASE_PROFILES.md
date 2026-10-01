# ILS Release Profiles

ILS Release Profiles let ILS orchestrate builds from an existing local Git checkout while keeping machine-specific configuration in ILS rather than the business repository.

## Preferred Apple build contract

New Apple projects should expose package/submission-only entrypoints:

```text
scripts/ils-build-macos.sh
scripts/ils-build-ios.sh
```

The preferred Release Profile uses:

```json
{
  "result_contract": "ils-result-v1"
}
```

The project script writes its machine-readable outcome to:

```text
$ILS_OUTPUT_DIR/ils-result.json
```

ILS owns Git readiness, task state, local artifact publication, logs and TestFlight submission feedback. The project script owns compile/sign/archive/export/package validation. It must not switch/stash/reset/clean/commit/push Git, install or launch the app, read the ILS administrator token, or call the ILS publication API.

The reusable cross-project rules live in Hero Skills `standards/apple-release-build-contract.md`.

## Ownership

Profiles are stored under:

```text
.localservice/release-profiles/<project>/<profile>.json
```

Creating, editing, running or deleting a profile does not write configuration into the linked business repository.

## Required project state

Before every build ILS verifies:

- the linked directory is a Git worktree;
- the current branch matches the configured branch;
- the worktree is clean;
- an upstream exists.

ILS then runs `git pull --ff-only` with Git hooks and autostash disabled. It does not switch branches, stash, reset, clean, rebase, or resolve conflicts.

## Profile fields

| Field | Meaning |
| --- | --- |
| `id` | Stable lowercase profile ID |
| `name` | Display name |
| `platform` | `ios` or `macos` |
| `architecture` | `arm64`, `x86_64`, or `universal`; iOS must be arm64 |
| `channel` | `dev`, `beta`, or `stable` |
| `variant` | Stable package identity |
| `lane` | `ios-adhoc`, `ios-testflight`, `macos-test`, or `macos-release` |
| `result_contract` | Preferred: `ils-result-v1`; empty keeps legacy artifact-field behavior |
| `apple_team_id` | Optional 10-character Apple Team ID. ILS exposes detected local signing teams in the Profile UI and exports the selected value as `ILS_APPLE_TEAM_ID`. Required by the UI when multiple signing teams are detected for an iOS profile. |
| `testflight_url` | Optional Apple TestFlight invitation URL. Valid only for `ios-testflight` and must use `https://testflight.apple.com/join/...`. The value is snapshotted into the build job for the **在 TestFlight 中打开** action. |
| `build_command` | Shell command executed from the linked project directory |
| `package_command` | Optional second command |
| `artifact` | Legacy mode only: relative artifact path/glob or path under `$ILS_OUTPUT_DIR` |
| `version_command` | Legacy macOS mode only |
| `build_number_command` | Legacy macOS mode only |
| `notes` | Default ILS release notes |

TestFlight profiles must use `ils-result-v1`.

## Standard execution environment

ILS exports the existing compatibility variables plus these preferred values:

```text
ILS_URL
ILS_TOKEN_FILE
ILS_ROOT
ILS_PROJECT_ID
ILS_JOB_ID
ILS_OUTPUT_DIR
ILS_GIT_COMMIT
ILS_PLATFORM
ILS_CHANNEL
ILS_ARCHITECTURE
ILS_VARIANT
ILS_LANE
ILS_APPLE_TEAM_ID   # only when selected in the Release Profile
```

The standard project script must place final release evidence beneath `ILS_OUTPUT_DIR`. Large compiler/package caches may remain in the project's configured development cache.

## Structured task progress

Project scripts may emit a line beginning with `ILS_EVENT ` followed by one JSON object:

```text
ILS_EVENT {"stage":"archive","state":"started","message":"Creating Release archive"}
ILS_EVENT {"stage":"upload","state":"started","message":"Uploading to App Store Connect"}
ILS_EVENT {"stage":"upload","state":"succeeded"}
ILS_EVENT {"stage":"submitted","state":"succeeded","message":"Upload accepted"}
```

ILS stores the latest valid stage/state/message on the build job and the management page refreshes it with the live log.

A numeric `progress` field from 0 to 100 is accepted only when the project tool has real progress evidence. Project scripts must not fabricate percentages from elapsed time.

## ils-result-v1: local artifact

Example Ad Hoc result:

```json
{
  "schema_version": 1,
  "lane": "ios-adhoc",
  "status": "succeeded",
  "platform": "ios",
  "artifact": "/absolute/path/under/ILS_OUTPUT_DIR/App.ipa",
  "version": "1.2.0",
  "build": "123",
  "architecture": "arm64",
  "bundle_id": "com.example.app",
  "distribution": "release-testing",
  "sha256": "..."
}
```

ILS verifies:

- profile platform/architecture/lane matches the result;
- version is valid SemVer and build is a positive integer;
- artifact resolves to a regular file **inside** `ILS_OUTPUT_DIR`;
- extension matches the platform;
- provided SHA-256 matches the file;
- for iOS, IPA version/build/Bundle ID match the result.

After validation ILS invokes its own immutable release publication path. The project script does not call `push.sh`.

## ils-result-v1: TestFlight

Example:

```json
{
  "schema_version": 1,
  "lane": "ios-testflight",
  "status": "submitted",
  "platform": "ios",
  "version": "1.2.0",
  "build": "123",
  "architecture": "arm64",
  "bundle_id": "com.example.app",
  "distribution": "app-store-connect",
  "archive": "/absolute/job/output/App.xcarchive",
  "submission_result": "upload-succeeded"
}
```

For this lane ILS requires an explicit successful App Store Connect submission result and **does not create a fake local release record**.

The job UI reports the upload as **submitted to App Store Connect**. This is intentionally different from “TestFlight available”: App Store Connect may still be processing the build. The task pipeline leaves **Apple Processing** and **TestFlight available** unresolved until a future/connected Apple status source provides evidence.

## Recommended profiles

iOS Ad Hoc:

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
  "build_command": "bash scripts/ils-build-ios.sh --adhoc"
}
```

iOS TestFlight:

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
  "apple_team_id": "YOURTEAMID",
  "build_command": "bash scripts/ils-build-ios.sh --testflight"
}
```

macOS internal distribution:

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
  "build_command": "bash scripts/ils-build-macos.sh --test"
}
```

macOS public direct distribution:

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
  "build_command": "bash scripts/ils-build-macos.sh --release"
}
```

## Legacy compatibility

Profiles with empty `result_contract` continue using the older `artifact` field and, for macOS, `version_command` / `build_number_command`.

Project-owned `release*.sh` scripts that self-publish also remain available as a compatibility runner. New Apple integrations should prefer Release Profiles plus `ils-result-v1`.
