package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ascDeviceAttributes struct {
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	UDID        string `json:"udid"`
	DeviceClass string `json:"deviceClass"`
	Status      string `json:"status"`
	Model       string `json:"model"`
	AddedDate   string `json:"addedDate"`
}

func appleRegistrationStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ENABLED":
		return "registered"
	case "PROCESSING":
		return "processing"
	case "DISABLED":
		return "disabled"
	case "INELIGIBLE":
		return "ineligible"
	default:
		return "pending"
	}
}

func appleDeviceDisplayName(d Device) string {
	product := strings.TrimSpace(d.Product)
	if product == "" {
		product = "iOS Device"
	}
	suffix := d.UDID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return "ILS " + product + " " + suffix
}

func lookupAppleDevice(ctx context.Context, cfg AppStoreConnectConfig, udid string) (ascResource, ascDeviceAttributes, bool, error) {
	query := url.Values{}
	query.Set("filter[udid]", udid)
	query.Set("filter[platform]", "IOS")
	query.Set("fields[devices]", "name,platform,udid,deviceClass,status,model,addedDate")
	query.Set("limit", "2")
	var response ascListResponse
	if e := ascGET(ctx, cfg, "/v1/devices", query, &response); e != nil {
		return ascResource{}, ascDeviceAttributes{}, false, e
	}
	if len(response.Data) == 0 {
		return ascResource{}, ascDeviceAttributes{}, false, nil
	}
	if len(response.Data) > 1 {
		return ascResource{}, ascDeviceAttributes{}, false, fmt.Errorf("Apple Developer 中 UDID %s 匹配多个设备", udid)
	}
	resource := response.Data[0]
	var attributes ascDeviceAttributes
	if e := json.Unmarshal(resource.Attributes, &attributes); e != nil {
		return ascResource{}, ascDeviceAttributes{}, false, fmt.Errorf("Apple Developer 设备响应格式无效")
	}
	return resource, attributes, true, nil
}

func createAppleDevice(ctx context.Context, cfg AppStoreConnectConfig, d Device) (ascResource, ascDeviceAttributes, error) {
	body := map[string]any{
		"data": map[string]any{
			"type": "devices",
			"attributes": map[string]any{
				"name":     appleDeviceDisplayName(d),
				"platform": "IOS",
				"udid":     d.UDID,
			},
		},
	}
	var response ascSingleResponse
	if e := ascPOST(ctx, cfg, "/v1/devices", body, &response); e != nil {
		return ascResource{}, ascDeviceAttributes{}, e
	}
	var attributes ascDeviceAttributes
	if e := json.Unmarshal(response.Data.Attributes, &attributes); e != nil {
		return ascResource{}, ascDeviceAttributes{}, fmt.Errorf("Apple Developer 设备响应格式无效")
	}
	return response.Data, attributes, nil
}

func (a *App) persistAppleDeviceRegistration(d Device, teamID string, resource ascResource, attributes ascDeviceAttributes, lastErr error) (Device, error) {
	if d.AppleRegistrations == nil {
		d.AppleRegistrations = map[string]AppleDeviceRegistration{}
	}
	now := time.Now().UTC()
	previous := d.AppleRegistrations[teamID]
	registration := AppleDeviceRegistration{
		TeamID:        teamID,
		DeviceID:      resource.ID,
		Name:          attributes.Name,
		Status:        appleRegistrationStatus(attributes.Status),
		AppleStatus:   strings.ToUpper(strings.TrimSpace(attributes.Status)),
		RegisteredAt:  previous.RegisteredAt,
		LastCheckedAt: now,
	}
	if lastErr != nil {
		registration.Status = "failed"
		registration.LastError = lastErr.Error()
		if registration.DeviceID == "" {
			registration.DeviceID = previous.DeviceID
		}
		if registration.Name == "" {
			registration.Name = previous.Name
		}
		if registration.AppleStatus == "" {
			registration.AppleStatus = previous.AppleStatus
		}
	}
	if registration.Status == "registered" && registration.RegisteredAt == nil {
		registeredAt := now
		registration.RegisteredAt = &registeredAt
	}
	d.AppleRegistrations[teamID] = registration

	switch registration.Status {
	case "registered":
		d.Status = "apple_registered"
	case "processing":
		d.Status = "apple_registration_processing"
	case "disabled", "ineligible", "failed":
		d.Status = "apple_registration_attention"
	default:
		d.Status = "pending_apple_registration"
	}
	if deviceHasRegisteredAppleTeam(d) {
		d.Status = "apple_registered"
	}
	_, e := a.saveDeviceRecord(d)
	return d, e
}

func (a *App) registerDeviceWithApple(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	udid := strings.TrimSpace(r.PathValue("udid"))
	if !udidRE.MatchString(udid) {
		fail(w, http.StatusBadRequest, "设备 UDID 无效")
		return
	}
	var input struct {
		TeamID string `json:"team_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&input); e != nil {
		fail(w, http.StatusBadRequest, "请选择 Apple Team")
		return
	}
	teamID := strings.ToUpper(strings.TrimSpace(input.TeamID))
	if !appleTeamIDRE.MatchString(teamID) {
		fail(w, http.StatusBadRequest, "Apple Team ID 必须是 10 位大写字母或数字")
		return
	}

	cfg, e := a.readAppStoreConnectConfig()
	if e != nil {
		fail(w, http.StatusConflict, "请先配置 App Store Connect API Key")
		return
	}
	a.enrollmentMu.Lock()
	d, e := a.readDeviceRecord(udid)
	a.enrollmentMu.Unlock()
	if e != nil {
		if strings.Contains(e.Error(), "UDID") {
			fail(w, http.StatusBadRequest, e.Error())
		} else {
			fail(w, http.StatusNotFound, "设备记录不存在，请先完成设备登记")
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resource, attributes, found, e := lookupAppleDevice(ctx, cfg, d.UDID)
	if e == nil && !found {
		resource, attributes, e = createAppleDevice(ctx, cfg, d)
	}
	if e != nil {
		a.enrollmentMu.Lock()
		_, saveErr := a.persistAppleDeviceRegistration(d, teamID, resource, attributes, e)
		a.enrollmentMu.Unlock()
		if saveErr != nil {
			fail(w, http.StatusInternalServerError, "Apple 注册失败，且无法保存设备状态")
			return
		}
		fail(w, http.StatusBadGateway, e.Error())
		return
	}

	a.enrollmentMu.Lock()
	updated, saveErr := a.persistAppleDeviceRegistration(d, teamID, resource, attributes, nil)
	a.enrollmentMu.Unlock()
	if saveErr != nil {
		fail(w, http.StatusInternalServerError, "设备已由 Apple 接受，但 ILS 无法保存注册状态")
		return
	}
	respond(w, http.StatusOK, updated)
}
