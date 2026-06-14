# Agent 工具系统 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| ---- | ---- | ---- |
| 新建 | `internal/tools/tool.go` | `Tool`、`Definition`、`Env`、`Call`、`Result` 和结果序列化 |
| 新建 | `internal/tools/limits.go` | 路径解析、文本/二进制判断、输出截断、默认执行限制 |
| 新建 | `internal/tools/registry.go` | 工具注册中心、默认工具集合、统一执行错误包装 |
| 新建 | `internal/tools/file.go` | `read_file`、`write_file`、`edit_file` |
| 新建 | `internal/tools/command.go` | `run_command` |
| 新建 | `internal/tools/search.go` | `find_files`、`search_code` |
| 新建 | `internal/tools/registry_test.go` | 注册、重复名称、未知工具、参数错误、超时测试 |
| 新建 | `internal/tools/file_test.go` | 文件读写、唯一匹配替换、边界错误测试 |
| 新建 | `internal/tools/command_test.go` | 命令成功、非零退出、超时测试 |
| 新建 | `internal/tools/search_test.go` | glob 查找、内容搜索、结果截断测试 |
| 修改 | `internal/llm/provider.go` | 扩展消息、工具调用、工具结果、Provider 接口 |
| 修改 | `internal/llm/anthropic.go` | Anthropic 工具定义转换、工具调用解析、工具结果回灌 |
| 修改 | `internal/llm/openai.go` | OpenAI 工具定义转换、工具调用解析、工具结果回灌 |
| 新建 | `internal/llm/anthropic_test.go` | Anthropic 工具消息转换测试 |
| 新建 | `internal/llm/openai_test.go` | OpenAI 工具消息转换和 accumulator 测试 |
| 修改 | `internal/conversation/conversation.go` | 增加工具调用和工具结果历史 |
| 修改 | `internal/conversation/conversation_test.go` | 覆盖工具历史顺序与深拷贝 |
| 修改 | `internal/tui/tui.go` | 注入工具注册中心和执行环境 |
| 修改 | `internal/tui/stream.go` | 工具事件执行、回灌、回到 idle |
| 修改 | `internal/tui/view.go` | 工具状态与结果展示样式 |
| 修改 | `internal/tui/select.go` | 保持 provider 选择后同一工具注册中心可用 |
| 修改 | `cmd/PseudoClaude/main.go` | 创建默认工具注册中心并注入 TUI |

## T1: 定义工具核心类型

**文件：** `internal/tools/tool.go`
**依赖：** 无
**步骤：**

1. 创建 `internal/tools` 包。
2. 定义 `Definition`，包含 `Name`、`Description`、`InputSchema`。
3. 定义 `Env`，包含 `CWD`、`Timeout`、`MaxReadBytes`、`MaxOutputBytes`、`MaxSearchResults`。
4. 定义 `Call`，包含 `ID`、`Name`、`Arguments`。
5. 定义 `Result`，包含 `OK`、`Tool`、`Content`、`ErrorType`、`Error`、`Metadata`。
6. 定义 `Tool` 接口：`Definition()` 和 `Execute(ctx, input, env)`。
7. 为 `Result` 增加 `JSON()` 或等价方法，返回可作为 tool result content 的 JSON 字符串；序列化失败时返回结构化 fallback。

**验证：** `go test ./internal/tools` 编译通过。

## T2: 实现工具公共限制 helper

**文件：** `internal/tools/limits.go`
**依赖：** T1
**步骤：**

1. 定义默认执行环境构造函数，例如 `DefaultEnv(cwd string) Env`。
2. 实现路径解析 helper：以 `Env.CWD` 解析相对路径，清理路径，并返回绝对或工作目录相关路径。
3. 实现文本内容判断 helper：对读取样本检测 NUL 字节，遇到明显二进制内容返回错误。
4. 实现字符串/字节输出截断 helper，超过 `MaxOutputBytes` 时返回截断内容和 `truncated=true`。
5. 实现结果列表数量限制 helper，用于搜索和文件匹配。

**验证：** `go test ./internal/tools` 编译通过。

## T3: 实现工具注册中心

**文件：** `internal/tools/registry.go`
**依赖：** T1, T2
**步骤：**

