package coordinator

import (
	"slices"
	"strings"
	"testing"

	"PseudoClaude/internal/config"
)

func TestIsEnabledDoubleLock(t *testing.T) {
	t.Setenv(EnvVar, "")
	if IsEnabled(&config.Config{Features: config.FeatureConfig{CoordinatorMode: true}}) {
		t.Fatal("enabled with config only")
	}

	t.Setenv(EnvVar, "1")
	if IsEnabled(&config.Config{}) {
		t.Fatal("enabled with environment only")
	}
	if !IsEnabled(&config.Config{Features: config.FeatureConfig{CoordinatorMode: true}}) {
		t.Fatal("disabled when both locks are enabled")
	}

	t.Setenv(EnvVar, "off")
	if IsEnabled(&config.Config{Features: config.FeatureConfig{CoordinatorMode: true}}) {
		t.Fatal("enabled for falsey environment")
	}
}

func TestAllowedTools(t *testing.T) {
	allowed := AllowedTools()
	for _, name := range []string{"Agent", "TeamCreate", "TeamDelete", "TeamKill", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "SendMessage", "read_file", "search_code", "find_files", "run_command"} {
		if !slices.Contains(allowed, name) {
			t.Fatalf("AllowedTools missing %s: %#v", name, allowed)
		}
	}
	for _, name := range []string{"write_file", "edit_file"} {
		if slices.Contains(allowed, name) {
			t.Fatalf("AllowedTools should not contain %s: %#v", name, allowed)
		}
	}
}

func TestSystemPromptSuffix(t *testing.T) {
	text := SystemPromptSuffix()
	for _, want := range []string{"team lead", "delegate", "merge"} {
		if !strings.Contains(text, want) {
			t.Fatalf("SystemPromptSuffix = %q, want %q", text, want)
		}
	}
}
