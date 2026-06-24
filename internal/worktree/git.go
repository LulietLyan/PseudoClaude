package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout string, stderr string, err error)
}

type execGitRunner struct{}

func (execGitRunner) Run(ctx context.Context, dir string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
	out, err := cmd.Output()
	if err == nil {
		return string(out), "", nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), string(exitErr.Stderr), err
	}
	return string(out), "", err
}

func runGitTrimmed(ctx context.Context, git GitRunner, dir string, args ...string) (string, error) {
	stdout, stderr, err := git.Run(ctx, dir, args...)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout), nil
}

func ensureRepoRoot(ctx context.Context, git GitRunner, repoRoot string) (string, error) {
	if repoRoot == "" {
		repoRoot = "."
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	top, err := runGitTrimmed(ctx, git, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	topAbs, err := filepath.Abs(top)
	if err != nil {
		return "", err
	}
	return filepath.Clean(topAbs), nil
}

func fastRecover(path string) (*Worktree, error) {
	gitFile := filepath.Join(path, ".git")
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return nil, err
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return nil, fmt.Errorf("unrecognized .git pointer")
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}
	headData, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return nil, err
	}
	head := strings.TrimSpace(string(headData))
	branch := head
	commit := head
	if strings.HasPrefix(head, "ref: ") {
		ref := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
		branch = strings.TrimPrefix(ref, "refs/heads/")
		if data, err := os.ReadFile(filepath.Join(gitDir, ref)); err == nil {
			commit = strings.TrimSpace(string(data))
		} else {
			commondir := filepath.Join(gitDir, "commondir")
			if commonData, commonErr := os.ReadFile(commondir); commonErr == nil {
				common := strings.TrimSpace(string(commonData))
				if !filepath.IsAbs(common) {
					common = filepath.Join(gitDir, common)
				}
				if data, commonRefErr := os.ReadFile(filepath.Join(common, ref)); commonRefErr == nil {
					commit = strings.TrimSpace(string(data))
				}
			}
		}
	}
	if branch == "" {
		return nil, fmt.Errorf("missing branch")
	}
	return &Worktree{Path: path, Branch: branch, HeadCommit: commit}, nil
}

func hasProtectedChanges(ctx context.Context, git GitRunner, wt *Worktree) (bool, string) {
	if wt == nil {
		return true, "worktree metadata is missing"
	}
	status, err := runGitTrimmed(ctx, git, wt.Path, "status", "--porcelain")
	if err != nil {
		return true, "status check failed: " + err.Error()
	}
	if strings.TrimSpace(status) != "" {
		return true, "uncommitted changes"
	}
	if wt.HeadCommit != "" {
		count, err := runGitTrimmed(ctx, git, wt.Path, "rev-list", "--count", wt.HeadCommit+"..HEAD")
		if err != nil {
			return true, "local commit check failed: " + err.Error()
		}
		if strings.TrimSpace(count) != "0" {
			return true, "new local commits"
		}
	}
	if _, err := runGitTrimmed(ctx, git, wt.Path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		count, err := runGitTrimmed(ctx, git, wt.Path, "rev-list", "--count", "@{u}..HEAD")
		if err != nil {
			return true, "unpushed commit check failed: " + err.Error()
		}
		if strings.TrimSpace(count) != "0" {
			return true, "unpushed commits"
		}
	}
	return false, ""
}
