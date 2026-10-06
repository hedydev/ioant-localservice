# ILS Release System Status

Updated: 2026-10-06

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

- A successful admin token is cached in browser `localStorage`.
- Refresh/reopen restores the session silently and validates the cached token before project loading starts.
- `退出管理` explicitly clears the cached session.
- Invalid/401 sessions are removed automatically.
- Global admin-only navigation is hidden when no valid admin session exists.

Do not reintroduce concurrent project loading before admin-session restore; that previously caused an auth-epoch race and a ~20 second wait for the next project polling cycle.

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

## 6. Build UI polling contract

The build page polls every 3 seconds, but polling must not disturb user navigation.

As of 2026-10-06:

- existing build cards are no longer re-appended on every poll;
- cards move only when the desired sort order actually differs;
- synchronous build-card updates preserve page `window.scrollY`;
- transient polling failures do not collapse an already-rendered job list;
- log updates append only the new suffix when possible;
- wheel/touch/pointer interaction with the log immediately disables auto-follow;
- auto-follow resumes only after the log is back near the bottom;
- log scroll position is preserved while reading historical output;
- build progress uses an independent animated bar and interpolates between backend progress samples instead of visually jumping every poll;
- dynamic build/log nodes disable browser scroll anchoring where necessary.

Regression coverage: `web/build_jobs_ui_test.go`.

## 7. iOS Release build / Simulator rule

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

If Simulator still becomes visible during a future release build, do not add `killall Simulator` as a workaround. Capture the process/Xcode logs for that build because the release command is already pinned to iPhoneOS and any remaining GUI launch is external Xcode behavior that should be diagnosed precisely.

## 8. Ad Hoc Release contract

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

## 9. Current completion state

Completed or accepted at this checkpoint:

- TestFlight upload and state synchronization
- TestFlight release presentation / public link
- global Device Registry
- public UDID enrollment
- Apple Developer device registration
- Ad Hoc Release Profile/build pipeline
- OTA Gateway and artifact sync
- signed enrollment-profile implementation/recovery path
- persistent local admin session
- global Device/OTA page separation
- build-page scroll/log/progress stabilization
- explicit iPhoneOS-only Sowhat Release build constraints

The next validation after pulling these changes is simply a normal ILS build while manually scrolling the page/log and observing that Simulator does not appear. Any failure should be diagnosed from the specific build log rather than changing TestFlight/OTA semantics.

## 10. Local verification

After pulling ILS main:

```bash
cd /Users/ted/Documents/workspace/ioant-localservice
git pull --ff-only
go test ./...
go build -o bin/localservice ./cmd/localservice
```

For Sowhat release-script validation:

```bash
cd /Users/ted/Documents/workspace/ioant-sowhat
git pull --ff-only
bash -n scripts/ils-build-ios.sh
```

Do not reset/clean/stash/switch branches as part of routine ILS release verification.
