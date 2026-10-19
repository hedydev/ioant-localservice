package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type nodeBinaryAssignment struct {
	Line    int
	Value   string
	Literal bool
}

type iosNodePreflightReport struct {
	NodePath    string
	NodeVersion string
	Checked     []string
	Repaired    []string
}

func envValue(env []string, key string) string {
	prefix := key + "="
	value := ""
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			value = strings.TrimPrefix(item, prefix)
		}
	}
	return value
}

func replaceEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		out = append(out, item)
	}
	return append(out, prefix+value)
}

// preferPreflightNodeOnPath keeps the validated Node ahead of stale Homebrew
// installations. NODE_BINARY alone is insufficient: project scripts, npm,
// Expo and CocoaPods also invoke the ordinary "node" command through PATH.
func preferPreflightNodeOnPath(env []string, node string) []string {
  preferred := filepath.Dir(node)
  parts := []string{preferred}
  for _, dir := range filepath.SplitList(envValue(env, "PATH")) {
    if dir == "" || filepath.Clean(dir) == filepath.Clean(preferred) {
      continue
    }
    parts = append(parts, dir)
  }
  return replaceEnvValue(env, "PATH", strings.Join(parts, string(os.PathListSeparator)))
}

// Expo builds must not start a login shell that rewrites the validated PATH.
// Other existing Profile types retain their historical login-shell behavior.
func profileBuildShellArgs(platform, buildType, command string) []string {
  if portableIOSNodePreflightEnabled(platform, buildType) {
    return []string{"-c", command}
  }
  return []string{"-lc", command}
}

func emitBuildEvent(log io.Writer, event BuildEvent) {
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(log, "ILS_EVENT %s\n", raw)
}

func portableIOSNodePreflightEnabled(platform, buildType string) bool {
	return strings.EqualFold(strings.TrimSpace(platform), "ios") && strings.EqualFold(strings.TrimSpace(buildType), "expo")
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func resolveNodeFromPath(pathValue string) string {
	for _, dir := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, "node")
		if executableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func resolveBuildNode(ctx context.Context, root string, env []string) (string, string, error) {
	node := resolveNodeFromPath(envValue(env, "PATH"))
	if node == "" {
		for _, candidate := range []string{"/opt/homebrew/bin/node", "/usr/local/bin/node", "/usr/bin/node"} {
			if executableFile(candidate) {
				node = candidate
				break
			}
		}
	}
	if node == "" {
		return "", "", fmt.Errorf("Expo / React Native iOS preflight: Node.js unavailable；请先安装 Node.js，并从能执行 node 的终端启动 ILS")
	}
	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, node, "--version")
	cmd.Dir = root
	cmd.Env = env
	raw, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("Expo / React Native iOS preflight: 无法执行 Node.js：%s", node)
	}
	version := strings.TrimSpace(string(raw))
	if version == "" || strings.ContainsAny(version, "\r\n") {
		return "", "", fmt.Errorf("Expo / React Native iOS preflight: Node.js version 输出无效：%s", node)
	}
	return node, version, nil
}

var iosNodeScanSkipDirs = map[string]bool{
	".git": true, ".localservice": true, "node_modules": true, "Pods": true,
	"build": true, "dist": true, "DerivedData": true, ".expo": true,
}

func findIOSXcodeEnvFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && iosNodeScanSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != ".xcode.env" && entry.Name() != ".xcode.env.local" {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != "ios" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func literalNodeBinaryValue(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		value = value[1 : len(value)-1]
	} else if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	if value == "" || strings.ContainsAny(value, "$`;&|<>") {
		return "", false
	}
	if !filepath.IsAbs(value) {
		return "", false
	}
	return value, true
}

func parseNodeBinaryAssignments(raw string) []nodeBinaryAssignment {
	out := []nodeBinaryAssignment{}
	for index, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}
		if !strings.HasPrefix(trimmed, "NODE_BINARY") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "NODE_BINARY"))
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		value, literal := literalNodeBinaryValue(strings.TrimSpace(strings.TrimPrefix(rest, "=")))
		out = append(out, nodeBinaryAssignment{Line: index, Value: value, Literal: literal})
	}
	return out
}

func readNodeBinaryAssignments(path string) ([]nodeBinaryAssignment, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("不是普通文件")
	}
	if info.Size() > 64<<10 {
		return nil, fmt.Errorf("文件超过 64 KiB")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseNodeBinaryAssignments(string(raw)), nil
}

func gitPathState(ctx context.Context, root, path string) (tracked, ignored bool, err error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, false, fmt.Errorf("Xcode env 文件不在项目目录内")
	}
	relative = filepath.ToSlash(relative)
	check := func(args ...string) (bool, error) {
		cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cmdCtx, "git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
		err := cmd.Run()
		if err == nil {
			return true, nil
		}
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	tracked, err = check("ls-files", "--error-unmatch", "--", relative)
	if err != nil {
		return false, false, err
	}
	ignored, err = check("check-ignore", "-q", "--", relative)
	if err != nil {
		return false, false, err
	}
	return tracked, ignored, nil
}

