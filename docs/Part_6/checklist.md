# MCP 客户端 Checklist

> 每一项都通过运行代码、检查命令输出或观察端到端行为验证；聚焦系统行为，不依赖具体函数名是否重构。

## 实现完整性

- [ ] 两层配置合并正确：用户级 `~/.PseudoClaude/config.yaml` 与项目级 `<project>/.PseudoClaude/config.yaml` 同时存在时，`mcp_servers` 按 Server 名合并，项目级同名 Server 完整覆盖用户级；缺失任一层视为空。（验证：运行配置单测，构造两层 YAML，断言合并结果与覆盖来源；覆盖 AC1）

- [ ] 配置错误安全降级：用户级或项目级配置读取失败、YAML 非法或只含无关字段时，该层被跳过并产生非致命 issue，其它层和内置工具仍可启动。（验证：运行配置单测，断言返回 issue 且合法层 Server 仍存在；覆盖 AC2）

- [ ] Server 字段校验完整：`stdio` 缺 `command`、`http` 缺 `url`、`type` 缺失或非法时，对应 Server 被跳过并产生 issue，其它 Server 不受影响。（验证：运行字段校验单测；覆盖 AC3）

- [ ] 环境变量展开边界正确：`env` 和 `headers` 的 `${VAR}` 被展开，未定义变量变为空串并产生 issue；`command`、`args`、`url`、Server 名和工具名中的 `${VAR}` 保持字面量。（验证：运行变量展开单测；覆盖 AC4）

- [ ] MCP 工具命名稳定：远端工具注册名符合 `mcp__<server>__<tool>`；合法名可拆分回原 Server 和工具名；包含 provider 禁用字符的名称被跳过并产生 issue。（验证：运行工具命名单测；覆盖 AC9）

- [ ] HTTP headers 注入可观察：HTTP transport 发出的请求包含配置 headers，且原始 request 不被原地污染。（验证：运行 `httptest` 或 RoundTripper 单测，断言服务端收到 header；覆盖 AC6）

- [ ] 远端工具定义适配正确：描述为空时有兜底描述，schema 为空时为对象 schema，只读声明映射为 `SafetyReadOnly`，未声明只读映射为 `SafetySideEffect`。（验证：运行工具适配单测；覆盖 AC9、AC12）

- [ ] MCP 工具调用参数处理正确：空参数可调用，JSON object 参数原样传给远端；非 object 或非法 JSON 返回 `invalid_arguments` 结构化工具错误。（验证：运行 `remoteTool.Execute` 参数解析单测；覆盖 AC10）

- [ ] MCP 工具调用结果映射正确：远端多个 text content 块按顺序拼接；远端 `IsError=true` 映射为工具错误；传输错误、协议错误、连接关闭、超时或取消映射为结构化工具错误。（验证：运行 `remoteTool.Execute` 成功和错误分支单测；覆盖 AC10、AC16）

- [ ] 非文本内容处理正确：远端返回文本与非文本混合内容时，工具结果正文只包含文本，metadata 或 issue 中可观察到丢弃数量，不发生 panic。（验证：运行非文本内容单测；覆盖 AC11）

- [ ] SDK stdio 连接路径可编译并可运行：stdio Server 使用配置的 `command`、`args` 和合并后的 env 启动，SDK 完成 initialize 和 tools/list。（验证：运行 SDK 编译测试；在端到端场景中接入真实或最小 stdio MCP Server；覆盖 AC5、AC8）

- [ ] SDK Streamable HTTP 连接路径可编译并可运行：HTTP Server 使用配置的 endpoint 和 headers，SDK 完成 initialize 和 tools/list；连接失败只形成该 Server 的 issue。（验证：运行 SDK 编译测试和 HTTP headers 测试；可选运行最小 HTTP MCP Server 场景；覆盖 AC6、AC8）

- [ ] Manager 失败隔离正确：一个 Server 连接失败、初始化失败、列工具失败或工具名非法时，只跳过对应 Server 或工具，成功 Server 的工具仍被缓存。（验证：运行 Manager stub 单测，包含失败与成功并存；覆盖 AC13）

