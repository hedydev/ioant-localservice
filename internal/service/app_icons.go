package service

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const maxAppIconBytes = int64(16 << 20)

type assetCatalogContents struct {
	Images []assetCatalogImage `json:"images"`
}

type assetCatalogImage struct {
	Filename string `json:"filename"`
	Idiom    string `json:"idiom"`
	Platform string `json:"platform"`
	Size     string `json:"size"`
	Scale    string `json:"scale"`
}

type projectIconCandidate struct {
	SetDir   string
	Filename string
	Score    int64
}

func appIconPixels(size, scale string) int64 {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0
	}
	width, e1 := strconv.ParseFloat(parts[0], 64)
	height, e2 := strconv.ParseFloat(parts[1], 64)
	if e1 != nil || e2 != nil || width <= 0 || height <= 0 {
		return 0
	}
	multiplier := 1.0
	if strings.HasSuffix(scale, "x") {
		if parsed, e := strconv.ParseFloat(strings.TrimSuffix(scale, "x"), 64); e == nil && parsed > 0 {
			multiplier = parsed
		}
	}
	return int64(width * multiplier * height * multiplier)
}

func assetImagePlatform(image assetCatalogImage) string {
	platform := strings.ToLower(strings.TrimSpace(image.Platform))
	idiom := strings.ToLower(strings.TrimSpace(image.Idiom))
	switch {
	case platform == "ios", idiom == "iphone", idiom == "ipad", idiom == "ios-marketing":
		return "ios"
	case platform == "macos", idiom == "mac":
		return "macos"
	default:
		return ""
	}
}

func pngBytes(raw []byte) bool {
	return len(raw) >= 8 &&
		raw[0] == 0x89 && raw[1] == 'P' && raw[2] == 'N' && raw[3] == 'G' &&
		raw[4] == 0x0d && raw[5] == 0x0a && raw[6] == 0x1a && raw[7] == 0x0a
}

func readCatalogPNG(root, setDir, filename string) ([]byte, error) {
	name := filepath.FromSlash(strings.TrimSpace(filename))
	if name == "" || filepath.IsAbs(name) || filepath.Base(name) != name || strings.ToLower(filepath.Ext(name)) != ".png" {
		return nil, os.ErrInvalid
	}
	full := filepath.Join(root, filepath.FromSlash(setDir), name)
	real, e := filepath.EvalSymlinks(full)
	if e != nil || real != full {
		return nil, os.ErrInvalid
	}
	info, e := os.Stat(full)
	if e != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxAppIconBytes {
		return nil, os.ErrInvalid
	}
	raw, e := os.ReadFile(full)
	if e != nil || int64(len(raw)) > maxAppIconBytes || !pngBytes(raw) {
		return nil, os.ErrInvalid
	}
	return raw, nil
}

