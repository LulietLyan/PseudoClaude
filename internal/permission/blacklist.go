package permission

import (
	"regexp"
	"strings"
)

// The command blacklist is heuristic, intentionally non-exhaustive, and not configurable.
var dangerousCommandPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(^|\s)rm\s+-(?:[^\s]*r[^\s]*f|[^\s]*f[^\s]*r)[^\n]*(?:\s|=)(/|~)(?:\s|$)`),
	regexp.MustCompile(`(?i)(^|\s)rm\s+-(?:[^\s]*r[^\s]*f|[^\s]*f[^\s]*r)[^\n]*(?:/bin|/boot|/dev|/etc|/home|/lib|/private|/sbin|/usr|/var)(?:\s|$)`),
	regexp.MustCompile(`(?i)(^|\s)dd\s+[^\n]*(?:^|\s)of=/dev/(?:disk|rdisk|sd|hd|vd|nvme|mapper/)`),
	regexp.MustCompile(`(?i)(^|\s)(mkfs|mke2fs|newfs|diskutil\s+eraseDisk|format)\b[^\n]*(?:/dev/|[A-Z]:)`),
	regexp.MustCompile(`:\s*\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
	regexp.MustCompile(`(?i)>\s*/dev/(?:disk|rdisk|sd|hd|vd|nvme)`),
	regexp.MustCompile(`(?i)(^|\s)chmod\s+-R\s+777\s+/(?:\s|$)`),
	regexp.MustCompile(`(?i)(^|\s)chown\s+-R\s+[^;\n]+\s+/(?:\s|$)`),
}

func hitsBlacklist(command string) (bool, string) {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return false, ""
	}
	for _, pattern := range dangerousCommandPatterns {
		if pattern.MatchString(normalized) {
			return true, pattern.String()
		}
	}
	return false, ""
}