1. 定义 `Registry`，内部用 `map[string]Tool` 保存工具。
2. 实现 `NewRegistry(toolList ...Tool)`，逐个注册工具。
3. 实现 `Register`：拒绝 nil 工具、空名称、重复名称。
4. 实现 `Get` 和 `Definitions`；`Definitions` 返回稳定顺序，便于测试。
5. 实现 `Execute(ctx, call, env)`：未知工具返回 `unknown_tool`；参数不是合法 JSON 返回 `invalid_arguments`；执行时用 `context.WithTimeout` 包裹。
6. 实现 `DefaultRegistry()`，先返回空实现或占位注册逻辑，后续工具完成后补齐六个内置工具。

**验证：** `go test ./internal/tools` 编译通过。

## T4: 覆盖注册中心基础测试

**文件：** `internal/tools/registry_test.go`
**依赖：** T3
**步骤：**

1. 添加测试用假工具，返回固定 `Definition` 和 `Result`。
2. 测试注册成功后能 `Get`，`Definitions` 顺序稳定。
3. 测试重复名称、空名称、nil 工具返回错误。
4. 测试未知工具执行返回 `OK=false` 和 `unknown_tool`。
5. 测试损坏 JSON 参数返回 `invalid_arguments`，且假工具未执行。
6. 添加一个等待 ctx 的假工具，验证超时后返回 `timeout`。

**验证：** `go test ./internal/tools -run 'TestRegistry'` 通过。

## T5: 实现文件读取工具

**文件：** `internal/tools/file.go`
**依赖：** T1, T2
**步骤：**

1. 定义 `read_file` 工具结构和构造函数 `NewReadFileTool()`。
2. 声明参数 schema：必填 `path` 字符串。
3. 解析参数并校验 path 非空。
4. 使用路径 helper 定位文件；路径不存在、目录、权限错误返回结构化错误。
5. 按 `MaxReadBytes` 限制读取文本内容，并标记是否截断。
6. 对二进制内容返回 `unsupported_content` 或等价错误类型。

**验证：** `go test ./internal/tools` 编译通过。

## T6: 实现文件写入工具

**文件：** `internal/tools/file.go`
**依赖：** T5
**步骤：**

1. 定义 `write_file` 工具结构和构造函数 `NewWriteFileTool()`。
2. 声明参数 schema：必填 `path` 和 `content` 字符串。
3. 解析参数并校验 path 非空。
4. 使用路径 helper 定位目标路径。
5. 必要时创建父目录。
6. 写入完整内容；失败返回 `io_error`。
7. 成功结果包含路径和写入字节数。

**验证：** `go test ./internal/tools` 编译通过。

## T7: 实现文件唯一匹配编辑工具

**文件：** `internal/tools/file.go`
**依赖：** T5, T6
**步骤：**

1. 定义 `edit_file` 工具结构和构造函数 `NewEditFileTool()`。
2. 声明参数 schema：必填 `path`、`old_text`、`new_text` 字符串。
3. 读取目标文件文本内容，复用文本/路径 helper。
4. 统计 `old_text` 在内容中的出现次数。
5. 出现 0 次或多次时不写文件，返回 `not_unique`，metadata 包含匹配数量。
6. 正好 1 次时替换并写回。
7. 成功结果包含路径、替换次数和文件大小变化。

**验证：** `go test ./internal/tools` 编译通过。

## T8: 覆盖文件工具测试

**文件：** `internal/tools/file_test.go`
**依赖：** T5, T6, T7
**步骤：**

1. 测试 `read_file` 读取文本文件成功。
2. 测试 `read_file` 对不存在路径、目录、二进制内容返回错误。
3. 测试 `read_file` 超过限制时截断并标记。
4. 测试 `write_file` 创建新文件和父目录。
5. 测试 `edit_file` 唯一匹配时替换成功。
6. 测试 `edit_file` 零匹配和多匹配时文件内容保持不变。

**验证：** `go test ./internal/tools -run 'Test(ReadFile|WriteFile|EditFile)'` 通过。

## T9: 实现命令执行工具

**文件：** `internal/tools/command.go`
**依赖：** T1, T2
**步骤：**

