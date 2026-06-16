# 权限系统 Plan

## 技术栈

- 语言：Go 1.26.4
- 配置格式：YAML，沿用 `gopkg.in/yaml.v3`
- LLM SDK：沿用 `github.com/anthropics/anthropic-sdk-go v1.33.0` 与 `github.com/openai/openai-go/v3 v3.32.0`
- TUI：沿用 bubbletea、bubbles、lipgloss、glamour
- 工具系统：沿用 `internal/tools` 的注册中心、工具定义、安全等级和结构化结果
- Agent Loop：沿用 `internal/agent` 的 Run 循环、工具分批执行、结果回灌和计划工作流
- 权限判定位置：新增 `internal/permission` 包，并在 `internal/agent` 工具执行前接入；本章不修改 provider 适配层
- 测试：Go 单元测试为主，补充 TUI 状态与人在回路的消息级测试

## 架构概览

Part_5 在现有 Agent 工具执行链路前增加一个统一权限闸门。权限判定与 provider 协议无关，发生在模型已经返回工具调用之后、工具真正执行之前。整体分为四个模块：

1. `permission` 包承载权限系统的核心：危险命令黑名单、路径沙箱、规则加载与匹配、权限模式兜底、会话临时规则和永久规则写入。它实现前四层自动判定，并用 `Ask` 表示需要进入人在回路。
2. `agent` 包在执行工具前调用权限引擎。`Allow` 时执行工具，`Deny` 时构造结构化拒绝结果回灌，`Ask` 时发出审批事件并等待用户选择。这一层承载第五层人在回路，因此 Agent Loop 不因权限拒绝而终止。
3. `tui` 包新增审批交互态和权限模式显示。用户可在会话中切换 strict/default/acceptEdits/bypassPermissions；当 Agent 发出审批事件时，TUI 展示待批准调用并回传允许本次、允许本会话、永久允许或拒绝本次。
4. `cmd/PseudoClaude` 在启动时构造权限引擎并注入 TUI。provider 配置继续由现有 `internal/config` 管理，权限配置使用独立文件，不与 API key 配置混在一起。

五层判定边界：

- `permission.Engine.Check` 实现黑名单、沙箱、规则引擎、权限模式四层。
- `agent.Runner` 在 `Check` 返回 `Ask` 后发起人在回路。
- 黑名单和沙箱永远在规则和模式之前；模式只处理规则未命中的兜底，不覆盖显式规则。

数据流：

```text
TUI / 测试消费者
  -> agent.Runner.Run(ctx, agent.Request{PermissionMode, WorkflowMode})
  -> provider.Stream(...) 生成工具调用
  -> agent.executeToolCalls(...)
  -> permission.Engine.Check(mode, call, safety)
       Deny  -> tools.Failure(... permission_denied ...) -> conversation tool result
       Allow -> tools.Registry.Execute(...)
       Ask   -> agent.EventApproval -> TUI stateApproving -> permission.ApprovalDecision
  -> conversation.AddToolResult(...)
  -> 下一轮模型请求继续
```

依赖方向保持无环：

```text
cmd/PseudoClaude -> tui
tui              -> agent, permission
agent            -> permission, llm, tools, conversation, prompt
permission       -> llm, tools, 标准库
tools            -> 标准库
llm              -> config, tools
```

## 核心数据结构

### `permission.Mode`

```go
type Mode string

const (
    ModeStrict            Mode = "strict"
    ModeDefault           Mode = "default"
    ModeAcceptEdits       Mode = "acceptEdits"
    ModeBypassPermissions Mode = "bypassPermissions"
)
```

职责：

- 表示规则未命中时的权限兜底档位。
- 与 `agent.Mode` 保持分离：`agent.Mode` 继续表示 chat/plan/do 工作流，`permission.Mode` 只表示权限策略。
- `Mode.String()` 返回配置和状态栏使用的稳定文本。
- `ParseMode` 从 YAML 或用户切换状态解析模式；未知值降级为 `ModeDefault`。

### `permission.Decision`

```go
type Decision string

const (
    DecisionAllow Decision = "allow"
    DecisionDeny  Decision = "deny"
    DecisionAsk   Decision = "ask"
)
```

职责：

- 表示权限流水线中的裁决。
- 黑名单、沙箱和显式 deny 规则可以返回 `Deny`。
- 显式 allow 规则和模式兜底可以返回 `Allow`。
- 模式兜底在需要人工确认时返回 `Ask`。

