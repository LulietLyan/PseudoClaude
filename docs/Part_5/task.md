# 权限系统 Tasks

## 文件清单

| 操作 | 文件 | 职责 |
| --- | --- | --- |
| 新建 | `internal/permission/mode.go` | 权限模式、裁决、类别、审批选择与模式矩阵 |
| 新建 | `internal/permission/blacklist.go` | 内置危险命令正则与黑名单命中判断 |
| 新建 | `internal/permission/sandbox.go` | 项目根解析、符号链接解析、沙箱前缀判断 |
| 新建 | `internal/permission/rule.go` | 规则结构、规则解析、精确/glob 匹配、同层 deny 优先 |
| 新建 | `internal/permission/settings.go` | YAML 配置结构、三层配置加载、默认模式选择 |
| 新建 | `internal/permission/target.go` | 工具友好名映射、工具分类、命令串与路径目标提取 |
| 新建 | `internal/permission/engine.go` | 权限引擎、四层自动判定流水线、配置加载问题暴露 |
| 新建 | `internal/permission/persist.go` | 会话级 allow、永久 allow 规则生成与本地配置写入 |
| 新建 | `internal/permission/*_test.go` | 黑名单、沙箱、规则、配置、引擎、持久化单元测试 |
| 修改 | `internal/agent/event.go` | 新增审批事件与 `ApprovalRequest` |
| 修改 | `internal/agent/runner.go` | 请求携带权限模式，Runner 持有权限引擎 |
| 修改 | `internal/agent/tools.go` | 工具执行前接入权限检查，处理 Allow/Deny/Ask |
| 修改 | `internal/agent/runner_test.go` | 增加权限回灌、审批、保序、计划工作流兼容测试 |
| 修改 | `internal/tui/tui.go` | 新增审批状态、权限模式字段、全局切换与取消分派 |
| 修改 | `internal/tui/stream.go` | 处理审批事件、审批按键、请求传递权限模式 |
| 修改 | `internal/tui/view.go` | 状态栏显示权限模式、审批块渲染 |
| 修改 | `internal/tui/stream_test.go` | 增加模式切换、审批选择、取消、状态栏测试 |
| 修改 | `cmd/PseudoClaude/main.go` | 构造权限引擎并注入 TUI |
| 修改 | `.gitignore` | 忽略 `.PseudoClaude/permissions.local.yaml` |
| 新建 | `.PseudoClaude/permissions.yaml.example` | 权限配置示例 |

## T1: 定义权限基础类型

**文件：** `internal/permission/mode.go`  
**依赖：** 无

**步骤：**

1. 定义 `Mode` 字符串类型和四个值：`ModeStrict`、`ModeDefault`、`ModeAcceptEdits`、`ModeBypassPermissions`。
2. 实现 `ParseMode`、`Mode.String`、`NextMode`，切换顺序为 `strict -> default -> acceptEdits -> bypassPermissions -> strict`。
3. 定义 `Decision` 及 `DecisionAllow`、`DecisionDeny`、`DecisionAsk`。
4. 定义 `Category` 及 `CategoryRead`、`CategoryWrite`、`CategoryExec`。
5. 定义 `ApprovalDecision` 及 `ApprovalAllowOnce`、`ApprovalAllowSession`、`ApprovalAllowForever`、`ApprovalDenyOnce`。
6. 实现 `modeFallback(mode Mode, category Category) Decision`，矩阵严格对齐 spec F5，且只返回 Allow 或 Ask。

**验证：** 新增 `internal/permission/mode_test.go`，运行 `go test ./internal/permission -run TestMode`，确认模式解析、循环顺序和矩阵结果符合 spec。

## T2: 实现危险命令黑名单

**文件：** `internal/permission/blacklist.go`  
**依赖：** T1

**步骤：**

1. 定义包内不可导出的 `dangerousCommandPatterns []*regexp.Regexp`。
2. 覆盖递归强删根目录/家目录、`dd of=/dev/*`、格式化命令、fork bomb、重定向覆盖磁盘设备、系统根目录破坏性权限变更等启发式模式。
3. 实现 `hitsBlacklist(command string) (bool, string)`，返回是否命中及命中的命令片段或规则说明。
4. 在文件注释中明确黑名单是启发式、非完备、不可配置关闭。

