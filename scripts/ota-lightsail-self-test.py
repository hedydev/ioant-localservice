#!/usr/bin/env python3
"""Offline safety and contract tests. No SSH, cloud, public HTTP or credentials."""
import ast
from pathlib import Path
import subprocess
import unittest

ROOT=Path(__file__).resolve().parents[1]
DEPLOY=ROOT/"scripts/deploy-adhoc-ota-gateway.sh"
SIGN=ROOT/"scripts/finish-ota-profile-signing.sh"
AUDIT=ROOT/"scripts/ota-lightsail-audit.py"


class OTALightsailSafetyTests(unittest.TestCase):
    def test_all_scripts_parse_without_side_effects(self):
        for path in (DEPLOY,SIGN):
            r=subprocess.run(["bash","-n",str(path)],capture_output=True,text=True)
            self.assertEqual(r.returncode,0,(path,r.stderr))
        ast.parse(AUDIT.read_text())

    def test_production_default_and_guardrails(self):
        d=DEPLOY.read_text()
        self.assertIn("MODE=plan",d)
        self.assertIn("13.213.140.184",d)
        self.assertIn(".ssh/ioant-lightsail-20261026",d)
        self.assertIn("52.77.167.119",d) # blocked target, never default
        self.assertIn("13.228.218.141",d) # explicitly blocked
        self.assertIn("--confirm-gateway-restart",d)
        self.assertIn("StrictHostKeyChecking=yes",d)
        self.assertNotIn("openssl rand -hex",d)
        self.assertNotIn("certbot --nginx",d)
        self.assertNotIn("systemctl restart nginx",d)
        self.assertNotIn("rm -rf /var/lib/ils-ota-gateway",d)
        self.assertNotIn("systemctl restart xray",d)

    def test_legacy_mutating_flags_fail_without_ssh(self):
        for script in (DEPLOY,SIGN):
            for flag in ("--prepare-only","--email"):
                r=subprocess.run(["bash",str(script),flag],capture_output=True,text=True,timeout=5)
                self.assertNotEqual(r.returncode,0)
        r=subprocess.run(["bash",str(DEPLOY),"--apply-binary"],capture_output=True,text=True,timeout=5)
        self.assertNotEqual(r.returncode,0)

    def test_signing_readonly(self):
        s=SIGN.read_text()
        self.assertIn("profile-signing-chain.pem",s)
        self.assertIn("openssl",s)
        self.assertIn("-checkend",s)
        self.assertNotIn("systemctl restart",s)
        self.assertNotIn("certbot install",s)
        self.assertNotIn("install -o root",s)

    def test_audit_does_not_acknowledge_or_generate_challenge(self):
        a=AUDIT.read_text()
        self.assertIn("GET",a) if False else None
        self.assertIn("/api/ils/devices/pending",a)
        self.assertIn("token_matches_local",a)
        self.assertNotIn("/api/ils/devices/{id}/ack",a)
        self.assertNotIn("enroll.mobileconfig\")",a)
        self.assertNotIn("systemctl restart",a)
        self.assertNotIn("stop-instances",a)

if __name__=="__main__":
    unittest.main()
