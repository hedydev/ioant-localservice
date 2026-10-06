# ILS Release System Status

Updated: 2026-10-07

This document is the current operational snapshot for ILS release/distribution work. It is intentionally concrete: it records what is implemented, what has been validated, and the behavior that must not regress.

## 1. Global architecture

ILS is a local Mac release/distribution service. Project release pages are project-scoped; Device Registry and OTA Gateway are ILS-global administrator capabilities.

Global services:

- Device Registry
- Public Device Enrollment
- Apple Developer device registration
- OTA Gateway configuration and health
- Ad Hoc artifact sync

Project services:

- Release Profiles
- build jobs / logs
- Release history
- iOS Ad Hoc release
- TestFlight release
- macOS packages

The Web UI therefore exposes `设备与 OTA` as a global admin page rather than nesting it under a project.

## 2. Administrator session

Administrator verification is intentionally lightweight for local use.

- A successful admin token is cached in browser `localStorage` under the ILS origin.
- Refresh/reopen restores the session silently and validates the cached token before project loading starts.
- `退出管理` explicitly clears the cached session.
- An explicit `401` or `403` clears an invalid cached session.
- A transient network error, service restart, temporary 5xx, or failed restore request does **not** erase the browser token.
- Global admin-only navigation is hidden when no valid admin session exists.

Do not reintroduce concurrent project loading before admin-session restore; that previously caused an auth-epoch race and a ~20 second wait for the next project polling cycle.

Regression coverage: `web/admin_session_ui_test.go`.

## 3. TestFlight state model

TestFlight is represented as a normal ILS Release with `delivery=testflight`.

Unified Release states remain:

- `submitted`
- `processing`
- `available`
- `unavailable`

Important verified semantics:

- upload success does not mean `available`;
- Internal Ready + External Ready to Submit remains non-available/processing;
- External Approved is surfaced separately from upload/internal state;
- the Apple Public Link is the real `open_url` when App Store Connect exposes it;
- the UI must not confuse the public-link source label with a second button.

See `TESTFLIGHT_STATE_MODEL.md` for the state contract.

## 4. Device enrollment and Apple registration

Public enrollment flow:

1. iPhone/iPad opens `https://ota.ioant.com/enroll`.
2. The profile-service payload requests UDID, PRODUCT and VERSION.
3. The device posts a CMS-signed response to the OTA Gateway callback.
4. Gateway stores the pending device.
5. ILS syncs it into the global Device Registry.
6. Admin registers the device with Apple through the App Store Connect Devices API.
7. The local device record tracks Apple registration per selected signing Team.

Device Registry and OTA Gateway are global, not project-specific.

The currently exercised device flow reached `apple_registered` / `ENABLED` for Team `64RS366WKG`.

Signed Enrollment Profile and final Ad Hoc OTA installation are considered operational by the user at this checkpoint; reopen them only if a real-device failure is reported.

## 5. OTA Gateway

Public endpoint: `https://ota.ioant.com`

EC2: `ubuntu@52.77.167.119`

Static root: `/srv/ils-adhoc-ota`

Private gateway: `127.0.0.1:8790`

Core routes:

- `GET /_ils/health`
- `GET /enroll`
- `GET /enroll.mobileconfig`
- `POST /device/callback/{challenge}`
- `GET /api/ils/devices/pending`
- `POST /api/ils/devices/{id}/ack`
- `/releases/...` static Ad Hoc artifacts

Local ILS configuration lives in `.localservice/ota-gateway.json`; sync token stays private in `.localservice/ota-gateway-sync-token`.

## 6. Local-checkout build model

ILS now builds exactly the local checkout the developer has prepared.

The release runner deliberately does **not** require:

- `main` branch;
- a clean working tree;
- local HEAD to match an upstream branch;
- an upstream branch to exist;
- local and remote content to be synchronized.

The runner also never performs release-time Git mutation or synchronization:

- no `git pull`;
- no branch checkout/switch;
- no stash;
- no reset;
- no clean.

Any current branch can be built. A detached HEAD is also buildable as long as Git has a concrete HEAD commit to record. Dirty tracked/untracked files are allowed because the desired artifact is the current local working tree, not an asserted copy of a remote commit.

Every new build snapshots provenance to `.localservice/builds/<job>/source.json` and exposes it through the build-job API/UI:

- current branch (or `detached`);
- HEAD commit;
- dirty flag;
- upstream, when present;
- remote, when present;
- declared build type.

The same values are passed to the project script through:

- `ILS_GIT_COMMIT`
- `ILS_GIT_BRANCH`
- `ILS_GIT_DIRTY`
- `ILS_GIT_UPSTREAM`
- `ILS_GIT_REMOTE`
- `ILS_BUILD_TYPE`

A build can therefore be reproduced or at least identified accurately even when it contains local uncommitted work.

Regression coverage: `internal/service/builds_local_checkout_test.go`.

## 7. Build types and single-script contract

Release Profiles may describe one of four build types:

- `native` — native Apple/Xcode or another directly native build;
- `expo` — Expo / React Native, including prebuild/pods/Xcode work owned by the project script;
- `hybrid-web-native` — Web/frontend assets plus a native shell/service;
- `tauri` — Tauri/Rust desktop packaging.

These values are metadata and UI descriptions, **not four separate ILS build engines**.

The preferred contract remains one project-owned entrypoint per Release Profile, for example:

```bash
bash scripts/ils-build-macos.sh
bash scripts/ils-build-ios.sh --adhoc
bash scripts/ils-build-ios.sh --testflight
```