### `permission.Category`

```go
type Category string

const (
    CategoryRead  Category = "read"
    CategoryWrite Category = "write"
    CategoryExec  Category = "exec"
)
```

职责：

- 对工具调用进行权限分类。
- `read_file`、`find_files`、`search_code` 归为只读。
- `write_file`、`edit_file` 归为文件写。
- `run_command` 归为命令执行。
- 未知工具或类别无法判断时按不确定调用处理，不静默放行。

### `permission.Rule`

```go
type Rule struct {
    Tool    string
    Pattern string
    Action  Decision
}
```

职责：

- 表示一条 allow 或 deny 规则。
- `Tool` 使用友好名：`Bash`、`Read`、`Write`、`Edit`、`Glob`、`Grep`。
- `Pattern` 为空表示匹配该工具全部调用。
- `Action` 只能是 `DecisionAllow` 或 `DecisionDeny`。

内部工具名映射：

| 友好名 | 内部工具名 |
| --- | --- |
| Bash | `run_command` |
| Read | `read_file` |
| Write | `write_file` |
| Edit | `edit_file` |
| Glob | `find_files` |
| Grep | `search_code` |

### `permission.RuleSet`

```go
type RuleSet struct {
    Allow []Rule
    Deny  []Rule
}
```

职责：

- 表示同一优先级层内的一组规则。
- 匹配时同层 deny 优先于 allow。
- 用户级、项目级、本地级、会话级都使用同一结构。

### `permission.Settings`

```go
type Settings struct {
    DefaultMode string `yaml:"defaultMode"`
    Permissions struct {
        Allow []string `yaml:"allow"`
        Deny  []string `yaml:"deny"`
    } `yaml:"permissions"`
}
```

职责：

- 映射单个 YAML 权限配置文件。
- `defaultMode` 可选，支持 `strict`、`default`、`acceptEdits`、`bypassPermissions`。
- `permissions.allow` 和 `permissions.deny` 使用 `Tool(pattern)` 字符串。
- 文件缺失或格式非法时降级为空设置。

推荐配置路径：

| 层级 | 路径 | 是否建议提交 |
| --- | --- | --- |
| 用户级 | `~/.PseudoClaude/permissions.yaml` | 否 |
| 项目级 | `.PseudoClaude/permissions.yaml` | 是 |
| 本地级 | `.PseudoClaude/permissions.local.yaml` | 否，加入 `.gitignore` |

### `permission.Engine`

```go
type Engine struct {
    root        string
    blacklist  []*regexp.Regexp
    session    RuleSet
    local      RuleSet
    project    RuleSet
    user       RuleSet
    localPath  string
    startMode  Mode
    loadIssues []LoadIssue
}
```

职责：

- 持有项目根目录、内置黑名单和四层规则集。
- `session` 存放“允许本会话”生成的临时规则，优先于本地级。
- `local`、`project`、`user` 来自 YAML 文件。
- `localPath` 是永久允许写入目标。
- `startMode` 是启动默认权限模式，按本地级、项目级、用户级读取。
- `loadIssues` 保存非致命配置加载问题，供 TUI 或测试观察。

### `permission.CheckResult`

```go
type CheckResult struct {
    Decision Decision
    Source   string
    Reason   string
    Rule     string
    Category Category
    Target   string
}
```

职责：

- 表达一次自动权限检查结果。
- `Source` 取值包括 `blacklist`、`sandbox`、`rule`、`mode`、`unknown`。
- `Reason` 用于 TUI 展示和模型回灌。
- `Target` 是规则匹配对象：命令串或项目相对路径。

### `permission.ApprovalDecision`

```go
type ApprovalDecision string

const (
    ApprovalAllowOnce    ApprovalDecision = "allow_once"
    ApprovalAllowSession ApprovalDecision = "allow_session"
    ApprovalAllowForever ApprovalDecision = "allow_forever"
    ApprovalDenyOnce     ApprovalDecision = "deny_once"
)
```

职责：

- 表示人在回路的用户选择。
- `ApprovalAllowOnce` 只放行当前调用。
- `ApprovalAllowSession` 写入内存会话级精确 allow 规则。
- `ApprovalAllowForever` 写入本地级 YAML 精确 allow 规则，并同步内存规则。
- `ApprovalDenyOnce` 返回结构化拒绝结果。

### `agent.ApprovalRequest`

