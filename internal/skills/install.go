package skills

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type InstallResult struct {
	Name string
	Path string
}

func Install(ctx context.Context, source string, userDir string) (InstallResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return InstallResult{}, fmt.Errorf("source is required")
	}
	tmp := ""
	zipPath := source
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		var err error
		tmp, err = downloadZip(ctx, source)
		if err != nil {
			return InstallResult{}, err
		}
		defer os.Remove(tmp)
		zipPath = tmp
	}
	if userDir == "" {
		home, _ := os.UserHomeDir()
		userDir = filepath.Join(home, ".PseudoClaude", "skills")
	}
	return installZip(zipPath, userDir)
}

func downloadZip(ctx context.Context, source string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	file, err := os.CreateTemp("", "pseudoclaude-skill-*.zip")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func installZip(zipPath, userDir string) (InstallResult, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return InstallResult{}, err
	}
	defer reader.Close()
	top := ""
	for _, file := range reader.File {
		name := strings.Trim(file.Name, "/")
		if name == "" {
			continue
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return InstallResult{}, fmt.Errorf("zip contains symlink %q", file.Name)
		}
		if filepath.IsAbs(name) || strings.Contains(name, "\\") {
			return InstallResult{}, fmt.Errorf("unsafe zip path %q", file.Name)
		}
		cleaned := filepath.Clean(name)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return InstallResult{}, fmt.Errorf("unsafe zip path %q", file.Name)
		}
		parts := strings.Split(cleaned, string(filepath.Separator))
		if top == "" {
			top = parts[0]
		} else if top != parts[0] {
			return InstallResult{}, fmt.Errorf("zip must contain exactly one top-level directory")
		}
	}
	if top == "" {
		return InstallResult{}, fmt.Errorf("zip is empty")
	}
	target := filepath.Join(userDir, top)
	for _, file := range reader.File {
		name := strings.Trim(file.Name, "/")
		if name == "" {
			continue
		}
		dest := filepath.Join(userDir, filepath.Clean(name))
		rel, err := filepath.Rel(target, dest)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return InstallResult{}, fmt.Errorf("unsafe zip path %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return InstallResult{}, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return InstallResult{}, err
		}
		src, err := file.Open()
		if err != nil {
			return InstallResult{}, err
		}
		dst, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.FileInfo().Mode().Perm())
		if err != nil {
			src.Close()
			return InstallResult{}, err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return InstallResult{}, copyErr
		}
		if closeErr != nil {
			return InstallResult{}, closeErr
		}
	}
	return InstallResult{Name: top, Path: target}, nil
}
