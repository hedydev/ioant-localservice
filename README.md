# Localservice

Localservice is a LAN package distribution service designed to run on a Mac. It uses Go 1.24+, has no third-party Go dependencies, and embeds the web UI into a single binary. iPhone, iPad, and Mac devices can browse multiple projects, view the latest builds, review release history, compare updates, and download installation packages. Uploads and downloads use streaming I/O, and artifacts are stored locally without requiring cloud storage.

## Start the service

~~~sh
go build -o bin/localservice ./cmd/localservice
./bin/localservice
~~~

Open `http://localhost:8787` on the Mac. Other devices on the same LAN can use `http://<mac-lan-ip>:8787`. The Mac firewall must allow inbound access and the devices must be on the same reachable network. The default listen address is `0.0.0.0:8787`; for Mac-only development use `-listen 127.0.0.1:8787`.

On first launch, Localservice creates `.localservice/admin-token` with file mode `0600`. Read this file locally on the Mac, open **Release Management** in the web UI, and enter the token to create projects and upload packages. **Never commit the token to Git or paste it into AI conversations.** Browsing and downloading are open to the LAN by default. Write operations, device UDID access, and signing requests require the token. The browser keeps the token only in memory. Plain HTTP is intended only for trusted development LANs; prefer HTTPS whenever the admin token is transmitted.

The default data directory is `.localservice/`. Use `-data /absolute/path` to change it. Packages are stored under `artifacts/`, while metadata is written atomically to `state.json`. Back up the entire data directory. **Only one Localservice process may use a given data directory at a time.** Multi-instance and clustered operation are not currently supported. Historical releases are not deleted automatically.

## Publish from a local project directory

The management page includes **Publish from Project Directory**. After a project directory is linked, Localservice detects the current Git repository, branch, upstream, and release scripts named `release*.sh` in the repository root or `scripts/` directory. A matching `.md` file can provide release-script documentation.

After a release script is selected, Localservice runs `git pull --ff-only` in that project directory and then executes the release script to build, package, and upload artifacts. The operation stops if the working tree is dirty or the branch does not match the expected branch. Multiple release scripts and multiple artifacts per release job are supported. See [RELEASE_SCRIPTS.md](docs/RELEASE_SCRIPTS.md) for the full contract.

## iOS: signing versus device trust

- Downloading or trusting a `.mobileconfig` profile in Safari **does not allow an iPhone to sign an IPA by itself**. Actual signing is performed on the Mac using the private key in Keychain and the Apple account configured in Xcode.
- A paid Apple Developer team can use `release-testing` / Ad Hoc distribution. The target device UDID must be registered with the Apple team, the provisioning profile must include the device, and the Bundle ID, entitlements, certificate, and profile must all match.
- `debugging` export uses development signing. Developer Mode must be enabled on the device, and installation normally uses Xcode or Apple Configurator. Localservice does not present a development-signed package as guaranteed to support browser-based installation.
- Ad Hoc over-the-air installation requires an **HTTPS certificate actually trusted by the iPhone**. Both the manifest and IPA must be served from the configured HTTPS origin. HTTP can download files, but it does not satisfy OTA installation requirements.
- When an IPA is uploaded, Localservice reads the main app Info.plist and reports the provisioning profile type, device count, and expiration time. This check **does not prove that the full code signature, device authorization, or all entitlements are valid**. The UI does not claim installation success.
- A free Personal Team is not appropriate for Ad Hoc browser distribution and typically uses development provisioning with short-lived profiles.

## Mac signing service

Localservice supports HTTP-triggered **Xcode archive export and signing**. It is not a generic IPA re-signing service for arbitrary packages. Xcode remains responsible for the project's extensions and entitlements. Before using signing, sign in to the Apple account in Xcode, ensure the required certificate and private key exist in Keychain, and prepare the project's `.xcarchive`.

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

Signing logs remain local at `.localservice/signing/<id>/xcodebuild.log` and are not exposed through public APIs. Completed and failed tasks are persisted locally. Any incomplete task interrupted by a service restart must be requested again. Changes to `signing.json` do not require a Localservice restart.

Re-signing the same version and build can produce a different binary. If a different file is uploaded under the same release identity, Localservice returns `409`. Increment the application's build number and archive again, or use a different release channel. Historical artifacts are never silently overwritten.

## Self-service device enrollment

1. Configure HTTPS as described below, then open Localservice in Safari on the iPhone or iPad.
2. Select **Enroll this iPhone / iPad** to download `localservice-device.mobileconfig`.
3. In iOS, open **Settings -> General -> VPN & Device Management** and approve the profile. The profile contains no MDM payload, root certificate, SCEP configuration, or signing capability. It only requests the UDID, product model, and operating system version, and it may appear as unsigned.
4. iOS sends the device information back to Localservice. An administrator can enter the release token, open the pending device list, and copy the UDID into Apple Developer Devices.
5. Update the Ad Hoc provisioning profile, request a new Mac signing job, and install the newly published build from the distribution page.

