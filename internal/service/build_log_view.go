package service

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	buildLogViewLines    = 300
	buildLogViewMaxBytes = 512 << 10
)

// readBuildLogTail returns only the newest part of a build log. The live UI
// never needs to parse or transfer the full multi-megabyte compiler log on
// every poll; the full log remains available through ?full=1.
func readBuildLogTail(path string, maxLines int) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	start := int64(0)
	truncated := false
	if info.Size() > buildLogViewMaxBytes {
		start = info.Size() - buildLogViewMaxBytes
		truncated = true
	}
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return "", false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", false, err
	}
	if start > 0 {
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	if maxLines <= 0 {
		maxLines = buildLogViewLines
	}

	lineBreaks := 0
	cut := 0
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] != '\n' {
			continue
		}
		lineBreaks++
		if lineBreaks > maxLines {
			cut = i + 1
			truncated = true
			break
		}
	}
	if cut > 0 {
		data = data[cut:]
	}
	return string(data), truncated, nil
}

func (a *App) buildLogView(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("full") == "1" {
		a.buildLog(w, r)
		return
	}
	if !a.authorized(w, r) {
		return
	}
	id := r.PathValue("job")
	if !jobRE.MatchString(id) {
		fail(w, http.StatusNotFound, "任务不存在")
		return
	}
	text, truncated, err := readBuildLogTail(filepath.Join(a.data, "builds", id, "build.log"), buildLogViewLines)
	if err != nil {
		fail(w, http.StatusNotFound, "日志尚未生成")
		return
	}
	if a.token != "" {
		text = strings.ReplaceAll(text, a.token, "[REDACTED]")
	}
	respond(w, http.StatusOK, map[string]any{
		"log":        text,
		"tail_lines": buildLogViewLines,
		"truncated":  truncated,
	})
}
