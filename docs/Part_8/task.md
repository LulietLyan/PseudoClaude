# PseudoClaude 项目记忆与会话恢复 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| ---- | ---- | ---- |
| 新建 | `internal/instructions/loader.go` | 三层指令文件加载、来源分隔、LoadResult |
| 新建 | `internal/instructions/include.go` | `@include` 展开、深度限制、环路检测、路径边界和二进制检测 |
| 新建 | `internal/instructions/loader_test.go` | 指令加载和 include 安全测试 |
| 新建 | `internal/session/context.go` | 会话 ID、目录布局、Context 创建和打开 |
| 新建 | `internal/session/writer.go` | JSONL Entry、Writer、append message、append replace |
| 新建 | `internal/session/scan.go` | 会话列表扫描、标题/模型/消息数/文件大小推导 |
| 新建 | `internal/session/load.go` | JSONL 恢复、坏行跳过、replace 边界、孤立工具调用截断 |
| 新建 | `internal/session/clean.go` | 30 天过期会话清理 |
| 新建 | `internal/session/session_test.go` | session ID、写入、列表、恢复、清理测试 |
| 新建 | `internal/memory/types.go` | Level、NoteType、Operation、常量 |
| 新建 | `internal/memory/store.go` | Markdown 笔记 CRUD、`MEMORY.md` 索引更新、路径安全 |
| 新建 | `internal/memory/manager.go` | 两级索引加载、裁剪、provider 设置、异步更新 |
| 新建 | `internal/memory/prompt.go` | 记忆更新 LLM prompt 和 JSON 操作解析 |
| 新建 | `internal/memory/memory_test.go` | Store、Manager、索引限制、mock provider 更新测试 |
| 修改 | `internal/conversation/conversation.go` | Hooks、ReplaceReason、New/NewFromMessages、SetHooks |
| 修改 | `internal/conversation/conversation_test.go` | hook 和替换原因测试 |
| 修改 | `internal/compact/runtime.go` | 使用新 session context、OpenRuntime、SwitchSession、新 ID 格式 |
| 修改 | `internal/compact/layer1.go` | 工具结果落盘后用 snapshot reason 替换历史 |
| 修改 | `internal/compact/summary.go` | 摘要压缩后用 compact reason 替换历史 |
| 修改 | `internal/compact/compact_test.go` | 新 ID、会话目录、replace reason 兼容测试 |
| 修改 | `internal/prompt/modules.go` | Optional modules 接收 instructions 和 memory 内容 |
| 修改 | `internal/prompt/prompt.go` | `PromptInputs` 和参数化 `BuildSystemPrompt` |
| 修改 | `internal/prompt/prompt_test.go` | 自定义指令和长期记忆注入测试 |
| 修改 | `internal/agent/runner.go` | 注入 instructions/memory，完成后异步记忆更新 |
| 修改 | `internal/agent/runner_test.go` | 系统提示注入和记忆更新触发测试 |
| 修改 | `internal/tui/tui.go` | Model 新增 instructions、memory、session writer、session context、resume list |
| 修改 | `internal/tui/stream.go` | `/resume` 命令分发、runner 注入、命令提示更新 |
| 修改 | `internal/tui/select.go` | provider 选定后设置 memory provider |
| 新建 | `internal/tui/resume.go` | `stateResuming`、会话列表 item、恢复执行 |
| 修改 | `internal/tui/stream_test.go` | `/resume`、互斥、恢复后追加写入测试 |
| 修改 | `cmd/PseudoClaude/main.go` | 启动时加载指令、初始化记忆、后台清理、传入 TUI |
| 删除 | `docs/Part_8/tasks.md` | 移除旧参考稿，保留规范命名 `task.md` |
| 修改 | `internal/memory/manager.go` | 增加刷新并返回索引的只读方法，供 `/memory` 使用 |
| 修改 | `internal/tui/resume.go` | 将 `/resume` 改为轻量选择面板，支持上下键、数字键、Enter、Esc |
| 修改 | `internal/tui/view.go` | 渲染 resume 选择面板；`/memory` 输出可读记忆块 |
| 修改 | `internal/tui/stream.go` | 增加 `/memory` 命令分发，更新命令提示 |

## T1: Conversation Hook 与替换原因

**文件：** `internal/conversation/conversation.go`, `internal/conversation/conversation_test.go`  
**依赖：** 无

**步骤：**