Enrollment challenges expire after 15 minutes and can be used only once. Device records are stored in the restricted local `devices/` directory. Public project and release APIs never expose UDIDs.

Localservice validates the signature of the returned CMS payload, but **does not currently validate the Apple device certificate chain**. Device records therefore remain `identity_verified: false` and `pending_apple_registration`. An administrator must verify the device before consuming an Apple device slot or trusting the record for signing decisions. Physical-device profile installation, redirects, and OTA installation still require manual validation.

## HTTPS

If you already have a hostname and certificate trusted by the target devices:

~~~sh
./bin/localservice -listen 0.0.0.0:8787 \
  -public-url https://builds.example.com:8787 \
  -tls-cert /path/to/fullchain.pem -tls-key /path/to/private-key.pem
~~~

TLS can also terminate at a local HTTPS reverse proxy while Localservice listens only on loopback. Set `-public-url` to the HTTPS origin that users actually open. Localservice uses this fixed origin to generate IPA, manifest, and device callback URLs instead of trusting arbitrary Host headers.

A self-signed certificate must first become fully trusted by the device. Merely trusting an enrollment profile is not sufficient. The current implementation does not automatically create or install a root certificate and does not modify DNS or firewall settings.

## Project and AI automated publishing

Machine-specific Sowhat integration parameters and its build/publish workflow are documented in [SOWHAT_RELEASE_HANDOFF.md](docs/SOWHAT_RELEASE_HANDOFF.md). The reusable workflow is also documented in the hero-skills `publish-localservice-builds` skill. Sowhat is the first integrated project, but the publishing contract is designed for additional projects.

**A project development AI may publish as an administrator when explicitly authorized for that project workflow.** On this development Mac, the authorized workflow may read `.localservice/admin-token` locally and use the Bearer API to create projects, upload installation packages, and verify release records without requiring a browser login for every release. `scripts/push.sh` supports this flow.

The credential must never be printed into model output or written into documentation. Other machines must configure their own protected credential file. Localservice currently uses one service-wide administrator token; it does not yet provide per-project or per-agent accounts or permission isolation. Automation should operate only on the project it is responsible for.

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

API responses return a relative `download_url`; clients should resolve it against the Localservice origin. For the same project, variant, platform, architecture, channel, version, and build, retrying an identical SHA-256 artifact returns `200`. Uploading different file contents under the same release identity returns `409`. The web UI refreshes every 20 seconds, new builds appear automatically, and historical builds remain available.

| Endpoint | Purpose | Authentication |
|---|---|---|
| `GET /api/health` | Service status, OTA configuration state, and upload limit | None |
| `GET /api/projects` | Project list | None |
| `POST /api/projects` | Create a project with JSON such as `{ "id":"demo-app", "name":"SoWhat" }` | Bearer |
| `GET /api/projects/{project}/releases` | Release history sorted by SemVer and build descending | None |
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

Localservice compares SemVer first and build number second. Channels are isolated. Architecture filtering includes compatible `universal` packages. SemVer build metadata (`+...`) does not affect precedence, and a prerelease is lower than the corresponding stable release. If the client is newer than the latest server release, Localservice does not recommend a downgrade.

A browser cannot automatically read the version of an application already installed on an iPhone. The app must call the update API itself, or the user must provide the current version manually.

## Delivery scope and remaining configuration

The service implementation, web UI, HTTP API, device-information collection flow, Xcode archive signing task, automated publishing script, and Sowhat integration documentation are implemented. Per the current development workflow, Localservice itself does not perform physical-device acceptance testing; final signing, profile installation, and OTA installation are validated by the user.

Successful real-device distribution still requires a valid project archive, Apple network access, registered devices, a suitable Ad Hoc provisioning profile, and an HTTPS origin trusted by the target device.

The Mac currently exposes team `64RS366WKG` and an Apple Distribution signing identity to the local diagnostic tooling. Localservice does not export Apple login tokens, account passwords, or private keys. Provisioning profiles found during the initial development setup were development profiles; Ad Hoc distribution requires Xcode to export an appropriate profile for the target project and registered devices.

## Apple references

- [Distribute to registered devices](https://developer.apple.com/documentation/xcode/distributing-your-app-to-registered-devices)
- [Create an Ad Hoc provisioning profile](https://developer.apple.com/help/account/provisioning-profiles/create-an-ad-hoc-provisioning-profile)
- [Enable Developer Mode on a device](https://developer.apple.com/documentation/xcode/enabling-developer-mode-on-a-device)
- [Device information enrollment protocol (archived)](https://developer.apple.com/library/archive/documentation/NetworkingInternet/Conceptual/iPhoneOTAConfiguration/profile-service/profile-service.html)
- [Apple Developer membership capabilities](https://developer.apple.com/support/compare-memberships/)
