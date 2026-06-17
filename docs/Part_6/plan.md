# MCP 客户端 Plan

## 架构概览

Part_6 新增 `internal/mcp` 包承载 MCP 客户端能力，并在 `cmd/PseudoClaude/main.go` 启动流程中完成装配。MCP 包负责读取和合并配置、展开环境变量、建立 stdio 或 Streamable HTTP 会话、按 MCP 会话流程发现工具、把远端工具适配为 `internal/tools.Tool`，以及在退出时关闭所有连接。

现有 `internal/tools.Registry` 保持工具中心职责不变。MCP 发现到的工具会被包装为普通 `tools.Tool` 并注册进去，Agent 请求模型时通过现有 `Registry.Definitions()` 暴露给 provider；模型调用时通过现有 `Registry.Execute()` 进入适配工具的 `Execute` 方法。`internal/agent`、`internal/llm`、`internal/tui` 不需要知道工具来自 MCP。

权限系统通过现有 `tools.Definition.Safety` 自然接入。远端工具声明只读时映射为 `tools.SafetyReadOnly`，否则映射为 `tools.SafetySideEffect`。`permission.Engine.Check` 已经按工具安全等级分类；规则层需要支持 MCP 工具名原样匹配与通配匹配，因此计划在 `internal/permission` 中做一处小型扩展：未知友好名如果以 `mcp__` 开头，按原始工具名参与规则匹配。

协议层优先使用官方 Go SDK `github.com/modelcontextprotocol/go-sdk/mcp`。SDK 负责 MCP 初始化握手、JSON-RPC 编解码、请求响应 ID 配对、stdio 和 Streamable HTTP 传输连接。PseudoClaude 在 SDK 外层保留 `Session` 小接口和超时、错误映射、生命周期管理，降低 SDK API 细节变化对上层的影响。

启动数据流：

```text
main
  -> config.Load(project config)                 // 既有 provider 配置
  -> tools.DefaultRegistry()                     // 既有内置工具
  -> mcp.LoadConfig(root)                        // 用户级 + 项目级 mcp_servers
  -> mcp.NewManager(ctx, cfg, logger)            // 连接、握手、列工具
  -> for tool in manager.Tools(): registry.Register(tool)
  -> permission.NewEngine(...)
  -> tui.New(... registry ...)
  -> defer manager.Close()
```

调用数据流：

```text
model tool call: mcp__github__create_issue
  -> agent.executeToolCalls
  -> permission.Engine.Check(mode, call, safety)
  -> tools.Registry.Execute
  -> mcp.remoteTool.Execute
  -> session.CallTool(remoteName, arguments)
  -> text content / MCP error / transport error
  -> tools.Result JSON 回灌给模型
```

## 核心数据结构

### `mcp.Config`

```go
type Config struct {
    Servers map[string]ServerConfig
}
```

职责：

- 表示合并、展开和校验后的 MCP Server 配置。
- `Servers` 的 key 是用户配置中的 Server 名。
- 同名 Server 已按项目级覆盖用户级完成合并。

### `mcp.ServerConfig`

```go
type ServerConfig struct {
    Type    TransportType
    Command string
    Args    []string
    Env     map[string]string
    URL     string
    Headers map[string]string
}

type TransportType string

const (
    TransportStdio TransportType = "stdio"
    TransportHTTP  TransportType = "http"
)
```

职责：

- 表示单个 Server 的归一化配置。
- `stdio` 使用 `Command`、`Args`、`Env`。
- `http` 使用 `URL`、`Headers`。
- `Env` 和 `Headers` 已完成 `${VAR}` 展开。

### `mcp.LoadIssue`

```go
type LoadIssue struct {
    Path    string
    Server  string
    Message string
}
```

职责：

- 记录配置加载、变量展开和字段校验中的非致命问题。
- 由 main 打印为启动提示，不进入模型上下文。

### `mcp.Manager`

```go
type Manager struct {
    mu       sync.Mutex
    sessions []*serverSession
    tools    []tools.Tool
    issues   []Issue
}
```

职责：

- 管理所有成功连接的 MCP Server 会话。
- 缓存适配后的工具列表。
- 统一关闭生命周期。
- 记录连接、列工具、工具跳过等启动问题。

