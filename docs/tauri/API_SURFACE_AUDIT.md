# Wails API 与事件面盘点

## 结论

当前前端不是直接遍历调用 Wails binding：
`desktop/frontend/src/lib/bridge.ts` 已是主 React-to-Go adapter。

- `AppBindings` 本体声明 **466** 个方法；其组合的子 binding 接口另有 **79** 个
  方法，即至少 **545** 个 host API 入口。
- `bridge.ts` 通过 `app` Proxy 执行调用，并集中实现 `agent:event`、terminal、
  updater、runtime、tab、session 和 remote 等主要订阅。
- `bridge.ts` 是 Tauri adapter 的正确替换点；不应把 545 个方法逐一变成 Tauri
  command，也不应让 React 组件直接调用 Tauri `invoke`。

## 现有分层

```text
React components/hooks
       │
       ▼
lib/bridge.ts: AppBindings + app Proxy + event subscription helpers
       │                         │
       │                         └── browser mock
       ▼
window.go.main.App + window.runtime (Wails generated/runtime API)
       ▼
desktop.App (Go) + Go core
```

迁移后的形态应为：

```text
React components/hooks
       │
       ▼
desktop API contract (保留 app/event helper 的调用形状)
       ├── Wails adapter（稳定版与回归对照）
       └── Tauri adapter（invoke + bridge stream）
                 │
                 ├── Rust host：窗口、菜单、系统能力、sidecar 生命周期
                 └── Go bridge：会话、Agent、Provider、MCP、工具
```

## 按迁移顺序分组

| 组别 | 首轮处理 | 示例 |
| --- | --- | --- |
| A：PoC 必需 | 建立精简 bridge | health、session snapshot、open/create、submit、cancel、event subscription、shutdown。 |
| B：核心稳定性 | PoC 通过后 | settings/provider、会话历史、workspace、文件与 diff。当前 Tauri 已有 provider/历史/附件、逐层 workspace 文件引用、受限文件预览，以及 Git 与本轮 session checkpoint 变更/diff；已接入单文件恢复、撤销、检查点多文件代码回滚、同日志会话头的对话回滚与版本导航、文件和对话组合回滚，以及旧格式独立会话分叉。 |
| C：进程与工具 | 需单独生命周期测试 | shell/terminal、MCP、Browser、plugins、worktree。 |
| D：host 平台能力 | 由 Rust 实现，不进 Go core | 窗口、文件对话框、外部链接、菜单、托盘、通知、钥匙串。 |
| E：后置 | Preview 稳定后 | remote host、bot、updater、复杂管理页。 |

### A→B→C 当前执行状态（2026-09-30）

| 组别 | 当前已验证 | 下一个缺口 |
| --- | --- | --- |
| A：PoC 必需 | 精简 bridge、会话/事件链路已存在；构建从 Go `App` 源码核对 587 个方法与 TypeScript `AppBindings`，不再依赖本地生成的 Wails 类型；Rust 测试已用本次构建的真实 bridge 验证启动、退出、重启和会话读取。 | 当前提交的真实安装包回归；其余 Wails runtime 直连按功能面迁移。 |
| B：核心稳定性 | Provider、历史、工作区预览/变更和回滚切片已接入；检查点与 Git 变更面板现于回合空闲确认后自动刷新。 | 继续补齐工作区文件与 diff 的交互覆盖，并在真实 Preview 窗口验证跨会话、异常和大目录状态。 |
| C：进程与工具 | MCP 服务器/运行时及插件设置已有 bridge 和 Preview 界面；Go MCP/插件测试、前端 MCP/工作区测试以及使用真实 bridge 的 Rust 测试通过。 | shell/terminal、Browser、worktree 的 Preview 入口与生命周期测试尚未接通；MCP/插件仍需真实打包进程验证。 |

本轮 A、B、C 验证不代表整组完成；下列 D→E 改造继续保留这些发布缺口。

