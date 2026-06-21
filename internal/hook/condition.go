package hook

func EvalCondition(cond *Condition, payload Payload) bool {
	if cond == nil {
		return true
	}
	switch cond.Mode {
	case CombineAllOf:
		for _, atom := range cond.Atoms {
			if atom.Matcher == nil || !atom.Matcher.Match(GetStringByPath(payload, atom.Field), false) {
				return false
			}
		}
		return true
	case CombineAnyOf:
		for _, atom := range cond.Atoms {
			if atom.Matcher != nil && atom.Matcher.Match(GetStringByPath(payload, atom.Field), false) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