### `mcp.serverSession`

```go
type serverSession struct {
    name    string
    session ClientSession
}
```

职责：

- 保存 Server 名和可关闭、可调用的 MCP 客户端会话。
- `session` 使用包内接口，生产实现包装 SDK session，测试可注入 stub。

### `mcp.Issue`

```go
type Issue struct {
    Server  string
    Tool    string
    Stage   string
    Message string
}
```

职责：

- 记录连接、列工具、工具适配、关闭等运行阶段的非致命问题。
- 由 main 打印为启动或退出提示。
- 不包含 header、env 等敏感值。

### `mcp.ClientSession`

```go
type ClientSession interface {
    ListTools(ctx context.Context) ([]RemoteTool, error)
    CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error)
    Close() error
}
```

职责：

- 隔离 PseudoClaude 和 SDK API 细节。
- 表示 MCP 会话最小能力：列工具、调用工具、关闭。
- SDK 包装层负责把 `mcp.ClientSession` 的具体方法和类型转换到该接口。

### `mcp.RemoteTool`

```go
type RemoteTool struct {
    Name        string
    Description string
    InputSchema map[string]any
    ReadOnly    bool
}
```

职责：

- 表示从 MCP Server 发现到的工具定义。
- 已经从 SDK 类型转换为 PseudoClaude 需要的中立结构。
- `ReadOnly` 只在远端明确声明只读时为 true。

### `mcp.CallResult`

```go
type CallResult struct {
    TextBlocks      []string
    IsError         bool
    NonTextDropped  int
    Metadata        map[string]any
}
```

职责：

- 表示一次 MCP `tools/call` 的中立结果。
- `TextBlocks` 用于按顺序拼接为 `tools.Result.Content`。
- `IsError` 来自远端工具结果。
- `NonTextDropped` 用于输出一次性可观测提示。

### `mcp.remoteTool`

```go
type remoteTool struct {
    fullName   string
    serverName string
    remoteName string
    definition tools.Definition
    session    ClientSession
}
```

职责：

- 实现 `tools.Tool`。
- 对外名称使用 `mcp__<server>__<tool>`。
- 执行时把 PseudoClaude JSON 参数转为 MCP `tools/call` 参数。
- 把 MCP 返回结果映射为 `tools.Success` 或 `tools.Failure`。

## 核心接口

### 配置加载

```go
func LoadConfig(root string) (Config, []LoadIssue)
```

行为：

- 读取 `~/.PseudoClaude/config.yaml` 和 `<root>/.PseudoClaude/config.yaml`。
- 只解析 `mcp_servers` 字段，不影响既有 provider 配置加载。
- 文件缺失视为空。
- 读取失败或 YAML 格式非法时跳过该层并返回 `LoadIssue`。
- 对 `env` 和 `headers` 的值展开 `${VAR}`。
- 校验 Server 类型和必填字段，非法 Server 从结果中移除。

### Manager 构造

```go
type ManagerOptions struct {
    ConnectTimeout time.Duration
    CallTimeout    time.Duration
    CloseTimeout   time.Duration
    ClientInfo     ClientInfo
    Dialer         Dialer
    Err            io.Writer
}

type ClientInfo struct {
    Name    string
    Version string
}

func NewManager(ctx context.Context, cfg Config, opts ManagerOptions) *Manager
```

行为：

- 并发尝试连接所有 Server。
- 每个 Server 的连接、初始化和列工具整体受 `ConnectTimeout` 限制。
- 连接失败、初始化失败、列工具失败或工具适配失败都记录 issue，不返回致命错误。
- 成功发现的工具被适配并缓存。
- `Dialer` 为空时使用官方 SDK 生产实现。
- `opts` 为空时使用默认超时：连接 30s、调用 30s、关闭 5s。

### Manager 查询和关闭

```go
func (m *Manager) Tools() []tools.Tool
func (m *Manager) Issues() []Issue
func (m *Manager) Close()
```

行为：

- `Tools` 返回稳定排序后的工具拷贝。
- `Issues` 返回启动和适配阶段的非致命问题。
- `Close` 并发关闭所有成功会话，并受总关闭超时限制。

### 工具适配