**验证：** 新增 `blacklist_test.go`，运行 `go test ./internal/permission -run TestBlacklist`，确认 `rm -rf /`、`rm -fr ~`、fork bomb、`dd if=/dev/zero of=/dev/sda` 命中，`rm -rf ./build`、`git status` 不命中。

## T3: 实现路径沙箱

**文件：** `internal/permission/sandbox.go`  
**依赖：** T1

**步骤：**

1. 实现 `resolveRoot(root string) (string, error)`，对项目根执行 `filepath.Abs` 和 `filepath.EvalSymlinks`。
2. 实现 `evalSymlinksOrAncestor(abs string) (string, error)`，路径存在时直接解析符号链接；路径不存在时回退到最近已存在祖先目录，解析后拼回剩余路径。
3. 实现 `insideRoot(root, target string) bool`，使用路径边界判断 `target == root || strings.HasPrefix(target, root+separator)`。
4. 实现 `sandboxTarget(root, raw string) (string, bool, error)`，支持相对路径按项目根解析，空路径视为项目根。
5. 保持跨平台路径处理，内部比较前使用 `filepath.Clean`。

**验证：** 新增 `sandbox_test.go`，运行 `go test ./internal/permission -run TestSandbox`，覆盖项目内路径、项目外路径、`../outside`、项目内软链接指向项目外、新建多级路径的最近祖先回退。

## T4: 实现规则解析与 glob 匹配

**文件：** `internal/permission/rule.go`  
**依赖：** T1

**步骤：**

1. 定义 `Rule` 和 `RuleSet`，`Rule.Action` 只能使用 `DecisionAllow` 或 `DecisionDeny`。
2. 实现 `parseRule(text string, action Decision) (Rule, bool)`，支持 `Tool(pattern)` 和 `Tool`。
3. 实现命令 glob 匹配：`*` 匹配任意字符序列，`**` 对命令等价于 `*`。
4. 实现路径 glob 匹配：`*` 匹配单段内字符，`**` 匹配跨目录段，统一使用 slash 路径。
5. 实现 `RuleSet.Match(tool, target string, isPath bool) (CheckResult, bool)`，同层先匹配 deny，再匹配 allow。

**验证：** 新增 `rule_test.go`，运行 `go test ./internal/permission -run TestRule`，覆盖 `Bash(git status)`、`Bash(git *)`、`Write(src/**)`、`Read`、非法规则跳过、同层 deny 优先。

## T5: 实现目标提取与工具映射

**文件：** `internal/permission/target.go`  
**依赖：** T1, T4

**步骤：**

1. 实现 `friendlyName(internal string) string`，映射 `run_command/read_file/write_file/edit_file/find_files/search_code` 到 `Bash/Read/Write/Edit/Glob/Grep`。
2. 实现 `classify(call llm.ToolCall, safety tools.Safety) Category`，已知工具按 plan 分类，未知或无法判断按不确定调用处理。
3. 实现 `commandText(call llm.ToolCall) (string, bool)`，解析 `run_command` 的 `command` 和 `args` 并生成稳定命令串。
4. 实现 `pathTarget(call llm.ToolCall) (target string, matchTarget string, ok bool)`，读取文件类路径目标；`search_code` 空 `path` 视为 `.`；`find_files` 提取 glob 的确定性根作为沙箱目标，并保留原始 pattern 作为规则匹配目标。
5. 参数 JSON 无法解析、缺必填字段或工具未知时返回 `ok=false`，供引擎走安全默认。

**验证：** 新增 `target_test.go`，运行 `go test ./internal/permission -run TestTarget`，覆盖六个内置工具、未知工具、无效 JSON、缺少 path/command、`find_files` 通配根提取。

## T6: 实现配置加载

**文件：** `internal/permission/settings.go`  
**依赖：** T1, T4

**步骤：**