1. 定义 `ReplaceReason` 类型和 `ReplaceReasonSnapshot`、`ReplaceReasonCompact` 常量。
2. 定义 `Hooks`，包含 `OnAppend func(llm.Message)` 和 `OnReplace func(ReplaceReason, []llm.Message)`。
3. 新增 `New(hooks Hooks)` 和 `NewFromMessages(messages []llm.Message, hooks Hooks)`，并保持零值 `Conversation` 可继续使用。
4. 新增 `SetHooks(hooks Hooks)`，用于恢复会话后替换写入器回调。
5. 修改 `AddUser`、`AddAssistant`、`AddAssistantToolCalls`、`AddToolResult`，在锁内完成深拷贝写入，在锁外触发 `OnAppend`。
6. 修改 `ReplaceMessages` 签名为 `ReplaceMessages(reason ReplaceReason, messages []llm.Message)`，在锁外触发 `OnReplace`。
7. 更新 conversation 测试，覆盖 append hook、replace hook、无 hook 兼容和 `NewFromMessages` 深拷贝。

**验证：** `go test ./internal/conversation/...`

## T2: Session Context 与新会话 ID

**文件：** `internal/session/context.go`, `internal/session/session_test.go`, `internal/compact/runtime.go`  
**依赖：** 无

**步骤：**

1. 新建 `internal/session/context.go`，定义 `Context`、目录常量、`NewID(now time.Time)`、`ParseID(id string)`。
2. `NewID` 生成 `YYYYMMDD-HHMMSS-xxxx`，后缀为 4 字符随机十六进制。
3. 实现 `NewContext(workspace string, now time.Time)`，创建 `.PseudoClaude/sessions/<id>/tool-results` 并返回 JSONL 路径。
4. 实现 `OpenContext(workspace, id string)`，校验 ID 和会话目录存在，确保 `tool-results` 目录存在。
5. 修改 `compact.NewRuntime` 使用 `session.NewContext`，保持 `compact.Session` 字段不变。
6. 增加 `compact.OpenRuntime(workspace, sessionID string, contextWindow int64)` 和 `SwitchSession(session compact.Session)`。
7. 添加 session ID 解析、目录布局和 compact runtime 目录测试。

**验证：** `go test ./internal/session/... ./internal/compact/...`

## T3: JSONL Writer

**文件：** `internal/session/writer.go`, `internal/session/session_test.go`  
**依赖：** T1, T2

**步骤：**

1. 定义 `Entry`、`EntryMessage`、`EntryReplace`、`ReplaceSnapshot`、`ReplaceCompact`。
2. 实现 `NewWriter(ctx Context, model string, onError func(error))`，创建目录并以 append 模式打开 `conversation.jsonl`。
3. 实现 `OpenWriter(ctx Context, model string, onError func(error))`，打开已有 JSONL 继续追加。
4. 实现 `AppendMessage(msg llm.Message)`，写入 `type=message`，第一条消息自动带 `model`。
5. 实现 `AppendReplace(reason string, msgs []llm.Message)`，先写 `type=replace` 标记，再逐条写 replacement 快照消息。
6. 每条完整 JSON 行写入后调用 `Sync`；写入失败通过 `onError` 汇报。
7. 实现 `Close()`，关闭文件句柄。
8. 添加测试：写入 user、assistant、tool call、tool result、replace snapshot、replace compact 后，每行都能解析。

**验证：** `go test ./internal/session/...`

## T4: 会话列表扫描

**文件：** `internal/session/scan.go`, `internal/session/session_test.go`  
**依赖：** T2, T3

**步骤：**

1. 定义 `Info`，包含 `ID`、`Title`、`Model`、`MessageCount`、`ModifiedAt`、`Size`、`Dir`。
2. 实现 `List(workspace string)`，扫描 `.PseudoClaude/sessions` 下的新格式目录。
3. 对每个会话读取 `conversation.jsonl`，跳过坏行，统计有效 message 行数。
4. 标题取第一条有效 user 消息内容，截断到固定长度；缺失时使用兜底标题。
5. 模型取第一条带 model 的有效行。
6. 文件大小和更新时间来自 JSONL 文件 stat。
7. 返回结果按 `ModifiedAt` 倒序排列。
8. 添加测试：多会话排序、标题截断、坏行不影响列表、旧格式目录跳过。

**验证：** `go test ./internal/session/...`

## T5: 会话恢复解析

**文件：** `internal/session/load.go`, `internal/session/session_test.go`  
**依赖：** T3

**步骤：**

