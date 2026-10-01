package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type IOSInfo struct {
	BundleID          string     `json:"bundle_id"`
	Version           string     `json:"version"`
	Build             string     `json:"build"`
	ProfileType       string     `json:"profile_type"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	DeviceCount       int        `json:"device_count"`
	TeamID            string     `json:"team_id,omitempty"`
	SignatureVerified bool       `json:"signature_verified"`
	DeviceTargets     []string   `json:"device_targets,omitempty"`
}

// Profile metadata is an installation prerequisite, never a signature verification.
func (i *IOSInfo) OTAEligible() bool {
	return oneOf(i.ProfileType, "ad-hoc", "enterprise") && i.ExpiresAt != nil && i.ExpiresAt.After(time.Now())
}

var appInfoRE = regexp.MustCompile(`^Payload/[^/]+\.app/Info\.plist$`)

func readZipLimit(f *zip.File, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) {
		return nil, fmt.Errorf("ZIP 条目过大")
	}
	r, e := f.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, max+1))
	if int64(len(b)) > max {
		return nil, fmt.Errorf("ZIP 条目过大")
	}
	return b, e
}

func readZip(f *zip.File) ([]byte, error) {
	return readZipLimit(f, 4<<20)
}
func plistJSON(raw []byte, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/plutil", "-convert", "json", "-o", "-", "--", "-")
	cmd.Stdin = bytes.NewReader(raw)
	b, e := cmd.Output()
	if e != nil {
		return fmt.Errorf("无法读取 plist（需要 macOS plutil）")
	}
	return json.Unmarshal(b, out)
}
func inspectIPA(path string) (*IOSInfo, error) {
	z, e := zip.OpenReader(path)
	if e != nil {
		return nil, fmt.Errorf("不是有效 ZIP / IPA")
	}
	defer z.Close()
	var entry *zip.File
	for _, f := range z.File {
		if appInfoRE.MatchString(f.Name) {
			if entry != nil {
				return nil, fmt.Errorf("IPA 中包含多个主应用")
			}
			entry = f
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("缺少 Payload/*.app/Info.plist")
	}
	raw, e := readZip(entry)
	if e != nil {
		return nil, e
	}
	var info map[string]any
	if e = plistJSON(raw, &info); e != nil {
		return nil, e
	}
	str := func(key string) string { v, _ := info[key].(string); return v }
	out := &IOSInfo{BundleID: str("CFBundleIdentifier"), Version: str("CFBundleShortVersionString"), Build: str("CFBundleVersion"), ProfileType: "unsigned"}
	if families, ok := info["UIDeviceFamily"].([]any); ok {
		for _, value := range families {
			number, ok := value.(float64)
			if !ok {
				continue
			}
			switch int(number) {
			case 1:
				if !oneOf("iphone", out.DeviceTargets...) {
					out.DeviceTargets = append(out.DeviceTargets, "iphone")
				}
			case 2:
				if !oneOf("ipad", out.DeviceTargets...) {
					out.DeviceTargets = append(out.DeviceTargets, "ipad")
				}
			}
		}
	}
	if out.BundleID == "" || out.Version == "" || out.Build == "" {
		return nil, fmt.Errorf("IPA 缺少 Bundle ID 或版本信息")
	}
	profilePath := strings.TrimSuffix(entry.Name, "Info.plist") + "embedded.mobileprovision"
	for _, f := range z.File {
		if f.Name != profilePath {
			continue
		}
		raw, e = readZip(f)
		if e != nil {
			return nil, e
		}
		temp, e := os.CreateTemp("", "localservice-profile-*")
		if e != nil {
			return nil, e
		}
		defer os.Remove(temp.Name())
		_, e = temp.Write(raw)
		closeErr := temp.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		decoded, e := exec.CommandContext(ctx, "/usr/bin/security", "cms", "-D", "-i", temp.Name()).Output()
		if e != nil {
			return nil, fmt.Errorf("无法解码 embedded.mobileprovision")
		}
		// plutil JSON conversion does not support plist date/data values. Extract
		// just the relevant fields through plutil, converting expiration to raw text.
		get := func(key string) string {
			c := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", key, "raw", "-o", "-", "--", "-")
			c.Stdin = bytes.NewReader(decoded)
			b, e := c.Output()
			if e != nil {
				return ""
			}
			return strings.TrimSpace(string(b))
		}
		out.ProfileType = "app-store"
		out.TeamID = get("TeamIdentifier.0")
		devicesCmd := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", "ProvisionedDevices", "json", "-o", "-", "--", "-")
		devicesCmd.Stdin = bytes.NewReader(decoded)
		d, _ := devicesCmd.Output()
		var devices []string
		_ = json.Unmarshal(d, &devices)
		out.DeviceCount = len(devices)
		if get("ProvisionsAllDevices") == "true" {
			out.ProfileType = "enterprise"
		} else if len(devices) > 0 {
			out.ProfileType = "ad-hoc"
		}
		if get("Entitlements.get-task-allow") == "true" {
			out.ProfileType = "development"
		}
		expiry := get("ExpirationDate")
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02T15:04:05Z"} {
			if t, e := time.Parse(layout, expiry); e == nil {
				out.ExpiresAt = &t
				break
			}
		}
		break
	}
	return out, nil
}


func extractIPAAppIcon(path string) ([]byte, error) {
	z, e := zip.OpenReader(path)
	if e != nil {
		return nil, e
	}
	defer z.Close()

	var prefix string
	for _, f := range z.File {
		if appInfoRE.MatchString(f.Name) {
			prefix = strings.TrimSuffix(f.Name, "Info.plist")
			break
		}
	}
	if prefix == "" {
		return nil, fmt.Errorf("IPA 缺少主应用")
	}

	var best []byte
	var bestScore uint64
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, prefix) {
			continue
		}
		relative := strings.TrimPrefix(f.Name, prefix)
		if relative == "" || strings.Contains(relative, "/") || !strings.HasSuffix(strings.ToLower(relative), ".png") {
			continue
		}
		name := strings.ToLower(relative)
		if !strings.Contains(name, "appicon") && !strings.HasPrefix(name, "icon") {
			continue
		}
		raw, e := readZipLimit(f, maxAppIconBytes)
		if e != nil || !pngBytes(raw) {
			continue
		}
		score := f.UncompressedSize64
		if strings.Contains(name, "appicon") {
			score += 1 << 40
		}
		if score > bestScore {
			bestScore = score
			best = raw
		}
	}
	if len(best) == 0 {
		return nil, fmt.Errorf("IPA 未找到可用 App Icon PNG")
	}
	return best, nil
}