func repairNodeBinaryAssignment(path string, line int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("不是普通文件")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	if line < 0 || line >= len(lines) {
		return fmt.Errorf("NODE_BINARY 行已变化")
	}
	lines[line] = `export NODE_BINARY="$(command -v node)"`
	updated := []byte(strings.Join(lines, "\n"))
	temp, err := os.CreateTemp(filepath.Dir(path), ".xcode-env-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(info.Mode().Perm()); err == nil {
		_, err = temp.Write(updated)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func relativeDisplayPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

func preparePortableIOSNodeEnvironment(ctx context.Context, root, platform, buildType string, log io.Writer, env []string) ([]string, iosNodePreflightReport, error) {
	report := iosNodePreflightReport{}
	if !portableIOSNodePreflightEnabled(platform, buildType) {
		return env, report, nil
	}

	emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "started", Message: "检查 Expo / React Native iOS Node/Xcode 环境"})
	node, version, err := resolveBuildNode(ctx, root, env)
	if err != nil {
		emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "failed", Message: err.Error()})
		fmt.Fprintf(log, "ERROR: %s\n", err.Error())
		return env, report, err
	}
	report.NodePath = node
	report.NodeVersion = version
	fmt.Fprintf(log, "ILS iOS Node preflight\nNode: %s\nNode version: %s\n", node, version)

	paths, err := findIOSXcodeEnvFiles(root)
	if err != nil {
		failure := fmt.Errorf("Expo / React Native iOS preflight: 扫描 Xcode Node 环境失败：%v", err)
		emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "failed", Message: failure.Error()})
		fmt.Fprintf(log, "ERROR: %s\n", failure.Error())
		return env, report, failure
	}
	for _, path := range paths {
		display := relativeDisplayPath(root, path)
		report.Checked = append(report.Checked, display)
		assignments, readErr := readNodeBinaryAssignments(path)
		if readErr != nil {
			failure := fmt.Errorf("Expo / React Native iOS preflight: 无法读取 %s：%v", display, readErr)
			emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "failed", Message: failure.Error()})
			fmt.Fprintf(log, "ERROR: %s\n", failure.Error())
			return env, report, failure
		}
		if len(assignments) == 0 {
			continue
		}
		assignment := assignments[len(assignments)-1]
		if !assignment.Literal {
			fmt.Fprintf(log, "Xcode Node env: %s uses dynamic NODE_BINARY\n", display)
			continue
		}
		if executableFile(assignment.Value) {
			fmt.Fprintf(log, "Xcode Node env: %s -> %s\n", display, assignment.Value)
			continue
		}

		tracked, ignored, stateErr := gitPathState(ctx, root, path)
		if stateErr != nil {
			failure := fmt.Errorf("Expo / React Native iOS preflight: 无法确认 %s 的 Git 状态：%v", display, stateErr)
			emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "failed", Message: failure.Error()})
			fmt.Fprintf(log, "ERROR: %s\n", failure.Error())
			return env, report, failure
		}
		canRepair := filepath.Base(path) == ".xcode.env.local" && len(assignments) == 1 && !tracked && ignored
		if canRepair {
			if repairErr := repairNodeBinaryAssignment(path, assignment.Line); repairErr != nil {
				failure := fmt.Errorf("Expo / React Native iOS preflight: 自动修复 %s 失败：%v", display, repairErr)
				emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "failed", Message: failure.Error()})
				fmt.Fprintf(log, "ERROR: %s\n", failure.Error())
				return env, report, failure
			}
			report.Repaired = append(report.Repaired, display)
			fmt.Fprintf(log, "Auto-repaired ignored local Xcode Node env: %s\nOld NODE_BINARY: %s\nNew NODE_BINARY: $(command -v node) -> %s\n", display, assignment.Value, node)
			continue
		}

		failure := fmt.Errorf("Expo / React Native iOS preflight: %s 的 NODE_BINARY 已失效：%s；当前 Node：%s。ILS 只会自动修复被 Git 忽略且未跟踪的 .xcode.env.local，请先修正此文件后重试", display, assignment.Value, node)
		emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "failed", Message: failure.Error()})
		fmt.Fprintf(log, "ERROR: %s\n", failure.Error())
		return env, report, failure
	}

	env = replaceEnvValue(env, "NODE_BINARY", node)
	env = replaceEnvValue(env, "ILS_NODE_BINARY", node)
	env = preferPreflightNodeOnPath(env, node)
	fmt.Fprintf(log, "Pinned Expo build PATH to validated Node: %s\n", node)
	message := "Node " + version + " · " + node
	if len(report.Repaired) > 0 {
		message += " · 已修复 " + strings.Join(report.Repaired, ", ")
	}
	emitBuildEvent(log, BuildEvent{Stage: "preflight", State: "succeeded", Message: message})
	fmt.Fprintf(log, "ILS iOS Node preflight succeeded: %s\n", message)
	return env, report, nil
}