1. 定义 `LoadResult`，包含 `ID`、`Messages`、`LastMessage`、`BadLines`、`Truncated`。
2. 实现 `Load(ctx Context)`，逐行解析 JSONL。
3. 坏行计数并跳过。
4. 遇到 `type=replace` 时清空当前累积消息，并从该标记之后重新加载快照消息。
5. 将有效 `message` 行转换为 `llm.Message`，保留 content、tool calls、tool result。
6. 实现 `truncateDanglingToolCalls`，如果末尾 assistant tool calls 没有后续完整 tool result，则截断到该 assistant 之前。
7. 记录最后一条有效消息时间，用于 TUI 恢复后的时间跨度提醒。
8. 添加测试：坏行跳过、replace 后只加载快照、孤立工具调用截断、LastMessage 正确。

**验证：** `go test ./internal/session/...`

## T6: 过期会话清理

**文件：** `internal/session/clean.go`, `internal/session/session_test.go`  
**依赖：** T2

**步骤：**

1. 实现 `CleanExpired(workspace string, now time.Time) []error`。
2. 扫描 `.PseudoClaude/sessions` 下目录，只处理 `ParseID` 成功的新格式 ID。
3. 删除创建时间距 `now` 超过 30 天的会话目录。
4. 单个目录删除失败时收集错误并继续处理其他目录。
5. 添加测试：31 天前目录被删除，1 天前目录保留，旧格式目录保留。

**验证：** `go test ./internal/session/...`

## T7: 项目指令 Loader

**文件：** `internal/instructions/loader.go`, `internal/instructions/include.go`, `internal/instructions/loader_test.go`  
**依赖：** 无

**步骤：**

1. 定义 `FileName = "PSEUDOCLAUDE.md"`、`DefaultMaxDepth = 5`、`Layer`、`LoadResult`、`Loader`。
2. `NewLoader(projectRoot string)` 解析项目根和用户 home。
3. `Layers()` 返回项目根、项目 `.PseudoClaude`、用户 `~/.PseudoClaude` 三层路径和边界。
4. `Load()` 按顺序加载存在的文件，给每层内容加来源分隔标题，拼接为 `LoadResult.Content`。
5. 缺失文件静默跳过；不可读取文件记录 warning 并跳过。
6. 添加测试：三层优先级、缺失层级、来源分隔、空文件处理。

**验证：** `go test ./internal/instructions/...`

## T8: `@include` 展开与安全边界

**文件：** `internal/instructions/include.go`, `internal/instructions/loader_test.go`  
**依赖：** T7

**步骤：**

1. 实现 `isIncludeLine(line string)`，只接受独占行 `@include <relative_path>`。
2. 实现 include 文件递归展开，路径相对当前文件目录解析。
3. 实现最大深度限制，超过时写入可见 Markdown 注释警告。
4. 实现展开链 visited 集合，检测环路并写入警告。
5. 实现 `insideBoundary(path, boundary string)`，拦截跳出项目根或用户配置目录的路径。
6. 实现前 512 字节 `\x00` 二进制检测，疑似二进制时跳过并写入警告。
7. 添加测试：正常 include、非独占行保持原文、6 层嵌套、环路、路径逃逸、二进制文件。

**验证：** `go test ./internal/instructions/...`

## T9: Prompt 参数化

**文件：** `internal/prompt/modules.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`  
**依赖：** 无

**步骤：**

1. 定义 `PromptInputs`，包含 `Instructions` 和 `Memory`。
2. 修改 `OptionalModules` 接收 `PromptInputs`，把非空内容放入 `Custom Instructions` 和 `Long-Term Memory`。
3. 修改 `BuildSystemPrompt(inputs PromptInputs)`，继续按 priority 稳定组装。
4. 更新现有调用处先传空输入，保持编译。
5. 更新 prompt 测试：空输入跳过可选模块，非空 instructions/memory 均出现，固定模块顺序不变。

**验证：** `go test ./internal/prompt/... ./internal/agent/...`

## T10: Memory Store

**文件：** `internal/memory/types.go`, `internal/memory/store.go`, `internal/memory/memory_test.go`  
**依赖：** 无

**步骤：**

1. 定义 `Level`、`NoteType`、四类 NoteType、`IndexFileName`、`MaxIndexLines`、`MaxIndexBytes`。
2. 定义 `Note` 和 `Operation`。
3. 实现 `NewStore(level Level, dir string)` 和 `LoadIndex()`；索引不存在时返回空字符串。
4. 实现 `NotePath(filename string)`，拒绝空文件名、路径分隔符和跳出 memory 目录的路径。
5. 实现 create：生成安全文件名，写 frontmatter 和正文，更新 `MEMORY.md` 行。
6. 实现 update：校验 filename，重写笔记，替换索引对应行。
7. 实现 delete：校验 filename，删除笔记，移除索引对应行。
8. 添加测试：create/update/delete、frontmatter、索引行、安全路径拒绝。

