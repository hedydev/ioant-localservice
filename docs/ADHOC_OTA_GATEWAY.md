# ILS OTA Gateway — existing Lightsail production (2026-10-30)

ILS Core remains on the developer Mac at `127.0.0.1:8787`. Only the shared public **OTA Gateway** (`cmd/ota-gateway`) was moved from EC2 to Lightsail. It is not an IWB, Sowhat or Free Master-specific service.

## Current production boundary

| Item | Current value |
| --- | --- |
| Region / AWS CLI | Singapore `ap-southeast-1`, profile `hedydev-sg` |
| Instance | Lightsail `ioant-ls-wfk-20261026` (Ubuntu 24.04) |
| Public IPv4 / domain | `13.213.140.184` / `https://ota.ioant.com` |
| SSH | `ubuntu` using private local key `~/.ssh/ioant-lightsail-20261026` |
| Binary | `/usr/local/bin/ils-ota-gateway` |
| Service | `ils-ota-gateway.service`, `127.0.0.1:8790`, active and boot-enabled per AWS migration acceptance |
| Web / published files | Nginx TCP 80/443; `/srv/ils-adhoc-ota/releases/` |
| Private state | `/var/lib/ils-ota-gateway` (devices/challenges) |
| Private config | `/etc/ils-ota-gateway/sync-token`, `profile-signing-cert.pem`, `profile-signing-key.pem`, `profile-signing-chain.pem` |
| Local ILS configuration | `.localservice/ota-gateway.json` and protected local `sync_token_file` |

**AWS migration was completed and website/DNS accepted on 2026-10-30**, based on the `hedydev/ioant-ai-sync-note` `aws` branch handoff and live MCP receipts. Route 53 change `/change/C0716287GDRLPTJEKY32` reached `INSYNC`: `ota.ioant.com`, `ioant.com`, `www.ioant.com` now point to `13.213.140.184`. After change, strict TLS HTTPS homepage checks were 200 even after the old EC2 was stopped. Old WFK EC2 `i-0359cf3e8c28a7df1` (`52.77.167.119`) is **stopped, not terminated**, EBS retained. Do not restart it, rerun the DNS cutover, or copy OTA state again.

**That AWS acceptance does not prove** a fresh authenticated OTA sync, new device callback, Apple signed enrollment profile install, actual IPA install/upgrade, or restart persistence of device records. The ILS-specific acceptance ledger below is separate.

Do not touch Xray/VLESS REALITY on this Lightsail, nor NetScale/CASE78 on **another** Lightsail `13.228.218.141`.

## Safe maintenance: plan before change

The old provisioning behavior was unsafe for a populated server: it could generate and upload a new sync token, rewrite signing materials, replace Nginx vhosts and systemd units, restart services and run Certbot. **It has been removed from the standard maintenance entrypoint.**

```sh
cd ~/Documents/workspace/ioant-localservice
bash scripts/deploy-adhoc-ota-gateway.sh --plan
bash scripts/finish-ota-profile-signing.sh --plan
```

Both default to **read-only** and connect over SSH with strict host-key verification. The deployment plan confirms the current Ubuntu service, existing paths, token/signing-material presence and loopback health. The signing plan additionally checks certificate expiration (24h threshold), installed cert/key public-key equality, Certbot lineage existence and public `profile_signed` flag. The tests do not print token/key material and do not create enrollment challenges.

For an explicitly approved **binary-only** Gateway update (not part of this migration closeout), after reviewing a fresh plan:

```sh
bash scripts/deploy-adhoc-ota-gateway.sh --apply-binary --confirm-gateway-restart
```

This compiles `cmd/ota-gateway` for Linux, stages a new executable, backs up the previous executable under restricted `/var/backups/ils-ota-gateway/`, updates only `/usr/local/bin/ils-ota-gateway`, restarts only `ils-ota-gateway.service`, and checks loopback health; on failure it attempts a binary rollback and a Gateway restart. **Do not use this against production without a separate planned change window and verified backup/rollback readiness.** It does not run Certbot, change Nginx, DNS, systemd unit, sync tokens, signed profile keys, or release data.

Supported optional overrides: `--host`, `--ssh-user`, `--ssh-key`, `--domain`; `ILS_OTA_LIGHTSAIL_HOST/USER/KEY` and `ILS_OTA_DOMAIN`. Legacy `ILS_OTA_EC2_HOST/USER/KEY` aliases are accepted for compatibility but the stopped EC2 address is explicitly refused.

Certbot/TLS renewals belong to the **existing running server's** established renewal hook. Do not reinstall or reissue the production certificate through ILS's old deployment script. A new installation requires its own reviewed provisioning plan; `--prepare-only`, `--email` and implicit first-time state creation are intentionally unavailable.

## Existing Mac ILS config: guarded local rebind

**Changing the deployment script default is not enough**: running ILS reads the Mac-local `.localservice/ota-gateway.json` when contacting the Gateway. That file may still contain the pre-migration SSH target. New ILS code **refuses the stopped EC2 `52.77.167.119` and the unrelated Lightsail `13.228.218.141`** to prevent accidental remote OTA writes, but it does not silently rewrite machine-local settings.

`scripts/ota-lightsail-rebind-local.py` provides a separate guarded local-only migration path:

```sh
python3 scripts/ota-lightsail-rebind-local.py --plan
# Only after the plan verifies remote token parity and authenticated HTTPS:
python3 scripts/ota-lightsail-rebind-local.py --apply --confirm-local-config-only
```

