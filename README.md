<a id="readme-top"></a>

<img src="./image/line-neon.gif" width="100%" alt="" /><br>

<div align="center">
  <h1>🌠 PseudoClaude</h1>
  <p><strong>A Claude Code-style terminal AI agent, built in Go.</strong></p>
  <p>
    <a href="https://github.com/LulietLyan/PseudoClaude"><strong>查看仓库 »</strong></a>
    ·
    <a href="https://github.com/LulietLyan/PseudoClaude/issues/new?labels=bug">报告问题</a>
    ·
    <a href="https://github.com/LulietLyan/PseudoClaude/issues/new?labels=enhancement">功能建议</a>
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
  <a href="https://www.sysu.edu.cn/"><img src="./image/SYSU.svg" height="50" alt="SYSU" /></a>
  <a href="https://nscc-gz.cn/"><img src="./image/NSCC-GZ.svg" height="50" alt="NSCC-GZ" /></a>
</p>

<img src="./image/line-neon.gif" width="100%" alt="" /><br>

## 📕 目录

- [项目简介](#-项目简介)
- [核心能力](#-核心能力)
- [快速开始](#-快速开始)
- [配置说明](#-配置说明)
- [常用命令](#-常用命令)
- [项目结构](#-项目结构)
- [开发路线图](#-开发路线图)
- [参与贡献](#-参与贡献)
- [许可协议](#-许可协议)

## 🤔 项目简介

**PseudoClaude** 是一个从零构建的终端 AI Agent。它以 Claude Code 风格的交互体验为目标，在本地工作区中连接大模型、工具、权限、长期记忆、MCP、Skill、Hook 与子 Agent 委派，让一次自然语言请求可以持续推进为一段完整的工程协作流程。

项目采用 Go 实现，界面基于 Charm 生态构建。当前版本聚焦本地终端里的“可控自主”：模型可以读写项目文件、搜索代码、运行验证、调用外部 MCP 工具，也会在危险操作前进入权限审批，而不是无边界地执行命令。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## ✨ 核心能力

| 能力 | 说明 |
| --- | --- |
| 多协议模型接入 | 支持 Anthropic 与 OpenAI 协议，也可通过 `base_url` 接入兼容端点。 |
| 终端 TUI | 支持 provider 选择、流式输出、Markdown 渲染、多行输入、运行状态与权限提示。 |
| Agent Loop | 一次请求内自动循环“模型推理 -> 工具调用 -> 结果回灌 -> 继续推理”，直到任务完成或触发停止条件。 |
| 本地工具 | 内置 `read_file`、`write_file`、`edit_file`、`run_command`、`find_files`、`search_code`。 |
| 权限系统 | 提供危险命令黑名单、路径沙箱、规则配置、权限模式和人在回路审批。 |
| MCP 客户端 | 可通过 stdio 或 Streamable HTTP 接入外部 MCP Server，并把远端工具注册进 Agent。 |
| 上下文管理 | 大工具结果自动落盘为可重读文件，长对话可自动或手动压缩。 |
| 会话与记忆 | 会话以 JSONL 追加存档，支持 `/resume` 恢复；长期记忆按项目级和用户级管理。 |
| Slash 命令 | `/help`、`/status`、`/compact`、`/memory`、`/skill`、`/agents` 等本地命令绕过模型执行。 |
| Skills / Hooks / Sub Agents | 用 Markdown 定义可复用 SOP，用 YAML 声明生命周期 Hook，并把探索、计划、通用任务委派给隔离子 Agent。 |

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## 😋 快速开始

### 环境要求

- Go `1.26.4` 或与项目 `go.mod` 兼容的版本
- 一个 Anthropic 或 OpenAI 协议兼容的模型服务
- 可选：Node.js / `npx`，用于运行部分 MCP Server

### 克隆与构建

```bash
git clone https://github.com/LulietLyan/PseudoClaude.git
cd PseudoClaude
go mod download
go build -o PseudoClaude ./cmd/PseudoClaude
```

### 创建配置

PseudoClaude 默认读取项目根目录下的 `.PseudoClaude/config.yaml`。最小配置示例：

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

> 请不要把真实 API Key 提交到仓库。项目已默认忽略本地 `.PseudoClaude/config.yaml`。

### 启动

```bash
./PseudoClaude
```

也可以直接用 Go 运行：

```bash
go run ./cmd/PseudoClaude
```

启动后，如果配置了多个 provider，先在 TUI 中选择模型；进入会话后直接输入任务即可。`Alt+Enter` 可输入多行，`Shift+Tab` 可切换权限模式。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## ⚙️ 配置说明

### Provider

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是 | TUI 中展示的 provider 名称。 |
| `protocol` | 是 | `anthropic` 或 `openai`。 |
| `base_url` | 否 | 自定义兼容端点；为空时使用 SDK 默认端点。 |
| `api_key` | 是 | 对应模型服务的密钥。 |
| `model` | 是 | 模型名称。 |
| `thinking` | 否 | 是否开启扩展思考；主要用于 Anthropic 协议。 |
| `context_window` | 否 | 上下文窗口 token 数。未配置时 Anthropic 默认 `200000`，OpenAI 默认 `128000`。 |

### MCP Server

同一个 `.PseudoClaude/config.yaml` 也可以声明 MCP Server：

```yaml
mcp_servers:
  context7:
    type: stdio
    command: npx
    args:
      - -y
      - "@upstash/context7-mcp"
```

MCP 工具会在启动时被发现并注册到工具中心。单个 Server 连接失败不会阻断内置工具和其它 Server。

### 权限规则

权限配置支持用户级、项目级和本地级多层加载。项目共享规则通常写在 `.PseudoClaude/permissions.yaml`，个人永久授权写在 `.PseudoClaude/permissions.local.yaml`。

```yaml
defaultMode: default

permissions:
  allow:
    - Read
    - Glob(docs/**)
    - Grep(internal/**)
    - Bash(go test ./...)
  deny:
    - Bash(git push *)
    - Write(.PseudoClaude/config.yaml)
```

可用模式包括 `strict`、`default`、`acceptEdits`、`bypassPermissions`。危险命令黑名单和路径沙箱始终优先于用户规则。

### 项目指令、记忆与扩展

| 路径 | 用途 |
| --- | --- |
| `PSEUDOCLAUDE.md` | 项目根指令，优先级最高。 |
| `.PseudoClaude/PSEUDOCLAUDE.md` | 项目配置目录指令。 |
| `~/.PseudoClaude/PSEUDOCLAUDE.md` | 用户级通用指令。 |
| `.PseudoClaude/memory/` | 项目级长期记忆。 |
| `~/.PseudoClaude/memory/` | 用户级长期记忆。 |
| `.PseudoClaude/skills/` / `~/.PseudoClaude/skills/` | 项目级或用户级 Skill。 |
| `.PseudoClaude/agents/` / `~/.PseudoClaude/agents/` | 自定义子 Agent 角色。 |
| `.PseudoClaude/hooks.yaml` / `~/.PseudoClaude/hooks.yaml` | 生命周期 Hook。 |

指令文件支持独占行 `@include <relative_path>`，用于把同目录下的规则或参考文档展开进系统提示。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## ⌨️ 常用命令

| 命令 | 说明 |
| --- | --- |
| `/help` | 查看可用 slash 命令。 |
| `/status` | 查看工作模式、权限模式、模型、token、会话和工作目录。 |
| `/session` | 查看当前会话 ID、存档位置、消息数量和模型。 |
| `/memory` | 刷新并展示当前长期记忆索引。 |
| `/permission` | 查看当前权限模式。 |
| `/plan` | 切换到计划模式，偏向只读分析和方案输出。 |
| `/do` | 切回默认执行模式。 |
| `/compact` | 手动压缩当前上下文。 |
| `/clear` | 清空当前可见对话区域，保留模型上下文。 |
| `/skill [reload]` | 查看或刷新可用 Skill。 |
| `/hooks` | 查看已加载生命周期 Hook。 |
| `/agents [reload\|name]` | 查看、刷新或检查子 Agent 角色。 |

可用 Skill 会自动注册为 `/<skill-name>` 命令，例如内置的 `commit`、`review`、`test` 工作流。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## 🧩 项目结构

```text
cmd/PseudoClaude/       CLI 入口与启动装配
internal/agent/         Agent Loop、子 Agent 工具与运行事件
internal/command/       Slash 命令注册、解析、分发与格式化
internal/compact/       工具结果落盘、上下文估算与摘要压缩
internal/config/        Provider 配置加载与校验
internal/conversation/  对话历史与持久化 hook
internal/hook/          生命周期 Hook 规则、条件与动作执行
internal/instructions/  PSEUDOCLAUDE.md 加载与 include 展开
internal/llm/           Anthropic / OpenAI 协议适配
internal/mcp/           MCP Server 配置、连接、工具适配
internal/memory/        长期记忆存储、索引与异步更新
internal/permission/    权限模式、规则、沙箱与黑名单
internal/session/       JSONL 会话存档、扫描、恢复与清理
internal/skills/        Skill 解析、加载、覆盖、渲染与安装
internal/subagent/      子 Agent 角色定义、加载与 fork 上下文
internal/task/          后台任务生命周期与查询工具
internal/tools/         本地工具注册、执行与过滤
internal/tui/           Bubble Tea 终端界面
```

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## 🗺️ 开发路线图

当前实现按开发文档分为 12 个阶段推进：

| 阶段 | 主题 |
| --- | --- |
| Part 1 | 多协议 LLM 终端对话客户端 |
| Part 2 | 本地工具系统 |
| Part 3 | Agent Loop 与 Plan/Do 工作流 |
| Part 4 | 系统提示工程化 |
| Part 5 | 权限系统 |
| Part 6 | MCP 客户端 |
| Part 7 | 上下文管理 |
| Part 8 | 项目记忆与会话恢复 |
| Part 9 | Slash 命令注册与分发 |
| Part 10 | Skill System |
| Part 11 | Hook 生命周期挂钩系统 |
| Part 12 | Sub Agent Delegation |

短期重点会继续围绕稳定性、权限持久化安全、文档完善、端到端测试和真实项目中的协作体验打磨。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## 🤝 参与贡献

欢迎提交 issue、改进建议和 pull request。建议在动手前先执行：

```bash
go test ./...
```

如果你要新增能力，优先保持现有分层边界：模型协议、Agent 编排、工具、权限、TUI 和持久化模块各自独立，避免把 UI 状态或 provider 细节泄漏到核心逻辑里。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

## ❗ 许可协议

本项目采用 MIT License 开源，完整条款请见 [LICENSE](./LICENSE)。

<p align="right">(<a href="#readme-top">返回顶部</a>)</p>

<img src="./image/line-neon.gif" width="100%" alt="" /><br>