1. 定义 `Settings`、`Options`、`LoadIssue`。
2. 实现 `DefaultOptions(root string) Options`，返回用户级、项目级、本地级权限文件路径。
3. 实现 `loadSettings(path string) (Settings, []LoadIssue)`，文件缺失返回空设置；读取或 YAML 解析失败返回空设置和 issue。
4. 实现 `settingsToRuleSet(settings Settings) (RuleSet, []LoadIssue)`，非法规则跳过并记录 issue。
5. 实现 `chooseStartMode(local, project, user Settings) Mode`，按本地、项目、用户优先级选择首个合法 `defaultMode`，否则 default。

**验证：** 新增 `settings_test.go`，运行 `go test ./internal/permission -run TestSettings`，覆盖文件缺失、非法 YAML、非法规则跳过、默认模式优先级。

## T7: 实现权限引擎流水线

**文件：** `internal/permission/engine.go`  
**依赖：** T1, T2, T3, T4, T5, T6

**步骤：**

1. 定义 `Engine` 和 `CheckResult`，包含 root、session/local/project/user 规则、本地路径、启动模式和加载问题。
2. 实现 `NewEngine(root string, opts Options) (*Engine, error)`，解析 root、加载三层配置、保存 load issues；配置错误不作为致命错误。
3. 实现 `StartMode()` 和 `LoadIssues()`，返回不可变拷贝。
4. 实现 `Check(mode Mode, call llm.ToolCall, safety tools.Safety) CheckResult` 的四层流水线：黑名单、沙箱、规则、模式。
5. 黑名单只对 `run_command` 生效；沙箱只对文件类工具生效；不适用层跳过。
6. 规则匹配顺序为 session、local、project、user；命中即短路。
7. 安全默认：无法解析文件路径时 Deny；未知工具或类别不明时 Ask 或 Deny，不返回 Allow。

**验证：** 新增 `engine_test.go`，运行 `go test ./internal/permission -run TestEngine`，覆盖五层短路、跳层、模式矩阵、会话优先级、本地/项目/用户优先级、黑名单在 bypass 下仍拦截。

## T8: 实现会话与永久 allow

**文件：** `internal/permission/persist.go`  
**依赖：** T5, T6, T7

**步骤：**

1. 实现 `ruleForCall(call llm.ToolCall) (Rule, string, bool)`，为当前调用生成精确 allow 规则和 YAML 字符串。
2. 文件类规则使用项目相对 slash 路径；命令规则使用稳定命令串。
3. 实现 `AllowForSession(call llm.ToolCall) error`，生成规则并加入 `Engine.session.Allow`，重复规则去重。
4. 实现 `PersistLocalAllow(call llm.ToolCall) error`，读取或创建本地配置，追加 allow 规则，写回 `.PseudoClaude/permissions.local.yaml`，并同步 `Engine.local.Allow`。
5. 确保写入前创建父目录，写入失败返回错误但不 panic。

**验证：** 新增 `persist_test.go`，运行 `go test ./internal/permission -run TestPersist`，覆盖会话规则即时生效、永久规则写入、重复写入去重、重建引擎后本地规则生效。

## T9: 扩展 Agent 事件与请求结构

**文件：** `internal/agent/event.go`、`internal/agent/runner.go`  
**依赖：** T7, T8

**步骤：**

1. 在 `event.go` 新增 `EventApproval` 和 `ApprovalRequest`。
2. `ApprovalRequest` 包含 `Call`、`Summary`、`Reason`、`Result`、`Respond chan permission.ApprovalDecision`。
3. 在 `Event` 增加 `Approval *ApprovalRequest` 字段。
4. 在 `Runner` 增加 `Permission *permission.Engine` 字段。
5. 在 `Request` 增加 `PermissionMode permission.Mode` 字段，保留现有 `Mode` 的 chat/plan/do 语义。
6. 在 `Runner.run` 中计算本轮权限模式：请求值为空时使用引擎启动模式；引擎为空时使用 default。

**验证：** 运行 `go test ./internal/agent/...`，确认新增事件、请求字段和 Runner 字段不破坏现有 Agent 测试。

## T10: 在 Agent 工具执行中接入权限

**文件：** `internal/agent/tools.go`、`internal/agent/runner.go`  
**依赖：** T9

**步骤：**

