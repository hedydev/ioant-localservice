package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SimulatorProfileDevice struct {
	DeviceID string `json:"device_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Runtime  string `json:"runtime,omitempty"`
}

func (a *App) simulatorProfileDevicePath(project, profile string) string {
	return filepath.Join(a.data, "simulator-profile-devices", project, profile+".json")
}

func (a *App) simulatorBuildDevicePath(job string) string {
	return filepath.Join(a.data, "builds", job, "simulator-device.json")
}

func (a *App) readSimulatorProfileDevice(project, profile string) SimulatorProfileDevice {
	var value SimulatorProfileDevice
	raw, err := os.ReadFile(a.simulatorProfileDevicePath(project, profile))
	if err == nil {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func (a *App) readSimulatorBuildDevice(job string) SimulatorProfileDevice {
	var value SimulatorProfileDevice
	raw, err := os.ReadFile(a.simulatorBuildDevicePath(job))
	if err == nil {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func simulatorProfileDeviceFromResolved(device IOSSimulatorDevice) SimulatorProfileDevice {
	return SimulatorProfileDevice{DeviceID: device.UDID, Name: device.Name, Runtime: device.Runtime}
}

func (a *App) writeSimulatorBuildDevice(job string, device IOSSimulatorDevice) error {
	return atomicJSON(a.simulatorBuildDevicePath(job), simulatorProfileDeviceFromResolved(device))
}

func (a *App) simulatorProfileDevices(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	project := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(project)
	a.mu.RUnlock()
	if !exists {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	profiles, err := a.listReleaseProfiles(project)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取 Release Profiles 失败")
		return
	}
	out := map[string]SimulatorProfileDevice{}
	for _, profile := range profiles {
		if profile.Platform == "ios" && profile.Lane == "ios-simulator" {
			out[profile.ID] = a.readSimulatorProfileDevice(project, profile.ID)
		}
	}
	respond(w, http.StatusOK, out)
}

func (a *App) simulatorBuildDevices(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	project := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(project)
	a.mu.RUnlock()
	if !exists {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	out := map[string]SimulatorProfileDevice{}
	paths, _ := filepath.Glob(filepath.Join(a.data, "builds", "*", "job.json"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var job BuildJob
		if json.Unmarshal(raw, &job) != nil || job.ProjectID != project || job.Lane != "ios-simulator" {
			continue
		}
		value := a.readSimulatorBuildDevice(job.ID)
		if value.DeviceID != "" {
			out[job.ID] = value
		}
	}
	respond(w, http.StatusOK, out)
}

func (a *App) setSimulatorProfileDevice(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	project, profileID := r.PathValue("project"), r.PathValue("profile")
	if !slugRE.MatchString(project) || !slugRE.MatchString(profileID) {
		fail(w, http.StatusNotFound, "Simulator Profile 不存在")
		return
	}
	profile, err := a.readReleaseProfile(project, profileID)
	if err != nil || profile.Platform != "ios" || profile.Lane != "ios-simulator" {
		fail(w, http.StatusNotFound, "Simulator Profile 不存在")
		return
	}
	var input struct {
		DeviceID string `json:"device_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "Simulator 设备配置无效")
		return
	}
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	value := SimulatorProfileDevice{}
	if input.DeviceID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		device, resolveErr := resolveIOSSimulator(ctx, input.DeviceID)
		if resolveErr != nil {
			fail(w, http.StatusConflict, resolveErr.Error())
			return
		}
		value = simulatorProfileDeviceFromResolved(device)
	}
	path := a.simulatorProfileDevicePath(project, profileID)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		fail(w, http.StatusInternalServerError, "保存 Simulator 设备失败")
		return
	}
	if value.DeviceID == "" {
		_ = os.Remove(path)
	} else if err = atomicJSON(path, value); err != nil {
		fail(w, http.StatusInternalServerError, "保存 Simulator 设备失败")
		return
	}
	respond(w, http.StatusOK, value)
}

func (a *App) deleteReleaseProfileLocalV2(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	project, profileID := r.PathValue("project"), r.PathValue("profile")
	if !slugRE.MatchString(project) || !slugRE.MatchString(profileID) {
		fail(w, http.StatusNotFound, "Release Profile 不存在")
		return
	}
	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != "" {
		fail(w, http.StatusConflict, "构建期间不能删除 Release Profile")
		return
	}
	if err := os.Remove(a.releaseProfilePath(project, profileID)); err != nil {
		if os.IsNotExist(err) {
			fail(w, http.StatusNotFound, "Release Profile 不存在")
		} else {
			fail(w, http.StatusInternalServerError, "删除 Release Profile 失败")
		}
		return
	}
	_ = os.Remove(a.releaseProfileMetaPath(project, profileID))
	_ = os.Remove(a.simulatorProfileDevicePath(project, profileID))
	respond(w, http.StatusOK, map[string]bool{"deleted": true})
}