func writeBinaryAtomic(path string, raw []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	temp, e := os.CreateTemp(filepath.Dir(path), ".icon-*")
	if e != nil {
		return e
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, e = temp.Write(raw); e == nil {
		e = temp.Sync()
	}
	closeErr := temp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func (a *App) projectIconPath(project, platform string) string {
	return filepath.Join(a.data, "project-icons", project, platform+".png")
}

func (a *App) releaseIconPath(release string) string {
	return filepath.Join(a.data, "release-icons", release+".png")
}

func (a *App) buildIconPath(job string) string {
	return filepath.Join(a.data, "builds", job, "app-icon.png")
}

func (a *App) snapshotBuildIcon(project, platform, job string) {
	if !oneOf(platform, "ios", "macos") || !jobRE.MatchString(job) {
		return
	}
	raw, e := os.ReadFile(a.projectIconPath(project, platform))
	if e == nil && pngBytes(raw) {
		_ = writeBinaryAtomic(a.buildIconPath(job), raw)
	}
}

func (a *App) refreshProjectIcons(project, root string) {
	list, e := gitRead(root, "ls-files", "-z")
	if e != nil {
		return
	}
	candidates := map[string][]projectIconCandidate{
		"ios":   make([]projectIconCandidate, 0),
		"macos": make([]projectIconCandidate, 0),
	}
	for _, contentsPath := range strings.Split(list, "\x00") {
		if !strings.HasSuffix(strings.ToLower(contentsPath), ".appiconset/contents.json") {
			continue
		}
		raw, e := safeTracked(root, contentsPath, 1<<20)
		if e != nil {
			continue
		}
		var contents assetCatalogContents
		if json.Unmarshal(raw, &contents) != nil {
			continue
		}
		setDir := filepath.Dir(contentsPath)
		preferredSet := strings.EqualFold(filepath.Base(setDir), "AppIcon.appiconset")
		for _, image := range contents.Images {
			platform := assetImagePlatform(image)
			if platform == "" || strings.TrimSpace(image.Filename) == "" {
				continue
			}
			score := appIconPixels(image.Size, image.Scale)
			if preferredSet {
				score += 1 << 50
			}
			candidates[platform] = append(candidates[platform], projectIconCandidate{
				SetDir:   filepath.ToSlash(setDir),
				Filename: image.Filename,
				Score:    score,
			})
		}
	}
	for _, platform := range []string{"ios", "macos"} {
		items := candidates[platform]
		sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
		var selected []byte
		for _, candidate := range items {
			raw, e := readCatalogPNG(root, candidate.SetDir, candidate.Filename)
			if e == nil {
				selected = raw
				break
			}
		}
		target := a.projectIconPath(project, platform)
		if len(selected) == 0 {
			_ = os.Remove(target)
			continue
		}
		_ = writeBinaryAtomic(target, selected)
	}
}

func (a *App) snapshotReleaseIcon(release Release, artifact string) {
	var raw []byte
	if release.Platform == "ios" {
		raw, _ = extractIPAAppIcon(artifact)
	}
	if len(raw) == 0 {
		raw, _ = os.ReadFile(a.projectIconPath(release.ProjectID, release.Platform))
	}
	if pngBytes(raw) {
		_ = writeBinaryAtomic(a.releaseIconPath(release.ID), raw)
	}
}

func serveAppIcon(w http.ResponseWriter, r *http.Request, path string) {
	f, e := os.Open(path)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

func (a *App) projectIcon(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	platform := r.URL.Query().Get("platform")
	if !slugRE.MatchString(project) || !oneOf(platform, "ios", "macos") {
		http.NotFound(w, r)
		return
	}
	a.mu.RLock()
	exists := a.projectExists(project)
	a.mu.RUnlock()
	if !exists {
		http.NotFound(w, r)
		return
	}
	serveAppIcon(w, r, a.projectIconPath(project, platform))
}

func (a *App) buildIcon(w http.ResponseWriter, r *http.Request) {
	job := r.PathValue("job")
	if !jobRE.MatchString(job) {
		http.NotFound(w, r)
		return
	}
	path := a.buildIconPath(job)
	if _, e := os.Stat(path); e == nil {
		serveAppIcon(w, r, path)
		return
	}

	b, e := os.ReadFile(a.buildJobPath(job))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	var build BuildJob
	if json.Unmarshal(b, &build) != nil {
		http.NotFound(w, r)
		return
	}
	platform := build.Platform
	if platform == "" && build.Result != nil {
		platform = build.Result.Platform
	}
	if !oneOf(platform, "ios", "macos") {
		http.NotFound(w, r)
		return
	}
	serveAppIcon(w, r, a.projectIconPath(build.ProjectID, platform))
}

func (a *App) releaseIcon(w http.ResponseWriter, r *http.Request) {
	release := r.PathValue("release")
	if _, ok := a.getRelease(release); !ok {
		http.NotFound(w, r)
		return
	}
	serveAppIcon(w, r, a.releaseIconPath(release))
}
