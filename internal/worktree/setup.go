package worktree

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) postCreateSetup(ctx context.Context, wt *Worktree) {
	for _, err := range []error{
		m.copyLocalConfig(wt),
		m.setupHooks(ctx, wt),
		m.symlinkLargeDirs(wt),
		m.copyIncludedIgnoredFiles(ctx, wt),
	} {
		if err != nil {
			m.logf("worktree setup warning for %s: %v", wt.Name, err)
		}
	}
}

func (m *Manager) copyLocalConfig(wt *Worktree) error {
	for _, rel := range []string{
		filepath.Join(DefaultMetaDirName, "config.yaml"),
		filepath.Join(DefaultMetaDirName, "permissions.local.yaml"),
		filepath.Join(DefaultMetaDirName, "hooks.yaml"),
		filepath.Join(DefaultMetaDirName, "agents"),
		filepath.Join(DefaultMetaDirName, "skills"),
	} {
		src := filepath.Join(m.repoRoot, rel)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(wt.Path, rel)
		if err := copyPath(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) setupHooks(ctx context.Context, wt *Worktree) error {
	hooks, err := runGitTrimmed(ctx, m.git, m.repoRoot, "config", "--get", "core.hooksPath")
	if err != nil || strings.TrimSpace(hooks) == "" {
		if _, statErr := os.Stat(filepath.Join(m.repoRoot, ".husky")); statErr != nil {
			return nil
		}
		hooks = filepath.Join(m.repoRoot, ".husky")
	}
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(m.repoRoot, hooks)
	}
	if _, err := os.Stat(hooks); err != nil {
		return nil
	}
	_, err = runGitTrimmed(ctx, m.git, wt.Path, "config", "core.hooksPath", hooks)
	return err
}

func (m *Manager) symlinkLargeDirs(wt *Worktree) error {
	for _, rel := range m.symlinkDirs {
		src := filepath.Join(m.repoRoot, rel)
		dst := filepath.Join(wt.Path, rel)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if err := os.Symlink(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) copyIncludedIgnoredFiles(ctx context.Context, wt *Worktree) error {
	rules, err := readIncludeRules(filepath.Join(m.repoRoot, ".worktreeinclude"))
	if err != nil || len(rules) == 0 {
		return nil
	}
	ignored, err := runGitTrimmed(ctx, m.git, m.repoRoot, "ls-files", "--others", "-i", "--exclude-standard")
	if err != nil {
		return err
	}
	for _, rel := range strings.Split(ignored, "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" || !matchesAnyRule(rel, rules) {
			continue
		}
		src := filepath.Join(m.repoRoot, rel)
		if !insidePath(m.repoRoot, src) {
			continue
		}
		if err := copyPath(src, filepath.Join(wt.Path, rel)); err != nil {
			return err
		}
	}
	return nil
}

func readIncludeRules(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rules []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, filepath.ToSlash(line))
	}
	return rules, scanner.Err()
}

func matchesAnyRule(rel string, rules []string) bool {
	rel = filepath.ToSlash(rel)
	for _, rule := range rules {
		if ok, _ := filepath.Match(rule, rel); ok {
			return true
		}
		if strings.HasSuffix(rule, "/") && strings.HasPrefix(rel, strings.TrimSuffix(rule, "/")+"/") {
			return true
		}
	}
	return false
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyFile(path, target)
		})
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func insidePath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
