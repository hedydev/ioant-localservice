#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

DOMAIN="${ILS_OTA_DOMAIN:-ota.ioant.com}"
HOST="${ILS_OTA_EC2_HOST:-52.77.167.119}"
SSH_USER="${ILS_OTA_EC2_USER:-ubuntu}"
SSH_KEY="${ILS_OTA_EC2_KEY:-/Users/ted/Documents/workspace/aws-sigapore-v2ray.pem}"

[[ -f "$SSH_KEY" ]] || { echo "ERROR: SSH key not found: $SSH_KEY" >&2; exit 1; }

SSH_OPTS=(
  -i "$SSH_KEY"
  -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=12
)

ssh "${SSH_OPTS[@]}" "$SSH_USER@$HOST" bash -s -- "$DOMAIN" "$SSH_USER" <<'REMOTE'
set -euo pipefail
DOMAIN="$1"
OWNER="$2"
LINEAGE="/etc/letsencrypt/live/${DOMAIN}"
TARGET="/etc/ils-ota-gateway"

sudo test -s "$LINEAGE/cert.pem"
sudo test -s "$LINEAGE/privkey.pem"
sudo test -s "$LINEAGE/chain.pem"
sudo install -d -o root -g "$OWNER" -m 0750 "$TARGET"
sudo install -o root -g "$OWNER" -m 0640 "$LINEAGE/cert.pem" "$TARGET/profile-signing-cert.pem"
sudo install -o root -g "$OWNER" -m 0640 "$LINEAGE/privkey.pem" "$TARGET/profile-signing-key.pem"
sudo install -o root -g "$OWNER" -m 0640 "$LINEAGE/chain.pem" "$TARGET/profile-signing-chain.pem"

sudo systemctl daemon-reload
sudo systemctl restart ils-ota-gateway
sudo systemctl is-active --quiet ils-ota-gateway

curl --fail --silent --show-error http://127.0.0.1:8790/_ils/health
printf '\n'
REMOTE

HEALTH="$(curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/_ils/health")"
printf '%s\n' "$HEALTH"
printf '%s' "$HEALTH" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("ok") is True, d; assert d.get("profile_signed") is True, d'

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
PROFILE="$TMP_DIR/enroll.mobileconfig"
curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/enroll.mobileconfig" -o "$PROFILE"

if command -v security >/dev/null 2>&1; then
  security cms -D -i "$PROFILE" >/dev/null
elif openssl cms -help >/dev/null 2>&1; then
  openssl cms -verify -inform DER -noverify -in "$PROFILE" -out /dev/null >/dev/null 2>&1
else
  openssl smime -verify -inform DER -noverify -in "$PROFILE" -out /dev/null >/dev/null 2>&1
fi

printf 'OTA enrollment profile signing is ready: https://%s/enroll\n' "$DOMAIN"
