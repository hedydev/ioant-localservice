# ILS

**ILS (Ioant Local Service)** is a LAN build and package distribution service designed to run on a Mac. It uses Go 1.24+, has no third-party Go dependencies, and embeds the web UI into a single binary. iPhone, iPad, and Mac devices can browse multiple projects, view the latest builds, review release history, compare updates, and download installation packages. Uploads and downloads use streaming I/O, and artifacts are stored locally without requiring cloud storage.

## Start the service

~~~sh
go build -o bin/localservice ./cmd/localservice
./bin/localservice
~~~

Open `http://localhost:8787` on the Mac. Other devices on the same LAN can use `http://<mac-lan-ip>:8787`. The Mac firewall must allow inbound access and the devices must be on the same reachable network. The default listen address is `0.0.0.0:8787`; for Mac-only development use `-listen 127.0.0.1:8787`.

On first launch, ILS creates `.localservice/admin-token` with file mode `0600`. Read this file locally on the Mac, open **Release Management** in the web UI, and enter the token to create projects and upload packages. **Never commit the token to Git or paste it into AI conversations.** Browsing and downloading are open to the LAN by default. Write operations, device UDID access, and signing requests require the token. On initial load, the UI shows only project/package browsing, filtering, release history and download/install links. Build configuration, uploads, project creation, device administration and signing controls stay hidden until the token is successfully verified by `GET /api/admin/session`. Entering arbitrary text does not enable management. Logout clears the token and loaded management data; a rejected credential returns the UI to the public view. Every protected API still independently validates Bearer authentication. The browser keeps the token only in memory; refreshing returns to the public view. Plain HTTP is intended only for trusted development LANs; prefer HTTPS whenever the admin token is transmitted.

The default data directory is `.localservice/`. Use `-data /absolute/path` to change it. Packages are stored under `artifacts/`, while metadata is written atomically to `state.json`. Back up the entire data directory. **Only one ILS process may use a given data directory at a time.** Multi-instance and clustered operation are not currently supported. Historical releases are not deleted automatically.

The web UI is organized as project workspace tabs (Overview, Build & Release, Release History, iOS Release, Automation / API) backed by focused ES modules instead of one growing script. Ad Hoc, TestFlight, and macOS outputs share one Release history; their delivery actions differ, but TestFlight is not a separate release catalog. See [WEB_UI_ARCHITECTURE.md](docs/WEB_UI_ARCHITECTURE.md) before adding UI behavior.

## Build and publish from local project directories

ILS can link a project to its existing Git working directory on the Mac. The project remains in its original location; ILS stores orchestration configuration in its own data directory and executes release commands with the project directory as the working directory.

For new Apple integrations, the preferred boundary is the **Apple Release Build Contract**:

~~~text
ILS
├─ verifies branch / clean worktree / upstream
├─ pulls with git pull --ff-only
├─ creates a job/output directory
├─ runs the project package/submission entrypoint
├─ shows live logs and structured progress
├─ validates the result
├─ publishes local artifacts
└─ records TestFlight submission feedback

project
├─ scripts/ils-build-macos.sh
└─ scripts/ils-build-ios.sh
~~~

The product scripts compile, sign, archive/export/package and validate. They do **not** switch/stash/reset/clean/commit/push Git, install or launch the app, read the ILS administrator token, or call the ILS release API.

Preferred Release Profiles set:

~~~json
{
  "result_contract": "ils-result-v1"
}
~~~

and the project writes:

~~~text
$ILS_OUTPUT_DIR/ils-result.json
~~~

ILS Release Profile lanes are:

- `macos-test` — internal/test Mac distribution;
- `macos-release` — public direct distribution, normally Developer ID + notarization;
- `ios-adhoc` — Release archive + `release-testing` export, then immutable ILS IPA publication;
- `ios-testflight` — Release archive + App Store Connect upload, with ILS task/log/result tracking.

Example iOS Ad Hoc profile:

~~~json
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
~~~

Example TestFlight profile:

~~~json
{
  "id": "ios-testflight",
  "name": "iOS TestFlight",
  "platform": "ios",
  "architecture": "arm64",
  "channel": "beta",
  "variant": "default",
  "lane": "ios-testflight",
  "result_contract": "ils-result-v1",
  "build_command": "bash scripts/ils-build-ios.sh --testflight"
}
~~~

Project scripts can emit machine-readable progress while keeping normal stdout/stderr as the full log:

~~~text
ILS_EVENT {"stage":"archive","state":"started"}
ILS_EVENT {"stage":"upload","state":"started","message":"Uploading to App Store Connect"}
ILS_EVENT {"stage":"submitted","state":"succeeded","message":"Upload accepted"}
~~~

An optional numeric `progress` is shown only when the underlying tool provides trustworthy progress. ILS does not invent percentages from elapsed time.

For a local artifact lane, ILS validates the result file and artifact, then publishes through its own release API. For TestFlight, ILS creates the same kind of Release metadata entry used by the rest of the UI, but with `delivery=testflight` and **no fake local IPA artifact**. **App Store Connect upload acceptance is not the same as TestFlight processing completion or tester availability.** The Release initially stays at `submitted`. When App Store Connect API access is configured, ILS synchronizes Apple Processing, Build Beta Detail, and an enabled Beta Group public link into that Release. The TestFlight action replaces download/install for that delivery method.

Existing legacy Release Profiles using `artifact`, `version_command`, and `build_number_command`, plus project-owned self-publishing `release*.sh` scripts, remain supported for compatibility.

See [RELEASE_PROFILES.md](docs/RELEASE_PROFILES.md) for the preferred profile/result contract and [RELEASE_SCRIPTS.md](docs/RELEASE_SCRIPTS.md) for the legacy project-script runner.

## iOS: signing versus device trust

- Downloading or trusting a `.mobileconfig` profile in Safari **does not allow an iPhone to sign an IPA by itself**. Actual signing is performed on the Mac using the private key in Keychain and the Apple account configured in Xcode.
- A paid Apple Developer team can use `release-testing` / Ad Hoc distribution. The target device UDID must be registered with the Apple team, the provisioning profile must include the device, and the Bundle ID, entitlements, certificate, and profile must all match.
- `debugging` export uses development signing. Developer Mode must be enabled on the device, and installation normally uses Xcode or Apple Configurator. ILS does not present a development-signed package as guaranteed to support browser-based installation.
- Ad Hoc over-the-air installation requires an **HTTPS certificate actually trusted by the iPhone**. Both the manifest and IPA must be served from the configured HTTPS origin. HTTP can download files, but it does not satisfy OTA installation requirements.
- When an IPA is uploaded, ILS reads the main app Info.plist and reports the provisioning profile type, device count, and expiration time. This check **does not prove that the full code signature, device authorization, or all entitlements are valid**. The UI does not claim installation success.
- A free Personal Team is not appropriate for Ad Hoc browser distribution and typically uses development provisioning with short-lived profiles.

## Mac signing service

ILS supports HTTP-triggered **Xcode archive export and signing**. It is not a generic IPA re-signing service for arbitrary packages. Xcode remains responsible for the project's extensions and entitlements. Before using signing, sign in to the Apple account in Xcode, ensure the required certificate and private key exist in Keychain, and prepare the project's `.xcarchive`.

~~~sh
# Show teams, valid signing identities, and provisioning profile type/expiration.
# This does not export private keys or login tokens.
python3 scripts/doctor.py

# Configure an existing project archive.
# Replace archive with the actual absolute path.
cp scripts/signing.example.json .localservice/signing.json
chmod 600 .localservice/signing.json
~~~

The example team ID `64RS366WKG` comes from an Apple Distribution identity already present on this Mac. Copying the configuration does not build the application. Create the archive in the application project first, following that project's build instructions. Example:

~~~sh
xcodebuild -project /path/to/DemoApp.xcodeproj -scheme DemoApp \
  -configuration Release -destination 'generic/platform=iOS' \
  -archivePath /path/to/DemoApp.xcarchive \
  DEVELOPMENT_TEAM=64RS366WKG CODE_SIGN_STYLE=Automatic \
  -allowProvisioningUpdates archive
