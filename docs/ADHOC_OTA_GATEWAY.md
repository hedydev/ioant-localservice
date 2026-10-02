# ILS Ad Hoc OTA EC2 Gateway

This document covers the public HTTPS gateway used for future iOS Ad Hoc OTA distribution.

The current deployment target is the existing Singapore Ubuntu EC2 that already runs Nginx and Certbot/Let's Encrypt:

```text
EC2: 52.77.167.119
SSH user: ubuntu
SSH key: /Users/ted/Documents/workspace/aws-sigapore-v2ray.pem
OTA root: /srv/ils-adhoc-ota
```

The workspace `.pem` file above is the **SSH private key used to log in to EC2**. It is not the HTTPS certificate. The public HTTPS certificate is issued and renewed on EC2 by **Certbot + Let's Encrypt**.

The deployment script deliberately creates a separate Nginx server block for the OTA hostname. It does not edit the existing `ioant.com` / V2Ray upstream configuration.

## DNS first

Choose a dedicated subdomain, for example:

```text
ota.ioant.com
```

Create a DNS **A** record:

```text
ota.ioant.com -> 52.77.167.119
```

The EC2 security group must allow inbound TCP 80 and 443. Nginx already owns the public HTTP/HTTPS ports on this host.

DNS must resolve to the EC2 address before Let's Encrypt can issue the certificate.

## Two-phase deployment

If the DNS record has not been created yet, prepare the Nginx site first:

```sh
cd /Users/ted/Documents/workspace/ioant-localservice

./scripts/deploy-adhoc-ota-gateway.sh \
  --domain ota.ioant.com \
  --prepare-only
```

This:

- connects to the current EC2 using the workspace SSH key;
- creates `/srv/ils-adhoc-ota` and `/srv/ils-adhoc-ota/releases`;
- adds a dedicated Nginx HTTP server block;
- runs `nginx -t` before reload;
- leaves the existing Nginx sites untouched;
- does **not** request a TLS certificate.

After the DNS A record resolves publicly, run the normal deployment:

```sh
./scripts/deploy-adhoc-ota-gateway.sh \
  --domain ota.ioant.com \
  --email YOUR_LETS_ENCRYPT_EMAIL
```

The script verifies the DNS target, installs Certbot only if it is missing, requests/renews the certificate through the Nginx plugin, enables HTTP -> HTTPS redirect, reloads Nginx, and verifies:

```text
https://ota.ioant.com/_ils/health
```

The script is safe to re-run. It backs up an existing ILS OTA site config before replacing it and runs `nginx -t` before each reload.

## Overrides

The current machine/server values are defaults, not hard requirements:

```sh
./scripts/deploy-adhoc-ota-gateway.sh \
  --domain ota.example.com \
  --email ops@example.com \
  --host 203.0.113.10 \
  --ssh-user ubuntu \
  --ssh-key /path/to/server.pem \
  --remote-root /srv/ils-adhoc-ota
```

Equivalent environment variables are available:

```text
ILS_OTA_EC2_HOST
ILS_OTA_EC2_USER
ILS_OTA_EC2_KEY
ILS_OTA_REMOTE_ROOT
```

## Static OTA layout

The gateway is prepared for static OTA content under:

```text
/srv/ils-adhoc-ota/
└── releases/
    └── <project>/
        └── <version>-<build>/
            ├── manifest.plist
            └── App.ipa
```

Nginx serves `.plist` as XML and `.ipa` as `application/octet-stream`, with directory listing disabled.

A future ILS Ad Hoc publishing step can upload the signed IPA and generated manifest into this root, then expose an install action using:

```text
itms-services://?action=download-manifest&url=https://ota.example.com/releases/.../manifest.plist
```

This deployment script intentionally provisions the HTTPS gateway only. It does not yet copy ILS Release artifacts to EC2 and does not change the local ILS `-public-url`. Artifact synchronization should be added as a separate authenticated publishing step so existing local/TestFlight release behavior is not coupled to server provisioning.

## Security

- Do not commit the EC2 SSH private key.
- The script enforces mode `0600` on the local SSH key before connecting.
- It uses `StrictHostKeyChecking=accept-new`, not disabled host-key verification.
- OTA Nginx locations allow only GET/HEAD and do not enable directory indexes.
- Certbot certificate/private-key material remains under `/etc/letsencrypt` on EC2.
- Ad Hoc HTTPS does not replace Apple device authorization: the IPA must still be signed by an Ad Hoc provisioning profile containing the target device UDID.
