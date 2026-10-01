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

- first open scrolls to the newest output;
- while the user remains near the bottom, new output auto-follows;
- scrolling upward pauses following;
- returning near the bottom resumes following.

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
