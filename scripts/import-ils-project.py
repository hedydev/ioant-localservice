#!/usr/bin/env python3
"""Import one repository's .ils/project.json into a running local ILS.

The project repository owns build commands and release-profile intent. ILS keeps
machine-local source paths, Apple Team selection, credentials, and runtime state.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

PROFILE_KEYS = {
    "id",
    "name",
    "platform",
    "architecture",
    "channel",
    "variant",
    "lane",
    "result_contract",
    "build_type",
    "testflight_url",
    "testflight_group_name",
    "testflight_group_type",
    "testflight_create_group",
    "testflight_submit_beta_review",
    "build_command",
    "package_command",
    "artifact",
    "version_command",
    "build_number_command",
    "notes",
}
TEAM_RE = re.compile(r"^[A-Z0-9]{10}$")


def die(message: str) -> "NoReturn":
    raise SystemExit(f"ERROR: {message}")


def load_json(path: pathlib.Path) -> dict:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        die(f"manifest not found: {path}")
    except (OSError, json.JSONDecodeError) as exc:
        die(f"cannot read manifest {path}: {exc}")
    if not isinstance(value, dict):
        die("project manifest must be a JSON object")
    return value


def api(base: str, token: str, method: str, path: str, payload=None):
    url = base.rstrip("/") + path
    body = None
    headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    if payload is not None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            raw = response.read()
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            detail = json.loads(raw.decode("utf-8")).get("error", raw.decode("utf-8"))
        except Exception:
            detail = raw.decode("utf-8", errors="replace")
        die(f"ILS {method} {path} returned HTTP {exc.code}: {detail}")
    except urllib.error.URLError as exc:
        die(f"cannot connect to ILS at {base}: {exc.reason}")
    if not raw:
        return None
    try:
        return json.loads(raw.decode("utf-8"))
    except json.JSONDecodeError:
        die(f"ILS {method} {path} returned non-JSON data")


def normalized_profile(source: dict, apple_team: str) -> dict:
    if not isinstance(source, dict):
        die("every profiles entry must be a JSON object")
    unknown = sorted(set(source) - PROFILE_KEYS - {"requires_apple_team"})
    if unknown:
        die(f"manifest profile contains unsupported fields: {', '.join(unknown)}")
    result = {key: source[key] for key in PROFILE_KEYS if key in source}
    if source.get("requires_apple_team"):
        if not apple_team:
            die(f"profile {source.get('id', '<unknown>')} requires --apple-team")
        result["apple_team_id"] = apple_team
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description="Import a repository .ils/project.json into local ILS")
    parser.add_argument("--project-dir", required=True, help="absolute or relative local project checkout")
    parser.add_argument("--apple-team", default="", help="10-character Apple Developer Team ID for iOS profiles")
    parser.add_argument("--ils-url", default="http://127.0.0.1:8787", help="running ILS origin")
    parser.add_argument(
        "--token-file",
        default=str(pathlib.Path(__file__).resolve().parents[1] / ".localservice" / "admin-token"),
        help="ILS admin token file",
    )
    parser.add_argument("--dry-run", action="store_true", help="validate and print intended changes without calling ILS")
    args = parser.parse_args()

    project_dir = pathlib.Path(args.project_dir).expanduser().resolve()
    if not project_dir.is_dir():
        die(f"project directory does not exist: {project_dir}")
    manifest_path = project_dir / ".ils" / "project.json"
    manifest = load_json(manifest_path)
    if manifest.get("schema_version") != 1:
        die("only ILS project manifest schema_version 1 is supported")

    project_id = str(manifest.get("id", "")).strip()
    project_name = str(manifest.get("name", "")).strip()
    branch = str(manifest.get("branch", "main")).strip() or "main"
    profiles = manifest.get("profiles")
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,62}", project_id):
        die(f"invalid project id: {project_id!r}")
    if not project_name:
        die("project name is required")
    if not isinstance(profiles, list) or not profiles:
        die("manifest must contain at least one release profile")
    if args.apple_team and not TEAM_RE.fullmatch(args.apple_team):
        die("--apple-team must be a 10-character uppercase Apple Team ID")

    prepared_profiles = [normalized_profile(profile, args.apple_team) for profile in profiles]
    print(f"Project: {project_name} ({project_id})")
    print(f"Source:  {project_dir}")
    print(f"Branch:  {branch} (reference only; ILS builds the current checkout)")
    for profile in prepared_profiles:
        team = profile.get("apple_team_id")
        suffix = f" · Team {team}" if team else ""
        build_type = profile.get("build_type", "native")
        print(f"Profile: {profile.get('id')} · {profile.get('lane')} · {build_type}{suffix}")

    if args.dry_run:
        print("Dry run only; ILS was not changed.")
        return 0

    token_path = pathlib.Path(args.token_file).expanduser().resolve()
    try:
        token = token_path.read_text(encoding="utf-8").strip()
    except OSError as exc:
        die(f"cannot read admin token {token_path}: {exc}")
    if len(token) < 32:
        die("admin token is invalid")

    projects = api(args.ils_url, token, "GET", "/api/projects")
    existing = next((item for item in projects if item.get("id") == project_id), None)
    if existing is None:
        api(args.ils_url, token, "POST", "/api/projects", {"id": project_id, "name": project_name})
        print("ILS project: created")
    else:
        print("ILS project: already exists")

    encoded_id = urllib.parse.quote(project_id, safe="")
    api(
        args.ils_url,
        token,
        "POST",
        f"/api/projects/{encoded_id}/build-source",
        {"path": str(project_dir), "branch": branch},
    )
    print("Build source: configured (current checkout will be used)")

    for profile in prepared_profiles:
        api(args.ils_url, token, "POST", f"/api/projects/{encoded_id}/release-profiles", profile)
        print(f"Release profile: {profile['id']} configured")

    print(f"Imported {project_name} into ILS successfully.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
