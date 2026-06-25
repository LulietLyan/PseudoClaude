package registry

import "sync"

type AgentNameRegistry struct {
	mu      sync.RWMutex
	byName  map[string]string
	byAgent map[string]string
}

func New() *AgentNameRegistry {
	return &AgentNameRegistry{byName: map[string]string{}, byAgent: map[string]string{}}
}

func (r *AgentNameRegistry) Register(name, agentID string) {
	if r == nil || name == "" || agentID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if oldAgent := r.byName[name]; oldAgent != "" && oldAgent != agentID {
		delete(r.byAgent, oldAgent)
	}
	if oldName := r.byAgent[agentID]; oldName != "" && oldName != name {
		delete(r.byName, oldName)
	}
	r.byName[name] = agentID
	r.byAgent[agentID] = name
}

func (r *AgentNameRegistry) Unregister(name string) {
	if r == nil || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if agentID := r.byName[name]; agentID != "" {
		delete(r.byAgent, agentID)
	}
	delete(r.byName, name)
}

func (r *AgentNameRegistry) UnregisterByAgentID(agentID string) {
	if r == nil || agentID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if name := r.byAgent[agentID]; name != "" {
		delete(r.byName, name)
	}
	delete(r.byAgent, agentID)
}

func (r *AgentNameRegistry) Resolve(value string) (string, bool) {
	if r == nil || value == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if agentID := r.byName[value]; agentID != "" {
		return agentID, true
	}
	if _, ok := r.byAgent[value]; ok {
		return value, true
	}
	return "", false
}

func (r *AgentNameRegistry) NameOf(agentID string) (string, bool) {
	if r == nil || agentID == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.byAgent[agentID]
	return name, ok
}

func (r *AgentNameRegistry) List() map[string]string {
	out := map[string]string{}
	if r == nil {
		return out
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, agentID := range r.byName {
		out[name] = agentID
	}
	return out
}
