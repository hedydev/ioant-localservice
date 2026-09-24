# ILS Release Profiles

ILS Release Profiles let the local service build and publish another local project without requiring that project to carry a release script.

## Ownership

Profiles are owned by ILS and stored under:

```text
.localservice/release-profiles/<project>/<profile>.json
```

The business repository is not modified when a profile is created, edited, run, or deleted.

## Required project state

Before every build ILS verifies:

- the linked directory is a Git worktree;
- the current branch matches the configured branch;
- the worktree is clean;
- an upstream exists.

ILS then runs `git pull --ff-only` with Git hooks and autostash disabled. It does not switch branches, stash, reset, or resolve conflicts.

## Profile fields

| Field | Meaning |
| --- | --- |
| `id` | Stable lowercase profile ID |
| `name` | Display name |
| `platform` | `ios` or `macos` |
| `architecture` | `arm64`, `x86_64`, or `universal` |
| `channel` | `dev`, `beta`, or `stable` |
| `variant` | Stable package identity, such as `default` or `desktop-dmg` |
| `build_command` | Shell command executed in the linked project directory |
| `package_command` | Optional second shell command |
| `artifact` | Relative artifact path, unique glob, or path under `$ILS_OUTPUT_DIR` |
| `version_command` | macOS only: command that prints one SemVer value |
| `build_number_command` | macOS only: command that prints one positive integer |
| `notes` | Default release notes |

For iOS, version and build are read from the IPA. The normal upload checks still verify that the values match the IPA metadata.

## Execution environment

ILS exports:

```text
ILS_URL
ILS_TOKEN_FILE
ILS_ROOT
ILS_PROJECT_ID
ILS_JOB_ID
ILS_OUTPUT_DIR
ILS_GIT_COMMIT
```

The older `LOCALSERVICE_*` aliases are exported as well for compatibility.

The profile commands run with the linked project directory as `cwd`. `ILS_OUTPUT_DIR` is a task-specific directory managed by ILS and is useful when the business project should not retain packaged artifacts.

## Publishing

After the configured commands succeed, ILS resolves exactly one artifact and publishes it through `scripts/push.sh`. The job succeeds only if at least one release record is associated with the build job.

Project-owned `release*.sh` scripts remain available for repositories that intentionally keep their release process in source control, but they are no longer required for ILS-managed distribution.
