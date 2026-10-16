package web

import (
	"strings"
	"testing"
)

func TestServicesManagementLayout(t *testing.T) {
	servicesRaw, err := Files.ReadFile("js/services.js")
	if err != nil {
		t.Fatal(err)
	}
	services := string(servicesRaw)
	for _, required := range []string{
		"services.css?v=20261016-times",
		"function ensureServicesLayout()",
		"heading.after(gateway)",
		"ota-gateway-card",
		"device-management-card",
		"device-registry-table-wrap",
		"<table class=\"device-registry-table\"",
		"<th>设备</th><th>系统 / 来源</th><th>登记 / 更新（本地）</th><th>UDID</th><th>Apple Team</th><th>Apple 注册状态</th><th>操作</th>",
		"String(device.name||'').trim()",
		"device.registered_at||device.collected_at",
		"device.updated_at||device.collected_at",
		"toLocaleString('zh-CN'",
		"data-apple-register",
		"刷新 Apple 状态",
	} {
		if !strings.Contains(services, required) {
			t.Fatalf("services.js missing management UI contract %q", required)
		}
	}
	if strings.Contains(services, "再次通过 ota.ioant.com 登记可设置设备名称") {
		t.Fatal("device rows must not show instructional helper copy")
	}

	cssRaw, err := Files.ReadFile("services.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssRaw)
	for _, required := range []string{
		"#view-services>.ota-gateway-card",
		"grid-template-columns:minmax(0,1fr) auto",
		"#view-services>.device-management-card",
		".device-registry-table",
		"position:sticky",
		".device-time-cell",
		".device-team-select",
		".device-action-cell",
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("services.css missing management layout %q", required)
		}
	}
}
