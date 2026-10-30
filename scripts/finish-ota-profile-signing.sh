#!/usr/bin/env bash
# Read-only signing audit for the EXISTING Lightsail. Never replaces keys.
set -Eeuo pipefail
DOMAIN="${ILS_OTA_DOMAIN:-ota.ioant.com}"
HOST="${ILS_OTA_LIGHTSAIL_HOST:-${ILS_OTA_EC2_HOST:-13.213.140.184}}"
USER="${ILS_OTA_LIGHTSAIL_USER:-${ILS_OTA_EC2_USER:-ubuntu}}"
KEY="${ILS_OTA_LIGHTSAIL_KEY:-${ILS_OTA_EC2_KEY:-$HOME/.ssh/ioant-lightsail-20261026}}"
usage() {
  cat <<'EOF'
Read-only OTA profile signing verification on the existing Lightsail:
  --plan (default), --host HOST, --ssh-user USER, --ssh-key PATH,
  --domain DOMAIN, --help
This command NEVER overwrites certificate, chain, private key, sync token
or registration data and NEVER restarts the Gateway or invokes Certbot.
Certificate renewal should use the existing audited renewal hook.
EOF
}
while (($#)); do
  case "$1" in
    --plan) shift ;;
    --host|--ssh-user|--ssh-key|--domain)
      (($#>=2)) || { echo 'Missing argument' >&2; exit 2; }
      case "$1" in
        --host) HOST="$2";; --ssh-user) USER="$2";;
        --ssh-key) KEY="$2";; --domain) DOMAIN="$2";;
      esac; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo 'ERROR: Signing material changes are not authorized in this read-only audit' >&2; exit 2 ;;
  esac
done
[[ "$HOST" != 52.77.167.119 && "$HOST" != 13.228.218.141 ]] || { echo 'ERROR: blocked non-OTA target' >&2; exit 1; }
[[ -f "$KEY" ]] || { echo 'ERROR: SSH key is unavailable' >&2; exit 1; }
SSH_OPTS=(-i "$KEY" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=12)
ssh "${SSH_OPTS[@]}" "$USER@$HOST" bash -s -- "$DOMAIN" <<'REMOTE'
set -Eeuo pipefail
domain="$1"
root=/etc/ils-ota-gateway
lineage="/etc/letsencrypt/live/$domain"
for f in profile-signing-cert.pem profile-signing-chain.pem profile-signing-key.pem; do
  sudo -n test -s "$root/$f" || { echo 'SIGNING_MATERIAL_MISSING'; exit 1; }
done
sudo -n openssl x509 -in "$root/profile-signing-cert.pem" -noout -checkend 86400 >/dev/null ||
  { echo 'SIGNING_CERT_INVALID_OR_EXPIRING'; exit 1; }
# Verify key correspondence by comparing public keys, never exporting key bytes.
cert_pub="$(sudo -n openssl x509 -in "$root/profile-signing-cert.pem" -pubkey -noout | openssl pkey -pubin -outform DER | sha256sum | cut -d' ' -f1)"
key_pub="$(sudo -n openssl pkey -in "$root/profile-signing-key.pem" -pubout -outform DER | sha256sum | cut -d' ' -f1)"
[[ "$cert_pub" == "$key_pub" ]] || { echo 'SIGNING_CERT_KEY_MISMATCH'; exit 1; }
sudo -n test -s "$lineage/cert.pem" || { echo 'CERTBOT_LINEAGE_ABSENT'; exit 1; }
installed_fp="$(sudo -n openssl x509 -in "$root/profile-signing-cert.pem" -noout -fingerprint -sha256)"
lineage_fp="$(sudo -n openssl x509 -in "$lineage/cert.pem" -noout -fingerprint -sha256)"
[[ "$installed_fp" == "$lineage_fp" ]] || { echo 'SIGNING_CERT_DIFFERS_FROM_CERTBOT_LINEAGE_REVIEW'; exit 1; }
sudo -n test -s "$lineage/chain.pem" || { echo 'CERTBOT_CHAIN_MISSING'; exit 1; }
sudo -n cmp -s "$root/profile-signing-chain.pem" "$lineage/chain.pem" ||
  { echo 'SIGNING_CHAIN_DIFFERS_FROM_CERTBOT_LINEAGE_REVIEW'; exit 1; }
# No signing material, fingerprint or public key hash printed.
echo 'SIGNING_CERT_VALID_KEY_MATCHES_LINEAGE'
echo 'CERTBOT_LINEAGE_PRESENT'
REMOTE
curl --fail --silent --show-error --max-time 12 "https://$DOMAIN/_ils/health" |
python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("ok") is True and d.get("profile_signed") is True; print("PUBLIC_HTTPS_PROFILE_SIGNED_FLAG_TRUE")'
echo 'SIGNING_READONLY_PLAN_PASS'
