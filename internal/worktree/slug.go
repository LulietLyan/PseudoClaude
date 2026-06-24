package worktree

import (
	"fmt"
	"strings"
)

func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if len(name) > DefaultNameLimit {
		return fmt.Errorf("%w: name exceeds %d characters", ErrInvalidName, DefaultNameLimit)
	}
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return fmt.Errorf("%w: name must not start or end with slash", ErrInvalidName)
	}
	if strings.Contains(name, "//") {
		return fmt.Errorf("%w: name must not contain empty path segments", ErrInvalidName)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" {
			return fmt.Errorf("%w: name contains an empty segment", ErrInvalidName)
		}
		if part == "." || part == ".." {
			return fmt.Errorf("%w: name contains reserved segment %q", ErrInvalidName, part)
		}
		for _, r := range part {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= 'A' && r <= 'Z':
			case r >= '0' && r <= '9':
			case r == '.', r == '_', r == '-':
			default:
				return fmt.Errorf("%w: name contains unsupported character %q", ErrInvalidName, r)
			}
		}
	}
	return nil
}

func FlatName(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	return strings.ReplaceAll(name, "/", "+"), nil
}

func branchName(flat string) string {
	return "worktree-" + flat
}
