#!/usr/bin/env bash
set -euo pipefail

DEFAULT_HOST="52.77.167.119"
DEFAULT_USER="ubuntu"
DEFAULT_KEY="/Users/ted/Documents/workspace/aws-sigapore-v2ray.pem"
DEFAULT_ROOT="/srv/ils-adhoc-ota"

HOST="${ILS_OTA_EC2_HOST:-$DEFAULT_HOST}"
SSH_USER="${ILS_OTA_EC2_USER:-$DEFAULT_USER}"
SSH_KEY="${ILS_OTA_EC2_KEY:-$DEFAULT_KEY}"
REMOTE_ROOT="${ILS_OTA_REMOTE_ROOT:-$DEFAULT_ROOT}"
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
  --prepare-only        Configure HTTP/Nginx only. Do not request a TLS certificate.
  --skip-dns-check      Skip local DNS -> EC2 host verification before Certbot.
  -h, --help            Show this help.

Environment aliases:
  ILS_OTA_EC2_HOST
  ILS_OTA_EC2_USER
  ILS_OTA_EC2_KEY
  ILS_OTA_REMOTE_ROOT
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

if [[ "$PREPARE_ONLY" -eq 0 ]]; then
  [[ -n "$EMAIL" ]] || fail "--email is required when requesting Let's Encrypt TLS"
  [[ "$EMAIL" == *@*.* ]] || fail "invalid email: $EMAIL"
fi

chmod 600 "$SSH_KEY"

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

printf 'Deploying ILS Ad Hoc OTA gateway\n'
printf '  EC2: %s@%s\n' "$SSH_USER" "$HOST"
printf '  Domain: %s\n' "$DOMAIN"
printf '  Root: %s\n' "$REMOTE_ROOT"
printf '  TLS: %s\n' "$([[ "$PREPARE_ONLY" -eq 1 ]] && echo 'prepare only' || echo "Let's Encrypt / Certbot")"

ssh "${SSH_OPTS[@]}" "$SSH_USER@$HOST" bash -s -- \
  "$DOMAIN" "$REMOTE_ROOT" "$SSH_USER" "$PREPARE_ONLY" "$EMAIL" <<'REMOTE'
set -euo pipefail

DOMAIN="$1"
REMOTE_ROOT="$2"
OWNER="$3"
PREPARE_ONLY="$4"
EMAIL="$5"
SITE_NAME="ils-adhoc-ota-${DOMAIN//./-}"
SITE_AVAILABLE="/etc/nginx/sites-available/${SITE_NAME}.conf"
SITE_ENABLED="/etc/nginx/sites-enabled/${SITE_NAME}.conf"
TMP_SITE="/tmp/${SITE_NAME}.conf.$$"
BACKUP=""

command -v nginx >/dev/null 2>&1 || {
  echo "ERROR: nginx is not installed on the EC2 host" >&2
  exit 1
}

sudo install -d -o "$OWNER" -g www-data -m 0755 "$REMOTE_ROOT" "$REMOTE_ROOT/releases"

cat > /tmp/ils-ota-index.html.$$ <<'HTML'
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ILS OTA</title></head>
<body><main><h1>ILS OTA</h1><p>Ad Hoc OTA gateway is online.</p></main></body>
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
    autoindex off;
    server_tokens off;

    location = /_ils/health {
        default_type application/json;
        add_header Cache-Control "no-store" always;
        return 200 '{"ok":true,"service":"ils-adhoc-ota"}';
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

sudo nginx -t
sudo systemctl reload nginx
sudo systemctl enable --now certbot.timer >/dev/null 2>&1 || true

echo "HTTPS gateway ready: https://${DOMAIN}/"
echo "Health: https://${DOMAIN}/_ils/health"
REMOTE

if [[ "$PREPARE_ONLY" -eq 1 ]]; then
  cat <<NEXT

Prepared successfully.
Next:
  1. Create a DNS A record: $DOMAIN -> $HOST
  2. Wait until DNS resolves publicly.
  3. Re-run this script with --email to request the Let's Encrypt certificate.
NEXT
  exit 0
fi

curl --fail --silent --show-error --max-time 15 "https://${DOMAIN}/_ils/health" >/dev/null
printf '\nHTTPS verification succeeded: https://%s/_ils/health\n' "$DOMAIN"
printf 'Static OTA root on EC2: %s\n' "$REMOTE_ROOT"
printf 'This script provisions the gateway only; publishing IPA/manifest files is a separate ILS step.\n'
