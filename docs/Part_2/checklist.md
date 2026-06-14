# Agent 工具系统 Checklist

> 每一项通过运行代码或观察行为来验证，聚焦系统行为。

## 实现完整性

- [ ] 六个内置工具均已注册，且每个工具暴露名称、描述和参数 schema（验证：运行 `go test ./internal/tools -run 'TestDefaultRegistry|TestRegistry'`，期望包含 `read_file`、`write_file`、`edit_file`、`run_command`、`find_files`、`search_code`）。
- [ ] 重复工具名称、空工具名称和未知工具调用会返回清晰错误（验证：运行 `go test ./internal/tools -run 'TestRegistry'`，期望重复注册失败，未知工具返回 `OK=false` 和 `unknown_tool`）。
- [ ] 非法 JSON、缺少必填字段或字段类型错误不会触发工具实际操作（验证：运行 `go test ./internal/tools -run 'TestRegistry|Test(ReadFile|WriteFile|EditFile|RunCommand|FindFiles|SearchCode)'`，期望返回 `invalid_arguments` 或对应参数错误）。
- [ ] `read_file` 能读取文本文件，并对不存在路径、目录、二进制内容和超限内容给出结构化结果（验证：运行 `go test ./internal/tools -run 'TestReadFile'`，期望成功内容、错误类型和截断标记均符合测试断言）。
- [ ] `write_file` 能写入完整内容并创建必要父目录，写入失败时返回结构化错误（验证：运行 `go test ./internal/tools -run 'TestWriteFile'`，期望文件内容与输入一致）。
- [ ] `edit_file` 只在旧文本唯一匹配时修改文件，零匹配或多匹配时文件保持不变（验证：运行 `go test ./internal/tools -run 'TestEditFile'`，期望成功替换一次，失败结果包含匹配数量）。
- [ ] `run_command` 返回 stdout、stderr、退出码和超时信息，非零退出不会中断会话（验证：运行 `go test ./internal/tools -run 'TestRunCommand'`，期望成功、非零退出、超时、截断场景均通过）。
- [ ] `find_files` 能按 glob 风格模式返回匹配路径并限制结果数量（验证：运行 `go test ./internal/tools -run 'TestFindFiles'`，期望递归匹配和截断标记通过）。
- [ ] `search_code` 能按普通文本和正则搜索内容，返回路径、行号和摘要，并处理非法正则（验证：运行 `go test ./internal/tools -run 'TestSearchCode'`，期望普通搜索、正则搜索、非法正则和截断场景通过）。
- [ ] 工具执行均受超时控制，超时以结构化工具结果返回（验证：运行 `go test ./internal/tools -run 'TestRegistry.*Timeout|TestRunCommand.*Timeout'`，期望 `error_type` 为 `timeout` 或等价短码）。
- [ ] 工具结果成功和失败都能序列化为可回灌给模型的 JSON 字符串（验证：运行 `go test ./internal/tools`，期望 `Result.JSON()` 相关断言通过）。

## 集成