The script requires the existing `public_url=https://ota.ioant.com`, existing `remote_root=/srv/ils-adhoc-ota`, and a local private token that cryptographically matches the **existing** Lightsail token. It must also successfully perform a bearer-authenticated `GET /api/ils/devices/pending`. It then changes **only** `ssh_host` and `ssh_key_path` to the new Lightsail, atomically, with a mode-0600 backup of the original `ota-gateway.json`. It never changes the bearer token, domain, release root, devices, PEM certificates or any remote file.

**Current execution:** not yet run on the Mac. OpenClaw ILS pinned execution was blocked by existing uncommitted local work. Do not report the local config rebound until it is observed.

## Safe read-only business audit

`scripts/ota-lightsail-audit.py` is an OpenClaw pinned, read-only task. It checks:

- Public strict HTTPS `/`, `/enroll`, `/_ils/health` response and enrollment-form presence.
- Unauthorized `GET /api/ils/devices/pending` response (should be 401). With locally configured bearer token, authenticated `GET` should return a JSON array. It reports counts/status only, **never UDIDs or token values**.
- Local `.localservice/ota-gateway.json` hostname, HTTPS origin and protected `sync_token_file`; compares local token SHA-256 to the existing remote token **internally**, reporting only a match boolean.
- Read-only remote systemd state, OTA loopback health, file/directory existence, public release counts, existing manifest/IPA HEAD status, signing certificate expiration and cert/private-key public-key match.

It **does not** request `/enroll.mobileconfig` (which would create a new challenge), post device callbacks, ACK pending devices, upload files, publish Releases, restart a service or mutate Route 53. A missing local token, unavailable SSH/sudo, lack of existing files or incomplete response is reported as `PARTIAL` rather than a fake PASS.

To run the committed OpenClaw task: `.openclaw/tasks/ota-lightsail-readonly.json` pinned to the exact ILS `main` commit. Offline checks: `scripts/ota-lightsail-self-test.py`, and `.openclaw/tasks/ota-lightsail-validate.json` for Bash syntax + Go unit/build. **The first ILS OpenClaw execution on 2026-10-30 was blocked by a dirty local worktree, without test/production mutation.** Resolve only the unrelated local changes with the owner; never force-clean them.

## Existing enrollment and artifact contract

Real gateway endpoints in `cmd/ota-gateway/main.go`:

| Endpoint | Behavior |
| --- | --- |
| `GET /_ils/health` | JSON service status; `profile_signed` is **presence/readability**, not proof certificate validity |
| `GET /enroll` | HTML page with manually entered friendly device name; GET itself is read-only |
| `GET/POST /enroll.mobileconfig` | Creates one-time Profile Service challenge and CMS-signed payload if signing configured; **writes state** |
| `POST /device/callback/{challenge}` | Verifies CMS content, consumes challenge and writes pending device identity; **writes state** |
| `GET /api/ils/devices/pending` | Bearer-authenticated read of pending devices; avoid logging UDIDs |
| `POST /api/ils/devices/{id}/ack` | Bearer-authenticated removal after ILS successfully imports; **writes state** |
| `/releases/<project>/<opaque-id>/manifest.plist`, `app.ipa` | Static Nginx files; GET/HEAD only, no directory listing |

The public enrollment workflow requests UDID, PRODUCT and VERSION, then local ILS imports into the global Device Registry. Device identities are not Apple-verified solely by CMS integrity. Apple Developer device registration and provisioning profiles remain separate. Gateway device state resides privately outside Nginx's static root.

ILS Ad Hoc/Enterprise Release publish records the local Release first; then OTA SSH/SCP uploads the IPA + manifest to an opaque Release ID path, verifies public HEAD and marks `ota.status=synced` only upon success. Failure is persisted for retries, without deleting the local Release. The manifest uses `https://ota.ioant.com/releases/...`. TestFlight uploads do not use this OTA artifact path. Existing URLs are opaque but not time-limited signed URLs.

## Remaining acceptance — do not infer from homepage 200

| Gate | Acceptance criterion | Current closeout status |
| --- | --- | --- |
| DNS / Lightsail / HTTPS site | Exact new A record and strict HTTPS 200 | **AWS migration confirmed (2026-10-30)** |
| Gateway loopback and boot persistence | Service active/enabled, loopback health | **AWS migration confirmed (2026-10-30)** |
| Signed enrollment | `profile_signed`, valid matching certificate/key/chain, real CMS and iOS enrollment | **ILS-specific fresh validation pending** |
| Private API and local token parity | Authenticated pending devices returns 200, token matches | **Pending** |
| Device Registry sync | New real device registration and ACK, no duplicate/leak | **Pending**, requires authorized device test |
| IPA/manifest | Existing files HEAD 200, downloads correct MIME, iOS install | **Pending** |
| Release OTA update | `pending→syncing→synced` on a real Release, retry errors preserved | **Pending** |
| Restart durability | Before/after state inventory with authorized restart window | **Pending; never restart solely for audit** |

Historical documents may describe an earlier EC2-based implementation or earlier successful test devices. They are not automatically a passing **post-Lightsail** test.

## Recovery boundary

For a failed explicit binary-only update, the remote script attempts restoring its restricted backup. If the result is uncertain, perform **read-only** service, hash and health checks before any manual retry. Never recreate `/var/lib/ils-ota-gateway` or tokens to repair a failed health check.

Full AWS rollback is a separately authorized AWS operation: the old EC2 is stopped, the old published IPv4 may no longer be available on restart, and Route 53's original RRsets were captured in the AWS handoff. **Do not hardcode `52.77.167.119` as an unconditional rollback destination.** Neither ILS maintenance script restarts old EC2 or changes DNS.
