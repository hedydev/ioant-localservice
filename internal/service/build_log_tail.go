package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	buildLogLimit  = 2 << 20
	buildLogRetain = 1792 << 10
)

const buildLogTrimMarker = "[较早日志已省略；以下保留最新构建输出]\n"

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// writeRollingBuildLog bounds the on-disk build log while retaining the newest
// output. Xcode errors are normally emitted near the end of a build, so keeping
// the tail is substantially more useful than keeping the first N bytes.
func writeRollingBuildLog(file *os.File, written *int, p string) {
	if file == nil || p == "" {
		return
	}
	if _, err := file.WriteString(p); err != nil {
		return
	}
	*written += len(p)
	if *written <= buildLogLimit {
		return
	}

	data, err := os.ReadFile(file.Name())
	if err != nil {
		return
	}
	start := len(data) - buildLogRetain
	if start < 0 {
		start = 0
	}
	// Avoid beginning the retained log in the middle of a line when possible.
	if start > 0 {
		if i := strings.IndexByte(string(data[start:]), '\n'); i >= 0 {
			start += i + 1
		}
	}
	tail := data[start:]
	if err := file.Truncate(0); err != nil {
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return
	}
	if _, err := file.WriteString(buildLogTrimMarker); err != nil {
		return
	}
	if _, err := file.Write(tail); err != nil {
		return
	}
	*written = len(buildLogTrimMarker) + len(tail)
}

func cleanFailureLine(line string) string {
	line = ansiEscapeRE.ReplaceAllString(strings.TrimSpace(line), "")
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > 700 {
		line = line[:700] + "…"
	}
	return line
}

func concreteBuildFailure(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil || len(data) == 0 {
		return ""
	}
	const inspectLimit = 192 << 10
	if len(data) > inspectLimit {
		data = data[len(data)-inspectLimit:]
	}
	lines := strings.Split(string(data), "\n")

	// First pass: prefer compiler/tool errors over generic shell summaries.
	for i := len(lines) - 1; i >= 0; i-- {
		line := cleanFailureLine(lines[i])
		lower := strings.ToLower(line)
		if line == "" || strings.Contains(lower, "see build log") || strings.Contains(lower, "详情见日志") {
			continue
		}
		if strings.Contains(lower, " error:") || strings.HasPrefix(lower, "error:") ||
			strings.Contains(lower, "fatal error:") || strings.HasPrefix(lower, "fatal:") ||
			strings.Contains(lower, "xcodebuild: error:") {
			return line
		}
	}

	// Second pass: common Xcode/signing/shell failure forms that do not always
	// contain a literal "error:" token.
	for i := len(lines) - 1; i >= 0; i-- {
		line := cleanFailureLine(lines[i])
		lower := strings.ToLower(line)
		if line == "" || strings.Contains(lower, "see build log") || strings.Contains(lower, "详情见日志") {
			continue
		}
		if strings.Contains(lower, "failed with a nonzero exit code") ||
			strings.Contains(lower, "provisioning profile") ||
			strings.Contains(lower, "codesign") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "no such file") ||
			strings.Contains(lower, "command not found") ||
			strings.Contains(lower, "could not") || strings.Contains(lower, "unable to") ||
			strings.Contains(lower, "archive failed") || strings.Contains(lower, "build failed") {
			return line
		}
	}
	return ""
}

func buildFailureMessage(logPath string, commandErr error, ctx context.Context, label string) string {
	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return label + " 超时"
	}
	if detail := concreteBuildFailure(logPath); detail != "" {
		return label + " 失败：" + detail
	}
	if commandErr != nil {
		return fmt.Sprintf("%s 失败：%v", label, commandErr)
	}
	return label + " 失败"
}
