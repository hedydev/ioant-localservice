#!/usr/bin/env python3
"""Read only public signing metadata; never export private keys or account tokens."""
import json
import pathlib
import plistlib
import subprocess

subprocess.run(["security", "find-identity", "-v", "-p", "codesigning"], check=True)
for root in [pathlib.Path.home() / "Library/Developer/Xcode/UserData/Provisioning Profiles",
             pathlib.Path.home() / "Library/MobileDevice/Provisioning Profiles"]:
    for path in root.glob("*.mobileprovision"):
        result = subprocess.run(["security", "cms", "-D", "-i", str(path)], capture_output=True)
        if result.returncode:
            continue
        p = plistlib.loads(result.stdout)
        print(json.dumps({"name": p.get("Name"), "team": p.get("TeamIdentifier"),
                          "application": p.get("Entitlements", {}).get("application-identifier"),
                          "development": p.get("Entitlements", {}).get("get-task-allow"),
                          "devices": len(p.get("ProvisionedDevices", [])),
                          "expires": str(p.get("ExpirationDate"))}, ensure_ascii=False))