- [ ] Manager 工具集合稳定且有序：所有启动尝试完成后，`Tools()` 返回固定快照；返回切片被外部修改不影响内部缓存；工具按名称稳定排序。（验证：运行 Manager 稳定排序和拷贝单测；覆盖 AC14）

- [ ] Manager 生命周期关闭可靠：所有成功会话在退出时被关闭；某个 session Close 阻塞时，`Close()` 在兜底时间内返回。（验证：运行 Manager Close 单测，使用短超时和阻塞 stub；覆盖 AC15）

- [ ] 权限规则支持 MCP 工具：`mcp__server__tool` 精确规则和 `mcp__server__*` 通配规则能命中 MCP 工具；同层 deny 优先于 allow；内置 Bash/Read/Write/Edit/Glob/Grep 规则行为不变。（验证：运行 permission 规则单测；覆盖 AC12）

- [ ] MCP safety 进入现有权限模式：声明只读的 MCP 工具按只读类别处理；未声明只读的 MCP 工具按现有有副作用类别处理，并触发现有 Ask/Allow 兜底。（验证：运行 permission 分类测试和 Agent 工具执行相关测试；覆盖 AC12）

- [ ] main 启动接线完整：无 MCP 配置时启动行为等价当前版本；有 MCP 配置时先发现并注册成功工具，再进入 TUI；配置和连接 issue 只打印为非致命提示。（验证：`go build ./...`，并用无 MCP 配置和失败 Server 配置分别启动观察；覆盖 AC13、AC18）

- [ ] 配置示例可解析且无真实密钥：`docs/Part_6/mcp-servers.example.yaml` 包含 stdio 与 HTTP 示例，所有密钥使用 `${VAR}`，示例 YAML 能被测试解析为合法配置。（验证：运行示例解析测试和凭据扫描；覆盖 AC17、AC20）

## 集成

- [ ] 工具中心无感注册：MCP 工具通过 `tools.Registry.Register` 注册后，`Definitions()` 能同时返回内置工具和 MCP 工具，`Execute()` 能按 MCP 工具名调用适配工具。（验证：运行 registry 或 Manager 集成测试，断言内置工具仍存在且 MCP 工具可执行；覆盖 AC9、AC18）

- [ ] Agent Loop 不因 MCP 错误中断：MCP 工具返回错误、超时或取消时，Agent 将其作为工具结果回灌，后续轮次仍能继续。（验证：运行 agent 层测试或端到端场景，构造 MCP 工具失败并观察 stop reason 不是致命崩溃；覆盖 AC10、AC16、AC18）

- [ ] JSON-RPC 响应不会错配：同一 Server 上多个并发工具调用返回顺序不同，结果仍回到对应调用 ID。（验证：优先依赖 SDK；补充 ClientSession stub 单测模拟乱序响应，断言调用结果对应各自请求；覆盖 AC7）

- [ ] provider 适配层保持无感：Anthropic/OpenAI provider 只接收普通 `tools.Definition`，不出现 MCP 专用分支。（验证：检查 `internal/llm/anthropic.go` 和 `internal/llm/openai.go` 无 MCP 相关改动；运行 provider 现有测试；覆盖 AC18）

- [ ] 权限系统仅做必要扩展：除允许 `mcp__` 工具名规则外，不扩展 MCP 专用黑名单、沙箱或独立权限系统。（验证：检查 `internal/permission` diff，运行全部 permission 测试；覆盖 AC12、AC19）

- [ ] 连接缓存被复用：同一 Server 的多次工具调用使用启动时建立的会话，不为每次调用重新连接。（验证：ClientSession stub 记录 Dial 次数和 CallTool 次数，断言一次 Dial、多次 Call；覆盖 AC14）

- [ ] 失败 Server 不影响内置工具：配置一个不存在的 stdio command 后，PseudoClaude 仍能进入 TUI，内置读写搜索命令工具仍在 registry 中。（验证：启动观察 stderr 提示和内置工具定义数量；覆盖 AC13、AC18）

- [ ] 本阶段边界没有外溢：未新增 resources、prompts、sampling、health check、auto reconnect、config hot reload 或 OAuth 流程。（验证：代码搜索相关关键字和 public API，确认没有对这些能力做可用实现；覆盖 AC19）

## 编译与测试