```go
type ApprovalRequest struct {
    Call    llm.ToolCall
    Summary string
    Reason  string
    Result  permission.CheckResult
    Respond chan permission.ApprovalDecision
}
```

职责：

- Agent 在权限结果为 `Ask` 时通过事件发给 TUI。
- `Summary` 是关键参数预览。
- `Respond` 使用容量为 1 的 channel，避免 TUI 回传时阻塞。
- Agent 等待 `Respond` 或 `ctx.Done()`，保证取消能打断人在回路。

## 核心接口

### `permission` 包

```go
func NewEngine(root string, opts Options) (*Engine, error)

type Options struct {
    UserPath    string
    ProjectPath string
    LocalPath   string
}

func DefaultOptions(root string) Options
func (e *Engine) StartMode() Mode
func (e *Engine) LoadIssues() []LoadIssue
func (e *Engine) Check(mode Mode, call llm.ToolCall, safety tools.Safety) CheckResult
func (e *Engine) AllowForSession(call llm.ToolCall) error
func (e *Engine) PersistLocalAllow(call llm.ToolCall) error
```

说明：

- `NewEngine` 解析项目根、加载三层配置、编译黑名单、确定启动默认模式。配置文件错误只进入 `LoadIssues`，不导致引擎构造失败。
- `DefaultOptions` 生成用户级、项目级、本地级默认路径。
- `Check` 执行前四层判定。`safety` 来自工具注册表，用于辅助未知工具和只读工具分类。
- `AllowForSession` 生成当前调用的精确 allow 规则并加入内存会话层。
- `PersistLocalAllow` 生成当前调用的精确 allow 规则，写入本地级配置并同步内存本地层。

内部辅助接口：

```go
func commandText(call llm.ToolCall) (string, bool)
func pathTarget(call llm.ToolCall) (string, bool)
func classify(call llm.ToolCall, safety tools.Safety) Category
func friendlyName(internal string) string
func parseRule(text string, action Decision) (Rule, bool)
func (rs RuleSet) Match(tool, target string, isPath bool) (CheckResult, bool)
func modeFallback(mode Mode, category Category) Decision
func nextMode(mode Mode) Mode
```

匹配对象规则：

- `run_command` 使用规范化命令串匹配：`command` 加 `args`，以单空格连接；包含空白的参数用 Go 字符串引号表示，保证生成规则和匹配规则稳定。
- `read_file`、`write_file`、`edit_file` 使用项目相对路径匹配。
- `search_code` 使用 `path` 字段作为搜索根；为空时目标为 `.`。
- `find_files` 使用 glob pattern 的确定性根作为沙箱目标，规则匹配使用原始 pattern 的项目相对形式。

### `agent.Runner`

```go
type Runner struct {
    Provider   llm.Provider
    Registry   *tools.Registry
    Env        tools.Env
    Config     Config
    Version    string
    Permission *permission.Engine
}

type Request struct {
    Mode           Mode
    PermissionMode permission.Mode
    UserText       string
    PlanTask       string
    PlanText       string
    Conversation   *conversation.Conversation
}
```

说明：

- `Mode` 继续表示工作流：`chat`、`plan`、`do`。
- `PermissionMode` 表示权限兜底档位。为空时使用 `Permission.StartMode()` 或 `permission.ModeDefault`。
- `Runner` 在构造模型请求时仍按 `ModePlan` 只暴露只读工具并注入计划提醒。
- 工具执行前统一调用 `Permission.Check`。

### `agent.Event`

```go
const EventApproval EventType = "approval"

type Event struct {
    Type       EventType
    Iteration  int
    Text       string
    Message    string
    ToolCall   *llm.ToolCall
    ToolResult *ToolResult
    Approval   *ApprovalRequest
    Usage      *llm.Usage
    Stop       *Stop
    Err        error
}
```

说明：

- `EventApproval` 表示 Agent Loop 暂停等待用户审批。
- TUI 收到后进入 `stateApproving`，回传 `ApprovalDecision` 后继续监听事件。

## 模块设计

### `permission` 包

**职责：** 权限核心逻辑，包括黑名单、沙箱、规则、模式兜底、配置加载和规则持久化。

**对外接口：** `NewEngine`、`DefaultOptions`、`StartMode`、`LoadIssues`、`Check`、`AllowForSession`、`PersistLocalAllow`、`ParseMode`、`NextMode`。

**实现要点：**

