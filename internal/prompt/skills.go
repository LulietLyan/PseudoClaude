package prompt

import (
	"strings"
)

type SkillCatalogItem struct {
	Name        string
	Description string
}

type ActiveSkillEntry struct {
	Name string
	Body string
}

func RenderSkillsCatalog(items []SkillCatalogItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available Skills\n\n")
	b.WriteString("Only names and short descriptions are listed here. When you need a skill's full SOP, call the load_skill system tool by name.\n\n")
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		description := strings.TrimSpace(item.Description)
		if name == "" || description == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(description)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func RenderActiveSkills(entries []ActiveSkillEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Active Skills\n")
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		body := strings.TrimSpace(entry.Body)
		if name == "" || body == "" {
			continue
		}
		b.WriteString("\n### ")
		b.WriteString(name)
		b.WriteString("\n\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
