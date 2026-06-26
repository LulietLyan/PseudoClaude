<a id="readme-top"></a>

<img src="./image/line-neon.gif" width="100%" alt="" />

<div align="center">
  <img src="./image/PseudoClaude-logo.svg" width="148" alt="PseudoClaude logo" />
  <h1>PseudoClaude</h1>
  <p><strong>A Claude Code-style terminal AI agent written in Go.</strong></p>
  <p>
    <a href="./README-zh.md">简体中文</a>
    ·
    <a href="https://github.com/LulietLyan/PseudoClaude/issues/new?labels=bug">Report Bug</a>
    ·
    <a href="https://github.com/LulietLyan/PseudoClaude/issues/new?labels=enhancement">Request Feature</a>
  </p>
</div>

<p align="center">
  <img src="https://img.shields.io/github/license/LulietLyan/PseudoClaude?style=for-the-badge" alt="license" />
  <img src="https://img.shields.io/github/stars/LulietLyan/PseudoClaude?style=for-the-badge" alt="stars" />
  <img src="https://img.shields.io/github/forks/LulietLyan/PseudoClaude?style=for-the-badge&color=187777" alt="forks" />
  <img src="https://img.shields.io/github/issues/LulietLyan/PseudoClaude?style=for-the-badge&color=777777" alt="issues" />
  <img src="https://img.shields.io/badge/Go-1.26.4-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="go version" />
  <img src="https://img.shields.io/badge/TUI-Bubble%20Tea-FF5F87?style=for-the-badge" alt="bubble tea tui" />
</p>

<p align="center">
  <a href="https://www.sysu.edu.cn/"><img src="./image/SYSU.svg" height="48" alt="SYSU" /></a>
  &nbsp;&nbsp;
  <a href="https://nscc-gz.cn/"><img src="./image/NSCC-GZ.svg" height="48" alt="NSCC-GZ" /></a>
</p>

<img src="./image/line-neon.gif" width="100%" alt="" />

## Overview

PseudoClaude is a local terminal AI agent for software work. It connects large language models, local tools, permissions, memory, MCP servers, skills, hooks, background tasks, and delegated sub agents into one interactive TUI.

The project focuses on controlled autonomy: the model can inspect files, edit code, search a workspace, run validation commands, call external MCP tools, and continue through multi-step tasks, while permission checks and workspace boundaries keep risky operations visible and reviewable.

<p align="right"><a href="#readme-top">Back to top</a></p>

## Highlights

<table>
  <tr>
    <td><strong>Terminal-first agent loop</strong></td>
    <td>Streams model output, executes tool calls, feeds results back into the conversation, and continues until the task completes or hits a stop condition.</td>
  </tr>
  <tr>
    <td><strong>Provider support</strong></td>
    <td>Works with Anthropic and OpenAI-compatible APIs, including custom <code>base_url</code> endpoints.</td>
  </tr>
  <tr>
    <td><strong>Local engineering tools</strong></td>
    <td>Includes file read/write/edit, code search, file globbing, and command execution tools.</td>
  </tr>
  <tr>
    <td><strong>Permission system</strong></td>
    <td>Combines permission modes, path sandboxing, dangerous command checks, policy files, and interactive approval.</td>
  </tr>
  <tr>
    <td><strong>MCP integration</strong></td>
    <td>Connects stdio and Streamable HTTP MCP servers, then registers remote tools for the agent.</td>
  </tr>
  <tr>
    <td><strong>Context and memory</strong></td>
    <td>Stores large tool results off-context, supports compaction, saves sessions as JSONL, and maintains project/user memory.</td>
  </tr>
  <tr>
    <td><strong>Skills, hooks, and slash commands</strong></td>
    <td>Loads reusable skill workflows, lifecycle hooks, and local commands such as <code>/status</code>, <code>/compact</code>, <code>/agents</code>, and <code>/worktree</code>.</td>
  </tr>
  <tr>
    <td><strong>Sub agents with worktree isolation</strong></td>
    <td>Delegates focused tasks to sub agents, optionally running them in isolated Git worktrees to avoid touching the main workspace.</td>
  </tr>
  <tr>
    <td><strong>Team Lead collaboration</strong></td>
    <td>Creates persistent teams, launches named teammates in isolated worktrees, shares task lists, exchanges mailbox messages, and feeds teammate updates back to the Lead.</td>
  </tr>
</table>

<p align="right"><a href="#readme-top">Back to top</a></p>

## Quick Start

### Requirements

- Go `1.26.4` or a compatible version from `go.mod`
- A model service compatible with Anthropic or OpenAI APIs
- Optional: Node.js / `npx` for MCP servers that run through Node

### Build