1. 修改 `executeToolCalls` 签名，传入权限引擎和权限模式。
2. 实现 `requestApproval(ctx, call, result, events)`，发送 `EventApproval` 并等待 `Respond` 或 `ctx.Done()`。
3. 实现 `permissionDeniedResult(call, check)`，返回 `tools.Failure(call.Name, "permission_denied", check.Reason, metadata)`。
4. 在串行工具执行前调用 `Permission.Check`：Allow 执行，Deny 生成拒绝结果，Ask 进入审批分支。
5. 在审批分支处理四种选择：允许本次直接执行；允许本会话先 `AllowForSession` 再执行；永久允许先 `PersistLocalAllow` 再执行；拒绝本次生成拒绝结果。
6. 在并发只读批中，Allow 的调用仍并发执行；Deny 的调用直接生成拒绝结果；Ask 的调用拆到串行审批路径，避免 goroutine 等待 TUI。
7. 权限拒绝也发送 `EventToolCallStart`、`EventToolResult`、`EventToolCallDone`，保持 TUI 行为一致。

**验证：** 运行 `go test ./internal/agent -run TestPermission`，新增或更新测试覆盖 Deny 回灌、Ask 允许/拒绝、会话允许、永久允许、结果保序。

## T11: 补齐 Agent 权限集成测试

**文件：** `internal/agent/runner_test.go`  
**依赖：** T10

**步骤：**

1. 更新既有测试，构造临时权限引擎传入 `Runner.Permission`。
2. 增加黑名单 Deny 回灌测试：模型请求危险命令，断言真实工具不执行，结果含 `permission_denied` 和 `blacklist`。
3. 增加沙箱 Deny 回灌测试：读写项目外路径被拒，Loop 继续到下一轮。
4. 增加 Ask 审批测试：default 下写文件触发 `EventApproval`，回传 AllowOnce 后执行，回传 DenyOnce 后回灌拒绝。
5. 增加 AllowSession 和 AllowForever 测试，断言后续同一精确调用不再 Ask，永久规则写入本地文件。
6. 增加多工具保序测试，混合拒绝和允许调用，断言结果按原调用 ID 配对。
7. 增加取消测试，在审批等待时取消 context，断言 Run 正常停止且不死锁。
8. 保留计划工作流测试，断言 `ModePlan` 仍只暴露只读工具并注入提醒。

**验证：** 运行 `go test ./internal/agent/...` 和 `go test -race ./internal/agent/...`。

## T12: TUI 状态与模式切换接入

**文件：** `internal/tui/tui.go`、`internal/tui/stream.go`  
**依赖：** T9

**步骤：**

1. 在 `sessionState` 中新增 `stateApproving`。
2. 在 `Model` 增加 `permissionMode`、`permissionEngine`、`pendingApproval`、`approvalCursor` 字段。
3. 修改 `New(providers, cwd, registry, engine)`，保存引擎并用 `engine.StartMode()` 初始化权限模式；引擎为空时使用 default。
4. 在 `submit` 构造 `agent.Request` 时填入当前 `permissionMode`。
5. 在 `Update` 顶部处理 idle 态 `shift+tab`，调用 `permission.NextMode` 并打印状态提示。
6. 保留 `/plan`、`/do` 的工作流语义；`/do` 关闭计划工作流，但不重置权限模式。
7. 在全局取消逻辑中覆盖 `stateApproving`，Esc/Ctrl+C 时向 pending respond 发送 `ApprovalDenyOnce` 后取消本轮。

**验证：** 运行 `go test ./internal/tui -run TestMode`，新增测试覆盖初始模式、Shift+Tab 循环、模式跨轮保持、`/plan`/`/do` 不重置权限模式。

## T13: TUI 审批交互接入

**文件：** `internal/tui/stream.go`、`internal/tui/view.go`  
**依赖：** T12

**步骤：**

