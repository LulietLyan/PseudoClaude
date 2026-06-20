package command

import (
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	entries []Command
	index   map[string]int
}

type Completion struct {
	Text        string
	Name        string
	Alias       string
	Description string
	Kind        Kind
	Skill       bool
}

func NewRegistry(commands []Command) (*Registry, error) {
	r := &Registry{
		entries: make([]Command, len(commands)),
		index:   make(map[string]int),
	}
	copy(r.entries, commands)
	for i := range r.entries {
		cmd := &r.entries[i]
		name, err := normalizeName(cmd.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid command %q: %w", cmd.Name, err)
		}
		if cmd.Handler == nil {
			return nil, fmt.Errorf("invalid command %s: handler is nil", name)
		}
		cmd.Name = name
		seen := map[string]struct{}{}
		if err := r.addIndex(name, i, seen); err != nil {
			return nil, err
		}
		for j, alias := range cmd.Aliases {
			normalized, err := normalizeName(alias)
			if err != nil {
				return nil, fmt.Errorf("invalid alias %q for %s: %w", alias, cmd.Name, err)
			}
			cmd.Aliases[j] = normalized
			if err := r.addIndex(normalized, i, seen); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

func MustNewRegistry(commands []Command) *Registry {
	r, err := NewRegistry(commands)
	if err != nil {
		panic(err)
	}
	return r
}

func (r *Registry) Lookup(token string) (*Command, bool) {
	if r == nil {
		return nil, false
	}
	normalized, err := normalizeName(token)
	if err != nil {
		return nil, false
	}
	i, ok := r.index[normalized]
	if !ok {
		return nil, false
	}
	cmd := r.entries[i]
	return &cmd, true
}

func (r *Registry) Register(cmd Command) error {
	if cmd.Handler == nil {
		return fmt.Errorf("invalid command %s: handler is nil", cmd.Name)
	}
	name, err := normalizeName(cmd.Name)
	if err != nil {
		return fmt.Errorf("invalid command %q: %w", cmd.Name, err)
	}
	cmd.Name = name
	seen := map[string]struct{}{}
	index := len(r.entries)
	if r.index == nil {
		r.index = make(map[string]int)
	}
	if err := r.addIndex(name, index, seen); err != nil {
		return err
	}
	for i, alias := range cmd.Aliases {
		normalized, err := normalizeName(alias)
		if err != nil {
			delete(r.index, name)
			return fmt.Errorf("invalid alias %q for %s: %w", alias, cmd.Name, err)
		}
		cmd.Aliases[i] = normalized
		if err := r.addIndex(normalized, index, seen); err != nil {
			delete(r.index, name)
			return err
		}
	}
	r.entries = append(r.entries, cmd)
	return nil
}

func (r *Registry) RemoveWhere(match func(Command) bool) {
	if r == nil || match == nil {
		return
	}
	next := make([]Command, 0, len(r.entries))
	for _, cmd := range r.entries {
		if !match(copyCommand(cmd)) {
			next = append(next, cmd)
		}
	}
	r.entries = next
	r.index = make(map[string]int)
	for i := range r.entries {
		seen := map[string]struct{}{}
		_ = r.addIndex(r.entries[i].Name, i, seen)
		for _, alias := range r.entries[i].Aliases {
			_ = r.addIndex(alias, i, seen)
		}
	}
}

func (r *Registry) Has(token string) bool {
	_, ok := r.Lookup(token)
	return ok
}

func (r *Registry) Visible() []Command {
	if r == nil {
		return nil
	}
	var out []Command
	for _, cmd := range r.entries {
		if cmd.Hidden {
			continue
		}
		out = append(out, copyCommand(cmd))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Complete(prefix string) []Completion {
	if r == nil {
		return nil
	}
	normalized, err := normalizeName(prefix)
	if err != nil {
		return nil
	}
	byName := map[string]Completion{}
	for _, cmd := range r.Visible() {
		if strings.HasPrefix(cmd.Name, normalized) {
			byName[cmd.Name] = Completion{Text: cmd.Name, Name: cmd.Name, Description: cmd.Description, Kind: cmd.Kind, Skill: cmd.Skill}
		}
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, normalized) {
				if _, exists := byName[cmd.Name]; !exists {
					byName[cmd.Name] = Completion{Text: cmd.Name, Name: cmd.Name, Alias: alias, Description: cmd.Description, Kind: cmd.Kind, Skill: cmd.Skill}
				}
			}
		}
	}
	out := make([]Completion, 0, len(byName))
	for _, item := range byName {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) addIndex(token string, commandIndex int, localSeen map[string]struct{}) error {
	if _, ok := localSeen[token]; ok {
		return fmt.Errorf("command %s has duplicate name or alias %s", r.entries[commandIndex].Name, token)
	}
	localSeen[token] = struct{}{}
	if existing, ok := r.index[token]; ok {
		return fmt.Errorf("command name conflict for %s between %s and %s", token, r.entries[existing].Name, r.entries[commandIndex].Name)
	}
	r.index[token] = commandIndex
	return nil
}

func normalizeName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("name is empty")
	}
	if !strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("name must start with /")
	}
	return name, nil
}

func copyCommand(cmd Command) Command {
	cmd.Aliases = append([]string(nil), cmd.Aliases...)
	return cmd
}