The project script owns all framework-specific work (Expo prebuild, npm/pnpm, Cargo, CocoaPods, Xcode, Tauri, asset embedding, etc.). ILS only supplies environment/context, consumes `ILS_EVENT`, reads `ILS_OUTPUT_DIR/ils-result.json`, validates the result, and publishes it.

`Package Command` remains available only for legacy/compatibility profiles; new standard profiles should finish their internal build/package workflow inside the single Build Command script.

Repository-owned `.ils/project.json` manifests may set `build_type`, and `scripts/import-ils-project.py` imports it into local ILS profile metadata.

## 8. Build UI polling, logs, and motion contract

The build page polls every 3 seconds, but polling must not disturb user navigation.

As of 2026-10-07:

- existing build cards are not re-appended on every poll;
- cards move only when desired sort order actually differs;
- synchronous build-card updates preserve page `window.scrollY`;
- transient polling failures do not collapse an already-rendered job list;
- log updates append only the new suffix when possible;
- **new log content never auto-scrolls the log**;
- the log keeps the exact current `scrollTop` (bounded only when content becomes shorter/replaced);
- opening a running build starts at the natural current log position rather than forcing the viewport to the newest line;
- dynamic build/log nodes disable browser scroll anchoring where necessary;
- determinate progress interpolates smoothly toward new backend samples instead of jumping every poll;
- stages without a real percentage use a continuously moving indeterminate flow;
- the active pipeline stage has a continuous pulse/flow animation, while completed steps transition to the completed state.

Regression coverage: `web/build_jobs_ui_test.go`.

## 9. iOS Release build / Simulator rule

ILS iOS release builds must be device-only, headless release builds. They must not use a Simulator destination.

The Sowhat ILS release entrypoint is:

```bash
bash scripts/ils-build-ios.sh --adhoc
bash scripts/ils-build-ios.sh --testflight
```

Sowhat commit `36a3bbcce40b68fd814dce85ada1643881cc7765` tightened the Archive invocation to:

- `-sdk iphoneos`
- `-destination generic/platform=iOS`
- `SDKROOT=iphoneos`
- `SUPPORTED_PLATFORMS=iphoneos`
- `ONLY_ACTIVE_ARCH=NO`

The script contains no `simctl`, app install, app launch, or Simulator launch command. The archive is also validated with `DTPlatformName=iphoneos`.

ChatDock and IWB iOS release scripts follow the same device-only rule. Framework type does not change that rule: an Expo project may run Expo prebuild internally, but its release archive still targets `iphoneos`.

If Simulator still becomes visible during a future release build, do not add `killall Simulator` as a workaround. Capture the process/Xcode logs for that build because the release command is already pinned to iPhoneOS and any remaining GUI launch is external Xcode behavior that should be diagnosed precisely.

## 10. Ad Hoc Release contract

For Sowhat the standard Release Profile is:

- platform: `ios`
- architecture: `arm64`
- lane: `ios-adhoc`
- result contract: `ils-result-v1`
- Apple Team: the Distribution Team used by the Ad Hoc profile
- build command: `bash scripts/ils-build-ios.sh --adhoc`

The script:

1. creates a signed iPhoneOS Release archive;
2. validates bundle/version/build/platform;
3. exports using `release-testing`;
4. validates the exported IPA and `embedded.mobileprovision`;
5. requires `ProvisionedDevices` to exist;
6. returns `ils-result.json` inside `ILS_OUTPUT_DIR`;
7. ILS publishes the Release and OTA Artifact Sync exposes IPA/manifest through the public Gateway.

## 11. Repository-owned project manifests

ILS-supported projects should keep `.ils/project.json` in the project repository. The manifest describes project identity plus Release Profiles, including each Profile's one standard build entrypoint and optional `build_type`.

Current intended types:

- Sowhat: `native`
- ChatDock: `hybrid-web-native`
- WinCat macOS: `tauri`
- IWB macOS: `tauri`
- IWB iOS: `expo`

The manifest `branch` remains useful as repository metadata/import context but is no longer a release gate. ILS always records and builds the checkout that is actually active at build start.

## 12. Current completion state

Completed or accepted at this checkpoint:

- TestFlight upload and state synchronization
- TestFlight release presentation / public link
- global Device Registry
- public UDID enrollment
- Apple Developer device registration
- Ad Hoc Release Profile/build pipeline
- OTA Gateway and artifact sync
- signed enrollment-profile implementation/recovery path
- persistent local admin session with transient-outage preservation
- global Device/OTA page separation
- local-checkout builds on arbitrary branches and dirty trees
- Git provenance snapshots for build jobs
- Native / Expo / Hybrid Web-Native / Tauri build-type descriptions
- single-script standard Release Profile execution model
- build-page page-scroll stability
- append-only, zero-auto-scroll build logs
- smooth determinate/indeterminate build-flow animation
- explicit iPhoneOS-only release build constraints

Do not change TestFlight/OTA semantics while refining the build runner or UI.

## 13. Local verification

After pulling ILS main:

```bash
cd /Users/ted/Documents/workspace/ioant-localservice
git pull --ff-only
git rev-parse HEAD
go test ./...
go build -o bin/localservice ./cmd/localservice
```

Then restart ILS:

```bash
PID="$(lsof -tiTCP:8787 -sTCP:LISTEN | head -1)"
[ -z "$PID" ] || kill "$PID"
./bin/localservice
```

Expected behavior during a normal build:

- a dirty or non-main checkout can build;
- no pull/stash/reset/clean/checkout appears in the build log;
- the card displays branch + HEAD + dirty/clean + upstream when available;
- new log text appears without changing the user's log scroll position;
- page scrolling is not moved by polling;
- stage/progress motion remains continuous between backend samples.

Do not reset/clean/stash/switch branches as part of routine ILS release verification.
