# ILS Web UI architecture

The ILS web UI is a small framework-free ES-module application. The browser entrypoint is still one page, but project functionality is separated into page-like workspace tabs and feature modules so release, build, signing and automation code do not grow inside one file.

## Workspace views

| View | Purpose | Authentication |
| --- | --- | --- |
| Overview | Latest release and project release counts | Public |
| Build & Release | Git source, Release Profiles, build jobs and live logs | Admin |
| Release History | Filters, downloads, history and manual upload | Public browsing; upload is Admin |
| iOS Install | Public device enrollment and installable iOS releases; admin-only device UDIDs, signing controls and TestFlight guidance live in the same view | Public + Admin enhancements |
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
│  ├─ platform-ui.js       # shared Mac/iPhone/iPad/Android target icons
│  ├─ releases.js          # overview, history, filters, manual upload
│  ├─ build-state.js       # build-page client state only
│  ├─ build-actions.js     # shared build start action
│  ├─ build-source.js      # Git source and legacy project scripts
│  ├─ build-profiles.js    # Release Profile CRUD and run actions
│  ├─ build-jobs.js        # build history, progress and live log following
│  ├─ ios.js               # devices and signing compatibility workflow
│  └─ automation.js        # API/push integration presentation
├─ style.css               # shared/base visual language
└─ workspace.css           # workspace tabs and feature-module layouts
```

`web/embed.go` embeds the shell, CSS, entrypoint and `js/*.js`.

## Module boundaries

Keep these boundaries when adding features:

- Authentication and Bearer-header behavior belong in `core.js`.
- Project selection and shared project/release fetching belong in `projects.js`.
- Release/package presentation belongs in `releases.js`.
- Platform/device icon rendering belongs in `platform-ui.js`; feature modules must reuse it instead of embedding their own SVG or emoji.
- Git source configuration must not be added to release-history code.
- Release Profile forms and CRUD belong in `build-profiles.js`.
- Task state, progress and build logs belong in `build-jobs.js`.
- Public iOS enrollment/install presentation and admin-only device/signing/TestFlight controls belong in `ios.js`.
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

- the active task's log renders inside that task card, directly below its status/progress/result UI; there is no page-level fixed log panel;
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

A successful Xcode upload or `ils-result-v1` result with `status=submitted` stops at **submitted to App Store Connect**. ILS must not mark Processing or tester availability unless a connected Apple status source proves those states. The current implementation has no App Store Connect processing-status query, so successful submissions remain visibly waiting for Apple Processing.

An `ios-testflight` Release Profile may store an optional `testflight_url` using Apple's `https://testflight.apple.com/join/...` invitation form. The build job snapshots that URL when it starts. Only a successful TestFlight job renders **在 TestFlight 中打开**; Ad Hoc and macOS jobs never receive that action.

## App Icon presentation

App Icon artwork is separate from the Mac/iPhone/iPad platform-outline glyphs.

ILS keeps a platform-specific project icon cache. When a linked Git source is inspected or pulled, ILS scans tracked `*.appiconset/Contents.json` catalogs and caches the best iOS and macOS PNG independently. It does not hard-code a product name or image path. Release Profiles use the current project cache; each new Build Job snapshots the platform icon into its own job directory so later source icon changes do not rewrite that task's presentation.

For published iOS IPA files, ILS additionally extracts the largest usable compiled main-app App Icon PNG when available. Every new local Release snapshots an icon into release-specific storage; macOS and fallback releases use the current platform-specific project cache. Existing releases are backfilled lazily from their retained artifact/cache when possible. Release History and the iOS install list use the release snapshot.

If no usable icon is available, the UI omits the App Icon and keeps the platform glyphs.
