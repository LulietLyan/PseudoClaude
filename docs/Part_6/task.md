# MCP 客户端 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| --- | --- | --- |
| 修改 | `go.mod` / `go.sum` | 添加官方 MCP Go SDK 依赖并整理间接依赖 |
| 新建 | `internal/mcp/config.go` | MCP 配置读取、两层合并、环境变量展开、字段校验 |
| 新建 | `internal/mcp/config_test.go` | 配置合并、降级、变量展开、字段校验测试 |
| 新建 | `internal/mcp/name.go` | `mcp__<server>__<tool>` 命名、校验、拆分 |
| 新建 | `internal/mcp/name_test.go` | MCP 工具命名合法性与拆分测试 |
| 新建 | `internal/mcp/http.go` | HTTP headers 注入 RoundTripper |
| 新建 | `internal/mcp/http_test.go` | headers 注入行为测试 |
| 新建 | `internal/mcp/types.go` | Manager、Session、RemoteTool、Issue 等共享类型 |
| 新建 | `internal/mcp/tool.go` | MCP 远端工具适配为 `tools.Tool` |
| 新建 | `internal/mcp/tool_test.go` | 工具定义、参数解析、调用结果和错误映射测试 |
| 新建 | `internal/mcp/sdk.go` | 官方 MCP Go SDK 的 Dialer 和 ClientSession 包装 |
| 新建 | `internal/mcp/manager.go` | 多 Server 并发连接、失败隔离、工具缓存、关闭生命周期 |
| 新建 | `internal/mcp/manager_test.go` | Manager 成功、失败、超时、关闭兜底和稳定排序测试 |
| 修改 | `internal/permission/target.go` | 允许 `mcp__` 工具名作为权限规则友好名原样匹配 |
| 修改 | `internal/permission/rule_test.go` / `target_test.go` | MCP 权限规则匹配和 safety 分类测试 |
| 修改 | `cmd/PseudoClaude/main.go` | 启动时加载 MCP 配置、连接 Server、注册工具、退出关闭 |
| 新建 | `docs/Part_6/mcp-servers.example.yaml` | stdio 与 HTTP MCP Server 配置示例 |
| 修改 | `docs/Part_6/task.md` | 本任务文档 |

## T1: 添加官方 MCP Go SDK 依赖

**文件：** `go.mod`、`go.sum`  
**依赖：** 无

**步骤：**

1. 执行 `go get github.com/modelcontextprotocol/go-sdk@latest`。
2. 执行 `go mod tidy`。
3. 检查 `go.mod` 中出现 `github.com/modelcontextprotocol/go-sdk` 依赖。
4. 暂不写业务代码；如果 SDK 包路径或版本与 plan 中示例有差异，在后续 `sdk.go` 中以实际编译通过的 API 为准。

**验证：** `go build ./...` 能完成依赖解析；若因为网络受限失败，重新在具备网络权限的环境执行同一命令。

## T2: 定义 MCP 共享类型

**文件：** `internal/mcp/types.go`  
**依赖：** 无

**步骤：**

1. 新建 `internal/mcp` 包。
2. 定义 `TransportType`、`Config`、`ServerConfig`、`LoadIssue`、`Issue`、`ClientInfo`。
3. 定义 `RemoteTool`、`CallResult`、`ClientSession`、`Dialer`。
4. 定义 `ManagerOptions`，包含 `ConnectTimeout`、`CallTimeout`、`CloseTimeout`、`ClientInfo`、`Dialer`、`Err`。
5. 提供默认超时常量或包内变量：连接 30s、调用 30s、关闭 5s；测试可临时覆盖时优先用 options 注入。

**验证：** `go test ./internal/mcp/...` 编译通过。

## T3: 实现 MCP 工具命名

**文件：** `internal/mcp/name.go`、`internal/mcp/name_test.go`  
**依赖：** T2

**步骤：**

1. 实现 `FullToolName(server, tool string) string`，格式固定为 `mcp__<server>__<tool>`。
2. 实现 `ValidToolName(name string) bool`，允许字符为 `^[A-Za-z0-9_-]+$`。
3. 实现 `SplitToolName(name string) (server, tool string, ok bool)`，只接受 `mcp__` 前缀和三段式名称。
4. 测试合法 server/tool 名能拼接和拆分。
5. 测试包含 `.`, `@`, 空 server、空 tool、非 MCP 前缀时校验或拆分失败。