1. 定义 `run_command` 工具结构和构造函数 `NewRunCommandTool()`。
2. 声明参数 schema：必填 `command` 字符串，可选 `args` 字符串数组。
3. 解析参数并校验 command 非空。
4. 使用 `exec.CommandContext(ctx, command, args...)`，工作目录设为 `Env.CWD`。
5. 捕获 stdout 和 stderr。
6. 成功退出返回 `OK=true`，metadata 包含 `exit_code=0`。
7. 非零退出返回 `OK=false`、`command_failed`，metadata 包含 exit code/stdout/stderr 截断状态。
8. ctx 超时返回 `timeout`，同时保留已捕获输出。

**验证：** `go test ./internal/tools` 编译通过。

## T10: 覆盖命令工具测试

**文件：** `internal/tools/command_test.go`
**依赖：** T9
**步骤：**

1. 测试成功命令返回 stdout 和 `OK=true`。
2. 测试非零退出命令返回 `OK=false`、退出码、stdout、stderr。
3. 测试长时间命令在短 timeout 下返回 `timeout`。
4. 测试输出超过限制时被截断并标记。

**验证：** `go test ./internal/tools -run 'TestRunCommand'` 通过。

## T11: 实现文件匹配工具

**文件：** `internal/tools/search.go`
**依赖：** T1, T2
**步骤：**

1. 定义 `find_files` 工具结构和构造函数 `NewFindFilesTool()`。
2. 声明参数 schema：必填 `pattern` 字符串。
3. 解析 pattern，拒绝空 pattern。
4. 以 `Env.CWD` 为根处理相对 glob。
5. 对普通 glob 使用 `filepath.Glob`；对递归需求通过 `filepath.WalkDir` 补充支持。
6. 返回匹配路径列表，超过 `MaxSearchResults` 时截断并标记。

**验证：** `go test ./internal/tools` 编译通过。

## T12: 实现代码内容搜索工具

**文件：** `internal/tools/search.go`
**依赖：** T11
**步骤：**

1. 定义 `search_code` 工具结构和构造函数 `NewSearchCodeTool()`。
2. 声明参数 schema：必填 `pattern` 字符串，可选 `regex` 布尔值，可选 `path` 字符串。
3. 解析参数；regex 为真时编译正则，编译失败返回 `invalid_arguments`。
4. path 为空时从 `Env.CWD` 搜索；path 指向文件时只搜该文件；path 指向目录时递归搜索。
5. 跳过目录、不可读文件和二进制文件。
6. 返回匹配摘要：路径、行号、行内容。
7. 超过 `MaxSearchResults` 时截断并标记。

**验证：** `go test ./internal/tools` 编译通过。

## T13: 覆盖搜索工具测试并补齐默认注册

**文件：** `internal/tools/search_test.go`, `internal/tools/registry.go`
**依赖：** T11, T12
**步骤：**

1. 测试 `find_files` 能匹配当前目录文件和递归文件。
2. 测试 `find_files` 结果超过限制时截断。
3. 测试 `search_code` 普通文本搜索返回文件、行号、摘要。
4. 测试 `search_code` 正则搜索和非法正则错误。
5. 测试 `search_code` 结果超过限制时截断。
6. 更新 `DefaultRegistry()`，注册六个内置工具。
7. 增加测试确认默认注册中心包含六个工具名称。

**验证：** `go test ./internal/tools` 通过。

## T14: 扩展 LLM 通用类型和接口

**文件：** `internal/llm/provider.go`, `internal/llm/anthropic.go`, `internal/llm/openai.go`, `internal/tui/stream.go`
**依赖：** T1
**步骤：**

1. 引入 `encoding/json` 和 `PseudoClaude/internal/tools`。
2. 扩展 `Message`，增加 `ToolCalls []ToolCall` 和 `ToolResult *ToolResult`。
3. 定义 `ToolCall`，包含 `ID`、`Name`、`Arguments json.RawMessage`。
4. 定义 `ToolResult`，包含 `CallID`、`Name`、`Content`、`IsError`。
5. 扩展 `StreamEvent`，增加 `ToolCall *ToolCall`。
6. 修改 `Provider.Stream` 签名为 `Stream(ctx, msgs, defs)`。
7. 对 `anthropic.go`、`openai.go` 和 `tui/stream.go` 做最小机械签名更新：provider 暂时忽略 `defs`，TUI 暂时传 nil。
8. 保持 `New` 工厂协议选择不变。