**验证：** `go test ./internal/memory/...`

## T11: Memory Manager 与索引裁剪

**文件：** `internal/memory/manager.go`, `internal/memory/memory_test.go`  
**依赖：** T10

**步骤：**

1. 实现 `Manager`，持有 project/user store、provider、锁和缓存索引。
2. 实现 `NewManager(projectDir, userDir string)`。
3. 实现 `SetProvider(provider llm.Provider)`。
4. 实现 `RefreshIndex()`，读取两级索引，项目级在前、用户级在后。
5. 实现 200 行 / 25KB 裁剪，超限追加 `(index truncated)` 标记。
6. 实现 `IndexText()`，返回缓存索引，必要时可空字符串降级。
7. 添加测试：项目级在前、用户级在后、行数裁剪、字节裁剪、缺失目录不报错。

**验证：** `go test ./internal/memory/...`

## T12: Memory LLM 更新

**文件：** `internal/memory/prompt.go`, `internal/memory/manager.go`, `internal/memory/memory_test.go`  
**依赖：** T10, T11

**步骤：**

1. 实现 `BuildUpdatePrompt(turn []llm.Message, projectIndex, userIndex string)`。
2. prompt 明确四类笔记、两级归属、去重交给 LLM、只输出 JSON 数组、无变化返回 `[]`。
3. 实现 `collectJSONOperations`，通过 provider stream 收集文本并解析 `[]Operation`。
4. 实现 `UpdateAsync(ctx, UpdateInput)`：provider nil 时直接返回；否则 goroutine 内加锁、构造请求、解析操作、按 level 分发到 store。
5. 更新成功后调用 `RefreshIndex()`。
6. 所有失败通过日志或内部错误记录处理，不影响调用方。
7. 添加 mock provider 测试：返回 create/update/delete/空数组/非法 JSON，验证成功路径和失败静默。

**验证：** `go test ./internal/memory/...`

## T13: Compact 替换原因接入

**文件：** `internal/compact/layer1.go`, `internal/compact/summary.go`, `internal/compact/compact_test.go`  
**依赖：** T1, T2

**步骤：**

1. 修改 Layer 1 工具结果落盘后的 `ReplaceMessages` 调用，传入 `conversation.ReplaceReasonSnapshot`。
2. 修改 Layer 2 摘要压缩后的 `ReplaceMessages` 调用，传入 `conversation.ReplaceReasonCompact`。
3. 检查所有测试和调用点，消除旧签名。
4. 添加测试 hook，验证 Layer 1 和 Layer 2 分别触发不同 reason。
5. 验证落盘路径仍位于当前 session 的 `tool-results`。

**验证：** `go test ./internal/compact/... ./internal/conversation/...`

## T14: Agent 注入 Prompt 与记忆更新

**文件：** `internal/agent/runner.go`, `internal/agent/runner_test.go`  
**依赖：** T9, T11, T12

**步骤：**

1. 在 `Runner` 增加 `Instructions string` 和 `Memory MemoryUpdater` 字段。
2. 定义 `MemoryUpdater` 接口，包含 `IndexText()` 和 `UpdateAsync(context.Context, memory.UpdateInput)`。
3. Run 开始时记录 `startLen := req.Conversation.Len()`。
4. 组装 system prompt 时传入 `Instructions` 和 `Memory.IndexText()`；Memory nil 时传空。
5. 自然完成且无工具调用时，截取本轮消息并调用 `Memory.UpdateAsync(context.Background(), ...)`。
6. 取消、流错误、最大迭代、未知工具超限时不触发记忆更新。
7. 添加测试：系统提示包含 instructions/memory；自然完成触发一次；工具迭代最终完成只触发一次；错误停止不触发。

**验证：** `go test ./internal/agent/...`

## T15: TUI 会话写入接入

**文件：** `internal/tui/tui.go`, `internal/tui/stream.go`, `internal/tui/stream_test.go`  
**依赖：** T1, T2, T3, T13, T14

**步骤：**

