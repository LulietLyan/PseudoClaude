package skills

import (
	"fmt"
	"strings"
)

func ReplaceArguments(body, args string) string {
	body = strings.ReplaceAll(body, "$ARGUMENTS", args)
	body = strings.ReplaceAll(body, "{{arguments}}", args)
	return body
}

func RenderInvocation(skill Skill, args string) string {
	body := ReplaceArguments(skill.Body, args)
	if len(skill.Meta.Tools) == 0 {
		return body
	}
	return fmt.Sprintf("Allowed tools for this skill: %s\n\n%s", strings.Join(skill.Meta.Tools, ", "), body)
}
