package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type IOSSimulatorDevice struct {
	UDID              string `json:"udid"`
	Name              string `json:"name"`
	Runtime           string `json:"runtime"`
	RuntimeIdentifier string `json:"runtime_identifier"`
	State             string `json:"state"`
	DeviceType        string `json:"device_type,omitempty"`
}

type simctlDevice struct {
	UDID                 string `json:"udid"`
	Name                 string `json:"name"`
	State                string `json:"state"`
	IsAvailable          bool   `json:"isAvailable"`
	DeviceTypeIdentifier string `json:"deviceTypeIdentifier"`
}

type simctlDeviceList struct {
	Devices map[string][]simctlDevice `json:"devices"`
}

func iosRuntimeLabel(identifier string) string {
	value := strings.TrimPrefix(identifier, "com.apple.CoreSimulator.SimRuntime.")
	value = strings.TrimPrefix(value, "iOS-")
	value = strings.ReplaceAll(value, "-", ".")
	if value == "" || value == identifier {
		return identifier
	}
	return "iOS " + value
}

func runtimeVersionParts(identifier string) []int {
	value := strings.TrimPrefix(identifier, "com.apple.CoreSimulator.SimRuntime.iOS-")
	parts := strings.Split(value, "-")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func compareRuntimeDesc(a, b string) int {
	av, bv := runtimeVersionParts(a), runtimeVersionParts(b)
	max := len(av)
	if len(bv) > max {
		max = len(bv)
	}
	for i := 0; i < max; i++ {
		ai, bi := 0, 0
		if i < len(av) {
			ai = av[i]
		}
		if i < len(bv) {
			bi = bv[i]
		}
		if ai > bi {
			return -1
		}
		if ai < bi {
			return 1
		}
	}
	return 0
}

func listIOSSimulators(ctx context.Context) ([]IOSSimulatorDevice, error) {
	xcrun, err := exec.LookPath("xcrun")
	if err != nil {
		return nil, fmt.Errorf("未找到 Xcode xcrun；请先安装并选择 Xcode")
	}
	cmd := exec.CommandContext(ctx, xcrun, "simctl", "list", "devices", "available", "-j")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 iOS Simulator 设备失败")
	}
	var payload simctlDeviceList
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("无法解析 iOS Simulator 设备列表")
	}
	devices := []IOSSimulatorDevice{}
	for runtimeID, values := range payload.Devices {
		if !strings.Contains(runtimeID, ".SimRuntime.iOS-") {
			continue
		}
		for _, device := range values {
			if !device.IsAvailable || strings.TrimSpace(device.UDID) == "" {
				continue
			}
			devices = append(devices, IOSSimulatorDevice{
				UDID:              strings.TrimSpace(device.UDID),
				Name:              strings.TrimSpace(device.Name),
				Runtime:           iosRuntimeLabel(runtimeID),
				RuntimeIdentifier: runtimeID,
				State:             strings.TrimSpace(device.State),
				DeviceType:        strings.TrimSpace(device.DeviceTypeIdentifier),
			})
		}
	}
	sort.SliceStable(devices, func(i, j int) bool {
		iBooted := strings.EqualFold(devices[i].State, "Booted")
		jBooted := strings.EqualFold(devices[j].State, "Booted")
		if iBooted != jBooted {
			return iBooted
		}
		if cmp := compareRuntimeDesc(devices[i].RuntimeIdentifier, devices[j].RuntimeIdentifier); cmp != 0 {
			return cmp < 0
		}
		if devices[i].Name != devices[j].Name {
			return devices[i].Name < devices[j].Name
		}
		return devices[i].UDID < devices[j].UDID
	})
	return devices, nil
}

func resolveIOSSimulator(ctx context.Context, requested string) (IOSSimulatorDevice, error) {
	devices, err := listIOSSimulators(ctx)
	if err != nil {
		return IOSSimulatorDevice{}, err
	}
	if len(devices) == 0 {
		return IOSSimulatorDevice{}, fmt.Errorf("没有可用的 iOS Simulator；请先在 Xcode 安装 iOS Simulator Runtime")
	}
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, device := range devices {
			if device.UDID == requested {
				return device, nil
			}
		}
		return IOSSimulatorDevice{}, fmt.Errorf("指定的 iOS Simulator 当前不可用，请重新选择设备")
	}
	for _, device := range devices {
		if strings.EqualFold(device.State, "Booted") {
			return device, nil
		}
	}
	return devices[0], nil
}

func bootIOSSimulator(ctx context.Context, device IOSSimulatorDevice) error {
	xcrun, err := exec.LookPath("xcrun")
	if err != nil {
		return fmt.Errorf("未找到 Xcode xcrun；请先安装并选择 Xcode")
	}
	if !strings.EqualFold(device.State, "Booted") {
		output, bootErr := exec.CommandContext(ctx, xcrun, "simctl", "boot", device.UDID).CombinedOutput()
		if bootErr != nil && !strings.Contains(strings.ToLower(string(output)), "current state: booted") {
			return fmt.Errorf("启动 Simulator 失败：%s", shortCommandOutput(output))
		}
	}
	output, err := exec.CommandContext(ctx, xcrun, "simctl", "bootstatus", device.UDID, "-b").CombinedOutput()
	if err != nil {
		return fmt.Errorf("等待 Simulator 启动失败：%s", shortCommandOutput(output))
	}
	// Bringing Simulator.app forward is best-effort. simctl remains the source
	// of truth for install/launch even if the GUI app cannot be activated.
	_ = exec.Command("/usr/bin/open", "-a", "Simulator").Start()
	return nil
}

func (a *App) iosSimulators(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	devices, err := listIOSSimulators(ctx)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	respond(w, http.StatusOK, map[string]any{"devices": devices})
}