本次验证：`pnpm build`（含绑定契约、lint、类型检查和 bundle 门禁）、`pnpm test:tauri`、`pnpm test:mcp-app`、`pnpm test:workspace`、`go test ./cmd/reasonix-desktop-bridge -run 'Test.*(MCP|Plugin)' -count=1`，以及设置 `REASONIX_TAURI_BRIDGE_TEST_BIN` 后的 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml`（124 项通过）。这些门禁验证源码和测试进程，尚未代替真实安装包验收。

### D→E 改造与验收目标（2026-09-30，进行中）

按 D→E 顺序推进。D 验收且 Preview 稳定后再推进 E；正式发布和默认下载项切换另行授权。

| 项目 | 当前实现与验证 | 待完成验收或改造 |
| --- | --- | --- |
| D：菜单与快捷键 | macOS 编辑项改为 Tauri 原生 responder-chain 角色；设置菜单接入 Preview 设置事件并测试卸载清理。沿用 Wails 基线，Windows/Linux 不显示 macOS 菜单。固定原生菜单不再占用可配置的设置和文字大小快捷键。 | 真实 WebView 的撤销/重做、剪切/复制/粘贴、全选、隐藏其他应用、全屏与快捷键冲突验收。 |
| D：剪贴板 | 接入官方 clipboard-manager，主窗口仅允许读写文本；共享写入/读取路径覆盖 Tauri、浏览器与 Wails。消息、存储路径、hooks 路径及输入框复用；复制成功反馈等待实际写入成功。原生调用模拟、拒绝/忙碌回退、失败剪切不删文本、空剪贴板不覆盖选择与成功反馈测试通过。 | 真实系统剪贴板与 WebView 交互验收。 |
| D：窗口与多显示器 | 保存普通窗口位置和显示器缩放，最大化/最小化不覆盖普通尺寸；按当前工作区限制恢复位置，移除外接屏后回到主屏。状态文件原子替换，兼容旧尺寸文件。窗口几何回归通过。 | 真实不同缩放显示器、拔插外接屏、最大化退出再恢复验收。 |
| D：托盘与退出 | 托盘有显示/退出菜单；托盘、Dock 重开及单实例唤起均恢复最小化窗口。退出沿用 supervisor 停止路径。 | 真实托盘点击、关闭后后台任务、Cmd+Q/托盘退出及第二实例唤起验收。 |
| D：对话框与链接 | 现有 Tauri 选择器及受限 HTTP(S) Rust 打开命令保留。 | 按业务入口补齐并验收取消、无权限、特殊路径和 OAuth。 |
| D：通知与钥匙串 | 现有系统通知、事件开关及凭据同步保留；Rust 相关回归通过。 | 权限拒绝、后台通知及点击定位；真实钥匙串不可用、迁移与回退。 |
| D：单实例与数据保护 | Tauri 单实例及独立默认 Preview 数据目录已存在。当前 Wails 与 bridge 启动均持有配置/状态两处目录锁；共享任一目录都会拒绝第二个写入宿主，目录别名去重，失败释放已取锁。真实 bridge/Wails 拒绝启动测试及配置原件/备份回退回归通过。导入页在操作前展示来源、目标目录与回退说明。 | 未参与目录锁协议的旧稳定版仍需兼容性验收；不能将当前两个宿主的测试推广为所有历史二进制互斥。真实 Wails 单实例通知/唤起、完整安装包导入与回退操作仍待验收。 |
| E：remote host / bot / updater / 管理页 | remote host 与 bot 已有部分设置/bridge 接口；updater 插件已注册。 | D 验收后对照 Wails 逐项审计和补齐；特别是当前“检查更新”仍是说明对话框，插件注册不能视为更新流程完成。 |

本轮门禁：`pnpm test:clipboard`、输入框剪贴板回归、terminal selection、`pnpm test:tauri`、`pnpm build`，以及使用真实 Go bridge 的 Rust 测试（127 项通过）。`pnpm tauri:build -- --bundles app` 构建并本地 ad-hoc 签名；`tools/tauri/smoke-packaged-app.py` 在临时 HOME 分别验证默认和显式数据目录、sidecar 就绪、未认证请求拒绝以及退出无残留。此 smoke 没有执行菜单/托盘等 UI 点击；两次桌面自动化分别超时和报 ScreenCaptureKit `SCStreamErrorDomain -3811`，所以真实 UI 验收保留待办。本地 ad-hoc 签名不是正式发布签名/公证。

剪贴板插件接入与文本权限参考 [Tauri 官方文档](https://v2.tauri.app/plugin/clipboard/)。

#### D：配置/状态目录互斥与回退验证（2026-09-30）

- `profilegate.TryAcquireDesktop` 同时保护配置 home 和独立 state home；canonical/文件身份去重避免符号链接及不区分大小写卷上的重复取锁。竞争失败会释放先前取到的目录锁，正常退出与进程崩溃沿用 OS 锁释放机制。
- bridge 在身份库迁移、listener 和后台运行时启动前取锁；Wails 在诊断与主运行时初始化前取锁，持有至 `wails.Run` 返回。冲突启动移除绑定及 startup/shutdown 写入回调，保留原生 single-instance handoff 并在 DOM ready 退出。
- 使用本次构建的真实 bridge，验证 Wails 目录所有者能拒绝“共享配置、不同状态”的第二个进程，且未发布 readiness、未修改配置。使用本次构建的原生 Wails，验证目录被占用时自动退出，原配置不变，未初始化 sessions。
- Rust 导入回归覆盖 Preview 配置被修改后，正式版原件与时间戳备份仍保持原内容。回退方法是在退出 Preview 后重新打开 Wails 原目录；Preview 中的新数据不会自动回写，配置备份不是完整会话备份。
- 门禁：Go profilegate/bridge 启动测试、Wails profile/single-instance 测试（提供 `REASONIX_TAURI_BRIDGE_TEST_BIN` 与 `REASONIX_WAILS_PROFILE_TEST_BIN`），前端构建及 `pnpm test:tauri`，使用真实 bridge 的 Rust 测试（128 项通过）。旧版兼容与真实 UI 待验项继续保留，D/E 目标未完成。
- 当前改动的 `.app` 已重新构建并本地签名，打包 bridge 也已通过上述真实互斥测试；完整 host smoke 因另一份 release-candidate Preview 正在运行而触发单实例转交、在 readiness 前正常退出，未通过本轮启动/退出验收。脚本现在在启动前检查同 bundle identifier 的其他 `.app` 副本，避免误唤起正在使用的应用；须待该实例退出后重跑。上一轮 smoke 的结果不作为本次安装包通过证据。

### 旧格式对话分叉切片验证（2026-09-30）

旧格式会话现可从检查点预览独立对话分叉。提交在原会话目录中生成新的 `tauri-*` 文件名，身份库登记新 ID，界面刷新目录后打开新会话；原 transcript 和工作区文件保持不变。无效方案被拒绝，失败且未生成文件时尝试清理预留身份。隔离测试验证新 ID 可经正常打开流程恢复，路由测试验证会话隔离和请求去重。

### 文件与对话组合回滚切片验证（2026-09-30）

- Preview 使用同一 `both` 方案预览文件与对话，旧格式会话禁用。提交需要覆盖缺口确认；结果区分全部成功与文件事务未完成的部分成功，部分成功时保留新对话版本并提示检查工作区文件。
- 完全成功的组合回滚提供专门撤销入口：文件事务撤销后，若新对话尚未继续，核心会同时返回原 head；若已经继续，只撤销文件，界面显示差异并保留版本导航。
- Go 隔离目录测试覆盖覆盖缺口确认、错误 scope、双侧成功、空与已继续 head 的撤销、提交前手工改文件导致的部分成功及旧格式禁用；bridge 路由覆盖会话隔离与 request ID 去重，前端测试覆盖预览、确认、历史刷新、撤销与部分成功提示。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、`pnpm test:tauri` 与 `pnpm build` 通过。`make lint` 因未安装 `golangci-lint` 未运行；手动 `repolint` 仍报仓库级 ratchet 超限，未更新基线；`internal/tool/builtin` 与 `internal/boot` 的完整测试因沙箱禁止监听回环端口失败。未改动真实 profile，未执行旧版兼容门禁。

### 同日志对话版本导航切片验证（2026-09-30）

- Preview 的检查点面板列出当前 transcript 的活跃版本，可切回原始版本或继续过的回滚版本；切换仅接受 head ID，旧格式会话返回空列表，工作区文件与会话路径不变。列表限制为原始版本、当前版本与最近版本，名称和预览长度受限。
- Go 测试覆盖继续回滚后往返切换、重新打开后保留选择、未知 head 与旧格式拒绝；bridge 路由覆盖会话隔离与 request ID 去重；前端测试覆盖版本列表与历史刷新。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、`pnpm test:tauri` 与 `pnpm build` 通过。`make lint` 因未安装 `golangci-lint` 未运行；手动 `repolint` 仍报仓库级 ratchet 超限，未更新基线；`internal/tool/builtin` 与 `internal/boot` 完整测试因沙箱禁止监听回环端口失败。

### 检查点对话回滚切片验证（2026-09-30）

- 仅对同 transcript 可切换 head 的会话开放；旧格式或文件分支策略在预览阶段禁用，提交再次校验 scope 与会话头。回滚后重新读取历史，文件不变；空 rewind head 可返回原对话。
- Go 隔离目录测试覆盖回滚、同路径、文件不变、错误 scope 和返回原对话；bridge 路由测试覆盖会话隔离与 request ID 去重，前端测试覆盖预览、提交、历史刷新与返回。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、前端完整构建与 `test:tauri` 通过。`make lint` 因未安装 `golangci-lint` 未运行；手动 `repolint` 仍报仓库级 ratchet 超限（含本切片触及的既有超限文件），未更新基线；`internal/tool/builtin` 与 `internal/boot` 完整测试因沙箱禁止监听回环端口失败。未改动真实 profile，未执行旧版兼容门禁。

### 检查点代码回滚切片验证（2026-09-30）

- 隔离目录测试覆盖多文件恢复、覆盖缺口确认、预览后手工改动拒绝、错误 scope 拒绝、对话不变和撤销；bridge 路由测试覆盖会话隔离与 request ID 去重。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、前端 `typecheck`、`lint:hooks`、`test:tauri` 与 `pnpm build` 通过；构建体积仍低于现有门限。
- 本机 `make lint` 因未安装 `golangci-lint` 未运行；`internal/tool/builtin` 与 `internal/boot` 的完整测试因沙箱禁止监听回环端口失败。未修改真实 profile，也未执行旧版兼容门禁。

### 对话版本与检查点归属（2026-09-30）

- schema-2 检查点保存其边界用户消息 ID；列表、预览、提交及按检查点分叉均核对当前对话版本中的对应消息，防止另一版本在相同位置的新消息误用旧回滚边界。
- 旧的未绑定检查点只在单版本会话中补写消息 ID；已有多个版本且无法证明归属的检查点不会显示为可回滚。回归测试覆盖切换版本、预览后切换及重开会话。
- `internal/checkpoint` 完整测试、`internal/control` 的检查点/回滚/分叉/版本定向测试、bridge 回滚定向测试及 `go vet ./...` 通过。完整 `internal/control`、`internal/tool/builtin` 和 `internal/boot` 测试在需监听回环端口的用例处受沙箱限制；`make lint` 因缺少 `golangci-lint` 未运行。

### 检查点面板回合刷新（2026-09-30）

- Tauri Preview 在回合运行时撤下旧检查点及回滚预览；bridge 快照确认会话空闲后，若面板仍打开则自动重读检查点与对话版本。关闭面板或切换会话后，过期读取结果不会写回当前列表。
- 组件回归覆盖旧读取延迟返回、新检查点自动出现和切换到分叉会话后的数据隔离。
- `pnpm test:tauri`、`pnpm build`（含类型检查、hooks lint 与体积门限）通过。该界面只在 Tauri WebView 中启用，本机未运行真实 host 的交互截图验证。

## 当前事件面

主 Agent 流为 `agent:event`。此外，前端消费了下列 Wails runtime 事件：

```text
InboxChanged                     agent:ready
app:open-settings                config:load-warnings
desktop:shell-status             history-index:changed-v1
project-tree:changed             project-tree:changed-v2
project-tree:runtime-changed     remote-tab:<tabId>:event
remote-tab:<tabId>:state         remote-tab:opened
remote-tab:updated               remote:forwards
remote:server                    remote:status
runtime-state:changed            runtime:rebuilt
session:active-version-changed   session:recovered
session:recovery-failed          tab:meta
terminal:exit                    terminal:output
topic:activation                 updater:progress
agent:event
```

Phase 2 bridge 只承诺 `agent:event` 的语义等价版本。其余事件按上述分组进入
版本化协议；没有对应协议前，Tauri UI 不显示或不启用相关功能，不能伪造本地状态。

## 尚存的直接 runtime 依赖

除 `bridge.ts` 外，少数前端模块仍直接使用 `window.runtime`（clipboard、主题、
文件拖拽、原生菜单、窗口状态、crash telemetry 和若干小型事件订阅）。这些应在
Phase 1 逐项归并到 desktop API 的 platform/event 子接口。此处不做机械替换，
以免在没有 Tauri host 实现时破坏 Wails 基线。

## 迁移守卫

1. 每一组必须先有 Wails adapter 测试，再增加 Tauri adapter 测试。
2. `app` 的调用点不直接感知 transport，不出现散落的 Tauri import。
3. 前端 reducer 按 `(session/tab, sequence)` 去重；不能假定 WebView 事件绝不重复。
4. 任何订阅断线都先获取权威 snapshot，不能凭内存事件重建会话。
