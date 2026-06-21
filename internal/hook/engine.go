package hook

import (
	"context"
	"sort"
	"sync"
)

type Engine struct {
	rules   []Rule
	sources []string
	exec    Executor

	mu        sync.Mutex
	onceFired map[string]bool
}

type DispatchResult struct {
	Blocked          bool
	Reason           string
	BlockingHookName string
	InjectedPrompts  []string
}

func NewEngine(rules []Rule, executor Executor, sources []string) *Engine {
	copied := append([]Rule(nil), rules...)
	sort.SliceStable(copied, func(i, j int) bool { return copied[i].Index < copied[j].Index })
	return &Engine{rules: copied, sources: append([]string(nil), sources...), exec: executor, onceFired: map[string]bool{}}
}

func (e *Engine) Dispatch(ctx context.Context, event Event, payload Payload) DispatchResult {
	if e == nil {
		return DispatchResult{}
	}
	blocking := IsBlocking(event)
	var out DispatchResult
	for _, rule := range e.rules {
		if rule.Event != event {
			continue
		}
		if e.shouldSkipOnce(rule) {
			continue
		}
		if !EvalCondition(rule.If, payload) {
			continue
		}
		e.markOnce(rule)
		if rule.Async {
			go e.runAsync(ctx, rule, payload, blocking)
			continue
		}
		result := e.exec.Run(ctx, rule, payload, blocking)
		if result.Err != nil {
			logf(e.exec.Logf, "hook %q failed: %v", rule.Name, result.Err)
			continue
		}
		if result.Prompt != "" {
			out.InjectedPrompts = append(out.InjectedPrompts, result.Prompt)
		}
		if blocking && result.Blocked {
			out.Blocked = true
			out.Reason = result.Reason
			out.BlockingHookName = rule.Name
			return out
		}
	}
	return out
}

func (e *Engine) runAsync(ctx context.Context, rule Rule, payload Payload, blocking bool) {
	result := e.exec.Run(ctx, rule, payload, blocking)
	if result.Err != nil {
		logf(e.exec.Logf, "hook %q failed: %v", rule.Name, result.Err)
	}
}

func (e *Engine) shouldSkipOnce(rule Rule) bool {
	if !rule.OnlyOnce {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.onceFired[rule.Name]
}

func (e *Engine) markOnce(rule Rule) {
	if !rule.OnlyOnce {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onceFired[rule.Name] = true
}

func (e *Engine) ResetForNewSession() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onceFired = map[string]bool{}
}

func (e *Engine) Rules() []Rule {
	if e == nil {
		return nil
	}
	return append([]Rule(nil), e.rules...)
}

func (e *Engine) Sources() []string {
	if e == nil {
		return nil
	}
	return append([]string(nil), e.sources...)
}

func (e *Engine) Summaries() []Summary {
	if e == nil {
		return nil
	}
	out := make([]Summary, 0, len(e.rules))
	for _, rule := range e.rules {
		flags := []string{}
		if rule.OnlyOnce {
			flags = append(flags, "only_once")
		}
		if rule.Async {
			flags = append(flags, "async")
		}
		if rule.Timeout > 0 && rule.Timeout != defaultTimeout {
			flags = append(flags, "timeout="+rule.Timeout.String())
		}
		out = append(out, Summary{
			Name:   rule.Name,
			Event:  string(rule.Event),
			Action: string(rule.Action.Type),
			Flags:  flags,
			Source: rule.Source,
		})
	}
	return out
}