```go
func AdaptTool(serverName string, remote RemoteTool, session ClientSession, callTimeout time.Duration) (tools.Tool, *Issue)
```

行为：

- 生成 `mcp__<server>__<tool>` 名称。
- 校验工具名是否满足 provider 允许字符。
- 构造 `tools.Definition`。
- description 为空时提供兜底。
- input schema 为空时提供 `{"type":"object"}`。
- read-only 映射为 `tools.SafetyReadOnly`，否则 `tools.SafetySideEffect`。

### SDK 会话工厂

```go
type Dialer interface {
    Dial(ctx context.Context, name string, cfg ServerConfig, info ClientInfo) (ClientSession, error)
}
```

行为：

- 生产实现根据 `ServerConfig.Type` 创建 stdio 或 Streamable HTTP transport。
- 测试实现可注入成功、失败、乱序、阻塞或关闭卡住的会话。
- Manager 通过该接口完成失败隔离和超时测试。

## 模块设计

### `internal/config`

**职责：** 继续加载 provider 配置。

**改动：**

- `Config` 增加可选字段 `MCPServers map[string]mcpRawServer` 不是推荐方向，因为会造成 `internal/config` 依赖 `internal/mcp` 或复制类型。计划保持 `internal/config.Load` 的 provider 职责不变。
- `internal/mcp.LoadConfig` 独立读取同一个 YAML 文件里的 `mcp_servers` 字段。这样既复用配置文件路径，又避免把 MCP 生命周期塞进 provider config 包。

**原因：**

- 现有 `config.Load(".PseudoClaude/config.yaml")` 会验证 `providers` 非空；MCP 配置加载需要支持用户级文件缺失、项目级文件缺失、单层非法时降级，语义不同。
- 独立加载能让 MCP 配置错误不影响 provider 配置的既有失败策略。

### `internal/mcp/config.go`

**职责：** 配置读取、两层合并、环境变量展开和字段校验。

内部类型：

```go
type rawConfig struct {
    MCPServers map[string]rawServer `yaml:"mcp_servers"`
}

type rawServer struct {
    Type    string            `yaml:"type"`
    Command string            `yaml:"command"`
    Args    []string          `yaml:"args"`
    Env     map[string]string `yaml:"env"`
    URL     string            `yaml:"url"`
    Headers map[string]string `yaml:"headers"`
}
```

关键函数：

- `configPaths(root string) (userPath, projectPath string)`
- `loadRawConfig(path string) (rawConfig, []LoadIssue)`
- `mergeRawServers(user, project map[string]rawServer) map[string]rawServer`
- `expandMapValues(server string, values map[string]string) (map[string]string, []LoadIssue)`
- `expandVars(value string) (string, []string)`
- `validateServer(name string, raw rawServer) (ServerConfig, []LoadIssue, bool)`

设计点：

- 用户级路径固定为 `~/.PseudoClaude/config.yaml`。
- 项目级路径固定为 `<root>/.PseudoClaude/config.yaml`。
- 两层合并只在 Server 名维度进行。
- `${VAR}` 使用正则匹配合法环境变量名。
- 未定义变量返回空串并记录 issue，但不阻断 Server。
- `url` 不做变量展开，保持 spec 明确边界。

### `internal/mcp/sdk.go`

**职责：** 包装官方 MCP Go SDK，提供 `Dialer` 和 `ClientSession` 的生产实现。

生产类型：

```go
type SDKDialer struct{}

type sdkSession struct {
    serverName string
    session    *sdkmcp.ClientSession
}
```

stdio 连接：

```go
cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
cmd.Env = mergeEnv(os.Environ(), cfg.Env)
cmd.Stderr = os.Stderr
transport := &sdkmcp.CommandTransport{Command: cmd}
client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: info.Name, Version: info.Version}, nil)
session, err := client.Connect(ctx, transport, nil)
```

HTTP 连接：

```go
client := &http.Client{
    Transport: headerRoundTripper{
        base: http.DefaultTransport,
        headers: cfg.Headers,
    },
}
transport := &sdkmcp.StreamableClientTransport{
    Endpoint: cfg.URL,
    HTTPClient: client,
}
session, err := sdkClient.Connect(ctx, transport, nil)
```