1. Model 增加 `instructions`、`memory`、`sessionCtx`、`sessionWriter` 字段。
2. `tui.New` 在 `compact.NewRuntime` 成功后，用 runtime session 创建 `session.Context` 和 `session.Writer`。
3. 用 writer hooks 创建 `conversation.New(...)`，替换当前直接 `&conversation.Conversation{}`。
4. writer 错误通过 transcript error 或 startup status 可见记录。
5. `submit` 前把 `m.instructions`、`m.memory` 注入 `m.runner`。
6. 退出或切换会话时关闭旧 writer。
7. 添加测试：普通 submit 不报错且 JSONL 出现 user 行；`/compact` 不作为 user 行写入。

**验证：** `go test ./internal/tui/...`

## T16: Main 启动集成

**文件：** `cmd/PseudoClaude/main.go`  
**依赖：** T7, T11, T15

**步骤：**

1. 读取 cwd 后调用 `instructions.NewLoader(cwd).Load()`。
2. 初始化 `memory.NewManager(filepath.Join(cwd, ".PseudoClaude", "memory"), filepath.Join(home, ".PseudoClaude", "memory"))`。
3. 调用 `memory.RefreshIndex()`。
4. 后台 goroutine 调用 `session.CleanExpired(cwd, time.Now())`。
5. 把 instructions content 和 memory manager 传入 `tui.New` 的新增参数或 option。
6. 将指令加载 warning、记忆索引加载状态、清理错误摘要加入 startup status。

**验证：** `go test ./cmd/PseudoClaude/...` 或 `go test ./...`

## T17: Provider 选择后设置 Memory Provider

**文件：** `internal/tui/select.go`, `internal/tui/tui.go`, `internal/tui/stream_test.go`  
**依赖：** T11, T15

**步骤：**

1. 单 provider 启动成功时调用 `m.memory.SetProvider(provider)`。
2. 多 provider 选择成功后调用 `m.memory.SetProvider(provider)`。
3. 切换 provider 后同步更新 `runner.Provider`、`runner.Memory`。
4. 添加测试：provider 选择后 memory manager 收到 provider；无 memory manager 时不 panic。

**验证：** `go test ./internal/tui/...`

## T18: `/memory` 记忆可见性命令

**文件：** `internal/memory/manager.go`, `internal/tui/stream.go`, `internal/tui/stream_test.go`  
**依赖：** T11, T15

**步骤：**

1. 在 memory manager 增加 `RefreshAndIndexText()`，同步刷新索引后返回当前注入文本。
2. 在 TUI 命令分发中增加 `/memory`。
3. `/memory` 在 idle 状态执行，不发送给模型，不写入 conversation。
4. memory 为空时展示空状态；非空时展示项目级和用户级索引内容。
5. 更新未知命令提示包含 `/memory`。

**验证：** `go test ./internal/memory/... ./internal/tui/...`

## T19: `/resume` 轻量选择面板

**文件：** `internal/tui/tui.go`, `internal/tui/resume.go`, `internal/tui/view.go`, `internal/tui/stream_test.go`  
**依赖：** T4, T5, T15

**步骤：**

1. 在 Model 增加 `resumeChoices` 和 `resumeCursor`。
2. `/resume` 扫描会话后写入选择状态，进入 `stateResuming`。
3. 渲染类似 approval 的选择面板，展示最近会话标题、相对时间、消息数、大小和模型。
4. 支持上下键移动、数字键快速选择、Enter 恢复、Esc 取消。
5. 恢复成功后清空选择状态并回到 idle。

**验证：** `go test ./internal/tui/...`

## T18: Resume 列表 UI

**文件：** `internal/tui/resume.go`, `internal/tui/tui.go`, `internal/tui/stream.go`, `internal/tui/stream_test.go`  
**依赖：** T4, T15

**步骤：**

1. 增加 `stateResuming`。
2. 定义 `resumeItem`，实现 `Title()`、`Description()`、`FilterValue()`。
3. 实现 `newResumeList(items []session.Info, width, height int)`，启用过滤和帮助。
4. `/resume` 只在 idle command 路径可用；调用 `session.List` 后进入 `stateResuming`。
5. 列表为空时显示状态消息并留在 idle。
6. Esc 返回 idle，不修改当前会话。
7. 更新 unknown command 提示，包含 `/resume`。
8. 添加测试：`/resume` 不进入 LLM；空列表提示；Esc 取消恢复。

**验证：** `go test ./internal/tui/...`

## T19: Resume 执行恢复

**文件：** `internal/tui/resume.go`, `internal/tui/tui.go`, `internal/tui/stream_test.go`  
**依赖：** T2, T3, T5, T13, T18

