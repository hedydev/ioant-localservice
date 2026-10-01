# ILS Web UI architecture

The ILS web UI is a small framework-free ES-module application. The browser entrypoint is still one page, but project functionality is separated into page-like workspace tabs and feature modules so release, build, signing and automation code do not grow inside one file.

## Workspace views

| View | Purpose | Authentication |
| --- | --- | --- |
| Overview | Latest release and project release counts | Public |
| Build & Release | Git source, Release Profiles, build jobs and live logs | Admin |
| Release History | Filters, downloads, history and manual upload | Public browsing; upload is Admin |
| iOS Release | Public Ad Hoc/TestFlight releases and device enrollment; admin-only device UDIDs, signing controls and App Store Connect status sync live in the same view | Public + Admin enhancements |
| Automation / API | External `push.sh` and API integration guidance | Admin |

The selected view is stored in the URL hash, for example `#builds` or `#releases`. Admin-only hashes fall back to Overview when the browser has not verified an administrator session.

## JavaScript modules

```text
web/
├─ app.js                  # tiny composition root
├─ js/
│  ├─ core.js              # shared state, API/auth, notices, admin session
│  ├─ navigation.js        # workspace tabs/hash navigation
│  ├─ projects.js          # project list and shared data refresh
│  ├─ platform-ui.js       # shared App Icon + Mac/iPhone/iPad/Android presentation
│  ├─ release-ui.js        # shared Release card/actions/TestFlight lifecycle
│  ├─ releases.js          # overview, history, filters, manual upload
│  ├─ build-state.js       # build-page client state only
│  ├─ build-actions.js     # shared build start action
│  ├─ build-source.js      # Git source and legacy project scripts
│  ├─ build-profiles.js    # Release Profile CRUD and run actions
│  ├─ build-jobs.js        # build history, progress and live log following
│  ├─ ios.js               # unified iOS releases, devices and signing compatibility workflow
│  ├─ app-store-connect.js # App Store Connect connection and TestFlight state refresh controls
│  └─ automation.js        # API/push integration presentation
├─ style.css               # shared/base visual language
└─ workspace.css           # workspace tabs and feature-module layouts
```

`web/embed.go` embeds the shell, CSS, entrypoint and `js/*.js`.

## Module boundaries

Keep these boundaries when adding features:

- Authentication and Bearer-header behavior belong in `core.js`.
- Project selection and shared project/release fetching belong in `projects.js`.
- Shared Release card/status/action/TestFlight lifecycle presentation belongs in `release-ui.js`. `releases.js` owns Overview/History composition and filters, while `ios.js` reuses the same renderer instead of maintaining a second Release UI.
- Platform/device icon rendering belongs in `platform-ui.js`; feature modules must reuse it instead of embedding their own SVG or emoji.
- Git source configuration must not be added to release-history code.
- Release Profile forms and CRUD belong in `build-profiles.js`.
- Task state, progress and build logs belong in `build-jobs.js`.
- Public iOS Release/enrollment presentation and device/signing compatibility controls belong in `ios.js`.
- App Store Connect connection, validation and manual TestFlight status refresh controls belong in `app-store-connect.js`; Apple API/JWT logic stays backend-only.
- External integration examples belong in `automation.js`.
- Navigation code must not perform build, release or signing API actions.

If a new feature does not clearly fit one of these modules, create a new module rather than expanding `core.js` or `app.js`.

## Cross-module events

Modules coordinate through small browser events instead of directly importing each other's rendering internals.

Current events include:

```text
project-changed
data-refreshed
admin-loaded
admin-cleared
admin-changed
view-changed
build-source-loaded
build-started
```

Shared action helpers may be imported when they represent a real reusable operation, such as `startBuild` or `refreshData`.

## Live logs

`build-jobs.js` owns the live build-log behavior:

- every Build Job is an independent card; its **查看日志 / 收起日志** action is inside that same card, below the task summary, and the log panel renders immediately below that action inside the same card; there is no page-level or list-bottom log panel;
- a running task auto-expands its log unless the user explicitly collapsed that task;
- only one task log is expanded at a time; historical tasks can be opened with **查看日志** and collapsed with **收起日志**;
- first open scrolls to the newest output;
- while the user remains near the bottom, new output auto-follows;
- scrolling upward pauses following and the scroll position is preserved across the three-second job refresh;
- returning near the bottom resumes following;
- standard `ILS_EVENT` progress remains authoritative;
- while an Xcode build/archive/export/upload/notarize stage is active, native `Progress N%: message` output is parsed as real task progress and shown in the build card instead of a fabricated percentage.

Do not implement separate log polling in Release Profile or source modules.

## Admin UI

Public project/package browsing is available without the administrator credential.

Admin controls are marked with `data-admin` and are shown only after `GET /api/admin/session` succeeds. Admin-only workspace panels use `data-admin-view`; navigation controls whether a particular admin page is active.

