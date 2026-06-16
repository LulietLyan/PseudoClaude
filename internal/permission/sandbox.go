package permission

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func evalSymlinksOrAncestor(abs string) (string, error) {
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}

	missing := []string{}
	cursor := abs
	for {
		if _, err := os.Stat(cursor); err == nil {
			resolved, err := filepath.EvalSymlinks(cursor)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", os.ErrNotExist
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = parent
	}
}

func insideRoot(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if target == root {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(target, root+sep)
}

func sandboxTarget(root, raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "."
	}
	target := raw
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", false, err
	}
	resolved, err := evalSymlinksOrAncestor(abs)
	if err != nil {
		return "", false, err
	}
	return resolved, insideRoot(root, resolved), nil
}
