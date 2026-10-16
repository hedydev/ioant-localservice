package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func buildReleaseIDSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		if historyRecordIDRE.MatchString(id) {
			set[id] = true
		}
	}
	return set
}

func (a *App) releasesForBuildDeletion(jobID string, releaseIDs []string) []Release {
	explicit := buildReleaseIDSet(releaseIDs)
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := []Release{}
	for _, release := range a.state.Releases {
		if release.BuildJobID == jobID || explicit[release.ID] {
			out = append(out, release)
		}
	}
	return out
}

func (a *App) removeBuildVersionHistory(jobID string, releaseIDs []string) ([]Release, error) {
	explicit := buildReleaseIDSet(releaseIDs)
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := make([]Release, 0, len(a.state.Releases))
	deleted := []Release{}
	for _, release := range a.state.Releases {
		if release.BuildJobID == jobID || explicit[release.ID] {
			deleted = append(deleted, release)
			continue
		}
		kept = append(kept, release)
	}
	if len(deleted) == 0 {
		return deleted, nil
	}
	next := state{Projects: a.state.Projects, Releases: kept}
	if err := a.save(next); err != nil {
		return nil, err
	}
	a.state = next
	return deleted, nil
}

func addBuildDeletionWarning(current, message string) string {
	if message == "" {
		return current
	}
	if current == "" {
		return message
	}
	return current + "；" + message
}

// deleteBuildRecordCascade treats a Build Job and the Release/version-history
// records created from that job as one lifecycle. Removing the build removes
// its local Release records and artifacts too. TestFlight content remains on
// Apple's service; only the ILS-side history is removed.
func (a *App) deleteBuildRecordCascade(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("job")
	if !historyRecordIDRE.MatchString(id) {
		fail(w, http.StatusNotFound, "构建任务不存在")
		return
	}

	a.buildMu.Lock()
	active := a.activeBuild == id
	a.buildMu.Unlock()
	if active {
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
	linked := a.releasesForBuildDeletion(id, job.ReleaseIDs)
	otaDeleted := 0
	for _, release := range linked {
		if !releaseHasRemoteOTA(release) {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err = a.deleteReleaseFromOTAGateway(ctx, release)
		cancel()
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		otaDeleted++
	}

	trashJob := filepath.Join(a.data, ".delete-build-"+id+"-"+randomID(4))
	if err = os.Rename(jobDir, trashJob); err != nil {
		fail(w, http.StatusInternalServerError, "无法移除构建任务文件")
		return
	}
	deletedReleases, err := a.removeBuildVersionHistory(id, job.ReleaseIDs)
	if err != nil {
		_ = os.Rename(trashJob, jobDir)
		fail(w, http.StatusInternalServerError, "删除关联版本历史失败")
		return
	}

	warning := ""
	if err = os.RemoveAll(trashJob); err != nil {
		warning = addBuildDeletionWarning(warning, "构建记录已移除，但临时构建文件清理失败")
	}
	deletedIDs := make([]string, 0, len(deletedReleases))
	testFlightRemoved := false
	for _, release := range deletedReleases {
		deletedIDs = append(deletedIDs, release.ID)
		if release.Delivery == "testflight" {
			testFlightRemoved = true
		}
		if err = os.RemoveAll(filepath.Join(a.data, "artifacts", release.ID)); err != nil {
			warning = addBuildDeletionWarning(warning, "版本历史已删除，但安装包文件清理失败："+release.ID)
		}
		_ = os.Remove(a.releaseIconPath(release.ID))
	}

	a.buildMu.Lock()
	delete(a.buildReceipts, id)
	a.buildMu.Unlock()
	respond(w, http.StatusOK, map[string]any{
		"deleted":                    true,
		"job_id":                     id,
		"release_ids":                 deletedIDs,
		"deleted_release_ids":         deletedIDs,
		"deleted_releases":            len(deletedIDs),
		"ota_remote_deleted":          otaDeleted,
		"testflight_remote_unchanged": testFlightRemoved,
		"warning":                     warning,
	})
}
