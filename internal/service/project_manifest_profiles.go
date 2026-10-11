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

func simulatorProfileFromProjectManifest(raw []byte, project string) (*releaseProfileLocal, error) {
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

	var found *releaseProfileLocal
	for _, item := range manifest.Profiles {
		var probe struct {
			Platform string `json:"platform"`
			Lane     string `json:"lane"`
		}
		if err := json.Unmarshal(item, &probe); err != nil {
			return nil, fmt.Errorf("项目 .ils/project.json 包含无效 Profile")
		}
		if strings.TrimSpace(probe.Platform) != "ios" || strings.TrimSpace(probe.Lane) != "ios-simulator" {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("iOS 项目只能声明一个 ios-simulator Profile")
		}
		var declared projectManifestProfile
		if err := json.Unmarshal(item, &declared); err != nil {
			return nil, fmt.Errorf("ios-simulator Profile 格式无效")
		}
		profile, err := normalizeSimulatorReleaseProfile(declared.ReleaseProfile)
		if err != nil {
			return nil, err
		}
		buildType, err := normalizeBuildType(declared.BuildType)
		if err != nil {
			return nil, err
		}
		found = &releaseProfileLocal{ReleaseProfile: profile, BuildType: buildType}
	}
	return found, nil
}

// syncSimulatorProfileFromProjectManifest fills the machine-local ILS profile
// store from the current checkout when an iOS project declares a Simulator
// profile but that profile has not been imported yet. Existing local profiles
// are deliberately left untouched so Apple Team/TestFlight settings and manual
// profile edits are never overwritten by a page refresh.
func (a *App) syncSimulatorProfileFromProjectManifest(project string) error {
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
	declared, err := simulatorProfileFromProjectManifest(raw, project)
	if err != nil {
		return err
	}
	if declared == nil {
		return nil
	}

	profilePath := a.releaseProfilePath(project, declared.ID)
	if _, err = os.Stat(profilePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	a.buildMu.Lock()
	defer a.buildMu.Unlock()
	if a.activeBuild != "" {
		// Never mutate Profile storage during a running build. The next normal
		// profile refresh after the build finishes will perform the sync.
		return nil
	}
	if _, err = os.Stat(profilePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(profilePath), 0700); err != nil {
		return err
	}
	if err = atomicJSON(profilePath, declared.ReleaseProfile); err != nil {
		return err
	}
	metaPath := a.releaseProfileMetaPath(project, declared.ID)
	if err = os.MkdirAll(filepath.Dir(metaPath), 0700); err != nil {
		return err
	}
	return atomicJSON(metaPath, map[string]string{"build_type": declared.BuildType})
}

// releaseProfilesAutoSync keeps the normal profile API semantics but ensures a
// repository-owned Simulator profile is visible without requiring a separate
// manual import step after every checkout update.
func (a *App) releaseProfilesAutoSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if !a.authorized(w, r) {
			return
		}
		if err := a.syncSimulatorProfileFromProjectManifest(r.PathValue("project")); err != nil {
			fail(w, http.StatusConflict, "同步项目 Simulator Profile 失败："+err.Error())
			return
		}
	}
	a.releaseProfilesLocalV2(w, r)
}
