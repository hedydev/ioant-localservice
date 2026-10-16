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

func writeCascadeBuildJob(t *testing.T, data string, job BuildJob) string {
	t.Helper()
	jobDir := filepath.Join(data, "builds", job.ID)
	if err := os.MkdirAll(filepath.Join(jobDir, "output"), 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "job.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(jobDir, "build.log"), []byte("completed build"), 0600); err != nil {
		t.Fatal(err)
	}
	return jobDir
}

func TestDeleteBuildRecordCascadeRemovesReleaseHistoryAndArtifact(t *testing.T) {
	data := t.TempDir()
	jobID := "11111111111111111111111111111111"
	releaseID := "22222222222222222222222222222222"
	release := Release{
		ID: releaseID, ProjectID: "demo", Version: "1.2.3", Build: 9,
		Platform: "ios", Architecture: "arm64", Channel: "dev", Variant: "default",
		Delivery: "artifact", Status: "published", Filename: "Demo.ipa", BuildJobID: jobID,
		CreatedAt: time.Now().UTC(),
	}
	a := &App{
		data: data, token: deletionTestToken,
		state: state{Projects: []Project{}, Releases: []Release{release}},
		buildReceipts: map[string][]string{jobID: {releaseID}},
	}
	jobDir := writeCascadeBuildJob(t, data, BuildJob{
		ID: jobID, ProjectID: "demo", Status: "succeeded", ReleaseIDs: []string{releaseID}, CreatedAt: time.Now().UTC(),
	})
	if err := os.MkdirAll(filepath.Join(data, "artifacts"), 0700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(data, "artifacts", releaseID)
	if err := os.WriteFile(artifact, []byte("ipa"), 0600); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	a.deleteBuildRecordCascade(rr, deleteRequest(http.MethodDelete, "job", jobID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(a.state.Releases) != 0 {
		t.Fatalf("releases = %d, want 0", len(a.state.Releases))
	}
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Fatalf("job directory still exists: %v", err)
	}
	if _, err := os.Stat(artifact); !os.IsNotExist(err) {
		t.Fatalf("artifact still exists: %v", err)
	}
	if _, ok := a.buildReceipts[jobID]; ok {
		t.Fatal("build receipts were not removed")
	}
	var response struct {
		DeletedReleaseIDs []string `json:"deleted_release_ids"`
		DeletedReleases   int      `json:"deleted_releases"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.DeletedReleases != 1 || len(response.DeletedReleaseIDs) != 1 || response.DeletedReleaseIDs[0] != releaseID {
		t.Fatalf("response = %+v", response)
	}
}

func TestDeleteBuildRecordCascadeFindsReleaseByBuildJobID(t *testing.T) {
	data := t.TempDir()
	jobID := "33333333333333333333333333333333"
	testFlightID := "44444444444444444444444444444444"
	unrelatedID := "55555555555555555555555555555555"
	a := &App{
		data: data, token: deletionTestToken,
		state: state{Projects: []Project{}, Releases: []Release{
			{
				ID: testFlightID, ProjectID: "demo", Version: "2.0.0", Build: 12,
				Platform: "ios", Architecture: "arm64", Channel: "beta", Variant: "default",
				Delivery: "testflight", Status: "available", BuildJobID: jobID, CreatedAt: time.Now().UTC(),
			},
			{
				ID: unrelatedID, ProjectID: "demo", Version: "1.0.0", Build: 1,
				Platform: "ios", Architecture: "arm64", Channel: "dev", Variant: "default",
				Delivery: "artifact", Status: "published", CreatedAt: time.Now().UTC(),
			},
		}},
		buildReceipts: map[string][]string{},
	}
	writeCascadeBuildJob(t, data, BuildJob{
		ID: jobID, ProjectID: "demo", Status: "succeeded", ReleaseIDs: []string{}, CreatedAt: time.Now().UTC(),
	})

	rr := httptest.NewRecorder()
	a.deleteBuildRecordCascade(rr, deleteRequest(http.MethodDelete, "job", jobID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(a.state.Releases) != 1 || a.state.Releases[0].ID != unrelatedID {
		t.Fatalf("remaining releases = %+v", a.state.Releases)
	}
	var response struct {
		DeletedReleaseIDs         []string `json:"deleted_release_ids"`
		TestFlightRemoteUnchanged bool     `json:"testflight_remote_unchanged"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.DeletedReleaseIDs) != 1 || response.DeletedReleaseIDs[0] != testFlightID || !response.TestFlightRemoteUnchanged {
		t.Fatalf("response = %+v", response)
	}
}