- `Check` 按黑名单、沙箱、规则、模式顺序 early return。
- 黑名单只对 `run_command` 生效，匹配规范化命令串；命中后直接返回 `Deny`，`Source=blacklist`。
- 沙箱只对文件类工具生效。路径先转绝对路径，再解析符号链接或最近已存在祖先目录，再做项目根前缀判断。
- 规则匹配顺序为会话级、本地级、项目级、用户级；每一层内先 deny 后 allow。
- 模式兜底矩阵与 spec F5 保持一致，且只返回 `Allow` 或 `Ask`。
- `AllowForSession` 和 `PersistLocalAllow` 都生成精确 allow 规则，不自动泛化。
- 本地权限文件写入时保留已有规则，追加缺失规则；写入失败不应让当前允许选择变成拒绝，但应产生可观察错误事件或进度提示。

黑名单示例集合：

- 递归强删根目录、家目录或通配根路径。
- `dd` 写入 `/dev/*` 块设备。
- `mkfs`、`diskutil erase`、`format` 等格式化模式。
- fork bomb 常见形态。
- 重定向覆盖磁盘设备。
- 对系统根目录执行明显破坏性权限变更。

### `permission` 配置加载

**职责：** 读取用户级、项目级、本地级 YAML，并转换为 `RuleSet`。

**对外接口：** 由 `NewEngine` 调用，不暴露给其他包作为主要入口。

**实现要点：**

- 文件不存在视为空配置。
- YAML 解析失败时记录 `LoadIssue`，该层降级为空规则集。
- 非法规则项跳过并记录 `LoadIssue`。
- `defaultMode` 按本地级、项目级、用户级优先级读取，首个合法值生效。
- `permissions.local.yaml` 应加入 `.gitignore`，避免永久允许规则误提交。

示例配置：

```yaml
defaultMode: default
permissions:
  allow:
    - Bash(git status)
    - Bash(go test *)
    - Read
    - Grep
  deny:
    - Bash(git push *)
    - Write(.PseudoClaude/config.yaml)
```

### `permission` 路径沙箱

**职责：** 对文件类目标做项目根限制。

**实现要点：**

- 项目根在 `NewEngine` 中解析为绝对路径并执行符号链接解析。
- 已存在路径直接使用 `filepath.EvalSymlinks`。
- 不存在路径从自身向上查找最近已存在祖先目录，对祖先目录执行 `EvalSymlinks`，再拼回剩余路径。
- 判断逻辑必须使用路径边界：`resolved == root || strings.HasPrefix(resolved, root + string(filepath.Separator))`。
- `find_files` 从 glob pattern 中提取首个通配符之前的目录作为沙箱检查目标；没有确定目录时使用项目根。
- `search_code` 的 `path` 为空时使用项目根。

### `agent` 包

**职责：** 在 Agent Loop 工具执行阶段接入权限检查和人在回路。

**对外接口：** 扩展 `Runner`、`Request`、`Event`；保持 provider 交互不变。

**实现要点：**

- `Runner.run` 读取 `Request.PermissionMode`，为空时使用引擎启动模式。
- `prepareRequest` 继续用 `Request.Mode` 判断 chat/plan/do；计划工作流仍只暴露只读工具。
- `executeToolCalls` 增加权限引擎和权限模式参数。
- 每个工具执行前发送 `EventToolCallStart`，权限拒绝也应有对应 `EventToolResult` 和 `EventToolCallDone`，便于 TUI 和测试观察。
- `Deny` 分支构造 `tools.Failure(call.Name, "permission_denied", reason, metadata)`，metadata 包含 `source`、`category`、`target`、`call_id`。
- `Ask` 分支调用 `requestApproval`，等待 TUI 回传或 context 取消。
- `ApprovalAllowSession` 调用 `Engine.AllowForSession` 后执行工具。
- `ApprovalAllowForever` 调用 `Engine.PersistLocalAllow` 后执行工具。
- `ApprovalDenyOnce` 生成 `permission_denied` 工具结果并继续 Loop。
- 只读批次中，明确 `Allow` 的调用仍并发执行；被 `Deny` 的调用生成拒绝结果；只读调用如果因 strict 模式返回 `Ask`，该批次需要拆成待确认串行路径，避免 goroutine 阻塞等待 TUI。

### `tui` 包

**职责：** 展示权限模式、处理模式切换、承载人在回路 UI。

**对外接口：** `New` 增加权限引擎参数；内部新增 approving 状态。

**实现要点：**

