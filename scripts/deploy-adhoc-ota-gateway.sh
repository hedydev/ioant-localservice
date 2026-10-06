#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

DEFAULT_HOST="52.77.167.119"
DEFAULT_USER="ubuntu"
DEFAULT_KEY="/Users/ted/Documents/workspace/aws-sigapore-v2ray.pem"
DEFAULT_ROOT="/srv/ils-adhoc-ota"
DEFAULT_DATA="$REPO_ROOT/.localservice"

HOST="${ILS_OTA_EC2_HOST:-$DEFAULT_HOST}"
SSH_USER="${ILS_OTA_EC2_USER:-$DEFAULT_USER}"
SSH_KEY="${ILS_OTA_EC2_KEY:-$DEFAULT_KEY}"
REMOTE_ROOT="${ILS_OTA_REMOTE_ROOT:-$DEFAULT_ROOT}"
ILS_DATA="${ILS_OTA_DATA_DIR:-$DEFAULT_DATA}"
DOMAIN=""
EMAIL=""
PREPARE_ONLY=0
SKIP_DNS_CHECK=0

usage() {
  cat <<'USAGE'
Provision an HTTPS Nginx gateway for ILS iOS Ad Hoc OTA artifacts.

Usage:
  scripts/deploy-adhoc-ota-gateway.sh --domain ota.example.com --email you@example.com
  scripts/deploy-adhoc-ota-gateway.sh --domain ota.example.com --prepare-only

Options:
  --domain DOMAIN       Required public hostname for OTA downloads.
  --email EMAIL         Let's Encrypt registration email. Required unless --prepare-only.
  --host HOST           EC2 host/IP. Default: 52.77.167.119
  --ssh-user USER       SSH user. Default: ubuntu
  --ssh-key PATH        SSH private key. Default: /Users/ted/Documents/workspace/aws-sigapore-v2ray.pem
  --remote-root PATH    Static OTA root. Default: /srv/ils-adhoc-ota
  --ils-data PATH       Local ILS data directory. Default: <repo>/.localservice
  --prepare-only        Configure HTTP/Nginx only. Do not request a TLS certificate.
  --skip-dns-check      Skip local DNS -> EC2 host verification before Certbot.
  -h, --help            Show this help.

Environment aliases:
  ILS_OTA_EC2_HOST
  ILS_OTA_EC2_USER
  ILS_OTA_EC2_KEY
  ILS_OTA_REMOTE_ROOT
  ILS_OTA_DATA_DIR

This script also deploys the private ota-gateway systemd service, creates
the shared sync token, writes <ILS_DATA>/ota-gateway.json for local ILS,
and signs enrollment profiles with the public Let's Encrypt identity.
USAGE
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="${2:-}"; shift 2 ;;
    --email) EMAIL="${2:-}"; shift 2 ;;
    --host) HOST="${2:-}"; shift 2 ;;
    --ssh-user) SSH_USER="${2:-}"; shift 2 ;;
    --ssh-key) SSH_KEY="${2:-}"; shift 2 ;;
    --remote-root) REMOTE_ROOT="${2:-}"; shift 2 ;;
    --ils-data) ILS_DATA="${2:-}"; shift 2 ;;
    --prepare-only) PREPARE_ONLY=1; shift ;;
    --skip-dns-check) SKIP_DNS_CHECK=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ -n "$DOMAIN" ]] || fail "--domain is required"