**验证：** `go test ./internal/mcp/... -run Test.*ToolName` 通过。

## T4: 实现配置文件读取与两层合并

**文件：** `internal/mcp/config.go`、`internal/mcp/config_test.go`  
**依赖：** T2

**步骤：**

1. 定义 `rawConfig`，只包含 `mcp_servers` 字段。
2. 定义 `rawServer`，包含 `type`、`command`、`args`、`env`、`url`、`headers`。
3. 实现 `configPaths(root)`，用户级为 `~/.PseudoClaude/config.yaml`，项目级为 `<root>/.PseudoClaude/config.yaml`。
4. 实现 `loadRawConfig(path)`：文件不存在返回空配置；读取或 YAML 解析失败返回 `LoadIssue`。
5. 实现 `mergeRawServers(user, project)`：先复制用户级，再用项目级同名完整覆盖。
6. 实现 `LoadConfig(root)` 的读取和合并骨架，先不做变量展开和字段校验。
7. 测试用户级、项目级、两层同名覆盖、文件缺失、非法 YAML 降级。

**验证：** `go test ./internal/mcp/... -run TestLoadConfig` 通过。

## T5: 实现变量展开

**文件：** `internal/mcp/config.go`、`internal/mcp/config_test.go`  
**依赖：** T4

**步骤：**

1. 实现 `expandVars(value string) (string, []string)`，匹配 `${VAR}`，变量名规则为 `[A-Za-z_][A-Za-z0-9_]*`。
2. 实现 `expandMapValues(server string, values map[string]string)`，只展开 map 的 value。
3. 未定义变量展开为空字符串，并返回包含变量名和 Server 名的 `LoadIssue`。
4. 在 `LoadConfig` 中对 `env` 和 `headers` 应用展开。
5. 测试已定义变量、未定义变量、多变量、重复变量。
6. 测试 `command`、`args`、`url`、Server 名和工具名中的 `${VAR}` 不展开。

**验证：** `go test ./internal/mcp/... -run TestExpand` 通过。

## T6: 实现 Server 字段校验

**文件：** `internal/mcp/config.go`、`internal/mcp/config_test.go`  
**依赖：** T5

**步骤：**

1. 实现 `validateServer(name string, raw rawServer) (ServerConfig, []LoadIssue, bool)`。
2. `type: stdio` 必须有非空 `command`，`args` 和 `env` 可选。
3. `type: http` 必须有非空 `url`，`headers` 可选。
4. 缺失 type、未知 type、必填字段缺失时返回 `ok=false` 和 issue。
5. 在 `LoadConfig` 中只保留合法 Server。
6. 测试合法 stdio、合法 http、非法 type、缺 command、缺 url、其它 Server 不受影响。

**验证：** `go test ./internal/mcp/... -run TestValidateServer` 通过。

## T7: 实现 HTTP headers 注入

**文件：** `internal/mcp/http.go`、`internal/mcp/http_test.go`  
**依赖：** T2

**步骤：**

1. 定义 `headerRoundTripper`，包含 `base http.RoundTripper` 和 `headers map[string]string`。
2. `RoundTrip` 中克隆请求，注入 headers，再调用 base。
3. base 为空时使用 `http.DefaultTransport`。
4. 测试请求能收到 `Authorization` 等配置 header。
5. 测试原始 request 不被原地污染。

**验证：** `go test ./internal/mcp/... -run TestHeaderRoundTripper` 通过。

## T8: 实现远端工具适配定义

**文件：** `internal/mcp/tool.go`、`internal/mcp/tool_test.go`  
**依赖：** T2、T3

**步骤：**

1. 定义 `remoteTool`，包含 full name、server name、remote name、`tools.Definition`、`ClientSession` 和调用超时。
2. 实现 `AdaptTool(serverName, remote, session, callTimeout)`。
3. 名称由 `FullToolName` 生成，并通过 `ValidToolName` 校验。
4. description 为空时生成包含 Server 和远端工具名的兜底描述。
5. input schema 为空时使用 `map[string]any{"type": "object"}`。
6. `remote.ReadOnly=true` 映射为 `tools.SafetyReadOnly`；否则映射为 `tools.SafetySideEffect`。
7. 实现 `Definition()` 返回缓存定义。
8. 测试合法适配、非法名称跳过、空描述兜底、空 schema 兜底、只读和副作用 safety。