- [ ] Provider 请求前能把注册中心 definitions 转成 Anthropic 和 OpenAI 可识别的工具定义（验证：运行 `go test ./internal/llm -run 'Test(Anthropic|OpenAI).*Tool'`，期望名称、描述、schema 字段完整）。
- [ ] Anthropic 流式响应中的文本增量仍正常输出，thinking 增量不进入正文（验证：运行 `go test ./internal/llm -run 'TestAnthropic'`，期望文本和 thinking 处理断言通过）。
- [ ] Anthropic 工具调用能从流式消息累积为统一 `ToolCall`，并能把 assistant tool_use 和 user tool_result 转回 SDK message（验证：运行 `go test ./internal/llm -run 'TestAnthropic'`，期望 tool id、name、arguments、tool result 均保留）。
- [ ] OpenAI 工具调用参数碎片能拼接为完整 JSON，并转成统一 `ToolCall`（验证：运行 `go test ./internal/llm -run 'TestOpenAI'`，期望分片 arguments 拼接后可解析）。
- [ ] OpenAI 历史转换能表达 assistant tool calls 和 tool result message（验证：运行 `go test ./internal/llm -run 'TestOpenAI'`，期望 tool_call_id、function name 和 content 均保留）。
- [ ] 会话历史能同时保存普通文本、assistant 工具调用和 user 工具结果，并返回深拷贝（验证：运行 `go test ./internal/conversation`，期望修改返回值不污染内部历史）。
- [ ] TUI 提交请求时会传入工具 definitions，普通文本请求路径不回退（验证：运行 `go test ./internal/tui ./internal/llm`，期望 provider.Stream 调用相关测试或编译检查通过）。
- [ ] TUI 收到工具调用后会执行工具、追加 assistant 工具调用历史、追加 user 工具结果历史，并回到可输入状态（验证：运行 `go test ./internal/tui`，期望工具调用分支测试通过）。
- [ ] 工具执行失败后 TUI 展示错误结果且用户可继续下一轮输入（验证：运行 `go test ./internal/tui`，期望失败工具调用分支保持 `stateIdle`）。
- [ ] 入口启动时创建默认工具注册中心并注入 TUI，注册失败时给出清晰启动错误（验证：运行 `go test ./cmd/PseudoClaude ./internal/tui`，期望编译和入口装配测试通过）。

## 编译与测试

- [ ] 项目全部包编译并测试通过（验证：运行 `go test ./...`，期望退出码为 0）。
- [ ] 核心工具包测试通过（验证：运行 `go test ./internal/tools`，期望退出码为 0）。
- [ ] LLM 协议适配测试通过（验证：运行 `go test ./internal/llm`，期望退出码为 0）。
- [ ] 会话历史测试通过（验证：运行 `go test ./internal/conversation`，期望退出码为 0）。
- [ ] TUI 集成编译通过（验证：运行 `go test ./internal/tui`，期望退出码为 0）。
- [ ] 测试输出不包含 API key、配置密钥或 panic 堆栈（验证：检查 `go test ./...` 输出，期望无敏感字段和 panic）。

## 端到端场景

- [ ] 场景 1：模型请求读取一个存在的文本文件 → TUI 显示工具调用状态和成功结果 → 本轮结束后回到输入状态（验证：用测试 provider 或真实 provider 触发 `read_file`，观察工具名、结果摘要和输入框恢复）。
- [ ] 场景 2：模型请求编辑文件且旧文本唯一匹配 → 文件内容被替换 → 工具结果回灌进历史 → 下一次请求携带该结果上下文（验证：在临时文件中触发 `edit_file`，随后检查文件内容和 conversation messages）。
- [ ] 场景 3：模型请求编辑文件但旧文本出现多次 → 文件保持不变 → TUI 显示失败结果且用户可继续输入（验证：在临时文件中触发多匹配 `edit_file`，检查文件未变、结果包含匹配数量、界面回 idle）。
- [ ] 场景 4：模型请求执行一个非零退出命令 → 工具结果包含退出码、stdout、stderr → 程序不退出（验证：触发 `run_command` 执行失败命令，观察错误结果和会话继续）。
- [ ] 场景 5：模型请求超长输出或大文件读取 → 结果被截断并明确标记 → TUI 不刷屏（验证：读取大文本或运行大量输出命令，观察 `truncated=true` 或等价标记）。
- [ ] 场景 6：未触发工具调用的普通聊天 → 流式显示、markdown 定型、错误展示与 Part 1 行为一致（验证：发送普通问题，观察没有工具状态，回复结束后正常回到输入状态）。
- [ ] 场景 7：Anthropic 与 OpenAI provider 分别触发同一类工具调用 → 用户看到的工具状态、结果格式和后续可输入行为一致（验证：使用两种 provider 配置各运行一次工具调用，比较可见行为和历史回灌）。