设计点：

- 以实际拉取的 SDK 版本为准做编译校准；若泛型签名为 `CallToolParams[json.RawMessage]`，包装层负责集中适配。
- SDK 完成 initialize、initialized notification、JSON-RPC ID 配对和 transport 收发。
- `sdkSession.ListTools` 把 SDK tool 转为 `RemoteTool`。
- `sdkSession.CallTool` 把 SDK content 转为 `CallResult`。
- `sdkSession.Close` 调用 SDK session close。

### `internal/mcp/http.go`

**职责：** HTTP headers 注入。

类型：

```go
type headerRoundTripper struct {
    base    http.RoundTripper
    headers map[string]string
}
```

行为：

- 克隆 request 后注入配置 headers。
- 不记录 header 值。
- base 为空时使用 `http.DefaultTransport`。

### `internal/mcp/manager.go`

**职责：** 多 Server 并发连接、失败隔离、工具缓存和生命周期。

关键流程：

1. 归一化 `ManagerOptions`。
2. 按 Server 名排序，给每个 Server 启动一个 goroutine。
3. 每个 goroutine 使用 `context.WithTimeout(ctx, ConnectTimeout)`。
4. 调用 `Dialer.Dial` 建立会话。
5. 调用 `session.ListTools` 获取工具列表。
6. 对每个工具调用 `AdaptTool`。
7. 成功会话和工具在锁内写入 Manager。
8. 所有 goroutine 结束后，对工具按名称稳定排序。

失败处理：

- Dial 失败：记录 `connect` issue。
- ListTools 失败：记录 `list_tools` issue，并关闭已建立 session。
- 单个工具适配失败：记录 `adapt_tool` issue，其它工具继续。
- 同名注册冲突：Manager 可先记录，最终由 main 注册时再次检查并提示。

关闭流程：

- 对每个 session 并发调用 `Close`。
- 整体等待 `CloseTimeout`。
- 超时后返回，不阻塞程序退出。

### `internal/mcp/tool.go`

**职责：** 远端工具适配为 `tools.Tool`。

`Definition()`：

- `Name`: `mcp__<server>__<tool>`
- `Description`: 远端描述或兜底描述
- `InputSchema`: 远端 schema 或 `{"type":"object"}`
- `Safety`: 远端只读声明为 true 时 `SafetyReadOnly`，否则 `SafetySideEffect`

`Execute(ctx, input, env)`：

1. 使用 `context.WithTimeout(ctx, callTimeout)`。
2. 空参数或空白 JSON 视为 nil 参数。
3. 非空参数必须解析为 JSON object；否则返回 `tools.Failure(fullName, "invalid_arguments", ...)`。
4. 调用 `session.CallTool(ctx, remoteName, args)`.
5. 调用失败、ctx 取消或超时返回 `tools.Failure(fullName, "mcp_call_error", ...)`，metadata 包含 Server 名和远端工具名。
6. 远端 `IsError=true` 返回 `tools.Failure(fullName, "mcp_tool_error", text, metadata)`。
7. 成功时返回 `tools.Success(fullName, strings.Join(textBlocks, "\n"), metadata)`。
8. 非文本块被丢弃；同一工具首次出现时记录 issue 或 stderr 提示，metadata 中包含 dropped count。

### `internal/mcp/name.go`

**职责：** MCP 工具命名空间和 provider 名称校验。

关键函数：

```go
func FullToolName(server, tool string) string
func ValidToolName(name string) bool
func SplitToolName(name string) (server, tool string, ok bool)
```

规则：

- 拼接格式固定为 `mcp__<server>__<tool>`。
- provider 安全字符采用 `^[A-Za-z0-9_-]+$`。
- 不对 server/tool 做自动替换，避免配置名和远端名不可追踪。
- 不合法时跳过工具并提示用户修改 Server 名或选择合法远端工具名。

### `internal/permission`

**职责：** 让权限规则能匹配 MCP 工具名。

改动点：

