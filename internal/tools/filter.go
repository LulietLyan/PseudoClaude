package tools

import "sort"

var AllSubAgentDisallowedTools = []string{"Agent"}

var AsyncSubAgentAllowedTools = []string{
	"edit_file",
	"find_files",
	"install_skill",
	"load_skill",
	"read_file",
	"run_command",
	"search_code",
	"write_file",
}

var AsyncTeamMemberAllowedTools = append(append([]string{}, AsyncSubAgentAllowedTools...), "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "SendMessage")

var TeamCollaborationTools = []string{
	"TaskCreate",
	"TaskUpdate",
	"TaskList",
	"TaskGet",
	"SendMessage",
}

type FilterPolicy struct {
	DefinitionTools      []string
	DefinitionDisallowed []string
	Background           bool
	Fork                 bool
	TeamMember           bool
	InProcessTeamMember  bool
}

func FilterSubAgentTools(reg *Registry, policy FilterPolicy) []string {
	names := setFromSlice(reg.Names())
	if !policy.TeamMember {
		removeAll(names, TeamCollaborationTools)
	}
	if !policy.Fork {
		removeAll(names, AllSubAgentDisallowedTools)
	}
	if policy.Background {
		allowed := AsyncSubAgentAllowedTools
		if policy.TeamMember {
			allowed = AsyncTeamMemberAllowedTools
		}
		names = intersect(names, setFromSlice(allowed))
	}
	removeAll(names, policy.DefinitionDisallowed)
	if len(cleanNames(policy.DefinitionTools)) > 0 {
		names = intersect(names, setFromSlice(policy.DefinitionTools))
	}
	out := make([]string, 0, len(names))
	for name := range names {
		if reg.IsKnown(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func setFromSlice(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range cleanNames(values) {
		set[value] = true
	}
	return set
}

func cleanNames(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func removeAll(set map[string]bool, values []string) {
	for _, value := range cleanNames(values) {
		delete(set, value)
	}
}

func intersect(left, right map[string]bool) map[string]bool {
	out := map[string]bool{}
	for name := range left {
		if right[name] {
			out[name] = true
		}
	}
	return out
}