- `Model` 新增字段：`permissionMode permission.Mode`、`permissionEngine *permission.Engine`、`pendingApproval *agent.ApprovalRequest`、`approvalCursor int`。
- `New` 使用 `engine.StartMode()` 初始化 `permissionMode`；没有引擎时用 `permission.ModeDefault`。
- `submit` 构造 `agent.Request` 时填入当前 `permissionMode`。
- `Shift+Tab` 在 idle 态循环 strict -> default -> acceptEdits -> bypassPermissions -> strict，切换后打印简短状态提示。
- `/plan`、`/do` 继续控制计划工作流；`/do` 固定关闭 `planMode`，但不强制重置 `permissionMode`。
- 状态栏左侧常驻显示权限模式；若处于计划工作流，可追加 `PLAN` 标识，但不能替代权限模式。
- 收到 `EventApproval` 时进入 `stateApproving`，停止继续读取事件，等待用户选择。
- 审批块展示工具名、参数预览、触发原因和四个选项：允许本次、允许本会话、永久允许、拒绝本次。
- 支持上下移动光标、回车确认、数字键 `1` 到 `4` 直选；默认高亮允许本次。
- `Esc` 或 `Ctrl+C` 在 approving 态取消当前轮：优先向 `Respond` 发送 `ApprovalDenyOnce` 解除 Agent 等待，再取消 context 并回到 idle。

### `cmd/PseudoClaude`

**职责：** 构造权限引擎并注入 TUI。

**实现要点：**

- 启动后取 `os.Getwd()` 作为项目根。
- 调用 `permission.NewEngine(cwd, permission.DefaultOptions(cwd))`。
- 如果引擎返回非致命加载问题，打印到 stderr 或让 TUI 启动后显示状态提示；不得中断启动。
- 把引擎传给 `tui.New`。

### `.gitignore` 与示例配置

**职责：** 避免本地永久授权误提交，并提供可读配置样例。

**实现要点：**

- `.gitignore` 增加 `.PseudoClaude/permissions.local.yaml`。
- 新增 `.PseudoClaude/permissions.yaml.example`，展示 `defaultMode`、allow、deny、工具友好名和 glob 写法。
- 不创建真实项目级权限文件，避免无意改变用户权限默认。

## 模块交互

```text
cmd/PseudoClaude
  -> permission.NewEngine(cwd, DefaultOptions(cwd))
  -> tui.New(providers, cwd, registry, engine)

TUI idle
  -> Shift+Tab 更新 permissionMode
  -> submit 用户输入
  -> agent.Runner.Run(ctx, Request{Mode, PermissionMode})

agent.Runner
  -> provider.Stream 取得工具调用
  -> splitToolBatches
  -> permission.Engine.Check(permissionMode, call, safety)
       Allow -> registry.Execute
       Deny  -> tools.Failure("permission_denied") -> conversation.AddToolResult
       Ask   -> EventApproval -> 等待 Respond

TUI approving
  -> 用户选择 ApprovalDecision
  -> Respond <- decision

agent Ask continuation
  -> AllowOnce    -> registry.Execute
  -> AllowSession -> engine.AllowForSession -> registry.Execute
  -> AllowForever -> engine.PersistLocalAllow -> registry.Execute
  -> DenyOnce     -> tools.Failure("permission_denied")
```

## 文件组织

```text
PseudoClaude/
├── cmd/PseudoClaude/
│   └── main.go                         — 构造 permission.Engine 并注入 TUI
├── internal/permission/
│   ├── mode.go                         — Mode、Decision、Category、ApprovalDecision、模式矩阵
│   ├── engine.go                       — Engine、NewEngine、Check、StartMode、LoadIssues
│   ├── blacklist.go                    — 内置危险命令正则与匹配
│   ├── sandbox.go                      — 项目根解析、符号链接解析、前缀判断
│   ├── rule.go                         — Rule、RuleSet、parseRule、glob 匹配
│   ├── settings.go                     — YAML Settings、三层加载、默认模式选择
│   ├── target.go                       — 工具分类、友好名映射、命令串和路径目标提取
│   ├── persist.go                      — 会话 allow 与本地级永久 allow 写入
│   ├── blacklist_test.go               — 高危命令与 bypass 不可绕过测试
│   ├── sandbox_test.go                 — 项目外路径、软链接逃逸、新建路径测试
│   ├── rule_test.go                    — 精确匹配、glob、deny 优先、友好名映射测试
│   ├── settings_test.go                — 缺失/非法配置降级、默认模式优先级测试
│   └── engine_test.go                  — 五层短路、模式矩阵、会话规则测试
├── internal/agent/
│   ├── event.go                        — 新增 EventApproval 与 ApprovalRequest
│   ├── runner.go                       — Request 增加 PermissionMode，Run 注入权限模式
│   ├── tools.go                        — 工具执行前调用权限引擎，Ask/Deny/Allow 分支
│   └── runner_test.go                  — 权限回灌、保序、取消、计划工作流兼容测试
├── internal/tui/
│   ├── tui.go                          — 新增 stateApproving、权限模式和审批字段
│   ├── stream.go                       — EventApproval 处理、审批按键、submit 传权限模式
│   ├── view.go                         — 状态栏模式显示、approvalBlock 渲染
│   └── stream_test.go                  — Shift+Tab、审批选择、Esc 取消、状态栏测试
├── .PseudoClaude/
│   └── permissions.yaml.example         — 权限配置示例
└── .gitignore                          — 忽略 .PseudoClaude/permissions.local.yaml
```