- `parseRule` 中，当工具名以 `mcp__` 开头时允许通过，不要求存在内置 friendly name 映射。
- `internalName` 或等价映射函数对 `mcp__` 工具返回原名。
- `friendlyName` 对未知工具保持原样或显式支持 `mcp__` 原样返回。
- `classify` 继续依赖 `tools.Safety`：`SafetyReadOnly` -> `CategoryRead`，`SafetySideEffect` 走现有有副作用类别。
- `pathTarget` 和黑名单逻辑不扩展到 MCP 工具。

这样规则可写：

```yaml
permissions:
  allow:
    - mcp__github__get_issue
    - mcp__github__*
  deny:
    - mcp__prod-db__*
```

### `cmd/PseudoClaude/main.go`

**职责：** 启动装配。

改动流程：

1. 先读取 provider 配置，保持现有失败行为。
2. 获取 `cwd`。
3. 构造权限引擎，保留现有提示输出。
4. 构造默认工具 registry。
5. 调用 `mcp.LoadConfig(cwd)` 并打印 load issues。
6. 调用 `mcp.NewManager(context.Background(), mcpConfig, opts)`。
7. 打印 Manager issues。
8. 把 `manager.Tools()` 逐个注册进 registry；注册冲突时打印提示并跳过冲突工具。
9. `defer manager.Close()`。
10. 进入 TUI。

顺序说明：

- MCP 工具注册需要发生在 `tui.New` 之前。
- 权限引擎可在 MCP 前后构造；推荐保持当前权限初始化位置，只要 registry 注册完成后再进入 TUI 即可。
- 没有 MCP 配置时，Manager 为空，启动行为等价于当前版本。

## 文件组织

```text
PseudoClaude/
├── cmd/PseudoClaude/
│   └── main.go                         # 改：启动时加载 MCP 配置、连接 Server、注册工具、defer Close
├── internal/mcp/
│   ├── config.go                       # 新：配置路径、raw YAML、合并、展开、校验
│   ├── config_test.go                  # 新：两层合并、降级、变量展开、字段校验
│   ├── name.go                         # 新：mcp__ 命名空间、校验、拆分
│   ├── name_test.go                    # 新：合法/非法名称、拆分
│   ├── http.go                         # 新：headerRoundTripper
│   ├── http_test.go                    # 新：headers 注入、不泄漏值
│   ├── sdk.go                          # 新：官方 SDK Dialer 和 session 包装
│   ├── manager.go                      # 新：Manager、并发连接、失败隔离、Close
│   ├── manager_test.go                 # 新：成功/失败/超时/关闭兜底/稳定排序
│   ├── tool.go                         # 新：remoteTool 实现 tools.Tool
│   └── tool_test.go                    # 新：Definition、Execute、错误映射、非文本丢弃
├── internal/permission/
│   ├── rule.go                         # 改：允许 mcp__ 工具规则
│   ├── target.go                       # 视需要改：friendly/internal 映射支持 mcp__ 原样
│   └── *_test.go                       # 改/增：MCP 规则匹配和分类测试
├── docs/Part_6/
│   ├── spec.md                         # 已生成
│   ├── plan.md                         # 本文件
│   ├── task.md                         # 后续生成
│   ├── checklist.md                    # 后续生成
│   └── mcp-servers.example.yaml         # 后续任务中新增：配置示例
├── go.mod                              # 改：添加官方 MCP Go SDK
└── go.sum                              # 改：SDK 校验和
```

## 模块交互

### 启动发现

```text
main
  -> mcp.LoadConfig(cwd)
      -> read ~/.PseudoClaude/config.yaml
      -> read <cwd>/.PseudoClaude/config.yaml
      -> parse mcp_servers only
      -> expand env/header values
      -> project overrides user by server key
      -> validate server configs
  -> mcp.NewManager(ctx, cfg, opts)
      -> for each server concurrently
          -> dial stdio/http
          -> SDK initialize handshake
          -> SDK initialized notification
          -> ListTools
          -> AdaptTool
      -> stable sort tools
  -> registry.Register(mcp tools)
  -> tui.Run
```

### 工具调用

```text
agent receives tool call mcp__server__tool
  -> registry.Safety(name)
  -> permission.Check(mode, call, safety)
      -> MCP read-only: CategoryRead
      -> MCP side-effect: existing side-effect category
      -> rules can match mcp__server__tool or mcp__server__*
  -> registry.Execute(call)
  -> remoteTool.Execute
      -> parse json object args
      -> session.CallTool(remoteName, args)
      -> map CallResult to tools.Result
  -> conversation.AddToolResult(...)
```

