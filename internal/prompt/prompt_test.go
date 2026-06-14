package prompt

import (
	"strings"
	"testing"
)

func TestSelectLogoByViewport(t *testing.T) {
	if got := SelectLogo(100, 24); got != LogoBanner {
		t.Fatalf("wide logo = %q, want file logo", got)
	}
	if got := SelectLogo(50, 12); got != MiniLogo {
		t.Fatalf("medium logo = %q, want mini logo", got)
	}
	if got := SelectLogo(30, 8); got != TinyLogo {
		t.Fatalf("small logo = %q, want tiny logo", got)
	}
}

func TestResponsiveBannerIncludesNextLineContent(t *testing.T) {
	banner := RenderResponsiveBanner("0.1.0", "/tmp", 30, 8)
	if !strings.Contains(banner, "PseudoClaude v0.1.0") {
		t.Fatalf("banner missing version line: %q", banner)
	}
	if strings.Contains(banner, "\n\nPseudoClaude v") {
		t.Fatalf("banner has extra blank line before version: %q", banner)
	}
}