~~~

Use **iOS Signing & Installation -> Request Mac Signing** in the UI, or call `POST /api/projects/demo-app/signing` with Bearer authentication. The service returns `202` and a signing task `id`. Query `GET /api/signing/{id}` with the same authentication to read `running`, `succeeded`, or `failed` status.

Only one signing task runs at a time. Each task has a 20-minute timeout. A successful export is automatically published to the selected project. Remote callers cannot supply arbitrary local paths or commands; signing uses only archives configured on the Mac.

`allow_provisioning_updates: true` allows Xcode to use the signed-in Apple account to update signing assets and may create provisioning profiles or certificates. **It does not guarantee automatic registration of a new UDID submitted through the browser.** Register a new device with the Apple Developer account before exporting a new Ad Hoc build.

Signing logs remain local at `.localservice/signing/<id>/xcodebuild.log` and are not exposed through public APIs. Completed and failed tasks are persisted locally. Any incomplete task interrupted by a service restart must be requested again. Changes to `signing.json` do not require a ILS restart.

Re-signing the same version and build can produce a different binary. If a different file is uploaded under the same release identity, ILS returns `409`. Increment the application's build number and archive again, or use a different release channel. Historical artifacts are never silently overwritten.

## Self-service device enrollment

1. Configure HTTPS as described below, then open ILS in Safari on the iPhone or iPad.
2. Select **Enroll this iPhone / iPad** to download `localservice-device.mobileconfig`.
3. In iOS, open **Settings -> General -> VPN & Device Management** and approve the profile. The profile contains no MDM payload, root certificate, SCEP configuration, or signing capability. It only requests the UDID, product model, and operating system version, and it may appear as unsigned.
4. iOS sends the device information back to ILS. An administrator can enter the release token, open the pending device list, and copy the UDID into Apple Developer Devices.
5. Update the Ad Hoc provisioning profile, request a new Mac signing job, and install the newly published build from the distribution page.

Enrollment challenges expire after 15 minutes and can be used only once. Device records are stored in the restricted local `devices/` directory. Public project and release APIs never expose UDIDs.

ILS validates the signature of the returned CMS payload, but **does not currently validate the Apple device certificate chain**. Device records therefore remain `identity_verified: false` and `pending_apple_registration`. An administrator must verify the device before consuming an Apple device slot or trusting the record for signing decisions. Physical-device profile installation, redirects, and OTA installation still require manual validation.

## HTTPS

If you already have a hostname and certificate trusted by the target devices:

~~~sh
./bin/localservice -listen 0.0.0.0:8787 \
  -public-url https://builds.example.com:8787 \
  -tls-cert /path/to/fullchain.pem -tls-key /path/to/private-key.pem
~~~

TLS can also terminate at a local HTTPS reverse proxy while ILS listens only on loopback. Set `-public-url` to the HTTPS origin that users actually open. ILS uses this fixed origin to generate IPA, manifest, and device callback URLs instead of trusting arbitrary Host headers.

A self-signed certificate must first become fully trusted by the device. Merely trusting an enrollment profile is not sufficient. The current implementation does not automatically create or install a root certificate and does not modify DNS or firewall settings.

## Project and AI automated publishing

Machine-specific Sowhat integration parameters and its build/publish workflow are documented in [SOWHAT_RELEASE_HANDOFF.md](docs/SOWHAT_RELEASE_HANDOFF.md). The reusable workflow is also documented in the hero-skills `publish-localservice-builds` skill. Sowhat is the first integrated project, but the publishing contract is designed for additional projects.

**A project development AI may publish as an administrator when explicitly authorized for that project workflow.** On this development Mac, the authorized workflow may read `.localservice/admin-token` locally and use the Bearer API to create projects, upload installation packages, and verify release records without requiring a browser login for every release. `scripts/push.sh` supports this flow.

