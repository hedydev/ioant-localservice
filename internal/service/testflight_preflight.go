package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var appStoreBundleIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{1,254}$`)

type projectASCManifest struct {
	AppStoreConnect struct {
		Profiles map[string]struct {
			BundleID string `json:"bundle_id"`
		} `json:"profiles"`
	} `json:"app_store_connect"`
}

func projectTestFlightBundleID(sourcePath, profileID string) (string, error) {
	path := filepath.Join(sourcePath, ".ils", "project.json")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("无法读取项目 .ils/project.json")
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, (256<<10)+1))
	if err != nil || len(raw) > 256<<10 {
		return "", fmt.Errorf("项目 .ils/project.json 无法读取或过大")
	}
	var manifest projectASCManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("项目 .ils/project.json 格式无效")
	}
	entry, ok := manifest.AppStoreConnect.Profiles[profileID]
	if !ok {
		return "", nil
	}
	bundleID := strings.TrimSpace(entry.BundleID)
	if bundleID == "" {
		return "", nil
	}
	if !appStoreBundleIDRE.MatchString(bundleID) || strings.Contains(bundleID, "..") {
		return "", fmt.Errorf("项目 App Store Connect Bundle ID 无效：%s", bundleID)
	}
	return bundleID, nil
}

func (a *App) previousTestFlightBundleID(project string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var newest Release
	found := false
	for _, release := range a.state.Releases {
		if release.ProjectID != project || release.Delivery != "testflight" || strings.TrimSpace(release.BundleID) == "" {
			continue
		}
		if !found || release.CreatedAt.After(newest.CreatedAt) {
			newest = release
			found = true
		}
	}
	if !found {
		return ""
	}
	return strings.TrimSpace(newest.BundleID)
}

func (a *App) verifyTestFlightAppRecord(ctx context.Context, project, profileID, sourcePath string) (string, error) {
	bundleID, err := projectTestFlightBundleID(sourcePath, profileID)
	if err != nil {
		return "", err
	}
	if bundleID == "" {
		bundleID = a.previousTestFlightBundleID(project)
	}
	if bundleID == "" {
		return "", fmt.Errorf("TestFlight 构建缺少 App Store Connect Bundle ID；请在项目 .ils/project.json 的 app_store_connect.profiles.%s.bundle_id 中配置", profileID)
	}

	cfg, err := a.readAppStoreConnectConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("TestFlight 构建前检查失败：ILS 尚未配置 App Store Connect")
		}
		return "", fmt.Errorf("TestFlight 构建前检查失败：%v", err)
	}
	resolver := &ascResolver{cfg: cfg, appIDs: map[string]string{}, preIDs: map[string]string{}}
	appID, err := resolver.appID(ctx, bundleID)
	if err != nil {
		a.recordASCCheck(err)
		return "", fmt.Errorf("App Store Connect App 检查失败：%v", err)
	}
	a.recordASCCheck(nil)
	if appID == "" {
		return "", fmt.Errorf("App Store Connect 尚未创建 Bundle ID %s 对应的 App；请先在 Apps → + → New App 创建 iOS App", bundleID)
	}
	return bundleID, nil
}
