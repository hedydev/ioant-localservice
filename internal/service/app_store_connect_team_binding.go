package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AppStoreConnectTeamBinding struct {
	TeamID    string    `json:"team_id"`
	KeyID     string    `json:"key_id"`
	IssuerID  string    `json:"issuer_id,omitempty"`
	BoundAt   time.Time `json:"bound_at"`
}

type AppStoreConnectTeamBindingStatus struct {
	Configured bool   `json:"configured"`
	TeamID     string `json:"team_id,omitempty"`
	KeyID      string `json:"key_id,omitempty"`
	IssuerID   string `json:"issuer_id,omitempty"`
	KeyType    string `json:"key_type,omitempty"`
	CurrentKey bool   `json:"current_key"`
	Stale      bool   `json:"stale"`
	Message    string `json:"message,omitempty"`
}

func (a *App) appStoreConnectTeamBindingPath() string {
	return filepath.Join(a.data, "app-store-connect-team.json")
}

func normalizeAppleTeamID(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !appleTeamIDRE.MatchString(value) {
		return "", fmt.Errorf("Apple Team ID 必须是 10 位大写字母或数字")
	}
	return value, nil
}

func (a *App) readAppStoreConnectTeamBinding() (AppStoreConnectTeamBinding, error) {
	var binding AppStoreConnectTeamBinding
	raw, err := os.ReadFile(a.appStoreConnectTeamBindingPath())
	if err != nil {
		return binding, err
	}
	if err = json.Unmarshal(raw, &binding); err != nil {
		return binding, fmt.Errorf("App Store Connect Apple Team 绑定配置损坏")
	}
	binding.TeamID, err = normalizeAppleTeamID(binding.TeamID)
	if err != nil {
		return binding, err
	}
	binding.KeyID = strings.ToUpper(strings.TrimSpace(binding.KeyID))
	binding.IssuerID = strings.TrimSpace(binding.IssuerID)
	if !ascKeyIDRE.MatchString(binding.KeyID) {
		return binding, fmt.Errorf("App Store Connect Apple Team 绑定的 Key ID 无效")
	}
	return binding, nil
}

func (a *App) writeAppStoreConnectTeamBinding(cfg AppStoreConnectConfig, teamID string) (AppStoreConnectTeamBinding, error) {
	teamID, err := normalizeAppleTeamID(teamID)
	if err != nil {
		return AppStoreConnectTeamBinding{}, err
	}
	binding := AppStoreConnectTeamBinding{
		TeamID:   teamID,
		KeyID:    cfg.KeyID,
		IssuerID: cfg.IssuerID,
		BoundAt:  time.Now().UTC(),
	}
	if err = atomicJSON(a.appStoreConnectTeamBindingPath(), binding); err != nil {
		return AppStoreConnectTeamBinding{}, err
	}
	_ = os.Chmod(a.appStoreConnectTeamBindingPath(), 0600)
	return binding, nil
}

func appStoreConnectKeyType(cfg AppStoreConnectConfig) string {
	if strings.TrimSpace(cfg.IssuerID) == "" {
		return "individual"
	}
	return "team"
}

func (a *App) appStoreConnectTeamBindingStatus() AppStoreConnectTeamBindingStatus {
	status := AppStoreConnectTeamBindingStatus{}
	cfg, cfgErr := a.readAppStoreConnectConfig()
	if cfgErr != nil {
		status.Message = "请先配置 App Store Connect API Key"
		return status
	}
	status.KeyID = cfg.KeyID
	status.IssuerID = cfg.IssuerID
	status.KeyType = appStoreConnectKeyType(cfg)
	binding, err := a.readAppStoreConnectTeamBinding()
	if err != nil {
		if os.IsNotExist(err) {
			status.Message = "当前 App Store Connect API Key 尚未绑定 Apple Team"
		} else {
			status.Message = err.Error()
		}
		return status
	}
	status.TeamID = binding.TeamID
	status.CurrentKey = binding.KeyID == cfg.KeyID && binding.IssuerID == cfg.IssuerID
	status.Stale = !status.CurrentKey
	status.Configured = status.CurrentKey
	if status.Stale {
		status.Message = "App Store Connect API Key 已变更，请重新绑定 Apple Team"
	} else {
		status.Message = "当前 API Key 已绑定 Apple Team " + binding.TeamID
	}
	return status
}

func (a *App) validateAppStoreConnectTeamSelection(cfg AppStoreConnectConfig, selectedTeamID string) error {
	selectedTeamID, err := normalizeAppleTeamID(selectedTeamID)
	if err != nil {
		return err
	}
	binding, err := a.readAppStoreConnectTeamBinding()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("请先在 App Store Connect 配置中绑定 Apple Team")
		}
		return err
	}
	if binding.KeyID != cfg.KeyID || binding.IssuerID != cfg.IssuerID {
		return fmt.Errorf("当前 App Store Connect API Key 已变更，请重新绑定 Apple Team")
	}
	if binding.TeamID != selectedTeamID {
		return fmt.Errorf("当前 App Store Connect API Key 绑定 Apple Team %s，不能注册到 Team %s", binding.TeamID, selectedTeamID)
	}
	return nil
}

func (a *App) appStoreConnectTeamBinding(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		respond(w, http.StatusOK, a.appStoreConnectTeamBindingStatus())
		return
	case http.MethodDelete:
		if err := os.Remove(a.appStoreConnectTeamBindingPath()); err != nil && !os.IsNotExist(err) {
			fail(w, http.StatusInternalServerError, "移除 App Store Connect Apple Team 绑定失败")
			return
		}
		respond(w, http.StatusOK, a.appStoreConnectTeamBindingStatus())
		return
	}

	cfg, err := a.readAppStoreConnectConfig()
	if err != nil {
		fail(w, http.StatusConflict, "请先配置 App Store Connect API Key")
		return
	}
	var input struct {
		TeamID string `json:"team_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "请选择 Apple Team")
		return
	}
	if _, err = a.writeAppStoreConnectTeamBinding(cfg, input.TeamID); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(w, http.StatusOK, a.appStoreConnectTeamBindingStatus())
}