```bash
git clone https://github.com/LulietLyan/PseudoClaude.git
cd PseudoClaude
go mod download
go build -o PseudoClaude ./cmd/PseudoClaude
```

### Configure

PseudoClaude reads `.PseudoClaude/config.yaml` from the project root. A minimal provider configuration looks like this:

```yaml
providers:
  - name: OpenAI
    protocol: openai
    base_url: https://api.openai.com/v1
    api_key: sk-...
    model: gpt-5
    thinking: false
    context_window: 128000

  - name: Claude
    protocol: anthropic
    api_key: sk-ant-...
    model: claude-sonnet-4-5
    thinking: true
    context_window: 200000
```

Keep real API keys out of version control. Local configuration files under `.PseudoClaude/` are ignored by default where appropriate.

### Run

```bash
./PseudoClaude
```

Or run directly with Go:

```bash
go run ./cmd/PseudoClaude
```

If multiple providers are configured, choose one in the TUI first. After that, type a task and let the agent work through it. Use `Alt+Enter` for multiline input and `Shift+Tab` to cycle permission modes.

<p align="right"><a href="#readme-top">Back to top</a></p>

## Configuration

### Provider fields

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | Display name in the TUI provider picker. |
| `protocol` | Yes | `anthropic` or `openai`. |
| `base_url` | No | Custom compatible API endpoint. |
| `api_key` | Yes | API key for the selected model service. |
| `model` | Yes | Model name. |
| `thinking` | No | Enables extended thinking where supported. |
| `context_window` | No | Context window size. Defaults are provider-specific. |

### MCP servers

MCP servers can be declared in the same configuration file:

```yaml
mcp_servers:
  context7:
    type: stdio
    command: npx
    args:
      - -y
      - "@upstash/context7-mcp"
```

If one MCP server fails to connect, built-in tools and other MCP servers remain available.

### Feature flags

Optional feature flags live under `features`:

```yaml
features:
  coordinator_mode: false
  fork_teammate: false
```

`coordinator_mode` only takes effect when both the config flag is `true` and the process is started with `PSEUDOCLAUDE_COORDINATOR_MODE=1`. In that mode, the Lead is guided to coordinate teammates, inspect results, and merge work rather than directly edit files. `fork_teammate` reserves the ability to launch teammates from the current Lead context when enabled by future team workflows.

### Permissions

Permission settings can be layered across user, project, and local files. A shared project policy usually lives at `.PseudoClaude/permissions.yaml`, while personal local decisions live at `.PseudoClaude/permissions.local.yaml`.

```yaml
defaultMode: default

permissions:
  allow:
    - Read
    - Glob(internal/**)
    - Grep(internal/**)
    - Bash(go test ./...)
  deny:
    - Bash(git push *)
    - Write(.PseudoClaude/config.yaml)
```

Available modes include `strict`, `default`, `acceptEdits`, and `bypassPermissions`. Path sandboxing and dangerous command checks are enforced before user rules.

### Project inputs

| Path | Purpose |
| --- | --- |
| `PSEUDOCLAUDE.md` | Root-level project instructions. |
| `.PseudoClaude/PSEUDOCLAUDE.md` | Project-local instruction file. |
| `~/.PseudoClaude/PSEUDOCLAUDE.md` | User-level instruction file. |
| `.PseudoClaude/memory/` | Project memory. |
| `~/.PseudoClaude/memory/` | User memory. |
| `.PseudoClaude/skills/` / `~/.PseudoClaude/skills/` | Project or user skills. |
| `.PseudoClaude/agents/` / `~/.PseudoClaude/agents/` | Custom sub agent roles. |
| `.PseudoClaude/hooks.yaml` / `~/.PseudoClaude/hooks.yaml` | Lifecycle hook rules. |

Instruction files support a standalone `@include <relative_path>` line for expanding nearby reference material into the prompt.

<p align="right"><a href="#readme-top">Back to top</a></p>

## Common Commands

| Command | Description |
| --- | --- |
| `/help` | Show slash commands. |
| `/status` | Show work mode, permission mode, model, token usage, session, and current workspace. |
| `/session` | Show current session metadata. |
| `/memory` | Refresh and display memory summary. |
| `/permission` | Show current permission mode. |
| `/plan` | Switch to read-oriented planning mode. |
| `/do` | Switch back to default execution mode. |
| `/compact` | Manually compact the current context. |
| `/clear` | Clear the visible transcript while keeping conversation context. |
| `/skill [reload]` | List or reload available skills. |
| `/hooks` | List loaded lifecycle hooks. |
| `/agents [reload\|name]` | List, reload, or inspect sub agent roles. |
| `/worktree ...` | Create, list, enter, exit, or remove managed Git worktrees. |
| `/team ...` | List teams, inspect team details, delete teams, or kill a member. |