The credential must never be printed into model output or written into documentation. Other machines must configure their own protected credential file. ILS currently uses one service-wide administrator token; it does not yet provide per-project or per-agent accounts or permission isolation. Automation should operate only on the project it is responsible for.

Create a project such as `demo-app`, then publish after packaging succeeds:

~~~sh
export LOCALSERVICE_URL=http://127.0.0.1:8787
export LOCALSERVICE_TOKEN_FILE=/path/to/ioant-localservice/.localservice/admin-token
export RELEASE_NOTES='Add voice entry point and fix connection disconnect handling'
./scripts/push.sh demo-app 1.0.0 12 ios /path/to/DemoApp.ipa dev arm64
./scripts/push.sh demo-app 1.0.0 12 macos /path/to/DemoApp.dmg dev universal
~~~

The script does not print the admin token in command arguments or normal output. The maximum supported upload size is 4 GiB. For iOS, `version` and `build` must exactly match `CFBundleShortVersionString` and `CFBundleVersion` in the IPA.

Versions use full SemVer: `x.y.z[-prerelease][+metadata]`. The build number must currently be a positive integer; dotted Apple build numbers are not accepted. macOS accepts `.dmg`, `.pkg`, and `.zip`. iOS accepts `.ipa`. Bare `.app` bundles and `.xcarchive` uploads are not supported.

Artifact Release API responses return a relative `download_url`; clients should resolve it against the ILS origin. TestFlight Release records instead return `delivery=testflight`, status metadata, and an optional `open_url`; they intentionally have no local download URL. For the same project, variant, platform, architecture, channel, version, and build, retrying an identical SHA-256 artifact returns `200`. Uploading different file contents under the same release identity returns `409`. The web UI refreshes every 20 seconds, new builds appear automatically, and historical builds remain available.

| Endpoint | Purpose | Authentication |
|---|---|---|
| `GET /api/health` | Service status, OTA configuration state, and upload limit | None |
| `GET /api/projects` | Project list | None |
| `POST /api/projects` | Create a project with JSON such as `{ "id":"demo-app", "name":"SoWhat" }` | Bearer |
| `GET /api/projects/{project}/release-profiles` | List ILS-owned Release Profiles | Bearer |
| `POST /api/projects/{project}/release-profiles` | Create or update an ILS Release Profile | Bearer |
| `DELETE /api/projects/{project}/release-profiles/{profile}` | Delete an ILS Release Profile | Bearer |
| `POST /api/projects/{project}/builds` | Start a build from `{"profile":"ios-dev"}` or a compatible project script | Bearer |
| `GET /api/projects/{project}/builds` | Read recent ILS build jobs | Bearer |
| `GET /api/builds/{job}/log` | Read a build log | Bearer |
| `POST /api/local/select-app-store-connect-key` | Open the ILS Mac file picker for a local .p8 key path | Bearer |
| `GET /api/app-store-connect/config` | Read App Store Connect connection status (never private-key contents) | Bearer |
| `POST /api/app-store-connect/config` | Save Key ID / Issuer ID / local .p8 path and verify the connection | Bearer |
| `DELETE /api/app-store-connect/config` | Remove ILS App Store Connect configuration; does not delete the .p8 file | Bearer |
| `POST /api/app-store-connect/check` | Verify the configured App Store Connect API connection | Bearer |
| `POST /api/app-store-connect/refresh` | Immediately refresh TestFlight Release states and public links | Bearer |
| `GET /api/projects/{project}/releases` | Unified Release history sorted by SemVer and build descending | None |
| `GET /api/projects/{project}/icon?platform=ios\|macos` | Cached platform-specific project App Icon for profile presentation | None |
| `GET /api/builds/{job}/icon` | Per-build App Icon snapshot, with project-cache fallback for older jobs | None |
| `GET /api/releases/{id}/icon` | Release-specific App Icon snapshot when available | None |
| `POST /api/projects/{project}/releases` | Multipart upload: version, build, platform, architecture, channel, variant, notes, file; build scripts may also send job_id | Bearer |
| `GET /api/projects/{project}/updates` | Check for updates | None |
| `GET /api/releases/{id}/download` | Download with HTTP Range support | None |
| `GET /api/releases/{id}/manifest.plist` | Generate an OTA manifest for eligible iOS packages | None |
| `POST /api/projects/{project}/signing` | Sign and publish a preconfigured archive | Bearer |
| `GET /api/signing/{id}` | Read signing task status | Bearer |
| `GET /api/devices/enroll.mobileconfig` | Generate a one-time device information enrollment profile | None |
| `POST /api/devices/callback/{challenge}` | Receive the iOS CMS device callback | One-time challenge |
| `GET /api/devices` | Read the pending device UDID list | Bearer |

