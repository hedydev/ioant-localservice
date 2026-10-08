package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const deletionTestToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func deleteRequest(method, pathValueName, id string) *http.Request {
	req := httptest.NewRequest(method, "/", nil)
	req.Header.Set("Authorization", "Bearer "+deletionTestToken)
	req.SetPathValue(pathValueName, id)
	return req
}

func TestDeleteBuildRecordRemovesWholeJobDirectory(t *testing.T) {
	data := t.TempDir()
	id := "0123456789abcdef0123456789abcdef"
	a := &App{data: data, token: deletionTestToken, buildReceipts: map[string][]string{id: {"release-a"}}}
	jobDir := filepath.Join(data, "builds", id)
	if err := os.MkdirAll(filepath.Join(jobDir, "output"), 0700); err != nil {
		t.Fatal(err)
	}
	job := BuildJob{ID: id, ProjectID: "demo", Status: "failed", CreatedAt: time.Now().UTC()}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "job.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "build.log"), []byte("failed build"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "output", "test.dmg"), []byte("artifact"), 0600); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	a.deleteBuildRecord(rr, deleteRequest(http.MethodDelete, "job", id))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if _, err = os.Stat(jobDir); !os.IsNotExist(err) {
		t.Fatalf("job directory still exists: %v", err)
	}
	if _, ok := a.buildReceipts[id]; ok {
		t.Fatal("build receipts were not removed")
	}
}

func TestDeleteBuildRecordRejectsActiveJob(t *testing.T) {
	data := t.TempDir()
	id := "fedcba9876543210fedcba9876543210"
	jobDir := filepath.Join(data, "builds", id)
	if err := os.MkdirAll(jobDir, 0700); err != nil {
		t.Fatal(err)
	}
	a := &App{data: data, token: deletionTestToken, activeBuild: id}

	rr := httptest.NewRecorder()
	a.deleteBuildRecord(rr, deleteRequest(http.MethodDelete, "job", id))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if _, err := os.Stat(jobDir); err != nil {
		t.Fatalf("active job directory was removed: %v", err)
	}
}

func TestDeleteReleaseArtifactRemovesFileAndBuildLink(t *testing.T) {
	data := t.TempDir()
	releaseID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	jobID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	release := Release{
		ID:           releaseID,
		ProjectID:    "demo",
		Version:      "1.0.0",
		Build:        7,
		Platform:     "ios",
		Architecture: "arm64",
		Channel:      "dev",
		Variant:      "default",
		Delivery:     "artifact",
		Status:       "published",
		Filename:     "Demo.ipa",
		BuildJobID:   jobID,
		CreatedAt:    time.Now().UTC(),
	}
	a := &App{
		data:          data,
		token:         deletionTestToken,
		state:         state{Projects: []Project{}, Releases: []Release{release}},
		buildReceipts: map[string][]string{jobID: {releaseID}},
	}
	if err := os.MkdirAll(filepath.Join(data, "artifacts"), 0700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(data, "artifacts", releaseID)
	if err := os.WriteFile(artifact, []byte("ipa"), 0600); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(data, "builds", jobID)
	if err := os.MkdirAll(jobDir, 0700); err != nil {
		t.Fatal(err)
	}
	job := BuildJob{ID: jobID, ProjectID: "demo", Status: "succeeded", ReleaseIDs: []string{releaseID}}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "job.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	a.deleteReleaseArtifactRecord(rr, deleteRequest(http.MethodDelete, "release", releaseID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(a.state.Releases) != 0 {
		t.Fatalf("releases = %d, want 0", len(a.state.Releases))
	}
	if _, err = os.Stat(artifact); !os.IsNotExist(err) {
		t.Fatalf("artifact still exists: %v", err)
	}
	raw, err = os.ReadFile(filepath.Join(jobDir, "job.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &job); err != nil {
		t.Fatal(err)
	}
	if len(job.ReleaseIDs) != 0 {
		t.Fatalf("job release_ids = %v, want empty", job.ReleaseIDs)
	}
	if len(a.buildReceipts[jobID]) != 0 {
		t.Fatalf("build receipts = %v, want empty", a.buildReceipts[jobID])
	}
}

func TestDeleteReleaseArtifactRejectsTestFlight(t *testing.T) {
	data := t.TempDir()
	id := "cccccccccccccccccccccccccccccccc"
	release := Release{
		ID:           id,
		ProjectID:    "demo",
		Version:      "1.0.0",
		Build:        8,
		Platform:     "ios",
		Architecture: "arm64",
		Channel:      "beta",
		Variant:      "default",
		Delivery:     "testflight",
		Status:       "available",
		CreatedAt:    time.Now().UTC(),
	}
	a := &App{data: data, token: deletionTestToken, state: state{Projects: []Project{}, Releases: []Release{release}}}

	rr := httptest.NewRecorder()
	a.deleteReleaseArtifactRecord(rr, deleteRequest(http.MethodDelete, "release", id))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if len(a.state.Releases) != 1 || a.state.Releases[0].ID != id {
		t.Fatalf("TestFlight release was changed: %+v", a.state.Releases)
	}
}
