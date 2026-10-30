#!/usr/bin/env bash
# Incremental maintenance of the EXISTING Ubuntu Lightsail OTA Gateway.
# Default is read-only. Do not use for first-time provisioning.
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOST="${ILS_OTA_LIGHTSAIL_HOST:-${ILS_OTA_EC2_HOST:-13.213.140.184}}"
USER="${ILS_OTA_LIGHTSAIL_USER:-${ILS_OTA_EC2_USER:-ubuntu}}"
KEY="${ILS_OTA_LIGHTSAIL_KEY:-${ILS_OTA_EC2_KEY:-$HOME/.ssh/ioant-lightsail-20261026}}"
DOMAIN="${ILS_OTA_DOMAIN:-ota.ioant.com}"
MODE=plan
CONFIRM=0
fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
usage() {
cat <<'EOF'
Existing ILS OTA Lightsail maintenance:
  --plan                         read-only default
  --apply-binary                 install only a new Gateway executable; backup
                                 old binary and restart only ils-ota-gateway
  --confirm-gateway-restart      mandatory for --apply-binary
  --host HOST                    default 13.213.140.184
  --ssh-user USER                default ubuntu
  --ssh-key PATH                 default ~/.ssh/ioant-lightsail-20261026
  --domain DOMAIN                default ota.ioant.com
  --help
Environment: ILS_OTA_LIGHTSAIL_HOST/USER/KEY, ILS_OTA_DOMAIN.
Legacy ILS_OTA_EC2_HOST/USER/KEY aliases accepted, old EC2 blocked.
No token, signing key, TLS, Nginx, static release, DNS or data writes.
Historical --prepare-only/--email full provisioning is intentionally disabled.
EOF
}
while (($#)); do
  case "$1" in
    --plan) MODE=plan; shift ;;
    --apply-binary) MODE=apply-binary; shift ;;
    --confirm-gateway-restart) CONFIRM=1; shift ;;
    --host|--ssh-user|--ssh-key|--domain)
      (($#>=2)) || fail "Missing value for $1"
      case "$1" in
        --host) HOST="$2";; --ssh-user) USER="$2";;
        --ssh-key) KEY="$2";; --domain) DOMAIN="$2";;
      esac
      shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "Unknown option: $1 (the old full-provisioning mode is disabled)" ;;
  esac
done
[[ "$HOST" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || fail 'Invalid SSH host'
[[ "$USER" =~ ^[A-Za-z_][A-Za-z0-9_-]*$ ]] || fail 'Invalid SSH user'
[[ "$DOMAIN" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$ ]] || fail 'Invalid domain'
[[ "$HOST" != 52.77.167.119 && "$HOST" != 13.228.218.141 ]] || fail 'Old EC2 / other Lightsail is NOT an OTA target'
[[ -f "$KEY" ]] || fail "SSH key not found: $KEY"
[[ "$MODE" != apply-binary || "$CONFIRM" == 1 ]] || fail 'Use --confirm-gateway-restart for binary-only apply'
[[ "$MODE" != plan || "$CONFIRM" == 0 ]] || fail 'Confirmation without apply is not valid'
SSH_OPTS=(-i "$KEY" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=12)
TARGET="$USER@$HOST"
echo "OTA LIGHTSAIL: $TARGET domain=$DOMAIN mode=$MODE"
ssh "${SSH_OPTS[@]}" "$TARGET" bash -s -- "$DOMAIN" <<'REMOTE'
set -Eeuo pipefail
domain="$1"
unit=ils-ota-gateway.service
. /etc/os-release
[[ "$ID" == ubuntu && "${VERSION_ID%%.*}" -ge 24 ]] || { echo 'BLOCKED: expected Ubuntu 24.04+'; exit 1; }
systemctl is-active --quiet "$unit" || { echo 'BLOCKED: gateway inactive'; exit 1; }
systemctl is-enabled --quiet "$unit" || { echo 'BLOCKED: gateway not enabled'; exit 1; }
[[ -s /usr/local/bin/ils-ota-gateway && -d /var/lib/ils-ota-gateway && -d /srv/ils-adhoc-ota/releases ]] || { echo 'BLOCKED: binary/state/releases missing'; exit 1; }
for part in sync-token profile-signing-cert.pem profile-signing-key.pem profile-signing-chain.pem; do
  sudo -n test -s "/etc/ils-ota-gateway/$part" || { echo 'BLOCKED: configured token/signing material absent'; exit 1; }
done
systemctl cat "$unit" | grep -Fq -- '--listen 127.0.0.1:8790' || exit 1
systemctl cat "$unit" | grep -Fq -- '--data /var/lib/ils-ota-gateway' || exit 1
systemctl cat "$unit" | grep -Fq -- '--sync-token-file /etc/ils-ota-gateway/sync-token' || exit 1
systemctl cat "$unit" | grep -Fq -- "--public-url https://$domain" || exit 1
curl -fsS --noproxy '*' --max-time 8 http://127.0.0.1:8790/_ils/health | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("ok") is True and d.get("service")=="ils-adhoc-ota"; print("LOOPBACK_HEALTH_PASS profile_signed="+str(d.get("profile_signed")))'
echo LIGHTSAIL_OTA_PREFLIGHT_PASS
REMOTE
if [[ "$MODE" == plan ]]; then
  echo 'PLAN_ONLY: no server or local state changed.'
  exit 0
fi
command -v go >/dev/null || fail 'Go is unavailable for cross-build'
TEMP="$(mktemp -d)"
trap 'rm -rf "$TEMP"' EXIT
BIN="$TEMP/ils-ota-gateway"
(cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$BIN" ./cmd/ota-gateway)
REMOTE_TMP="/tmp/ils-ota-binary-$RANDOM$RANDOM"
scp "${SSH_OPTS[@]}" "$BIN" "$TARGET:$REMOTE_TMP"
ssh "${SSH_OPTS[@]}" "$TARGET" bash -s -- "$REMOTE_TMP" <<'REMOTE'
set -Eeuo pipefail
src="$1"
bin=/usr/local/bin/ils-ota-gateway
unit=ils-ota-gateway.service
[[ "$src" == /tmp/ils-ota-binary-* && -s "$src" ]] || exit 2
sudo -n install -d -o root -g root -m 0700 /var/backups/ils-ota-gateway
backup="/var/backups/ils-ota-gateway/ils-ota-gateway.$(date -u +%Y%m%dT%H%M%SZ).$$"
sudo -n cp -p "$bin" "$backup"
rollback() {
  echo 'OTA_UPGRADE_FAILED_ROLLBACK_ATTEMPTED'
  sudo -n cp -p "$backup" "$bin" || true
  sudo -n systemctl restart "$unit" || true
}
trap rollback ERR
sudo -n install -o root -g root -m 0755 "$src" "$bin"
rm -f "$src"
sudo -n systemctl restart "$unit"
sudo -n systemctl is-active --quiet "$unit"
curl -fsS --noproxy '*' --max-time 8 http://127.0.0.1:8790/_ils/health |
  python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("ok") is True and d.get("service")=="ils-adhoc-ota"'
trap - ERR
echo 'OTA_BINARY_ONLY_UPGRADE_PASS backup retained'
REMOTE