**验证：** `go test ./internal/llm ./internal/tui` 编译通过。

## T15: 更新 Anthropic 工具适配

**文件：** `internal/llm/anthropic.go`
**依赖：** T14
**步骤：**

1. 修改 `Stream` 签名，接收 `[]tools.Definition`。
2. 在 `MessageNewParams` 中填充 `Tools`。
3. 实现 `toAnthropicTools(defs)`，把通用 schema 转为 `anthropic.ToolInputSchemaParam`。
4. 更新 `toAnthropicMessages`：支持纯文本、assistant tool_use、user tool_result 三类历史。
5. 在流式循环中保留现有文本增量发送和 thinking 丢弃。
6. 使用 `anthropic.Message{}` 的 `Accumulate(event)` 累积完整消息。
7. 流结束后扫描完整消息的 `ToolUseBlock`，发送 `StreamEvent.ToolCall`。
8. 无工具调用时发送 `Done`；有工具调用时发送工具事件后再发送 `Done` 或直接结束 channel，保持 TUI 能回到 idle。

**验证：** `go test ./internal/llm` 编译通过。

## T16: 更新 OpenAI 工具适配

**文件：** `internal/llm/openai.go`
**依赖：** T14
**步骤：**

1. 修改 `Stream` 签名，接收 `[]tools.Definition`。
2. 在 `ChatCompletionNewParams` 中填充 `Tools`，并设置 `ParallelToolCalls` 为 false。
3. 实现 `toOpenAITools(defs)`，使用 `openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{...})`。
4. 更新 `toOpenAIMessages`：支持纯文本、assistant tool calls、tool result message。
5. 在流式循环中继续发送文本增量。
6. 使用 `openai.ChatCompletionAccumulator` 累积 chunk。
7. 每次 `JustFinishedToolCall()` 返回完成调用时发送 `StreamEvent.ToolCall`。
8. 流结束后补发 accumulator 中尚未发出的工具调用。
9. 无工具调用时发送 `Done`。

**验证：** `go test ./internal/llm` 编译通过。

## T17: 覆盖 LLM 工具转换测试

**文件：** `internal/llm/anthropic_test.go`, `internal/llm/openai_test.go`
**依赖：** T15, T16
**步骤：**

1. 测试 Anthropic 工具定义转换包含名称、描述、properties、required。
2. 测试 Anthropic 历史转换能表达 assistant tool_use 和 user tool_result。
3. 测试 OpenAI 工具定义转换包含 function name、description、parameters。
4. 测试 OpenAI 历史转换能表达 assistant tool calls 和 tool message。
5. 测试 OpenAI accumulator 能从分片 arguments 中得到完整 `ToolCall`。

**验证：** `go test ./internal/llm` 通过。

## T18: 扩展会话历史

**文件：** `internal/conversation/conversation.go`
**依赖：** T14
**步骤：**

1. 保留 `AddUser` 和 `AddAssistant` 行为不变。
2. 添加 `AddAssistantToolCall(call llm.ToolCall)`，追加 role 为 assistant、ToolCalls 含该调用的消息。
3. 添加 `AddToolResult(result llm.ToolResult)`，追加 role 为 user、ToolResult 指向结果的消息。
4. 更新 `Messages()`，深拷贝 `ToolCalls` slice 和 `ToolResult` 指针内容。

**验证：** `go test ./internal/conversation` 通过。

## T19: 覆盖会话工具历史测试

**文件：** `internal/conversation/conversation_test.go`
**依赖：** T18
**步骤：**

1. 保留现有文本顺序和深拷贝测试。
2. 添加 assistant tool call 后，验证 role、call id、名称、arguments。
3. 添加 tool result 后，验证 role、call id、content、isError。
4. 修改 `Messages()` 返回值中的 tool call/result，验证内部历史未被污染。

**验证：** `go test ./internal/conversation` 通过。

## T20: 注入工具注册中心到 TUI

**文件：** `internal/tui/tui.go`, `cmd/PseudoClaude/main.go`
**依赖：** T3, T13
**步骤：**

