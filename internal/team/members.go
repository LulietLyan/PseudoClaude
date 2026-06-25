package team

import "time"

func (t *Team) AddMember(homeDir string, member MemberInfo) error {
	if t == nil {
		return ErrTeamNotFound
	}
	if err := t.reloadMembers(homeDir); err != nil {
		return err
	}
	if _, ok := findMember(t.Members, member.Name); ok {
		return ErrMemberExists
	}
	member.LastUpdatedAt = time.Now()
	t.Members = append(t.Members, member)
	return t.save()
}

func (t *Team) SetMemberActive(homeDir, name string, active bool) error {
	if t == nil {
		return ErrTeamNotFound
	}
	if err := t.reloadMembers(homeDir); err != nil {
		return err
	}
	for i := range t.Members {
		if t.Members[i].Name == name || t.Members[i].AgentID == name {
			t.Members[i].IsActive = boolPtr(active)
			t.Members[i].LastUpdatedAt = time.Now()
			return t.save()
		}
	}
	return ErrMemberNotFound
}

func (t *Team) RemoveMember(homeDir, name string) error {
	if t == nil {
		return ErrTeamNotFound
	}
	if err := t.reloadMembers(homeDir); err != nil {
		return err
	}
	for i := range t.Members {
		if t.Members[i].Name == name || t.Members[i].AgentID == name {
			t.Members = append(t.Members[:i], t.Members[i+1:]...)
			return t.save()
		}
	}
	return ErrMemberNotFound
}

func (t *Team) MemberByName(name string) (MemberInfo, bool) {
	if t == nil {
		return MemberInfo{}, false
	}
	return findMember(t.Members, name)
}

func (t *Team) MemberByAgentID(agentID string) (MemberInfo, bool) {
	if t == nil {
		return MemberInfo{}, false
	}
	for _, member := range t.Members {
		if member.AgentID == agentID {
			return member, true
		}
	}
	return MemberInfo{}, false
}

func findMember(members []MemberInfo, name string) (MemberInfo, bool) {
	for _, member := range members {
		if member.Name == name || member.AgentID == name {
			return member, true
		}
	}
	return MemberInfo{}, false
}

func boolPtr(v bool) *bool {
	return &v
}