- [ ] MCP 包测试通过。（验证：运行 `go test ./internal/mcp/...`）

- [ ] 权限包测试通过。（验证：运行 `go test ./internal/permission/...`）

- [ ] 全项目单元测试通过。（验证：运行 `go test ./...`）

- [ ] 竞态测试通过。（验证：运行 `go test -race ./internal/mcp/... ./internal/agent/... ./internal/permission/...`）

- [ ] 全项目可编译。（验证：运行 `go build ./...`）

- [ ] Go 文件格式化完成。（验证：运行 `gofmt -l .`，期望无输出）

- [ ] 文档和示例无真实凭据。（验证：运行 `rg -n "sk-\|ghp_\|github_pat_\|Bearer [A-Za-z0-9]" docs/Part_6 .PseudoClaude`，确认无真实 token 命中）

- [ ] Part_6 文档文件名符合流程。（验证：`docs/Part_6/` 下存在 `spec.md`、`plan.md`、`task.md`、`checklist.md`，不存在旧的 `tasks.md`）

## 端到端场景

- [ ] 场景 1：无 MCP 配置启动。项目级和用户级都没有 `mcp_servers` 时，PseudoClaude 正常进入 TUI，内置工具可用，无 MCP 错误提示。（验证：临时使用无 MCP 配置启动并观察 stderr/TUI；覆盖 AC1、AC18）

- [ ] 场景 2：stdio Server 成功接入。在项目级 `.PseudoClaude/config.yaml` 配置一个真实或最小 stdio MCP Server，启动后工具以 `mcp__<server>__<tool>` 形式出现在工具集合中；调用工具后文本结果回灌给模型。（验证：启动 PseudoClaude，触发一次 MCP 工具调用并观察工具结果；覆盖 AC5、AC8、AC9、AC10）

- [ ] 场景 3：stdio env 注入和退出清理。配置 stdio Server 的 `env` 引用 `${VAR}`，Server 能观察到展开后的值；退出 PseudoClaude 后该子进程无残留。（验证：使用可控 Server 或系统进程检查；覆盖 AC4、AC5、AC15、AC17）

- [ ] 场景 4：HTTP Server headers 注入。配置 Streamable HTTP MCP Server 和 `headers`，服务端能观察到 header；若 header 缺失导致鉴权失败，只跳过该 Server。（验证：本地最小 HTTP MCP Server 或测试服务端日志；覆盖 AC6、AC17）

- [ ] 场景 5：失败隔离。配置一个不存在 command 的 Server 和一个可用 Server，启动时看到失败 Server 的非致命提示，可用 Server 工具仍注册且能调用，内置工具仍可用。（验证：启动观察并调用可用工具；覆盖 AC13）

- [ ] 场景 6：权限复用。对只读 MCP 工具，在 default 模式下按只读工具处理；对未声明只读的 MCP 工具，在 default 模式下触发现有确认流程；写入 `mcp__server__*` allow 规则后对应工具直接放行。（验证：通过 TUI 操作或 permission 集成测试观察裁决；覆盖 AC12）

- [ ] 场景 7：调用超时和取消。让 MCP 工具调用阻塞，达到超时后模型收到结构化错误；用户取消当前轮时等待中的 MCP 调用结束，下一轮仍可继续。（验证：使用阻塞 stub 或可控 Server 触发；覆盖 AC16）

- [ ] 场景 8：工具集合稳定。进入 TUI 后关闭外部 Server，模型仍看到启动时的 MCP 工具定义；调用掉线工具返回结构化错误，不自动删除或新增工具。（验证：启动后手动停止 Server 并触发调用；覆盖 AC14）

- [ ] 场景 9：跨 provider 一致。在 Anthropic 与 OpenAI provider 配置下运行同一个 MCP 工具调用场景，工具定义、权限裁决和工具结果语义一致。（验证：切换 provider 配置重复场景 2 或使用 provider 测试替身；覆盖 AC18）

- [ ] 场景 10：范围边界。尝试配置或调用 MCP resources/prompts/sampling 相关能力时，PseudoClaude 不把它们注册为工具，也不宣称支持。（验证：使用带非工具能力的 Server 或代码搜索确认只调用 tools/list；覆盖 AC19）
