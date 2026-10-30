package service

import (
  "strings"
  "testing"
)

func TestOTAGatewayRejectsStoppedEC2AndUnrelatedLightsail(t *testing.T) {
  for _, host := range []string{"52.77.167.119", "13.228.218.141"} {
    cfg := OTAGatewayConfig{
      PublicURL: "https://ota.ioant.com",
      SSHHost: host,
      SSHUser: "ubuntu",
      SSHKeyPath: "/unused-private-key",
      RemoteRoot: "/srv/ils-adhoc-ota",
      SyncTokenFile: "/unused-token",
    }
    _, err := normalizeOTAGatewayConfig(cfg)
    if err == nil || !strings.Contains(err.Error(), "SSH target is retired") {
      t.Fatalf("host %s must be rejected before attempting sync, got %v",host,err)
    }
  }
}
