#!/usr/bin/env python3
"""Bounded read-only production OTA audit: never export secrets or device identities."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
BASE = "https://ota.ioant.com"
HOST = "13.213.140.184"
KEY = Path.home() / ".ssh/ioant-lightsail-20261026"
OUT = Path(os.environ.get("OPENCLAW_OUTPUT_DIR", "")).expanduser()
REMOTE = r'''
import hashlib,json,os,pathlib,subprocess,stat
data=pathlib.Path("/var/lib/ils-ota-gateway")
release=pathlib.Path("/srv/ils-adhoc-ota/releases")
token="/etc/ils-ota-gateway/sync-token"
cert="/etc/ils-ota-gateway/profile-signing-cert.pem"
key="/etc/ils-ota-gateway/profile-signing-key.pem"
chain="/etc/ils-ota-gateway/profile-signing-chain.pem"
def cmd(args,timeout=10,stdin=None):
    try:
        r=subprocess.run(args,input=stdin,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=timeout)
        return r.returncode,r.stdout
    except (OSError,subprocess.TimeoutExpired): return 255,b""
r={"os":"unknown","ota_active":False,"ota_enabled":False,"nginx_active":False,"binary_present":False,
   "data_present":data.is_dir(),"releases_present":release.is_dir(),"token_present":False,
   "token_matches_local":False,"signing_files_present":False,"cert_not_expiring_7d":False,
   "cert_key_match":False,"loopback_health_ok":False,"release_plist_count":0,"release_ipa_count":0,
   "existing_plist_head_ok":None,"existing_ipa_head_ok":None}
try:
    vals=dict(x.split("=",1) for x in pathlib.Path("/etc/os-release").read_text().splitlines() if "=" in x)
    r["os"]=vals.get("ID","").strip('"')+" "+vals.get("VERSION_ID","").strip('"')
except OSError: pass
r["ota_active"]=cmd(["systemctl","is-active","--quiet","ils-ota-gateway.service"])[0]==0
r["ota_enabled"]=cmd(["systemctl","is-enabled","--quiet","ils-ota-gateway.service"])[0]==0
r["nginx_active"]=cmd(["systemctl","is-active","--quiet","nginx.service"])[0]==0
r["binary_present"]=pathlib.Path("/usr/local/bin/ils-ota-gateway").is_file()
code,raw=cmd(["sudo","-n","cat",token])
if code==0 and raw.strip():
    r["token_present"]=True
    r["token_matches_local"]=hashlib.sha256(raw.strip()).hexdigest()=="__TOKEN_SHA__"
r["signing_files_present"]=all(cmd(["sudo","-n","test","-s",p])[0]==0 for p in (cert,key,chain))
if r["signing_files_present"]:
    r["cert_not_expiring_7d"]=cmd(["sudo","-n","openssl","x509","-checkend","604800","-noout","-in",cert])[0]==0
    a,cp=cmd(["sudo","-n","openssl","x509","-pubkey","-noout","-in",cert])
    b,kp=cmd(["sudo","-n","openssl","pkey","-pubout","-in",key])
    r["cert_key_match"]=a==0 and b==0 and cp.strip()==kp.strip()
code,raw=cmd(["curl","-fsS","--noproxy","*","--max-time","6","http://127.0.0.1:8790/_ils/health"])
if code==0:
    try: r["loopback_health_ok"]=json.loads(raw).get("ok") is True
    except ValueError: pass
if release.is_dir():
    plists=[p for p in release.glob("**/*.plist") if p.is_file()]
    ipas=[p for p in release.glob("**/*.ipa") if p.is_file()]
    r["release_plist_count"]=len(plists)
    r["release_ipa_count"]=len(ipas)
    for items,k in ((plists,"existing_plist_head_ok"),(ipas,"existing_ipa_head_ok")):
        if items:
            rel=items[0].relative_to(pathlib.Path("/srv/ils-adhoc-ota"))
            path="/"+str(rel)
            c,body=cmd(["curl","--silent","--output","/dev/null","--write-out","%{http_code} %{content_type}",
                "--noproxy","*","--resolve","ota.ioant.com:443:127.0.0.1","--max-time","8",
                "-I","https://ota.ioant.com"+path])
            parts=body.decode("ascii","replace").split()
            expected_mime="application/xml" if k=="existing_plist_head_ok" else "application/octet-stream"
            r[k]=c==0 and len(parts)>=2 and parts[0]=="200" and parts[1].split(";")[0]==expected_mime
print(json.dumps(r,sort_keys=True))
'''

def request(path, token=None):
    headers={"User-Agent":"ILS-OTA-ReadOnly-Audit/1"}
    if token is not None:
        headers["Authorization"]="Bearer "+token
    req=urllib.request.Request(BASE+path,headers=headers)
    try:
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req,timeout=12) as resp:
            raw=resp.read(1<<20)
            return resp.status,raw
    except urllib.error.HTTPError as e:
        return e.code,b""
    except (OSError,TimeoutError) as e:
        return None,b""

def main():
    if not os.environ.get("OPENCLAW_OUTPUT_DIR") or not OUT.is_dir():
        print("BLOCKED: OPENCLAW_OUTPUT_DIR unavailable");return 2
    report={"audit":"ILS_OTA_LIGHTSAIL_READONLY_V1","observed_utc":datetime.datetime.now(datetime.timezone.utc).isoformat(),
      "read_only":True,"target":HOST,"domain":"ota.ioant.com","result":"INCOMPLETE",
      "local_config_present":False,"local_config_host_correct":False,"local_public_url_correct":False,
      "local_token_available":False,"remote":None,"public":{},"authenticated_pending":None,
      "no_device_mutation":True,"no_service_restart":True}
    conf=ROOT/".localservice/ota-gateway.json"
    token=None
    try:
        cfg=json.loads(conf.read_text())
        report["local_config_present"]=True
        report["local_config_host_correct"]=cfg.get("ssh_host")==HOST
        report["local_public_url_correct"]=cfg.get("public_url")==BASE
        path=Path(cfg.get("sync_token_file",""))
        if path.is_file():
            token=path.read_text().strip()
            report["local_token_available"]=len(token)>=32
    except (OSError,ValueError,TypeError): pass
    for path in ("/","/enroll","/_ils/health","/api/ils/devices/pending"):
        status,body=request(path)
        row={"http_status":status}
        if path=="/_ils/health" and status==200:
            try:
                d=json.loads(body)
                row["service_ok"]=d.get("ok") is True and d.get("service")=="ils-adhoc-ota"
                row["profile_signed"]=d.get("profile_signed")
            except ValueError: row["service_ok"]=False
        if path=="/enroll" and status==200: row["form_present"]=b'enroll.mobileconfig' in body
        report["public"][path]=row
    if token and len(token)>=32:
        status,body=request("/api/ils/devices/pending",token)
        row={"http_status":status}
        if status==200:
            try:
                devices=json.loads(body)
                row["valid_response"]=isinstance(devices,list)
                row["pending_count"]=len(devices) if isinstance(devices,list) else None
            except ValueError:row["valid_response"]=False
        report["authenticated_pending"]=row
    if token and len(token)>=32 and KEY.is_file():
        sha=hashlib.sha256(token.encode()).hexdigest()
        code=REMOTE.replace("__TOKEN_SHA__",sha)
        try:
            proc=subprocess.run(["ssh","-i",str(KEY),"-o","BatchMode=yes","-o","IdentitiesOnly=yes",
               "-o","StrictHostKeyChecking=yes","-o","ConnectTimeout=10","ubuntu@"+HOST,"python3","-"],
               input=code,text=True,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=55)
            if proc.returncode==0:
                report["remote"]=json.loads(proc.stdout)
            else:report["remote_error"]="SSH_OR_REMOTE_INSPECTION_FAILED"
        except (OSError,subprocess.TimeoutExpired,ValueError):
            report["remote_error"]="SSH_OR_REMOTE_INSPECTION_FAILED"
    else:
        report["remote_error"]="LOCAL_TOKEN_OR_SSH_KEY_UNAVAILABLE"
    statuses=[report["public"].get(p,{}).get("http_status") for p in ("/","/enroll","/_ils/health")]
    if (statuses==[200,200,200] and report["local_config_host_correct"] and report["local_public_url_correct"]
        and report["public"]["/api/ils/devices/pending"]["http_status"]==401
        and report["public"]["/_ils/health"].get("service_ok") is True
        and report["public"]["/enroll"].get("form_present") is True
        and report["remote"] and report["remote"].get("token_matches_local")
        and report["authenticated_pending"] and report["authenticated_pending"].get("http_status")==200
        and report["authenticated_pending"].get("valid_response") is True):
        report["result"]="READONLY_HTTP_TOKEN_SYNC_PASS"
    else:
        report["result"]="READONLY_AUDIT_PARTIAL_REVIEW"
    (OUT/"ota_audit.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print("audit="+report["result"])
    print("public_status="+str(statuses))
    return 0

if __name__=="__main__":
    raise SystemExit(main())