This is UI state only. Every protected backend API must still enforce Bearer authentication independently.

## Adding a new page or feature

1. Decide the owning feature domain.
2. Add a focused ES module under `web/js/`.
3. Add a workspace view only when the feature has enough independent UI to deserve one.
4. Keep the backend API contract independent from the visual tab layout.
5. Add the module to `web/app.js`.
6. Ensure `web/embed.go` still covers the file pattern.
7. Validate JavaScript syntax, Go embed/build, and the relevant real workflow before calling the feature complete.


## Responsive dialogs

All dialogs share the base rules in `style.css`:

- dialog width is capped by the viewport;
- long content scrolls vertically inside the form;
- horizontal dialog scrolling is not allowed;
- form grids use `minmax(0, 1fr)` tracks so selects and textareas cannot force overflow;
- below 640 px, multi-column forms become one column;
- the dialog heading remains visible while long forms scroll.

Feature modules should not add ad-hoc dialog widths unless a genuinely different interaction requires it.


## Platform and device icons

Titles, labels, release badges, Release Profiles and build-task titles use the shared `platform-ui.js` renderer.

Supported target keys are currently:

```text
mac       Mac
iphone    iPhone
ipad      iPad
android   Android (reserved for the future Android lane)
```

The icons are local inline SVG outlines and do not depend on external fonts or icon CDNs.

For published IPA files, ILS reads `UIDeviceFamily` from the IPA's app `Info.plist` and exposes detected device targets through `ios.device_targets`, so release-history icons can distinguish iPhone and iPad support from artifact evidence. UI code should prefer explicit target metadata when available and use platform defaults only when target-level metadata is unavailable.

When Android distribution is added, extend the backend platform contract and target metadata; the visual renderer already has an Android target and should remain the single icon implementation.


## TestFlight task semantics

The TestFlight build pipeline separates upload acceptance from later Apple states:

```text
upload
→ submitted to App Store Connect
→ Apple Processing
→ TestFlight available
```

A successful Xcode upload or `ils-result-v1` result with `status=submitted` creates a normal ILS Release metadata record with `delivery=testflight` and initially stops at **submitted to App Store Connect**. ILS does not invent Processing or tester availability.

When App Store Connect API access is configured, ILS polls Apple for the matching Bundle ID / marketing version / build number and synchronizes `processingState`, Build Beta Detail, and an enabled Beta Group public link. Those verified states update both the Release card and its associated Build Job.

Overview, Release History and iOS Release all call the same `release-ui.js` card renderer. TestFlight cards show the same verified lifecycle (`submitted → Apple Processing → available`) and use **在 TestFlight 中打开** instead of IPA install/download. An `ios-testflight` Release Profile may still store an optional `testflight_url` invitation as a fallback when Apple does not expose a public link through the configured account.

## App Icon presentation

App Icon artwork is separate from the Mac/iPhone/iPad platform-outline glyphs.

ILS keeps a platform-specific project icon cache. When a linked Git source is inspected or pulled, ILS scans tracked `*.appiconset/Contents.json` catalogs and caches the best iOS and macOS PNG independently. It does not hard-code a product name or image path. Release Profiles use the current project cache; each new Build Job snapshots the platform icon into its own job directory so later source icon changes do not rewrite that task's presentation.

For published iOS IPA files, ILS additionally extracts the largest usable compiled main-app App Icon PNG when available. Every new Release snapshots an icon into release-specific storage. TestFlight releases and macOS/fallback releases use the current platform-specific project cache when no artifact icon can be extracted. Existing releases are backfilled lazily from their retained artifact/cache when possible. Release History and the iOS release list use the release snapshot.

The UI wraps artwork in a shared App Icon shell with a small platform badge. If the artwork endpoint is missing or fails to load, the shell remains visible and falls back to the Mac/iPhone/iPad glyphs instead of leaving an empty identity slot.

## App Store Connect module boundary

App Store Connect authentication and status resolution live in the backend service; the browser never signs JWTs and never receives `.p8` contents.

The administrator-facing connection controls live on the iOS Release page because they affect TestFlight delivery/status, but TestFlight releases themselves remain ordinary Release records and are not rendered in a separate history.

The backend stores only:

```text
key_id
issuer_id        # optional for Individual API keys
private_key_path # absolute path on the ILS Mac
```

The private key stays on disk. Browser/API responses may show its path and file permission to an authenticated administrator, but never its contents.

The iOS administrator panel presents Team vs Individual key type, connection/refresh state, last check time, and the effective upload-auth route. Team keys can be injected into `xcodebuild`; Individual keys remain status-query credentials while upload uses the signed-in Xcode account. Saving a connected key triggers one immediate TestFlight status refresh; the panel then polls only the local config/status endpoint every 10 seconds while visible.

Public release-list reads may schedule a rate-limited background TestFlight refresh when a valid App Store Connect configuration exists. Network refresh does not block the current release-list response.