1. 修改 `tui.New` 签名为 `New(providers []config.ProviderConfig, cwd string, registry *tools.Registry)`。
2. 在 `Model` 中新增 `registry *tools.Registry` 和 `toolEnv tools.Env`。
3. 如果传入 registry 为 nil，使用空注册中心或默认注册中心，避免 panic。
4. 使用 `tools.DefaultEnv(cwd)` 初始化执行环境。
5. 在入口 `main.go` 创建 `tools.DefaultRegistry()`。
6. 启动期工具注册失败时打印清晰错误并退出。
7. 更新所有 `tui.New` 调用。

**验证：** `go test ./internal/tui ./cmd/PseudoClaude` 编译通过。

## T21: 接入工具定义到请求提交

**文件：** `internal/tui/stream.go`
**依赖：** T14, T20
**步骤：**

1. 在 `submit` 中取 `m.registry.Definitions()`。
2. 调用 `m.provider.Stream(ctx, m.conv.Messages(), defs)`。
3. 保持用户消息打印、计时、spinner 行为不变。
4. 处理 registry 为空时传入 nil 或空 definitions。

**验证：** `go test ./internal/tui ./internal/llm` 编译通过。

## T22: 实现 TUI 工具调用执行与回灌

**文件：** `internal/tui/stream.go`
**依赖：** T18, T21
**步骤：**

1. 在 `updateStreaming` 的 `streamMsg` 分支中优先处理 `event.ToolCall != nil`。
2. 若 `curReply` 有非空文本，先 `conv.AddAssistant(reply)` 并打印 assistant block。
3. 调用 `conv.AddAssistantToolCall(*event.ToolCall)`。
4. 将 `llm.ToolCall` 映射为 `tools.Call`。
5. 调用 `m.registry.Execute(context.Background(), call, m.toolEnv)` 或当前请求 ctx 派生上下文；保持超时由 registry 控制。
6. 将 `tools.Result.JSON()` 封装为 `llm.ToolResult`，追加 `conv.AddToolResult`。
7. 停止当前流，清理 `curReply`，回到 `stateIdle`。
8. 返回 textarea focus 和工具结果打印命令；不再次调用模型。

**验证：** `go test ./internal/tui` 或 `go test ./...` 编译通过。

## T23: 增加工具状态与结果展示

**文件：** `internal/tui/view.go`
**依赖：** T22
**步骤：**

1. 新增工具展示样式，例如 `toolStyle`。
2. 实现 `toolCallBlock(call llm.ToolCall)`，显示工具名称和调用 id。
3. 实现 `toolResultBlock(result tools.Result)`，成功/失败用不同文案或颜色，显示简短内容和错误信息。
4. 确保输出过长时按终端宽度或已有截断结果展示，避免破坏布局。
5. 在 T22 的打印命令中使用这些展示函数。

**验证：** `go test ./internal/tui` 或 `go test ./...` 编译通过。

## T24: 调整 provider 选择与取消行为

**文件：** `internal/tui/select.go`, `internal/tui/tui.go`, `internal/tui/stream.go`
**依赖：** T20, T22
**步骤：**

1. 确认 provider 选择后 `Model.registry` 和 `Model.toolEnv` 不被重置。
2. 确认 `stopStream()` 在工具调用处理后会取消 LLM 流。
3. 如果工具执行期间使用新的 context，确保 Ctrl+C 能调用 cancel 并退出。
4. 清理 `curTool` 等临时字段，避免下一轮残留。

**验证：** `go test ./internal/tui` 或 `go test ./...` 编译通过。

## T25: 全量编译和单元测试

**文件：** 全项目
**依赖：** T1-T24
**步骤：**

1. 运行 `go test ./...`。
2. 修复所有编译错误和失败测试。
3. 运行 `go test ./internal/tools ./internal/llm ./internal/conversation`，确认核心包测试稳定。
4. 检查 `go test` 输出中没有 panic、data race 提示或密钥泄露。

**验证：** `go test ./...` 通过。

## 执行顺序

```text
T1 → T2 → T3 → T4
              ↘
               T5 → T6 → T7 → T8
               T9 → T10
               T11 → T12 → T13
T14 → T15 → T16 → T17
T18 → T19
T20 → T21 → T22 → T23 → T24
T25
```
