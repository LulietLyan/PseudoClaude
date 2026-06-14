package tools

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultTimeout          = 10 * time.Second
	defaultMaxReadBytes     = 256 * 1024
	defaultMaxOutputBytes   = 64 * 1024
	defaultMaxSearchResults = 100
)

func DefaultEnv(cwd string) Env {
	if cwd == "" {
		cwd = "."
	}
	return Env{
		CWD:              cwd,
		Timeout:          defaultTimeout,
		MaxReadBytes:     defaultMaxReadBytes,
		MaxOutputBytes:   defaultMaxOutputBytes,
		MaxSearchResults: defaultMaxSearchResults,
	}
}

func normalizeEnv(env Env) Env {
	if env.CWD == "" {
		env.CWD = "."
	}
	if env.Timeout <= 0 {
		env.Timeout = defaultTimeout
	}
	if env.MaxReadBytes <= 0 {
		env.MaxReadBytes = defaultMaxReadBytes
	}
	if env.MaxOutputBytes <= 0 {
		env.MaxOutputBytes = defaultMaxOutputBytes
	}
	if env.MaxSearchResults <= 0 {
		env.MaxSearchResults = defaultMaxSearchResults
	}
	return env
}

func resolvePath(env Env, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(raw, "~") {
		return "", fmt.Errorf("~ paths are not supported")
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(env.CWD, path)
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return abs, nil
}

func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func readTextFile(path string, limit int64) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, errIsDir
	}

	readLimit := limit
	if readLimit <= 0 {
		readLimit = defaultMaxReadBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, readLimit+1))
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(data)) > readLimit
	if truncated {
		data = data[:readLimit]
	}
	if !isText(data) {
		return "", truncated, errBinary
	}
	return string(data), truncated, nil
}

func truncateString(s string, limit int) (string, bool) {
	if limit <= 0 {
		limit = defaultMaxOutputBytes
	}
	if len(s) <= limit {
		return s, false
	}
	if limit < len("...[truncated]") {
		return s[:limit], true
	}
	return s[:limit-len("...[truncated]")] + "...[truncated]", true
}

func limitStrings(items []string, limit int) ([]string, bool) {
	if limit <= 0 {
		limit = defaultMaxSearchResults
	}
	if len(items) <= limit {
		return items, false
	}
	return items[:limit], true
}