Loaded skills can also register their own slash commands, such as `/<skill-name>`.

<p align="right"><a href="#readme-top">Back to top</a></p>

## Worktree-Isolated Sub Agents

Sub agents can run in isolated Git worktrees. This is useful when a delegated task may edit files, run tests, or commit changes while the main workspace should stay stable.

There are two ways to enable worktree isolation:

```yaml
---
name: isolated-worker
description: Runs in an isolated Git worktree.
isolation: worktree
---
You are a focused implementation agent.
```

Or pass the mode in an Agent tool call:

```json
{
  "subagent_type": "general-purpose",
  "description": "Update one file in isolation",
  "prompt": "Edit the target file and commit the result.",
  "isolation": "worktree"
}
```

When the isolated sub agent finishes, clean worktrees are removed automatically. Worktrees with protected changes or commits are retained so they can be reviewed.

<p align="right"><a href="#readme-top">Back to top</a></p>

## Team Lead Collaboration

PseudoClaude can create persistent local teams for longer-running collaboration. A Lead can create a team, assign named members such as `alice` or `bob`, and let each member work in an isolated Git worktree. Team state is stored under `~/.PseudoClaude/teams/<team>/`, including:

| Path | Purpose |
| --- | --- |
| `config.json` | Team metadata, backend choice, Lead id, member roster, worktree/session locations. |
| `inboxes/` | Per-agent mailbox files used for Lead/member communication. |
| `tasks.json` | Shared team task list with status and dependency metadata. |
| `sessions/` | Persistent member session directories. |

Core team tools include:

| Tool | Purpose |
| --- | --- |
| `TeamCreate` | Create a persistent team and return the config, inbox, and task paths. |
| `Agent` with `team_name` | Launch a named teammate in the selected team. |
| `TaskCreate` / `TaskUpdate` | Manage shared team tasks. |
| `TaskList` / `TaskGet` | List or inspect shared team tasks when `team_name` is provided; otherwise they keep the legacy background-task behavior. |
| `SendMessage` | Send a team mailbox message, including broadcast messages; idle in-process teammates are resumed automatically; legacy background-agent messages still work when no team recipient is detected. |
| `TeamDelete` / `TeamKill` | Delete a team or terminate a member. |

Example flow:

```text
Create a team named demo, launch alice, and ask her to read README.md and report the main sections.
```

PseudoClaude creates the team, starts `alice` with an in-process backend when no terminal pane backend is available, and injects teammate replies back into the Lead as `<team-update>` reminders. Idle in-process teammates keep their conversation context, so a later `SendMessage` resumes the same teammate instead of starting over. When the Lead is idle, unread team updates trigger a new coordination turn automatically. Use `/team list` and `/team info demo` to inspect the current state.

Current scope: the in-process backend is the reliable default path. The tmux/iTerm2 pane backends are detected but still intended for follow-up hardening before being treated as the primary workflow.

<p align="right"><a href="#readme-top">Back to top</a></p>

## Project Structure

```text
cmd/PseudoClaude/       Application entry point
internal/agent/         Agent loop, sub agent tool, runner events
internal/command/       Slash command parsing, dispatch, formatting
internal/compact/       Tool-result offloading and context compaction
internal/config/        Configuration loading and validation
internal/conversation/  Conversation history helpers
internal/hook/          Lifecycle hooks and prompt injection
internal/instructions/  Instruction loading and include expansion
internal/llm/           Anthropic and OpenAI protocol adapters
internal/mcp/           MCP server connection and tool adapters
internal/memory/        Project and user memory
internal/permission/    Permission rules, sandboxing, and safety checks
internal/session/       JSONL session storage and restore support
internal/skills/        Skill parsing, loading, rendering, and install logic
internal/subagent/      Sub agent definitions, catalog, and fork context
internal/task/          Background task lifecycle and task tools
internal/team/          Persistent Team Lead collaboration state, mailboxes, tasks, and team tools
internal/tools/         Built-in tool registry and execution
internal/tui/           Bubble Tea terminal interface
internal/worktree/      Managed Git worktree lifecycle
image/                  README images and visual assets
```

<p align="right"><a href="#readme-top">Back to top</a></p>

## Development

Run the full verification suite:

```bash
go test ./...
go vet ./...
go build ./...
```

For a local binary:

```bash
go build -o ./PseudoClaude ./cmd/PseudoClaude
```

<p align="right"><a href="#readme-top">Back to top</a></p>

## License

This project is released under the license included in [`LICENSE`](./LICENSE).

<img src="./image/line-neon.gif" width="100%" alt="" />