**验证：** `go test ./internal/mcp/... -run TestAdaptTool` 通过。

## T9: 实现 MCP 工具执行与结果映射

**文件：** `internal/mcp/tool.go`、`internal/mcp/tool_test.go`  
**依赖：** T8

**步骤：**

1. 实现 `remoteTool.Execute(ctx, input, env)`。
2. 空参数或空白参数视为 nil 参数。
3. 非空参数必须解析为 JSON object；解析失败返回 `tools.Failure`，`error_type=invalid_arguments`。
4. 使用 `context.WithTimeout(ctx, callTimeout)` 包裹调用。
5. 调用 `session.CallTool(ctx, remoteName, args)`。
6. session 返回 error、ctx cancel 或 deadline exceeded 时返回 `tools.Failure`，`error_type=mcp_call_error`。
7. `CallResult.IsError=true` 时返回 `tools.Failure`，`error_type=mcp_tool_error`，正文使用文本块拼接内容。
8. 成功时返回 `tools.Success`，正文为文本块按顺序用换行拼接。
9. metadata 包含 `server`、`remote_tool`、`non_text_dropped`。
10. 测试成功、多文本块、远端错误、协议错误、超时、取消、参数非 object、非文本丢弃计数。

**验证：** `go test ./internal/mcp/... -run TestRemoteToolExecute` 通过。

## T10: 实现 SDK Dialer 骨架

**文件：** `internal/mcp/sdk.go`  
**依赖：** T1、T2、T7

**步骤：**

1. 引入官方 SDK 包，别名为 `sdkmcp`。
2. 定义 `SDKDialer`。
3. 实现 `Dial(ctx, name, cfg, info)` 的分支骨架：`stdio` 和 `http`。
4. stdio 分支创建 `exec.CommandContext(ctx, cfg.Command, cfg.Args...)`。
5. HTTP 分支创建带 `headerRoundTripper` 的 `http.Client`。
6. 创建 SDK client 并 Connect。
7. 返回包装后的 `sdkSession`。
8. 根据实际 SDK 版本调整 Connect 参数，确保编译通过。

**验证：** `go test ./internal/mcp/...` 编译通过。

## T11: 实现 SDK session 的 ListTools 转换

**文件：** `internal/mcp/sdk.go`  
**依赖：** T10

**步骤：**

1. 定义 `sdkSession`，持有 Server 名和 SDK client session。
2. 实现 `ListTools(ctx)`。
3. 调用 SDK 的 `ListTools` 或等价 iterator API。
4. 将远端工具名、描述、input schema 转为 `RemoteTool`。
5. 从远端 annotations 中读取只读声明；只有明确 true 时 `ReadOnly=true`。
6. schema 转换失败或为空时留给 `AdaptTool` 兜底。

**验证：** `go test ./internal/mcp/...` 编译通过；如 SDK 类型允许构造，增加一个 schema 转换单测。

## T12: 实现 SDK session 的 CallTool 和 Close

**文件：** `internal/mcp/sdk.go`  
**依赖：** T11

**步骤：**

1. 实现 `CallTool(ctx, name, arguments)`。
2. 调用 SDK 的 `CallTool`。
3. 遍历返回 content，只收集 text content。
4. 非 text content 增加 `NonTextDropped` 计数。
5. 保留远端 `IsError`。
6. 实现 `Close()`，调用 SDK session 的 close 方法。
7. 根据实际 SDK 版本集中处理泛型参数或 content 类型断言差异。

**验证：** `go test ./internal/mcp/...` 编译通过。

## T13: 实现 Manager 连接流程

**文件：** `internal/mcp/manager.go`、`internal/mcp/manager_test.go`  
**依赖：** T2、T8、T10

**步骤：**

1. 定义 `Manager` 和 `serverSession`。
2. 实现 options 归一化：空 timeout 使用默认值，空 Dialer 使用 `SDKDialer`。
3. 实现 `NewManager(ctx, cfg, opts)`。
4. 按 Server 名排序并为每个 Server 启动 goroutine。
5. 每个 goroutine 使用 `context.WithTimeout(ctx, ConnectTimeout)`。
6. 调用 `Dialer.Dial`。
7. Dial 失败记录 stage=`connect` issue，并跳过该 Server。
8. 成功后调用 `session.ListTools`。
9. ListTools 失败记录 stage=`list_tools` issue，关闭该 session，并跳过该 Server。
10. 对工具列表逐个调用 `AdaptTool`，失败记录 stage=`adapt_tool` issue，其它工具继续。
11. 成功 session 和工具在锁内写入 Manager。
12. 所有 goroutine 完成后按工具 Definition 名称稳定排序。

