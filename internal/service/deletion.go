package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var historyRecordIDRE = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (a *App) deleteBuildRecord(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("job")
	if !historyRecordIDRE.MatchString(id) {
		fail(w, http.StatusNotFound, "构建任务不存在")
		return
	}

	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild == id {
		fail(w, http.StatusConflict, "正在运行的构建任务不能删除；请先等待完成或停止任务")
		return
	}

	jobDir := filepath.Join(a.data, "builds", id)
	info, err := os.Stat(jobDir)
	if err != nil {
		if os.IsNotExist(err) {
			fail(w, http.StatusNotFound, "构建任务不存在")
		} else {
			fail(w, http.StatusInternalServerError, "读取构建任务失败")
		}
		return
	}
	if !info.IsDir() {
		fail(w, http.StatusConflict, "构建任务目录无效")
		return
	}

	var job BuildJob
	raw, _ := os.ReadFile(filepath.Join(jobDir, "job.json"))
	_ = json.Unmarshal(raw, &job)
	if err = os.RemoveAll(jobDir); err != nil {
		fail(w, http.StatusInternalServerError, "删除构建任务文件失败")
		return
	}
	delete(a.buildReceipts, id)
	respond(w, http.StatusOK, map[string]any{
		"deleted":     true,
		"job_id":      id,
		"release_ids": job.ReleaseIDs,
	})
}

func releaseHasRemoteOTA(release Release) bool {
	if release.OTA == nil {
		return false
	}
	return release.OTA.Status == "synced" || release.OTA.PublicURL != "" || release.OTA.ArtifactURL != "" || release.OTA.ManifestURL != ""
}

func (a *App) deleteReleaseFromOTAGateway(ctx context.Context, release Release) error {
	if !releaseHasRemoteOTA(release) {
		return nil
	}

	a.otaArtifactMu.Lock()
	defer a.otaArtifactMu.Unlock()
	cfg, err := a.readOTAGatewayConfig()
	if err != nil {
		return fmt.Errorf("OTA Gateway 上仍有公开安装包，但当前 Gateway 配置不可用；请恢复配置后重试删除")
	}
	remoteDir := cfg.RemoteRoot + "/releases/" + release.ProjectID + "/" + release.ID
	target := cfg.SSHUser + "@" + cfg.SSHHost
	args := append(append([]string{}, otaSSHOptions(cfg)...), target, "rm -rf -- "+remoteDir)
	if err = runOTACommand(ctx, "ssh", args...); err != nil {
		return fmt.Errorf("删除 OTA Gateway 公开安装包失败：%v", err)
	}
	return nil
}

func (a *App) deleteReleaseArtifactRecord(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("release")
	if !historyRecordIDRE.MatchString(id) {
		fail(w, http.StatusNotFound, "发布记录不存在")
		return
	}

	release, ok := a.getRelease(id)
	if !ok {
		fail(w, http.StatusNotFound, "发布记录不存在")
		return
	}
	if release.Delivery == "testflight" {
		fail(w, http.StatusConflict, "TestFlight 构建由 Apple 托管，ILS 不把它当作可删除的本地安装包")
		return
	}
	if release.BuildJobID != "" {
		a.buildMu.Lock()
		active := a.activeBuild == release.BuildJobID
		a.buildMu.Unlock()
		if active {
			fail(w, http.StatusConflict, "关联构建仍在运行，暂不能删除这个安装包")
			return
		}
	}

	if releaseHasRemoteOTA(release) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := a.deleteReleaseFromOTAGateway(ctx, release); err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	a.mu.Lock()
	index := -1
	for i := range a.state.Releases {
		if a.state.Releases[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		a.mu.Unlock()
		fail(w, http.StatusNotFound, "发布记录不存在")
		return
	}

	artifact := filepath.Join(a.data, "artifacts", id)
	trash := ""
	if _, err := os.Lstat(artifact); err == nil {
		trash = filepath.Join(a.data, ".delete-release-"+id+"-"+randomID(4))
		if err = os.Rename(artifact, trash); err != nil {
			a.mu.Unlock()
			fail(w, http.StatusInternalServerError, "无法移除安装包文件")
			return
		}
	} else if !os.IsNotExist(err) {
		a.mu.Unlock()
		fail(w, http.StatusInternalServerError, "无法读取安装包文件")
		return
	}

	releases := append([]Release{}, a.state.Releases[:index]...)
	releases = append(releases, a.state.Releases[index+1:]...)
	next := state{Projects: a.state.Projects, Releases: releases}
	if err := a.save(next); err != nil {
		if trash != "" {
			_ = os.Rename(trash, artifact)
		}
		a.mu.Unlock()
		fail(w, http.StatusInternalServerError, "删除发布记录失败")
		return
	}
	a.state = next
	a.mu.Unlock()

	warning := ""
	if trash != "" {
		if err := os.Remove(trash); err != nil {
			warning = "发布记录已删除，但临时安装包文件清理失败"
		}
	}
	_ = os.Remove(a.releaseIconPath(id))
	respond(w, http.StatusOK, map[string]any{
		"deleted":            true,
		"release_id":         id,
		"ota_remote_deleted": releaseHasRemoteOTA(release),
		"warning":            warning,
	})
}
