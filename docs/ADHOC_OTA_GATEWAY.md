# ILS Ad Hoc OTA EC2 Gateway

This document covers the shared ILS public OTA Gateway for **Public Device Enrollment** and **Ad Hoc OTA Artifact Sync**. The Gateway is service-level infrastructure: every ILS project can reuse it; it is not a Sowhat-specific service.

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
- cross-builds and deploys the small `cmd/ota-gateway` Go service;
- runs that service only on `127.0.0.1:8790` behind Nginx/systemd;
- creates `/srv/ils-adhoc-ota/releases` for public IPA/manifest files;
- keeps enrollment state privately under `/var/lib/ils-ota-gateway`, outside the Nginx static root;
- creates a random ILS↔Gateway sync bearer token and stores its local copy under `.localservice/`;
- writes `.localservice/ota-gateway.json` so the local ILS can discover the Gateway;
- adds a dedicated Nginx HTTP server block;
- runs `nginx -t` before reload;
- leaves the existing Nginx sites untouched;
- does **not** request a TLS certificate in `--prepare-only` mode.

After the DNS A record resolves publicly, run the normal deployment:

```sh
./scripts/deploy-adhoc-ota-gateway.sh \
  --domain ota.ioant.com \
  --email YOUR_LETS_ENCRYPT_EMAIL
```

The script verifies the DNS target, installs Certbot only if it is missing, requests/renews the certificate through the Nginx plugin, enables HTTP -> HTTPS redirect, reloads Nginx, and verifies all three paths:

```text
https://ota.ioant.com/
https://ota.ioant.com/enroll
https://ota.ioant.com/_ils/health
```

It also performs an authenticated check of the private ILS sync endpoint. The script is safe to re-run. It backs up an existing ILS OTA site config before replacing it and runs `nginx -t` before each reload.

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
ILS_OTA_DATA_DIR
```

## Public Device Enrollment

The public enrollment flow is:

```text
iPhone / iPad
  -> https://ota.ioant.com/enroll
  -> one-time 15 minute mobileconfig challenge
  -> iOS Profile Service callback
  -> CMS integrity verification on EC2
  -> private pending device record
  -> local ILS pulls with Bearer auth
  -> existing ILS Device Registry
```

The Gateway only stores a pending device until ILS acknowledges a successful import. The local Device Registry remains the source of truth. Publicly enrolled devices are marked with `source=public_ota_gateway`; the existing local enrollment path remains available and uses `source=local_ils`.

The Gateway does **not** register the UDID with Apple Developer automatically. A collected device remains `pending_apple_registration` until the administrator completes the Apple-side registration/profile workflow.

## OTA Artifact Sync

Eligible Ad Hoc/Enterprise iOS Releases are synchronized by ILS after the local Release has already been saved. A temporary SSH failure therefore does not destroy the local Release.

State:

```text
local Release saved
  -> ota.status=pending/syncing
  -> SSH/SCP staging upload
  -> atomic remote directory replace
  -> public HEAD verification
  -> ota.status=synced
  -> install_url becomes itms-services://...
```

Failures are persisted as `ota.status=failed` with an error and can be retried from the ILS Web UI.

Remote layout uses the random ILS Release ID rather than a predictable version/build path:

```text
/srv/ils-adhoc-ota/releases/
└── <project>/
    └── <opaque-release-id>/
        ├── app.ipa
        └── manifest.plist
```

The manifest references the public EC2 IPA URL. TestFlight Releases never enter this artifact sync path.

## ILS Web

The iOS page now exposes the service-level Gateway state to administrators:

- verify Gateway;
- sync pending public devices into the existing Device Registry;
- sync/retry eligible Ad Hoc artifacts;
- show device source;
- show per-Release OTA sync state;
- use the public `/enroll` URL for device registration whenever the Gateway is configured.

The sync token itself is never returned to the browser.

## Security

- Do not commit the EC2 SSH private key.
- The script enforces mode `0600` on the local SSH key before connecting.
- It uses `StrictHostKeyChecking=accept-new`, not disabled host-key verification.
- Public OTA artifact locations allow only GET/HEAD and directory listing is disabled.
- Public enrollment uses a random one-time challenge with a 15 minute expiry.
- The iOS callback body is capped at 1 MiB and its CMS integrity is verified before accepting UDID data.
- Pending device records are kept outside the public static root.
- `/api/ils/*` requires a separate random bearer token shared only between local ILS and the Gateway.
- The Gateway process listens only on `127.0.0.1:8790`; Nginx is the only public frontend.
- Certbot certificate/private-key material remains under `/etc/letsencrypt` on EC2.
- Release paths contain opaque random Release IDs and Nginx does not expose directory indexes. These URLs are currently stable, not time-limited signed URLs.
- Ad Hoc HTTPS does not replace Apple device authorization: the IPA must still be signed by an Ad Hoc provisioning profile containing the target device UDID.
