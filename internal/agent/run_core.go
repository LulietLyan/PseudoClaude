package agent

// run_core.go intentionally holds shared-runner extension points.
// The main ReAct loop remains in runner.go while subagent support is wired in
// through Runner.Sub and RunToCompletion.