### 退出

```text
main deferred manager.Close()
  -> close all sessions concurrently
  -> stdio child process closed by SDK transport/session
  -> HTTP session closed by SDK transport/session when supported
  -> return after all close or close timeout
```

## 技术决策

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| MCP 协议层 | 使用官方 Go SDK | 避免自研 JSON-RPC、握手和 Streamable HTTP 细节；文档确认 SDK 支持 `CommandTransport`、`StreamableClientTransport`、`ListTools`、`CallTool` |
| SDK 隔离 | 在 `internal/mcp` 内包装为 `ClientSession` / `Dialer` | SDK API 版本可能有泛型或签名差异，集中适配能减少影响 |
| 配置路径 | `~/.PseudoClaude/config.yaml` + `<project>/.PseudoClaude/config.yaml` | 用户已选择；复用现有配置入口和项目目录习惯 |
| 配置加载职责 | MCP 包独立读取 `mcp_servers` | provider 配置的验证语义不同，避免互相影响 |
| 合并语义 | 项目级按 Server 名完整覆盖用户级 | 避免字段级半合并导致 command/env/url 混合出不可预期 Server |
| 变量展开 | 只展开 `env` 和 `headers` 的值 | 满足凭据注入，避免 command、args、url、名称受环境间接改变 |
| Server 启动 | 进入 TUI 前同步完成，内部并发 | TUI 看到稳定工具集合；并发减少多 Server 启动等待 |
| 超时 | 连接发现 30s、调用 30s、关闭 5s，测试可覆盖为短值 | 防止卡死；保持行为可测 |
| 工具命名 | `mcp__<server>__<tool>` | 冲突隔离、来源可追踪、规则可匹配 |
| 非法工具名 | 跳过并提示，不自动替换 | 自动替换会让权限规则和来源追踪变模糊 |
| 只读映射 | 只信远端明确 read-only 声明 | 安全默认；未声明按有副作用处理 |
| 权限集成 | 小改 permission 规则解析，允许 `mcp__` 工具名 | 复用现有权限模式和人在回路，不做 MCP 专用权限系统 |
| 非文本内容 | 丢弃并提示，文本继续回灌 | 当前工具结果是文本 JSON；非文本能力留到后续 |
| 错误处理 | MCP 调用错误转 `tools.Failure` | 保持 Agent Loop 不因工具错误终止 |
| HTTP headers | 自定义 `RoundTripper` 注入 | SDK 支持 `HTTPClient`，这是标准 Go 做法 |
| 健康检查/重连 | 不做 | 与 spec 范围一致，保持本阶段可控 |
| Provider 改动 | 不改 | MCP 工具作为普通 `tools.Definition` 暴露，provider 无需感知 |

## Spec 覆盖映射

| Spec | Plan 覆盖 |
| --- | --- |
| F1 | `internal/mcp/config.go` 两层读取和合并 |
| F2 | `validateServer` 校验 stdio/http 必填字段 |
| F3 | `expandVars` 和 `expandMapValues` |
| F4 | `SDKDialer` stdio `CommandTransport` |
| F5 | `SDKDialer` HTTP `StreamableClientTransport` + `headerRoundTripper` |
| F6 | 官方 SDK + `ClientSession` 包装层 |
| F7 | `SDKDialer.Dial` + `ClientSession.ListTools/CallTool` |
| F8 | `RemoteTool` + `AdaptTool` + registry 注册 |
| F9 | `name.go` 命名空间和校验 |
| F10 | `remoteTool.Execute` 参数和结果映射 |
| F11 | `sdkSession.CallTool` / `remoteTool.Execute` 非文本丢弃提示 |
| F12 | `tools.Definition.Safety` 映射 + permission 规则支持 |
| F13 | `Manager` 并发连接和失败隔离 |
| F14 | `Manager.sessions` 缓存 + `Close` |
| F15 | Manager/Tool 超时和 ctx 取消 |
| F16 | `docs/Part_6/mcp-servers.example.yaml` 后续任务 |