Example update check:

~~~sh
curl 'http://127.0.0.1:8787/api/projects/demo-app/updates?platform=ios&architecture=arm64&channel=dev&current_version=1.0.0&current_build=11'
~~~

ILS compares SemVer first and build number second. Channels are isolated. Architecture filtering includes compatible `universal` packages. SemVer build metadata (`+...`) does not affect precedence, and a prerelease is lower than the corresponding stable release. TestFlight submissions that are still submitted/processing/unavailable remain visible in release history but are not recommended by the update-check endpoint until Apple status is `available`. If the client is newer than the latest server release, ILS does not recommend a downgrade.

A browser cannot automatically read the version of an application already installed on an iPhone. The app must call the update API itself, or the user must provide the current version manually.

## Delivery scope and remaining configuration

The service implementation, web UI, HTTP API, device-information collection flow, Xcode archive signing task, automated publishing script, and Sowhat integration documentation are implemented. Per the current development workflow, ILS itself does not perform physical-device acceptance testing; final signing, profile installation, and OTA installation are validated by the user.

Successful real-device distribution still requires a valid project archive, Apple network access, registered devices, a suitable Ad Hoc provisioning profile, and an HTTPS origin trusted by the target device.

The Mac currently exposes team `64RS366WKG` and an Apple Distribution signing identity to the local diagnostic tooling. ILS does not export Apple login tokens, account passwords, or private keys. Provisioning profiles found during the initial development setup were development profiles; Ad Hoc distribution requires Xcode to export an appropriate profile for the target project and registered devices.

## Apple references

- [Distribute to registered devices](https://developer.apple.com/documentation/xcode/distributing-your-app-to-registered-devices)
- [Create an Ad Hoc provisioning profile](https://developer.apple.com/help/account/provisioning-profiles/create-an-ad-hoc-provisioning-profile)
- [Enable Developer Mode on a device](https://developer.apple.com/documentation/xcode/enabling-developer-mode-on-a-device)
- [Device information enrollment protocol (archived)](https://developer.apple.com/library/archive/documentation/NetworkingInternet/Conceptual/iPhoneOTAConfiguration/profile-service/profile-service.html)
- [Apple Developer membership capabilities](https://developer.apple.com/support/compare-memberships/)

## ILS environment aliases

ILS-owned build jobs export both the short ILS names and the existing compatibility names. New automation may use `ILS_URL`, `ILS_TOKEN_FILE`, `ILS_ROOT`, `ILS_PROJECT_ID`, `ILS_JOB_ID`, `ILS_OUTPUT_DIR`, and `ILS_GIT_COMMIT`. Existing `LOCALSERVICE_*` variables continue to work, and `scripts/push.sh` accepts either naming scheme.

## App Store Connect status synchronization

ILS can optionally connect to the official App Store Connect API from the administrator UI. Configure the API Key ID, optional Issuer ID, and the absolute path to the downloaded `.p8` private key on the ILS Mac. ILS stores only those references in its protected data directory; it does not copy private-key contents into project files, Release metadata, browser responses, or logs.

When configured, ILS resolves each recent TestFlight Release by Bundle ID, marketing version, and build number. It synchronizes Apple binary processing, Build Beta Detail, and enabled Beta Group public links into the same Release record shown in Overview, Release History, and iOS Release.

See [docs/APP_STORE_CONNECT.md](docs/APP_STORE_CONNECT.md) for setup, security, and state mapping.