**步骤：**

1. 在 `updateResuming` 中处理 Enter 选择。
2. 调用 `session.OpenContext` 和 `session.Load`。
3. 如果恢复消息估算超过阈值，创建临时 conversation 并调用 `compact.ForceCompact`。
4. 如果最后消息距当前超过 6 小时，追加时间跨度提醒消息。
5. 成功前先准备新 writer、新 compact runtime、新 conversation；都成功后再关闭旧 writer 并替换 Model 字段。
6. 替换 `runner.Compact`，保持 provider、registry、permission 不变。
7. 显示 `"已恢复会话 <id>，共 <N> 条消息"`。
8. 失败时保留原会话并显示错误。
9. 添加测试：恢复成功替换 conv；恢复后新消息追加到同 JSONL；加载失败不破坏旧 conv。

**验证：** `go test ./internal/tui/...`

## T20: Resume 互斥与状态显示

**文件：** `internal/tui/stream.go`, `internal/tui/resume.go`, `internal/tui/view.go`, `internal/tui/stream_test.go`  
**依赖：** T18, T19

**步骤：**

1. streaming 和 approving 状态下输入 `/resume` 不应触发列表；显示等待当前任务完成。
2. `stateResuming` 时屏蔽普通 submit，只处理列表键盘事件。
3. view 中为 `stateResuming` 渲染会话列表，保持底部输入区不抢焦点。
4. 恢复过程中显示加载状态，完成后回到 idle 并 focus textarea。
5. 添加测试：Agent 运行中不能恢复；resuming 状态 Enter/Esc 行为正确。

**验证：** `go test ./internal/tui/...`

## T21: Memory 与 Session 端到端单元集成

**文件：** `internal/agent/runner_test.go`, `internal/tui/stream_test.go`, `internal/session/session_test.go`  
**依赖：** T12, T14, T19

**步骤：**

1. 用 mock provider 跑一次普通 Agent Run，验证 JSONL 中有 user 和 assistant 行。
2. mock provider 返回记忆更新 JSON，验证最终回复后 memory 文件和索引被创建。
3. 构造 JSONL 带 replace compact 标记，TUI 恢复后只加载 replace 后消息。
4. 构造恢复后工具结果落盘，验证文件路径仍在恢复会话的 `tool-results`。

**验证：** `go test ./internal/agent/... ./internal/tui/... ./internal/session/...`

## T22: 命令与文案收口

**文件：** `internal/tui/stream.go`, `internal/tui/view.go`, `internal/tui/stream_test.go`  
**依赖：** T18, T20

**步骤：**

1. 更新命令帮助文案，包含 `/resume`、`/compact`、Plan Mode 相关命令。
2. 统一恢复成功、恢复失败、写入失败、清理失败的中文状态消息。
3. 确保所有新增状态消息不会被写入 conversation JSONL。
4. 添加测试：命令消息只进 transcript，不增加 conversation 长度。

**验证：** `go test ./internal/tui/...`

## T23: 全量格式化与静态编译

**文件：** 所有新增和修改的 Go 文件  
**依赖：** T1-T22

**步骤：**

1. 对新增和修改 Go 文件运行 `gofmt`。
2. 运行 `go test ./...`，修复编译、竞态明显错误和测试失败。
3. 检查 `find docs/Part_8 -name tasks.md -print`，期望无输出，确保旧任务文件已删除。
4. 检查 `rg -n "mewcode|MEWCODE" internal cmd docs/Part_8 --glob '!task.md'`，确保没有旧参考项目名残留。
5. 检查 `rg "BuildSystemPrompt\\("`，确认所有调用点已适配新签名。
6. 检查 `rg "ReplaceMessages\\("`，确认所有调用点都传入替换原因。

**验证：** `go test ./...` 通过；上述 `rg` 检查无旧项目名残留。

## 执行顺序

```text
T1 ─┬─ T3 ─ T4 ─ T5 ─ T6
    ├─ T2 ──────────────┬─ T13
    └───────────────────┘

T7 ─ T8 ─┐
T9 ──────┼─ T14
T10 ─ T11 ─ T12 ┘

T15 ─ T16 ─ T17 ─ T18 ─ T19 ─ T20 ─ T21 ─ T22 ─ T23
```

可并行部分：

- T7/T8 可与 T2-T6 并行。
- T10-T12 可与 T3-T8 并行。
- T9 可与 T1-T8 并行。
- T23 必须最后执行。
