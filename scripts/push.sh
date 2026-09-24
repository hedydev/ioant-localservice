#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 5 || $# -gt 8 ]]; then
  echo 'Usage: push.sh PROJECT VERSION BUILD ios|macos FILE [dev|beta|stable] [arm64|x86_64|universal] [VARIANT]' >&2
  exit 2
fi
LOCALSERVICE_URL="${LOCALSERVICE_URL:-${ILS_URL:-}}"
LOCALSERVICE_TOKEN_FILE="${LOCALSERVICE_TOKEN_FILE:-${ILS_TOKEN_FILE:-}}"
LOCALSERVICE_JOB_ID="${LOCALSERVICE_JOB_ID:-${ILS_JOB_ID:-}}"
: "${LOCALSERVICE_URL:?Set ILS_URL or LOCALSERVICE_URL (e.g. http://192.168.1.10:8787)}"
: "${LOCALSERVICE_TOKEN_FILE:?Set ILS_TOKEN_FILE or LOCALSERVICE_TOKEN_FILE to the local admin-token file}"
project="$1"; version="$2"; build="$3"; platform="$4"; artifact="$5"; channel="${6:-dev}"
architecture="${7:-arm64}"
variant="${8:-default}"
[[ "$project" =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || { echo 'Invalid project ID' >&2; exit 2; }
[[ -f "$artifact" ]] || { echo 'Artifact does not exist' >&2; exit 2; }
# Keep the bearer token out of command-line arguments and logs.
header_file="$(mktemp)"
trap 'rm -f "$header_file"' EXIT
chmod 600 "$header_file"
printf 'Authorization: Bearer %s\n' "$(cat "$LOCALSERVICE_TOKEN_FILE")" > "$header_file"
curl --fail-with-body --show-error --silent --retry 2 --retry-delay 2 \
  --header "@$header_file" \
  --form-string "version=$version" --form-string "build=$build" \
  --form-string "platform=$platform" --form-string "channel=$channel" \
  --form-string "variant=$variant" --form-string "job_id=${LOCALSERVICE_JOB_ID:-}" \
  --form-string "architecture=$architecture" --form-string "notes=${RELEASE_NOTES:-}" \
  --form "file=@\"$artifact\"" \
  "${LOCALSERVICE_URL%/}/api/projects/$project/releases"
printf '\n'