**验证：** `go test ./internal/mcp/... -run TestManager` 覆盖空配置、单 Server 成功、单 Server 失败、失败与成功并存、工具稳定排序。

## T14: 实现 Manager 查询与关闭

**文件：** `internal/mcp/manager.go`、`internal/mcp/manager_test.go`  
**依赖：** T13

**步骤：**

1. 实现 `Tools()` 返回工具切片拷贝。
2. 实现 `Issues()` 返回 issue 切片拷贝。
3. 实现 `Close()`，对所有 session 并发调用 Close。
4. 使用 `CloseTimeout` 作为整体兜底，超时后返回。
5. 测试外部修改 `Tools()` 返回值不影响 Manager 内部缓存。
6. 测试 Close 正常完成。
7. 测试注入 Close 阻塞的 session 时，Close 在短超时内返回。

**验证：** `go test ./internal/mcp/... -run TestManagerClose` 通过。

## T15: 扩展权限规则支持 MCP 工具名

**文件：** `internal/permission/target.go`、`internal/permission/rule.go`、`internal/permission/rule_test.go`、`internal/permission/target_test.go`  
**依赖：** T3

**步骤：**

1. 在权限包内增加 `isMCPToolName(name string) bool` 或等价判断，识别 `mcp__` 前缀。
2. 修改 `internalName(friendly)`：如果 friendly 是 MCP 工具名或 MCP glob 模式，返回原值。
3. 修改 `friendlyName(internal)`：如果 internal 是 MCP 工具名，返回原值。
4. 确认 `RuleSet.Match` 对 MCP 精确名和 `*` 通配能匹配。
5. 保持内置工具 Bash/Read/Write/Edit/Glob/Grep 行为不变。
6. 增加测试：`mcp__github__get_issue` 精确 allow 命中。
7. 增加测试：`mcp__github__*` allow 命中同 Server 工具，不命中其它 Server。
8. 增加测试：deny 仍优先于 allow。
9. 增加测试：`classify` 对 `tools.SafetyReadOnly` 返回只读类别，对 `tools.SafetySideEffect` 返回现有有副作用类别。

**验证：** `go test ./internal/permission/...` 通过。

## T16: 接入 main 启动流程

**文件：** `cmd/PseudoClaude/main.go`  
**依赖：** T4、T13、T15

**步骤：**

1. import `context` 和 `PseudoClaude/internal/mcp`。
2. 保持现有 provider 配置加载失败行为不变。
3. 在 registry 构造后调用 `mcp.LoadConfig(cwd)`。
4. 把 `LoadIssue` 打印到 stderr，格式为非致命 “MCP 配置提示”。
5. 调用 `mcp.NewManager(context.Background(), mcpCfg, mcp.ManagerOptions{ClientInfo: ...})`。
6. 把 Manager issues 打印到 stderr，格式为非致命 “MCP 连接提示”。
7. 遍历 `manager.Tools()` 注册到 registry。
8. `Register` 冲突时打印非致命提示并跳过冲突工具。
9. `defer manager.Close()`，确保 TUI 退出后关闭会话。
10. 无 MCP 配置时，不输出多余噪音，仍正常进入 TUI。

**验证：** `go build ./...` 通过；临时移除 MCP 配置时启动路径与当前行为一致。

## T17: 新增配置示例

**文件：** `docs/Part_6/mcp-servers.example.yaml`  
**依赖：** T4

**步骤：**

1. 写明用户级路径 `~/.PseudoClaude/config.yaml` 和项目级路径 `<project>/.PseudoClaude/config.yaml`。
2. 写明同名 Server 项目级完整覆盖用户级。
3. 写明只展开 `env` 和 `headers` 的值。
4. 提供 stdio 示例，使用 `command`、`args`、`env`。
5. 提供 HTTP 示例，使用 `url`、`headers`。
6. 所有 token 使用 `${VAR}`，不写真实密钥。
7. 增加配置测试 fixture 或直接在测试中读取示例，确认 YAML 能解析为合法 Server。

