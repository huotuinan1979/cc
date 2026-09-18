//go:build windows

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

//go:embed runtime
var runtimeFS embed.FS

var (
	processMu sync.Mutex
	currentCmd *exec.Cmd
)

func cacheRoot() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" { base = os.TempDir() }
	return filepath.Join(base, "906SubtitleTool", "v1.4")
}

func ensureRuntime() (string, error) {
	root := cacheRoot()
	marker := filepath.Join(root, ".ready")
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == "v1.4" {
		return root, nil
	}
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0755); err != nil { return "", err }
	err := fs.WalkDir(runtimeFS, "runtime", func(path string, d fs.DirEntry, err error) error {
		if err != nil { return err }
		rel := strings.TrimPrefix(path, "runtime")
		rel = strings.TrimPrefix(rel, "/")
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if d.IsDir() { return os.MkdirAll(dst, 0755) }
		data, err := runtimeFS.ReadFile(path)
		if err != nil { return err }
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil { return err }
		return os.WriteFile(dst, data, 0755)
	})
	if err != nil { return "", err }
	if err := os.WriteFile(marker, []byte("v1.4"), 0644); err != nil { return "", err }
	return root, nil
}

func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	processMu.Lock()
	currentCmd = cmd
	processMu.Unlock()
	err := cmd.Run()
	processMu.Lock()
	if currentCmd == cmd { currentCmd = nil }
	processMu.Unlock()
	return err
}

func execHidden(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}

func cancelRunningProcess() {
	processMu.Lock()
	cmd := currentCmd
	processMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func componentPath(root, name string) (string, error) {
	p := filepath.Join(root, name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("本地组件缺失：%s", name)
	}
	return p, nil
}
