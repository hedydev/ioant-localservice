#!/usr/bin/env bash
# Deterministic local-only checks; no SSH or deployment.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
OUT="${OPENCLAW_OUTPUT_DIR:?OPENCLAW_OUTPUT_DIR required}"
report="$OUT/ota_validation.json"
pass_syntax=false
pass_unit=false
pass_go=false
pass_build=false
bash -n scripts/deploy-adhoc-ota-gateway.sh scripts/finish-ota-profile-signing.sh scripts/ota-lightsail-validate.sh && pass_syntax=true
python3 scripts/ota-lightsail-self-test.py >/dev/null && pass_unit=true
go test ./... >/dev/null && pass_go=true
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/ils-ota-gateway" ./cmd/ota-gateway && pass_build=true
export OTA_VALIDATION_SYNTAX="$pass_syntax" OTA_VALIDATION_TESTS="$pass_unit" OTA_VALIDATION_GO="$pass_go" OTA_VALIDATION_BUILD="$pass_build"
python3 - "$report" <<'PY'
import json,os,sys
d={k:os.environ["OTA_VALIDATION_"+k] == "true" for k in ("SYNTAX","TESTS","GO","BUILD")}
d["result"]="PASS" if all(d.values()) else "FAIL"
d["read_only_production"]=True
open(sys.argv[1],"w").write(json.dumps(d,indent=2)+"\n")
print("ILS_OTA_LOCAL_VALIDATION_"+d["result"])
PY
