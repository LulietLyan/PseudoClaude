package skills

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	body := ReplaceArguments("a $ARGUMENTS b {{arguments}}", "x")
	if body != "a x b x" {
		t.Fatalf("body = %q", body)
	}
	body = ReplaceArguments("$ARGUMENTS{{arguments}}", "")
	if body != "" {
		t.Fatalf("empty args body = %q", body)
	}
	rendered := RenderInvocation(Skill{Meta: SkillMeta{Tools: []string{"read_file"}}, Body: "Body"}, "x")
	if !strings.Contains(rendered, "read_file") || !strings.Contains(rendered, "Body") {
		t.Fatalf("rendered = %q", rendered)
	}
}
