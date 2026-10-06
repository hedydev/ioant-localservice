# Repository-owned ILS project manifests

ILS supports a small repository-owned manifest at `.ils/project.json` so a project can declare its release lanes without storing machine-local state in Git.

## Boundary

The project repository owns:

- project ID and display name;
- expected source branch;
- Release Profile IDs and lanes;
- platform / architecture / channel / variant;
- project-side build entry commands;
- `ils-result-v1` contract selection.

ILS remains the owner of:

- absolute local checkout path;
- admin token;
- App Store Connect private key configuration;
- Apple Developer Team selection when a manifest marks an iOS profile with `requires_apple_team`;
- build jobs, releases, device registry and OTA Gateway state.

Do not commit API keys, admin tokens, local absolute paths or Apple private-key paths into `.ils/project.json`.

## Example

```json
{
  "schema_version": 1,
  "id": "demo",
  "name": "Demo",
  "branch": "main",
  "profiles": [
    {
      "id": "macos-test",
      "name": "Demo · macOS Test",
      "platform": "macos",
      "architecture": "arm64",
      "channel": "dev",
      "variant": "default",
      "lane": "macos-test",
      "result_contract": "ils-result-v1",
      "build_command": "bash scripts/ils-build-macos.sh"
    },
    {
      "id": "ios-adhoc",
      "name": "Demo · iOS Ad Hoc",
      "platform": "ios",
      "architecture": "arm64",
      "channel": "dev",
      "variant": "default",
      "lane": "ios-adhoc",
      "result_contract": "ils-result-v1",
      "requires_apple_team": true,
      "build_command": "bash scripts/ils-build-ios.sh --adhoc"
    }
  ]
}
```

`requires_apple_team` is manifest metadata consumed by the importer. It is not sent to the Release Profile API. The importer injects the Team ID supplied on the local command line.

## Import

Run ILS first, then from the `ioant-localservice` checkout:

```bash
python3 scripts/import-ils-project.py \
  --project-dir /absolute/path/to/project \
  --apple-team 64RS366WKG
```

For a project with no iOS lanes, omit `--apple-team`.

The importer is idempotent for existing project IDs and Release Profile IDs: it keeps the existing project record, refreshes the local build-source association, and writes the declared profiles by ID.

Use `--dry-run` to validate a manifest without changing ILS:

```bash
python3 scripts/import-ils-project.py \
  --project-dir /absolute/path/to/project \
  --apple-team 64RS366WKG \
  --dry-run
```

The default ILS origin is `http://127.0.0.1:8787`, and the default token file is `<ioant-localservice>/.localservice/admin-token`.

## Standard project entrypoints

Prefer these repository-owned names when the platform exists:

```text
scripts/ils-build-macos.sh
scripts/ils-build-ios.sh
```

They must not install or launch the built application. They build, sign/export/package, validate the real distributable, and write `ILS_OUTPUT_DIR/ils-result.json`.

For iOS Release builds, use an iPhoneOS-only archive target (`-sdk iphoneos`, `generic/platform=iOS`, `SDKROOT=iphoneos`) rather than a Simulator target.

## Current project manifests

As of 2026-10-06:

- ChatDock: macOS Test, iOS Ad Hoc, iOS TestFlight.
- WinCat: macOS Internal Test only. The ILS lane does **not** invoke the separate CrossOver/WineCX compatibility package script.
- Ioant Workbench (IWB): macOS Test, iOS Ad Hoc, iOS TestFlight.
- Sowhat remains the first end-to-end validated reference integration; its existing local profiles continue to work independently of this importer.