1. 在 `handleAgentEvent` 中处理 `EventApproval`：保存 pending，重置 cursor，切换到 `stateApproving`，不继续 `waitForAgentEvent`。
2. 实现 `updateApproving`，支持上下键或 `j/k` 移动光标，Enter/Space 确认。
3. 支持数字键 `1` 到 `4` 直选：允许本次、允许本会话、永久允许、拒绝本次。
4. 实现 `sendApprovalDecision(req, decision) tea.Cmd`，向 buffered `Respond` 回传选择。
5. 在选择后回到 `stateStreaming`，清空 pending，并继续 `waitForAgentEvent`。
6. 在 `view.go` 实现 `approvalBlock`，展示工具名、参数摘要、原因和四项菜单，默认高亮允许本次。
7. 修改 `view()`，在 `stateApproving` 时展示审批块，同时保留输入框和状态栏。

**验证：** 运行 `go test ./internal/tui -run TestApproval`，覆盖收到审批事件、按键回传四种选择、Esc 取消兜底、状态切换。

## T14: 状态栏与显示细节

**文件：** `internal/tui/view.go`  
**依赖：** T12, T13

**步骤：**

1. 修改 `statusBar` 左侧显示权限模式，不再显示 provider 名。
2. 为 strict、default、acceptEdits、bypassPermissions 提供稳定大写标签：`STRICT`、`DEFAULT`、`ACCEPT EDITS`、`BYPASS`。
3. 若计划工作流开启，在模式标签旁追加 `PLAN WORKFLOW` 或等价短标签，避免与权限模式混淆。
4. 右侧继续显示模型名和 token 用量相关信息。
5. 更新 banner 或 ready 文案，提示 Shift+Tab 可切换权限模式。

**验证：** 运行 `go test ./internal/tui -run TestStatusBar`，断言状态栏包含权限模式、不包含 provider 名、计划工作流标识不覆盖权限模式。

## T15: 启动接线与配置样例

**文件：** `cmd/PseudoClaude/main.go`、`.gitignore`、`.PseudoClaude/permissions.yaml.example`  
**依赖：** T7, T12

**步骤：**

1. 在 `main.go` 中导入 `internal/permission`。
2. 获取当前工作目录后调用 `permission.NewEngine(cwd, permission.DefaultOptions(cwd))`。
3. 对 `NewEngine` 返回的非致命错误或 load issues 输出简短 stderr 提示，但不终止程序。
4. 修改 `tui.New` 调用，传入权限引擎。
5. 在 `.gitignore` 添加 `.PseudoClaude/permissions.local.yaml`。
6. 新建 `.PseudoClaude/permissions.yaml.example`，包含 `defaultMode`、allow/deny 示例、三层优先级说明和友好名说明。

**验证：** 运行 `go build ./cmd/PseudoClaude`，并用 `git check-ignore .PseudoClaude/permissions.local.yaml` 确认本地权限文件被忽略。

## T16: 全量测试与规范验证

**文件：** 全项目  
**依赖：** T1-T15

**步骤：**

1. 运行 `gofmt -w` 格式化新增和修改的 Go 文件。
2. 运行 `go test ./internal/permission/...`，确认权限核心单测通过。
3. 运行 `go test ./internal/agent/...`，确认 Agent Loop 集成测试通过。
4. 运行 `go test ./internal/tui/...`，确认 TUI 消息级测试通过。
5. 运行 `go test ./...`，确认全项目测试通过。
6. 运行 `go vet ./...`，确认无 vet 告警。
7. 运行 `go build ./...`，确认所有包编译通过。

**验证：** `gofmt -l .` 无输出；`go test ./...`、`go vet ./...`、`go build ./...` 均通过。

## 执行顺序

```text
T1
├─ T2
├─ T3
├─ T4 ─┬─ T5
│      └─ T6
└───────────────┐
T2 + T3 + T4 + T5 + T6 ─→ T7 ─→ T8
                                  ├─→ T9 ─→ T10 ─→ T11
                                  ├─→ T12 ─→ T13 ─→ T14
                                  └─→ T15
T11 + T14 + T15 ─→ T16
```

依赖摘要：

- T7 依赖 T1-T6。
- T8 依赖 T5-T7。
- T10 依赖 T9。
- T11 依赖 T10。
- T13 依赖 T12。
- T14 依赖 T12-T13。
- T15 依赖 T7 和 T12。
- T16 依赖全部实现任务。
