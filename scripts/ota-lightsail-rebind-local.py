#!/usr/bin/env python3
"""Local-only OTA SSH target migration; never change tokens or remote services.

Default is plan. Apply requires an explicit opt-in and verified local/remote
sync-token parity plus authenticated HTTPS GET. No device identities are logged.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import urllib.request

ROOT=Path(__file__).resolve().parents[1]
CONFIG=ROOT/".localservice/ota-gateway.json"
NEW_HOST="13.213.140.184"
OLD_HOST="52.77.167.119"
NEW_KEY=Path.home()/".ssh/ioant-lightsail-20261026"
PUBLIC_URL="https://ota.ioant.com"
EXPECTED_ROOT="/srv/ils-adhoc-ota"

def checked_token(cfg):
    file=Path(cfg.get("sync_token_file",""))
    if not file.is_absolute() or not file.is_file():
        raise RuntimeError("Local token path unavailable; will not create or replace it")
    token=file.read_text().strip()
    if len(token)<32:
        raise RuntimeError("Local sync token invalid")
    return token

def remote_parity(token):
    expected=hashlib.sha256(token.encode()).hexdigest()
    script="""
import hashlib,subprocess
p=subprocess.run(["sudo","-n","cat","/etc/ils-ota-gateway/sync-token"],
 stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=12)
if p.returncode!=0 or not p.stdout.strip():
 raise SystemExit(2)
actual=hashlib.sha256(p.stdout.strip()).hexdigest()
print("TOKEN_PARITY_OK" if actual=="__SHA__" else "TOKEN_PARITY_MISMATCH")
""".replace("__SHA__",expected)
    args=["ssh","-i",str(NEW_KEY),"-o","BatchMode=yes","-o","IdentitiesOnly=yes",
       "-o","StrictHostKeyChecking=yes","-o","ConnectTimeout=12","ubuntu@"+NEW_HOST,"python3","-"]
    r=subprocess.run(args,input=script,text=True,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=25)
    return r.returncode==0 and r.stdout.strip()=="TOKEN_PARITY_OK"

def https_auth(token):
    req=urllib.request.Request(PUBLIC_URL+"/api/ils/devices/pending",
       headers={"Authorization":"Bearer "+token,"Accept":"application/json"})
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(req,timeout=12) as resp:
        if resp.status!=200:
            return False
        body=json.loads(resp.read(2<<20))
        return isinstance(body,list)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan",action="store_true",help="read-only default")
    p.add_argument("--apply",action="store_true",help="local config fields only")
    p.add_argument("--confirm-local-config-only",action="store_true")
    a=p.parse_args()
    if a.apply and (a.plan or not a.confirm_local_config_only):
        p.error("--apply requires --confirm-local-config-only and cannot accompany --plan")
    if a.confirm_local_config_only and not a.apply:
        p.error("confirmation requires --apply")
    if not CONFIG.is_file() or not NEW_KEY.is_file():
        print("BLOCKED: local config or Lightsail SSH key absent");return 2
    cfg=json.loads(CONFIG.read_text())
    if cfg.get("public_url","").rstrip("/")!=PUBLIC_URL or cfg.get("remote_root")!=EXPECTED_ROOT:
        print("BLOCKED: unexpected public URL or root, no file changed");return 2
    if cfg.get("ssh_host") not in {OLD_HOST,NEW_HOST}:
        print("BLOCKED: unexpected existing SSH host, no file changed");return 2
    token=checked_token(cfg)
    if not remote_parity(token):
        print("BLOCKED: local and remote token parity not established");return 2
    try:
        if not https_auth(token):
            print("BLOCKED: authenticated HTTPS pending GET did not validate");return 2
    except Exception:
        print("BLOCKED: authenticated HTTPS unavailable");return 2
    change=cfg.get("ssh_host")!=NEW_HOST or cfg.get("ssh_key_path")!=str(NEW_KEY)
    print("LOCAL_OTA_REBIND_PREFLIGHT_PASS")
    print("changes_required="+str(change))
    if not a.apply:
        print("PLAN_ONLY: unchanged");return 0
    if not change:
        print("ALREADY_CURRENT: no local config write");return 0
    new=dict(cfg)
    new["ssh_host"]=NEW_HOST
    new["ssh_key_path"]=str(NEW_KEY)
    # Preserve original config, every other setting and token file by identity.
    before=CONFIG.read_bytes()
    if CONFIG.read_bytes()!=before:
        print("BLOCKED: concurrent local config change");return 2
    stamp=datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    backup=CONFIG.with_name("ota-gateway.json.backup-"+stamp)
    with open(backup,"xb") as f:
        os.fchmod(f.fileno(),0o600)
        f.write(before)
        f.flush()
        os.fsync(f.fileno())
    fd,tmp=tempfile.mkstemp(prefix="ota-gateway.",suffix=".tmp",dir=CONFIG.parent)
    try:
        with os.fdopen(fd,"w",encoding="utf-8") as f:
            os.fchmod(f.fileno(),0o600)
            json.dump(new,f,ensure_ascii=False,indent=2)
            f.write("\n")
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp,CONFIG)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)
    print("LOCAL_OTA_CONFIG_REBOUND_TO_LIGHTSAIL backup retained; no remote mutation")
    return 0

if __name__=="__main__":
    raise SystemExit(main())