[[ "$DOMAIN" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]] || fail "invalid domain: $DOMAIN"
[[ "$DOMAIN" == *.* ]] || fail "domain must contain at least one dot"
[[ "$HOST" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || fail "invalid host: $HOST"
[[ "$SSH_USER" =~ ^[A-Za-z_][A-Za-z0-9_-]*$ ]] || fail "invalid SSH user: $SSH_USER"
[[ "$REMOTE_ROOT" == /* && "$REMOTE_ROOT" =~ ^/[A-Za-z0-9._/-]+$ ]] || fail "remote root must be a simple absolute path"
[[ -f "$SSH_KEY" ]] || fail "SSH private key not found: $SSH_KEY"
command -v go >/dev/null 2>&1 || fail "Go is required locally to build cmd/ota-gateway"
command -v openssl >/dev/null 2>&1 || fail "openssl is required locally"

if [[ "$PREPARE_ONLY" -eq 0 ]]; then
  [[ -n "$EMAIL" ]] || fail "--email is required when requesting Let's Encrypt TLS"
  [[ "$EMAIL" == *@*.* ]] || fail "invalid email: $EMAIL"
fi

mkdir -p "$ILS_DATA"
ILS_DATA="$(cd "$ILS_DATA" && pwd)"
SSH_KEY="$(cd "$(dirname "$SSH_KEY")" && pwd)/$(basename "$SSH_KEY")"
chmod 600 "$SSH_KEY"

SYNC_TOKEN_FILE="$ILS_DATA/ota-gateway-sync-token"
GATEWAY_CONFIG="$ILS_DATA/ota-gateway.json"
if [[ ! -s "$SYNC_TOKEN_FILE" ]]; then
  umask 077
  openssl rand -hex 32 > "$SYNC_TOKEN_FILE"
fi
chmod 600 "$SYNC_TOKEN_FILE"

if [[ "$PREPARE_ONLY" -eq 0 && "$SKIP_DNS_CHECK" -eq 0 ]]; then
  python3 - "$DOMAIN" "$HOST" <<'PY'
import ipaddress
import socket
import sys

domain, host = sys.argv[1:]
try:
    resolved = sorted({item[4][0] for item in socket.getaddrinfo(domain, 443, type=socket.SOCK_STREAM)})
except socket.gaierror as exc:
    raise SystemExit(f"ERROR: DNS for {domain} is not ready: {exc}")

try:
    expected = str(ipaddress.ip_address(host))
except ValueError:
    expected = None

if expected and expected not in resolved:
    raise SystemExit(
        f"ERROR: {domain} resolves to {', '.join(resolved) or 'nothing'}, not EC2 {expected}. "
        f"Create/update the DNS A record and wait for propagation, or use --skip-dns-check intentionally."
    )
print(f"DNS OK: {domain} -> {', '.join(resolved)}")
PY
fi

SSH_OPTS=(
  -i "$SSH_KEY"
  -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=12
  -o ServerAliveInterval=20
  -o ServerAliveCountMax=3
)

REMOTE_ARCH="$(ssh "${SSH_OPTS[@]}" "$SSH_USER@$HOST" uname -m)"
case "$REMOTE_ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) fail "unsupported EC2 architecture: $REMOTE_ARCH" ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
GATEWAY_BIN="$TMP_DIR/ils-ota-gateway"
(
  cd "$REPO_ROOT"
  CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -trimpath -o "$GATEWAY_BIN" ./cmd/ota-gateway
)
REMOTE_BIN_TMP="/tmp/ils-ota-gateway.$$"
REMOTE_TOKEN_TMP="/tmp/ils-ota-sync-token.$$"
scp "${SSH_OPTS[@]}" "$GATEWAY_BIN" "$SSH_USER@$HOST:$REMOTE_BIN_TMP"
scp "${SSH_OPTS[@]}" "$SYNC_TOKEN_FILE" "$SSH_USER@$HOST:$REMOTE_TOKEN_TMP"

printf 'Deploying ILS Ad Hoc OTA gateway\n'
printf '  EC2: %s@%s (%s)\n' "$SSH_USER" "$HOST" "$REMOTE_ARCH"
printf '  Domain: %s\n' "$DOMAIN"
printf '  Root: %s\n' "$REMOTE_ROOT"
printf '  Local ILS data: %s\n' "$ILS_DATA"
printf '  TLS: %s\n' "$([[ "$PREPARE_ONLY" -eq 1 ]] && echo 'prepare only' || echo "Let's Encrypt / Certbot")"

ssh "${SSH_OPTS[@]}" "$SSH_USER@$HOST" bash -s -- \
  "$DOMAIN" "$REMOTE_ROOT" "$SSH_USER" "$PREPARE_ONLY" "$EMAIL" "$REMOTE_BIN_TMP" "$REMOTE_TOKEN_TMP" <<'REMOTE'
set -euo pipefail

[[ $# -ge 7 ]] || {
  echo "ERROR: incomplete remote deploy arguments" >&2
  exit 2
}

DOMAIN="$1"
REMOTE_ROOT="$2"
OWNER="$3"
PREPARE_ONLY="$4"
EMAIL="${5:-}"
REMOTE_BIN_TMP="$6"
REMOTE_TOKEN_TMP="$7"

[[ "$PREPARE_ONLY" == "0" || "$PREPARE_ONLY" == "1" ]] || {
  echo "ERROR: invalid PREPARE_ONLY value: $PREPARE_ONLY" >&2
  exit 2
}
if [[ "$PREPARE_ONLY" != "1" && -z "$EMAIL" ]]; then
  echo "ERROR: email is required for Let's Encrypt TLS provisioning" >&2
  exit 2
fi

SITE_NAME="ils-adhoc-ota-${DOMAIN//./-}"
SITE_AVAILABLE="/etc/nginx/sites-available/${SITE_NAME}.conf"
SITE_ENABLED="/etc/nginx/sites-enabled/${SITE_NAME}.conf"
TMP_SITE="/tmp/${SITE_NAME}.conf.$$"
BACKUP=""
SIGNING_CERT="/etc/ils-ota-gateway/profile-signing-cert.pem"
SIGNING_KEY="/etc/ils-ota-gateway/profile-signing-key.pem"
SIGNING_CHAIN="/etc/ils-ota-gateway/profile-signing-chain.pem"

command -v nginx >/dev/null 2>&1 || {
  echo "ERROR: nginx is not installed on the EC2 host" >&2
  exit 1
}
if ! command -v openssl >/dev/null 2>&1; then
  sudo apt-get update
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y openssl
fi

sudo install -d -o "$OWNER" -g www-data -m 0755 "$REMOTE_ROOT" "$REMOTE_ROOT/releases"
sudo install -d -o "$OWNER" -g "$OWNER" -m 0700 \
  /var/lib/ils-ota-gateway \
  /var/lib/ils-ota-gateway/devices \
  /var/lib/ils-ota-gateway/challenges
sudo install -d -o root -g "$OWNER" -m 0750 /etc/ils-ota-gateway
sudo install -o root -g root -m 0755 "$REMOTE_BIN_TMP" /usr/local/bin/ils-ota-gateway
sudo install -o root -g "$OWNER" -m 0640 "$REMOTE_TOKEN_TMP" /etc/ils-ota-gateway/sync-token
rm -f "$REMOTE_BIN_TMP" "$REMOTE_TOKEN_TMP"

cat > /tmp/ils-ota-gateway.service.$$ <<SERVICE
[Unit]
Description=ILS Public OTA Gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${OWNER}
Group=${OWNER}
ExecStart=/usr/local/bin/ils-ota-gateway --listen 127.0.0.1:8790 --public-url https://${DOMAIN} --data /var/lib/ils-ota-gateway --sync-token-file /etc/ils-ota-gateway/sync-token --profile-signing-cert ${SIGNING_CERT} --profile-signing-key ${SIGNING_KEY} --profile-signing-chain ${SIGNING_CHAIN}
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ils-ota-gateway

[Install]
WantedBy=multi-user.target
SERVICE
sudo install -o root -g root -m 0644 /tmp/ils-ota-gateway.service.$$ /etc/systemd/system/ils-ota-gateway.service
rm -f /tmp/ils-ota-gateway.service.$$
sudo systemctl daemon-reload
sudo systemctl enable --now ils-ota-gateway
sudo systemctl restart ils-ota-gateway

cat > /tmp/ils-ota-index.html.$$ <<'HTML'
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ILS OTA</title></head>
<body><main><h1>ILS OTA</h1><p>Ad Hoc OTA gateway is online.</p><p><a href="/enroll">登记这台 iPhone / iPad</a></p></main></body>
</html>
HTML
sudo install -o "$OWNER" -g www-data -m 0644 /tmp/ils-ota-index.html.$$ "$REMOTE_ROOT/index.html"
rm -f /tmp/ils-ota-index.html.$$

cat > "$TMP_SITE" <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    root ${REMOTE_ROOT};
    index index.html;
    autoindex off;
    server_tokens off;
    client_max_body_size 1m;

    location = / {
        limit_except GET HEAD { deny all; }
        try_files /index.html =404;
    }

    location = /_ils/health {
        proxy_pass http://127.0.0.1:8790;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }

    location = /enroll {
        proxy_pass http://127.0.0.1:8790;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }

    location = /enroll.mobileconfig {
        proxy_pass http://127.0.0.1:8790;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }

    location ^~ /device/callback/ {
        limit_except POST { deny all; }
        proxy_pass http://127.0.0.1:8790;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }

    location ^~ /api/ils/ {
        proxy_pass http://127.0.0.1:8790;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }

    location /releases/ {
        limit_except GET HEAD { deny all; }
        try_files \$uri =404;
    }

    location / {
        limit_except GET HEAD { deny all; }
        try_files \$uri =404;
    }

    location ~* \\.plist\$ {
        default_type application/xml;
        add_header Cache-Control "no-store" always;
        try_files \$uri =404;
    }

    location ~* \\.ipa\$ {
        default_type application/octet-stream;
        add_header Cache-Control "private, no-store" always;
        try_files \$uri =404;
    }
}
NGINX

if sudo test -f "$SITE_AVAILABLE"; then
  BACKUP="${SITE_AVAILABLE}.bak.$(date +%Y%m%d%H%M%S)"
  sudo cp "$SITE_AVAILABLE" "$BACKUP"
fi
sudo install -o root -g root -m 0644 "$TMP_SITE" "$SITE_AVAILABLE"
rm -f "$TMP_SITE"
sudo ln -sfn "$SITE_AVAILABLE" "$SITE_ENABLED"

if ! sudo nginx -t; then
  echo "ERROR: nginx configuration test failed; restoring previous site" >&2
  if [[ -n "$BACKUP" ]]; then
    sudo cp "$BACKUP" "$SITE_AVAILABLE"
  else
    sudo rm -f "$SITE_ENABLED" "$SITE_AVAILABLE"
  fi
  sudo nginx -t || true
  exit 1
fi
sudo systemctl reload nginx

if [[ "$PREPARE_ONLY" == "1" ]]; then
  echo "HTTP gateway prepared. DNS can now be pointed at this EC2 host."
  exit 0
fi

if ! command -v certbot >/dev/null 2>&1; then
  sudo apt-get update
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y certbot python3-certbot-nginx
fi

sudo certbot --nginx \
  --non-interactive \
  --agree-tos \
  --redirect \
  --keep-until-expiring \
  --email "$EMAIL" \
  -d "$DOMAIN"

# Certbot may keep an already-valid certificate without rewriting a freshly
# regenerated HTTP-only site. Explicitly reinstall the existing lineage so
# repeated deployments cannot silently drop the domain's TLS vhost.
sudo certbot install \
  --non-interactive \
  --cert-name "$DOMAIN"

CERT_LINEAGE="/etc/letsencrypt/live/${DOMAIN}"
sudo test -s "$CERT_LINEAGE/cert.pem"
sudo test -s "$CERT_LINEAGE/privkey.pem"
sudo test -s "$CERT_LINEAGE/chain.pem"
sudo install -o root -g "$OWNER" -m 0640 "$CERT_LINEAGE/cert.pem" "$SIGNING_CERT"
sudo install -o root -g "$OWNER" -m 0640 "$CERT_LINEAGE/privkey.pem" "$SIGNING_KEY"
sudo install -o root -g "$OWNER" -m 0640 "$CERT_LINEAGE/chain.pem" "$SIGNING_CHAIN"

# Keep the CMS signing identity in sync with future Let's Encrypt renewals.
cat > /tmp/ils-ota-profile-signing-renew.$$ <<HOOK
#!/usr/bin/env bash
set -euo pipefail
CERT_LINEAGE="/etc/letsencrypt/live/${DOMAIN}"
install -o root -g "${OWNER}" -m 0640 "\$CERT_LINEAGE/cert.pem" "${SIGNING_CERT}"
install -o root -g "${OWNER}" -m 0640 "\$CERT_LINEAGE/privkey.pem" "${SIGNING_KEY}"
install -o root -g "${OWNER}" -m 0640 "\$CERT_LINEAGE/chain.pem" "${SIGNING_CHAIN}"
systemctl try-restart ils-ota-gateway.service
HOOK
sudo install -d -o root -g root -m 0755 /etc/letsencrypt/renewal-hooks/deploy
sudo install -o root -g root -m 0755 /tmp/ils-ota-profile-signing-renew.$$ /etc/letsencrypt/renewal-hooks/deploy/ils-ota-gateway-profile-signing
rm -f /tmp/ils-ota-profile-signing-renew.$$

sudo nginx -t
sudo systemctl reload nginx
sudo systemctl daemon-reload
sudo systemctl restart ils-ota-gateway
sudo systemctl enable --now certbot.timer >/dev/null 2>&1 || true

sudo systemctl is-active --quiet ils-ota-gateway

echo "HTTPS gateway ready: https://${DOMAIN}/"
echo "Enrollment: https://${DOMAIN}/enroll"
echo "Health: https://${DOMAIN}/_ils/health"
REMOTE

python3 - "$GATEWAY_CONFIG" "https://$DOMAIN" "$HOST" "$SSH_USER" "$SSH_KEY" "$REMOTE_ROOT" "$SYNC_TOKEN_FILE" <<'PY'
import json, os, sys
path, public_url, host, user, key, root, token = sys.argv[1:]
payload = {
    "public_url": public_url,
    "ssh_host": host,
    "ssh_user": user,
    "ssh_key_path": key,
    "remote_root": root,
    "sync_token_file": token,
}
tmp = path + ".tmp"
with open(tmp, "w", encoding="utf-8") as f:
    json.dump(payload, f, ensure_ascii=False, indent=2)
    f.write("\n")
os.chmod(tmp, 0o600)
os.replace(tmp, path)
PY
chmod 600 "$GATEWAY_CONFIG"

if [[ "$PREPARE_ONLY" -eq 1 ]]; then
  cat <<NEXT

Prepared successfully.
Local ILS config written:
  $GATEWAY_CONFIG
Next:
  1. Create a DNS A record: $DOMAIN -> $HOST
  2. Wait until DNS resolves publicly.
  3. Re-run this script with --email to request the Let's Encrypt certificate.
NEXT
  exit 0
fi

HEALTH_JSON="$(curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/_ils/health")"
printf '%s' "$HEALTH_JSON" | python3 -c 'import json,sys; data=json.load(sys.stdin); assert data.get("ok") is True, data; assert data.get("profile_signed") is True, data'
curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/" >/dev/null
curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/enroll" >/dev/null
SIGNED_PROFILE="$TMP_DIR/enroll.mobileconfig"
curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/enroll.mobileconfig" -o "$SIGNED_PROFILE"
openssl cms -verify -inform DER -noverify -in "$SIGNED_PROFILE" -out /dev/null >/dev/null 2>&1 || fail "enrollment mobileconfig is not a valid CMS signed profile"
TOKEN="$(cat "$SYNC_TOKEN_FILE")"
curl --fail --silent --show-error --max-time 15 \
  -H "Authorization: Bearer $TOKEN" \
  "https://${DOMAIN}/api/ils/devices/pending" >/dev/null
printf '\nOTA Gateway verification succeeded:\n'
printf '  https://%s/_ils/health\n' "$DOMAIN"
printf '  https://%s/\n' "$DOMAIN"
printf '  https://%s/enroll\n' "$DOMAIN"
printf '  enrollment profile: CMS signed\n'
printf 'Local ILS config: %s\n' "$GATEWAY_CONFIG"
printf 'Static OTA root on EC2: %s\n' "$REMOTE_ROOT"