**验证：** `go test ./internal/mcp/... -run TestExampleConfig` 通过；`rg -n "sk-\|ghp_\|github_pat_\|Bearer [A-Za-z0-9]" docs/Part_6` 无真实密钥命中。

## T18: 增加 stdio 集成测试用最小 MCP Server

**文件：** `internal/mcp/manager_test.go` 或 `internal/mcp/sdk_test.go`  
**依赖：** T12、T13

**步骤：**

1. 优先使用 Go 测试内的可控 stub 验证 Manager 行为。
2. 为 SDK stdio 路径增加一个最小外部进程测试；如果实现真实 MCP Server 成本过高，则保留为 `testing.Short()` 下跳过的集成测试，并在 checklist 中用端到端场景补足。
3. 测试 env 注入至少在子进程可观察。
4. 测试进程退出时 Close 被调用。

**验证：** 默认 `go test ./internal/mcp/...` 不依赖网络、不长时间阻塞；需要真实外部 Server 的测试在明确条件下运行。

## T19: 增加 HTTP 集成测试

**文件：** `internal/mcp/sdk_test.go` 或 `internal/mcp/http_test.go`  
**依赖：** T7、T12

**步骤：**

1. 使用 `httptest.Server` 验证 headers 注入已覆盖基础行为。
2. 如果 SDK Streamable HTTP 握手可用测试轻量模拟，则实现最小 handler 完成 initialize、tools/list、tools/call。
3. 如果模拟完整协议过重，则把 HTTP 协议级集成留给 checklist 端到端场景，并在单测中覆盖 headers 注入和 Dialer 构造错误路径。
4. 记录测试边界，避免伪造不完整 MCP 协议导致测试脆弱。

**验证：** `go test ./internal/mcp/...` 通过；HTTP headers 注入测试稳定。

## T20: 全量测试、竞态和格式化

**文件：** 全项目  
**依赖：** T1-T19

**步骤：**

1. 执行 `gofmt -w` 处理新增和修改的 Go 文件。
2. 执行 `go test ./internal/mcp/...`。
3. 执行 `go test ./internal/permission/...`。
4. 执行 `go test ./...`。
5. 执行 `go test -race ./internal/mcp/... ./internal/agent/... ./internal/permission/...`。
6. 执行 `go build ./...`。
7. 执行凭据扫描：`rg -n "sk-\|ghp_\|github_pat_\|Bearer [A-Za-z0-9]" .`，确认无真实密钥。

**验证：** 所有命令通过；若 race 或集成测试暴露问题，修复后重跑对应命令。

## T21: 手工端到端验证

**文件：** 无  
**依赖：** T16、T17、T20

**步骤：**

1. 准备一个真实 stdio MCP Server，例如官方示例或本地可控 demo Server。
2. 在项目级 `.PseudoClaude/config.yaml` 中添加 `mcp_servers`，保留现有 provider 配置。
3. 启动 PseudoClaude。
4. 观察 stderr 中无敏感值泄漏，成功 Server 的工具被注册，失败 Server 不阻塞启动。
5. 让模型调用一个 MCP 只读工具，确认权限按只读工具处理。
6. 让模型调用一个未声明只读的 MCP 工具，确认权限按有副作用工具处理并触发现有确认流程。
7. 允许一次调用，确认工具结果回灌给模型且 Agent Loop 继续。
8. 退出 PseudoClaude，确认 stdio Server 子进程无残留。

**验证：** 端到端观察结果满足 checklist 中 stdio、权限、失败隔离和生命周期条目。

## 执行顺序

```text
T1 ─┬─ T2 ─ T3 ─┬─ T8 ─ T9 ─┐
    │           │           │
    │           ├─ T4 ─ T5 ─ T6 ─┐
    │           │                │
    │           ├─ T7 ───────────┤
    │           │                │
    │           └─ T15 ──────────┤
    │                            │
    └─ T10 ─ T11 ─ T12 ──────────┤
                                 ▼
                         T13 ─ T14 ─ T16
                                  │      │
                                  ├─ T17 │
                                  ├─ T18 │
                                  └─ T19 │
                                         ▼
                                  T20 ─ T21
```

最小可并行组：

- T3、T4、T7 可在 T2 后并行。
- T8 和 T15 可在 T3 后并行。
- T10-T12 依赖 SDK，可与配置和权限任务并行推进。
- T17 可在配置格式稳定后提前完成。
