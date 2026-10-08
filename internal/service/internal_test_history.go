package service

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// InternalTestRecord is the public, sanitized history view of a successful
// package-only Internal Test build. It deliberately does not turn the build
// into a Release and does not expose source paths, branch names, commits, or
// other BuildJob diagnostics.
type InternalTestRecord struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	Kind         string     `json:"kind"`
	Platform     string     `json:"platform"`
	Lane         string     `json:"lane"`
	Version      string     `json:"version,omitempty"`
	Build        string     `json:"build,omitempty"`
	Architecture string     `json:"architecture,omitempty"`
	Channel      string     `json:"channel,omitempty"`
	Variant      string     `json:"variant,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	DownloadURL  string     `json:"download_url"`
}

func (a *App) internalTestRecords(project string) []InternalTestRecord {
	out := []InternalTestRecord{}
	paths, _ := filepath.Glob(filepath.Join(a.data, "builds", "*", "job.json"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var job BuildJob
		if json.Unmarshal(raw, &job) != nil || job.ProjectID != project || job.Status != "succeeded" || job.Lane != "macos-test" || job.Result == nil {
			continue
		}
		result := job.Result
		if result.Status != "succeeded" || result.Platform != "macos" || strings.TrimSpace(result.Artifact) == "" {
			continue
		}
		output := filepath.Join(a.data, "builds", job.ID, "output")
		artifact, err := outputArtifact(output, result.Artifact)
		if err != nil {
			continue
		}
		info, err := os.Stat(artifact)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			continue
		}
		architecture := strings.TrimSpace(result.Architecture)
		if architecture == "" {
			architecture = strings.TrimSpace(job.ReleaseArchitecture)
		}
		out = append(out, InternalTestRecord{
			ID:           job.ID,
			ProjectID:    job.ProjectID,
			Kind:         "internal-test",
			Platform:     "macos",
			Lane:         "macos-test",
			Version:      strings.TrimSpace(result.Version),
			Build:        strings.TrimSpace(result.Build),
			Architecture: architecture,
			Channel:      strings.TrimSpace(job.ReleaseChannel),
			Variant:      strings.TrimSpace(job.ReleaseVariant),
			Notes:        strings.TrimSpace(job.ReleaseNotes),
			Filename:     filepath.Base(artifact),
			Size:         info.Size(),
			CreatedAt:    job.CreatedAt,
			FinishedAt:   job.FinishedAt,
			DownloadURL:  "/api/builds/" + job.ID + "/artifact",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

func (a *App) listInternalTestRecords(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	a.mu.RLock()
	exists := a.projectExists(project)
	a.mu.RUnlock()
	if !exists {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	respond(w, http.StatusOK, a.internalTestRecords(project))
}
