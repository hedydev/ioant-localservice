# ILS App Store Connect integration

ILS uses the official App Store Connect API to turn a submitted TestFlight upload into a continuously updated Release record.

## Product model

TestFlight is not a separate release catalog. It is one delivery method of a normal ILS Release:

```text
delivery=artifact   -> local install/download action
delivery=testflight -> TestFlight action
```

A successful TestFlight upload creates Release metadata only. ILS never creates or advertises a fake local IPA for an App Store Connect upload.

## Configure in the UI

Open the project's **iOS Release** page as an administrator and use **App Store Connect Status Sync**.

Provide:

1. **Key ID** — the App Store Connect API key identifier.
2. **Issuer ID** — required for a Team API Key; leave blank for an Individual API Key.
3. **Private Key (.p8)** — choose the downloaded private-key file on the ILS Mac.

The `.p8` file must remain outside Git. Restrict it to the local user, normally mode `0600`.

ILS stores only the identifiers and absolute local key path in:

```text
.localservice/app-store-connect.json
```

The key contents are read only when signing an App Store Connect JWT. They are never copied into ILS JSON configuration, logs, browser responses, Release metadata, or project repositories.

## Status resolution

For each recent TestFlight Release with a Bundle ID, ILS resolves:

```text
Bundle ID
-> App Store Connect App
-> Build Upload (marketing version + build number)
-> Build Upload.state
-> iOS prerelease marketing version
-> Build
-> Build.processingState
-> Build Beta Detail
-> Beta Group / public link
-> optional Group assignment / Beta App Review automation
```

The Build Upload layer is authoritative during Apple's early import phase. Apple exposes `AWAITING_UPLOAD`, `PROCESSING`, `FAILED`, and `COMPLETE`; a normal Build resource may not exist yet while Build Upload is still Processing. ILS therefore mirrors Build Upload first instead of leaving a real Apple Processing upload at `submitted`.

ILS maps the evidence into the common Release status:

| ILS status | Meaning |
| --- | --- |
| `submitted` | Upload was accepted; Build Upload is not visible yet or is still awaiting the upload |
| `processing` | Build Upload is Processing/Complete-but-waiting-for-Build, or the processed Build is waiting on compliance/review/readiness |
| `available` | Internal or external Build Beta Detail reports a beta-testing-ready/testing state |
| `unavailable` | Apple reports invalid/failed processing, expiration, processing exception, or beta rejection |

The raw Apple fields are also retained under the Release's `testflight` metadata for diagnosis, including `build_upload_state`, Build `processing_state`, internal/external beta states, resolved Group information, and Beta Review state. The Release/Build Job badge surfaces Build Upload `PROCESSING / COMPLETE / FAILED` directly so it can be compared with App Store Connect's **Build Uploads** table.

## TestFlight link

If a Beta Group has an enabled public link, ILS stores it as `testflight.public_link`. The optional Release Profile `testflight_url` is retained separately as `testflight.fallback_url`.

The effective Release `open_url` is resolved in this order:

1. enabled Apple Beta Group public link;
2. Release Profile fallback link;
3. no link.

Because the two sources are stored separately, a later successful Apple refresh can remove a disabled/stale public link and fall back to the profile URL instead of keeping an obsolete `open_url`. The shared Release UI labels whether the current action comes from **Apple Public Link** or **Profile fallback**. Ad Hoc/local releases keep their install/download actions.

## Refresh behavior

Release-list access schedules a background refresh at most once every 60 seconds. Submitted/processing releases stay eligible for frequent refresh; terminal available/unavailable releases are checked less often. The current response is not held open while Apple is queried.

An administrator can also run **Refresh Release Status** from the iOS Release page for an immediate refresh. Saving a successfully validated key immediately runs one release-state refresh so the UI does not require a second manual action.

While the iOS Release view is open for an administrator, the browser re-reads the local App Store Connect configuration status every 10 seconds. This status read does not itself call Apple; it keeps the connection type, refresh-in-progress state, last check time, and upload-auth path current in the UI.

The associated Build Job is updated from the Release state so the pipeline can move from:

```text
submitted -> Apple Processing -> TestFlight available
```

A successfully uploaded job remains a successful upload even if Apple later reports a beta-processing/review problem; the pipeline state/message records the Apple-side problem rather than rewriting history as an upload failure.

## Build-script integration

When a **Team API Key** is configured, ILS exposes these protected local-path references to an `ios-testflight` project build:

```text
ILS_ASC_KEY_ID
ILS_ASC_KEY_PATH
ILS_ASC_ISSUER_ID
```

This lets a standard project release script use the same ILS-managed Team key for xcodebuild/App Store Connect upload without embedding secrets in the project. Individual API Keys are valid for ILS status queries, but ILS does not inject them into xcodebuild because xcodebuild distribution authentication requires an issuer ID; in that case the project script can continue using the Apple account already signed into Xcode.

## Apple references

- App Store Connect API: https://developer.apple.com/documentation/appstoreconnectapi
- Generating tokens for API requests: https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests
- Builds: https://developer.apple.com/documentation/appstoreconnectapi/builds
- Build Beta Detail: https://developer.apple.com/documentation/appstoreconnectapi/buildbetadetail
- Beta Groups: https://developer.apple.com/documentation/appstoreconnectapi/betagroup


## TestFlight Group automation

An `ios-testflight` Release Profile may optionally define a target TestFlight Group:

- `testflight_group_name`;
- `testflight_group_type=internal|external`;
- `testflight_create_group=true` to create the named group when it does not exist;
- `testflight_submit_beta_review=true` for an **external** group when ILS should explicitly submit the processed Build for Beta App Review.

Automation starts only after Build Upload is `COMPLETE` and Apple exposes a normal Build with `processingState=VALID`. ILS never treats a successful upload as proof that the build is testable.

For an internal group, ILS can create/find the group and add the Build. For an external group, ILS can add the Build and, when explicitly enabled, create the Beta App Review submission. Apple may reject that submission when required TestFlight information, export-compliance declarations, or other review prerequisites are missing. Such an automation error is stored on the Release and shown in the UI; it does not rewrite the original upload as failed.
