package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type projectManifestProfile struct {
	ReleaseProfile
	BuildType string `json:"build_type,omitempty"`
}

type projectProfileManifest struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	Profiles      []json.RawMessage `json:"profiles"`
}

func profilesFromProjectManifest(raw []byte, project string) ([]releaseProfileLocal, error) {
	var manifest projectProfileManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("项目 .ils/project.json 格式无效")
	}
	if manifest.SchemaVersion != 1 {
		return nil, fmt.Errorf("项目 .ils/project.json schema_version 必须是 1")
	}
	if strings.TrimSpace(manifest.ID) != project {
		return nil, fmt.Errorf("项目 .ils/project.json id=%q 与 ILS 项目 %q 不一致", manifest.ID, project)
	}

	out := make([]releaseProfileLocal, 0, len(manifest.Profiles))
	seen := map[string]bool{}
	simulatorCount := 0
	for _, item := range manifest.Profiles {
		var declared projectManifestProfile
		if err := json.Unmarshal(item, &declared); err != nil {
			return nil, fmt.Errorf("项目 .ils/project.json 包含无效 Profile")
		}

		var (
			profile ReleaseProfile
			err     error
		)
		if strings.TrimSpace(declared.Platform) == "ios" && strings.TrimSpace(declared.Lane) == "ios-simulator" {
			simulatorCount++
			if simulatorCount > 1 {
				return nil, fmt.Errorf("iOS 项目只能声明一个 ios-simulator Profile")
			}
			profile, err = normalizeSimulatorReleaseProfile(declared.ReleaseProfile)
		} else {
			profile, err = normalizeReleaseProfile(declared.ReleaseProfile)
		}
		if err != nil {
			return nil, fmt.Errorf("Profile %q 无效：%w", declared.ID, err)
		}
		if seen[profile.ID] {
			return nil, fmt.Errorf("项目 .ils/project.json 重复声明 Profile %q", profile.ID)
		}
		seen[profile.ID] = true

		buildType, err := normalizeBuildType(declared.BuildType)
		if err != nil {
			return nil, fmt.Errorf("Profile %q 构建类型无效：%w", profile.ID, err)
		}
		out = append(out, releaseProfileLocal{ReleaseProfile: profile, BuildType: buildType})
	}
	return out, nil
}

// Kept as a focused helper for Simulator-specific tests and callers. The
// profile API now synchronizes every missing repository-owned profile, not just
// the Simulator lane.
func simulatorProfileFromProjectManifest(raw []byte, project string) (*releaseProfileLocal, error) {
	profiles, err := profilesFromProjectManifest(raw, project)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].Platform == "ios" && profiles[i].Lane == "ios-simulator" {
			profile := profiles[i]
			return &profile, nil
		}
	}
	return nil, nil
}

// syncProfilesFromProjectManifest fills the machine-local ILS profile store
// from the current checkout. Only profiles that do not already exist locally
// are added. Existing local profiles are deliberately left untouched so Apple
// Team, TestFlight automation and other machine-local edits are never
// overwritten just by opening the Build page.
func (a *App) syncProfilesFromProjectManifest(project string) error {
	source, err := a.readSource(project)
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(source.Path, ".ils", "project.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取项目 .ils/project.json 失败: %w", err)
	}
	declared, err := profilesFromProjectManifest(raw, project)
	if err != nil {
		return err
	}
	if len(declared) == 0 {
		return nil
	}

	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != "" {
		// Never mutate Profile storage during a running build. The next normal
		// profile refresh after the build finishes will perform the sync.
		return nil
	}

	for _, item := range declared {
		profilePath := a.releaseProfilePath(project, item.ID)
		if _, statErr := os.Stat(profilePath); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		if err = os.MkdirAll(filepath.Dir(profilePath), 0700); err != nil {
			return err
		}
		if err = atomicJSON(profilePath, item.ReleaseProfile); err != nil {
			return err
		}
		metaPath := a.releaseProfileMetaPath(project, item.ID)
		if err = os.MkdirAll(filepath.Dir(metaPath), 0700); err != nil {
			return err
		}
		if err = atomicJSON(metaPath, map[string]string{"build_type": item.BuildType}); err != nil {
			return err
		}
	}
	return nil
}

// Backward-compatible name for the first iteration of automatic discovery.
func (a *App) syncSimulatorProfileFromProjectManifest(project string) error {
	return a.syncProfilesFromProjectManifest(project)
}

// releaseProfilesAutoSync keeps normal profile API semantics while making all
// repository-owned profiles visible without a separate manual import step.
func (a *App) releaseProfilesAutoSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if !a.authorized(w, r) {
			return
		}
		if err := a.syncProfilesFromProjectManifest(r.PathValue("project")); err != nil {
			fail(w, http.StatusConflict, "同步项目 Release Profiles 失败："+err.Error())
			return
		}
	}
	a.releaseProfilesLocalV2(w, r)
}