## 技术决策

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| 权限判定落点 | `permission` 包 + `agent` 执行前编排 | 与 provider 解耦，满足跨协议一致，也避免把规则散落到各个工具 |
| 计划工作流与权限模式 | 分成 `agent.Mode` 和 `permission.Mode` 两个轴 | `/plan`/`/do` 是工作流，strict/default 等是权限兜底；分离后语义更稳定 |
| 黑名单 | 包内内置正则，不提供配置入口 | 满足不可绕过要求；任何配置和模式都不能放开 |
| 沙箱 | 先解析符号链接或最近已存在祖先，再做项目根前缀判断 | 防止软链接逃逸，同时允许项目内新建路径 |
| 命令执行沙箱 | 不对 `run_command` 做路径沙箱 | 任意命令的文件访问无法可靠静态解析；由黑名单、规则、模式和人在回路覆盖 |
| 规则层级 | 会话级 > 本地级 > 项目级 > 用户级；同层 deny 优先 | 会话批准应即时生效；deny 优先更符合安全默认 |
| 永久允许 | 写入 `.PseudoClaude/permissions.local.yaml` | 本地长期生效但不影响团队项目规则 |
| 允许本会话 | 写入内存会话规则 | 满足本会话复用，不污染磁盘 |
| 规则匹配目标 | 命令匹配规范化命令串，文件匹配项目相对路径 | 贴合用户规则示例，并让永久规则可读 |
| 模式兜底 | 只返回 Allow 或 Ask | Deny 来源保持可解释：黑名单、沙箱、显式 deny、用户拒绝 |
| 人在回路通信 | `EventApproval` + buffered `Respond` channel | TUI 不阻塞主更新循环，Agent 能被 context 取消打断 |
| 只读并发 | Allow 的只读调用仍并发；Ask 调用拆到串行确认 | 保留现有性能，同时避免并发 goroutine 等待用户输入 |
| provider 适配层 | 不修改 | 权限行为在本地工具执行链路统一完成，Anthropic/OpenAI 保持一致 |
| 配置与 provider 配置 | 独立权限 YAML | provider 配置含 API key，权限规则可项目共享或本地私有，职责不同 |
| 本地权限文件提交风险 | `.gitignore` 忽略 local 文件 | 防止永久允许规则误提交 |

## Spec 覆盖检查

| Spec | Plan 归属 |
| --- | --- |
| F1 危险命令黑名单 | `permission/blacklist.go`、`Engine.Check` 第一层 |
| F2 路径沙箱 | `permission/sandbox.go`、`target.go` |
| F3 可配置规则 | `permission/rule.go`、`settings.go` |
| F4 三层配置与优先级 | `permission/settings.go`、`Engine` 规则顺序 |
| F5 权限模式兜底 | `permission/mode.go`、`modeFallback` |
| F6 五层判定流水线 | `Engine.Check` + `agent` Ask 分支 |
| F7 人在回路确认 | `agent.ApprovalRequest`、`tui` approving 态 |
| F8 会话级临时规则 | `Engine.session`、`AllowForSession` |
| F9 运行时权限模式切换 | `tui` Shift+Tab、状态栏、`PermissionMode` |
| F10 拒绝不中断 Agent Loop | `tools.Failure(permission_denied)` 回灌 |
| F11 跨 provider 一致 | 权限位于 `agent`，provider 不改 |
| F12 安全默认 | `classify`、参数解析失败和未知工具分支 |
