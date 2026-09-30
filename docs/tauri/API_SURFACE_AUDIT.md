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

**当前平台范围（2026-09-30 用户确认）：本轮只推进 macOS。** Windows 和 Linux 因缺少实际测试环境，改造及原生验收均延期，不作为本轮 D/E 完成或 macOS 发布候选的阻塞项。此前已提交的跨平台实现和验证记录保留，不能据此宣称 Windows/Linux 已受支持；本轮尚未提交的 Windows 通知改动已撤回。下方历史记录中的跨平台待办转入延期范围，后续有环境再恢复。

macOS 继续按以下顺序收尾；每项分别记录源码回归和真实安装包验收，未执行的交互不标记通过：

1. 菜单/快捷键与系统剪贴板：真实 WebView 编辑操作、可配置快捷键冲突、复制/粘贴失败反馈。
2. 窗口/托盘与退出：隐藏和最小化恢复、关闭后后台任务、Cmd+Q/托盘退出、第二实例唤起；不同缩放与外接屏拔插仍需实际显示器验证。
3. 对话框/外部应用：选择与取消、另存为、Finder/编辑器/终端工作目录、浏览器/邮件和 OAuth。
4. 通知/钥匙串：授权拒绝和重新开启、实际横幅及前后台/冷启动点击、钥匙串拒绝与设置页凭据迁移。
5. 数据兼容/回退：旧 Wails 二进制、导入与回退、旧 Global 文件引用/附件/检查点；再次核对安装包生命周期和无残留。
6. macOS D 验收且 Preview 稳定后，依次审计并迁移 E 的 remote host、bot、updater、复杂管理页。A/B/C 剩余缺口及正式签名/公证继续保留为 macOS 发布门禁。

| 项目 | 当前实现与验证 | 待完成验收或改造 |
| --- | --- | --- |
| D：菜单与快捷键 | macOS 编辑项改为 Tauri 原生 responder-chain 角色；设置菜单先恢复主窗口并发出设置事件。Reload 移除 Cmd+R，保留给可配置的会话刷新；设置和文字大小也不安装固定原生组合。原生编辑/退出/隐藏/最小化/全屏及系统 Emoji 组合禁止保存为 Preview 动作；旧冲突组合回落默认值，单项重置校验默认组合占用。真实 AppKit 菜单组合与共享保留表、设置菜单动作、组件与独立 Chrome 页面回归通过。 | 真实 WKWebView 的撤销/重做、剪切/复制/粘贴、全选、隐藏其他应用、全屏与可配置快捷键按键路由仍待验收；程序化原生菜单动作和 Chrome 页面回归不代替用户原生交互。 |
| D：剪贴板 | 接入官方 clipboard-manager，主窗口仅允许读写文本；共享写入/读取路径覆盖 Tauri、浏览器与 Wails。消息、存储路径、hooks 路径及输入框复用；复制成功反馈等待实际写入成功。原生调用模拟、拒绝/忙碌回退、失败剪切不删文本、空剪贴板不覆盖选择与成功反馈测试通过。 | 真实系统剪贴板与 WebView 交互验收。 |
| D：窗口与多显示器 | 保存普通窗口位置和显示器缩放，最大化/最小化不覆盖普通尺寸；按当前工作区限制恢复位置，移除外接屏后回到主屏。状态文件原子替换，兼容旧尺寸文件。窗口几何回归通过；真实 macOS `.app` 原生 API 已验证窗口隐藏/最小化恢复、最大化退出后重启与取消最大化、普通尺寸/位置重启恢复。 | 真实不同缩放显示器与拔插外接屏验收。原生 API 验证不代替窗口按钮/菜单的实际点击。 |
| D：托盘与退出 | 托盘有显示/退出菜单；托盘、Dock 重开及单实例唤起共用主线程恢复入口，补上 macOS 应用取消隐藏。退出沿用 supervisor 停止路径。真实包原生 CloseRequested 验证关闭后继续运行、关闭即退出、偏好重启恢复与 sidecar 清理；应用隐藏/取消隐藏也已验证。实际 Go 流式任务在关闭后继续产生事件并完成历史保存；原生 Show 菜单恢复几何，Quit 菜单在第二个任务运行中退出并清理上游及 sidecar。 | 真实托盘点击、物理 Cmd+Q/托盘退出及 Dock 重开仍待验收。程序化原生菜单动作不能代替按键/点击；第二实例恢复可见已观察到，但键盘焦点严格门禁未通过，不能标记完整唤起已验收。 |
| D：对话框与链接 | 现有 Tauri 选择器保留；共享外部链接及本地文档 adapter 已接入 Rust host。Markdown 默认打开、定位、另存为及指定已安装应用均走原生入口，错误不退回 browser mock。文档可执行目标拒绝、特殊路径、取消保存、源文件别名保护及权限拒绝已有回归。外部链接支持 HTTP(S)/受限 mailto，OAuth 入口仅接受 HTTP(S)。 | macOS 已增加系统应用注册查询、Spotlight 自定义安装位置和原生 64×64 图标；项目会话顶部选择器已接入配置偏好与卸载回退。Linux 已增加 XDG desktop entry 发现、GIO 原生启动、六类终端目录策略和有界 PNG 图标转换，共用逻辑回归通过；Linux 原生分支编译/图标/GUI 验收待执行。Windows 已增加 App Paths、安装目录/Toolbox 发现、终端目录策略和原生 PNG 图标代码；共用逻辑与 Win32 API 类型检查通过，Windows 原生 host 编译/注册表/图标/GUI 验收待执行；Global 会话已接入档案内稳定目录与会话身份查询。系统对话框、浏览器/邮件、指定应用的真实 UI 交互及 OAuth 仍需验收。 |
| D：通知与钥匙串 | macOS 通知改为原生 UserNotifications：读取实际授权、报告发送失败、点击恢复对应会话；冷启动队列、档案隔离、失效会话与重复点击已有回归。Linux 已接入 XDG 服务/能力查询、实际发送与运行中点击；独立真实 D-Bus 联调通过，授权无标准查询时报告 unknown。凭据按持久档案身份隔离；设置页显式迁移旧 Preview 凭据，保留原件并拒绝覆盖。迁移/保存/删除与重启串行，写入及桥接同步失败回滚；macOS 原生隔离读写、真实 bridge 迁移/重启/删除及不落盘回归通过。 | 真实系统通知授权拒绝、横幅显示与前后台/冷启动点击、钥匙串锁定/授权拒绝、原生设置页迁移操作仍待验收。Linux native host、桌面环境/Wayland 焦点及冷启动点击待验收/补齐；Windows 原生授权读取/点击和 Windows/Linux 凭据后端仍待补齐/验收。 |
| D：单实例与数据保护 | Tauri 单实例及独立默认 Preview 数据目录已存在。当前 Wails 与 bridge 启动均持有配置/状态两处目录锁；共享任一目录都会拒绝第二个写入宿主，目录别名去重，失败释放已取锁。真实 bridge/Wails 拒绝启动测试及配置原件/备份回退回归通过。导入页在操作前展示来源、目标目录与回退说明。 | 未参与目录锁协议的旧稳定版仍需兼容性验收；不能将当前两个宿主的测试推广为所有历史二进制互斥。真实 Wails 单实例通知/唤起、完整安装包导入与回退操作仍待验收。 |
| E：remote host / bot / updater / 管理页 | remote host 与 bot 已有部分设置/bridge 接口；updater 插件已注册。macOS 菜单改为“Updates…”说明入口，如实提示 Preview 尚无更新检查并给出手动下载地址，移除没有实现依据的“启动时自动检查”文案。 | D 验收后对照 Wails 逐项审计和补齐；当前更新入口仍是说明对话框，插件注册不能视为更新流程完成。 |

累计门禁：`pnpm test:clipboard`、输入框剪贴板回归、terminal selection、`pnpm test:tauri`、`pnpm build`，以及使用真实 Go bridge 的 Rust 测试（最新 XDG 通知切片 189 项通过、2 项默认忽略；其中 1 项独立真实 D-Bus 联调已显式通过，另有前轮 1 项显式 macOS 原生钥匙串测试通过）。最新 XDG 通知切片已从干净提交 `f8aba01793814b7119c4de7829bf747e42a94d53` 通过 `pnpm tauri:build -- --bundles app` 构建与本地 ad-hoc 签名；`tools/tauri/smoke-packaged-app.py` 在临时 HOME 分别验证默认和显式数据目录、私有凭据身份、真实 macOS 通知授权查询、实际 Global 工作区解析与私有目录权限、sidecar 就绪、未认证请求拒绝以及退出无残留。此 smoke 没有执行菜单/托盘等 UI 点击；两次桌面自动化分别超时和报 ScreenCaptureKit `SCStreamErrorDomain -3811`，所以真实 UI 验收保留待办。本地 ad-hoc 签名不是正式发布签名/公证。

剪贴板插件接入与文本权限参考 [Tauri 官方文档](https://v2.tauri.app/plugin/clipboard/)。

#### D：macOS 运行任务时关闭、后台完成与原生退出（2026-10-01）

- 新增真实包验收场景 `task-background-menu-quit`：临时私有档案配置有界 loopback 流式 provider，由实际 Go core 发起请求并通过宿主 SSE 转发读取真实 text 事件；没有调用外部收费服务、修改用户配置或新增 renderer 命令/权限。
- 首个任务收到实际内容且状态为 running 后触发原生窗口 close，严格要求窗口隐藏而任务仍运行。外部 runner 核对原宿主/sidecar PID、同一 ready 身份和未认证 health 被拒绝，再放行后续流；必须在隐藏期间收到新的真实内容、保持 running，最后核对 idle 和完整 assistant 历史。随后调用已安装的 AppKit Show Reasonix 菜单动作，要求恢复保存的普通窗口几何。
- 显式切换到第二个会话，读取实际流内容并确认 running，再调用已安装的 Quit Reasonix 菜单动作。成功路径不由 runner 调用 `app.exit`；必须观察实际宿主退出码 0、上游未完成流主动断开，以及 sidecar 和 ready 文件无残留。菜单项沿用实际 Cmd+Q 处理器；这不证明物理 Cmd+Q 或托盘点击已验收。
- 测试初始化曾遗漏 snapshot 后的 SSE 订阅，另曾用 Open 替换另一个已完成会话；依据现有协议补上正常订阅顺序及显式 Switch（仅空闲任务可切换）。这些是验收脚本修正，没有放宽任务状态、事件、历史或退出断言，也没有改变产品会话所有权规则。
- 探测包默认托管和显式 core 档案各 12 个场景通过，共 24 个。探测包复用既有前端/sidecar；干净提交的完整生产包另行记录。真实物理交互、第二实例焦点、外接屏及其他 macOS D 待办继续保留，E 仍等待 D 验收且 Preview 稳定。
- 使用真实 Go bridge 的 Rust 全量回归 189 项通过、2 项默认忽略；严格 clippy（含测试）、Rust 格式、Python 语法和 diff 检查通过。未修改前端渲染或会话运行时实现。
- 随后从干净提交 `c2da30b25cd3219e59d1562a340abb0ceb0381ca` 完整运行 `pnpm tauri:build -- --bundles app`：前端生产门禁/现有体积预算、Go sidecar 与 Rust host 均通过。对同一 arm64 macOS `.app` 的严格本地 ad-hoc 签名校验、24 个原生场景与原有两种档案包级 smoke 全部通过；实际通知授权只读查询、私有凭据身份、Global 工作区、鉴权拒绝和退出无残留同时通过。构建及验收后工作树干净；未执行正式 Developer ID 签名/公证，不能据此标记正式发布就绪。

#### D：macOS 原生菜单与配置快捷键冲突（2026-10-01）

- 对照当前 Wails 原生 Edit/Window 角色及 Preview 处理器，发现会话刷新默认 Cmd+R 与原生 Reload 的固定 Cmd+R 重复。移除 Reload 的原生 accelerator，菜单动作仍可手动使用；Cmd+R 继续由可配置的会话刷新处理器使用。
- 新增共享 `macosMenuShortcuts.json`，保留原生编辑、退出、隐藏、隐藏其他应用、最小化与全屏组合；保存配置及按键派发均拒绝占用它们。旧 Preview 已保存的原生冲突组合在解析有效绑定时回落默认值，保留存储原件供重置，不修改稳定版 Wails 快捷键偏好。单项重置若默认组合已被其他 Preview 动作使用，会提示冲突并保留配置；重置全部恢复默认组合。
- 在真实 `.app` 中读取当前 `NSApplication.mainMenu` 的 [AppKit `keyEquivalent`](https://developer.apple.com/documentation/appkit/nsmenuitem/keyequivalent) 与修饰键，要求 11 个应用原生组合全部存在，且每个实际 Cmd/Ctrl 菜单组合均在前端保留表中。首轮严格检查发现 macOS 自动补充 Emoji、听写与 Fn 项；两个实际 Emoji Cmd/Ctrl 组合加入可选保留表，空格统一为 `Space`。系统项目是否存在随 macOS/输入设置变化，其标题可本地化；可选项按组合匹配，不能因此放行未知 Cmd/Ctrl 菜单组合。Fn/无主修饰键组合不能配置为 Preview 全局动作，仍由系统处理。
- 探测包默认托管和显式 core 档案各 11 个场景通过，共 22 个（新增实际菜单组合契约场景）。原生检查没有伪造菜单键值，也没有新增 renderer 权限；实际物理按键、responder 编辑与第二实例焦点仍不在该证明范围。探测包复用已有前端/sidecar，干净提交的完整生产包另行记录。
- `test:tauri` 的快捷键单元和真实设置组件回归通过，覆盖保留组合拒绝、大小写/空格、旧原生冲突回落、替换组合保存、冲突重置不改配置与重置全部；Rust 使用真实 bridge 的 189 项回归通过、2 项默认忽略，严格 clippy 通过。
- 页面验证使用 `frontend-testing-debugging`：Browser 插件不可用，本机 Playwright WebKit 引擎缺失，改用已安装 Chrome 的独立浏览器上下文；未安装新依赖。临时页面 `http://127.0.0.1:5179/__shortcuts-qa.html` 直接挂载真实 Tauri 设置组件，独立 localStorage、Mac 平台标识和英文语言；不连接原生 host。1280×900 与 1024×768 下验证页面身份、有内容、无错误覆盖层、控制台零警告/错误、截图、录制原生组合后拒绝/保持录制、换键保存、重置冲突、重置全部与重载后再录制。截图检查反馈与按钮可见，没有横向溢出；这不是 WKWebView 或 macOS 原生按键路由验收。
- 随后从干净提交 `b9fccd040334ea1d68da5d02d358962a818c648c` 完整构建 macOS `.app`：前端生产门禁与现有体积预算、Go sidecar、Rust host 均通过；同一包的严格本地 ad-hoc 签名校验、两种临时档案共 22 个原生菜单/窗口/关闭场景及原有启动/退出 smoke 全部通过。凭据身份稳定、实际 macOS 通知授权只读查询、Global 工作区、鉴权拒绝及退出无残留同时通过，构建与验收后工作树干净。独立页面 QA 服务器已关闭；没有操作用户原生配置或安装新浏览器依赖。正式 Developer ID 签名/公证仍待完成。
- D 继续保留真实原生键盘编辑、系统剪贴板、托盘/通知/对话框交互、外接屏及数据回退待办；E 仍等 macOS D 验收且 Preview 稳定后推进。

#### D：macOS 设置菜单恢复与原生动作验收（2026-10-01）

- 真实 AppKit 设置菜单动作复现了旧问题：`host:open-settings` 已送达，但隐藏的窗口仍不可见。设置菜单处理器现先调用共用主线程窗口恢复入口，再发送设置事件；应用隐藏和窗口最小化也使用该恢复路径。
- 新增 `native_menu_smoke`，仅由现有显式环境变量验收入口调用。通过当前进程实际 `NSApplication.mainMenu` 查找已安装的 Settings 项，检查启用状态后调用 [Apple `NSMenu.performAction(forItemAt:)`](https://developer.apple.com/documentation/appkit/nsmenu/performactionforitem%28at%3A%29)，经真实 Tauri 菜单回调验证主窗口恰好收到一次设置事件，以及原生窗口可见、非最小化且应用已取消隐藏；监听随测试范围结束清理。没有直接伪造 Rust 菜单事件，也没有新增 renderer 命令或权限。
- 测试在启动就绪后直接最小化时，曾无法建立最小化前提：应用未激活、窗口保持可见且非最小化。初始化改为确认一次真实隐藏/显示转换，沿用已通过窗口门禁的准备流程；后续仍严格要求先读到真实最小化，再通过菜单动作恢复。未放宽最小化状态断言，未将启动时序问题宣称已由产品修复。
- 探测包中默认托管和显式 core 档案各 10 个场景通过，共 20 个：原有 7 个窗口/关闭场景，加隐藏窗口、最小化窗口、隐藏应用三种 Settings 菜单动作。档案身份稳定、普通几何保存、真实 sidecar 就绪/鉴权拒绝及退出无残留同时检查。使用真实 Go bridge 的 Rust 回归 189 项通过、2 项默认忽略；前端 `test:tauri`（含设置监听清理、设置覆盖层和快捷键组件回归）以及严格 clippy 通过。探测包复用已有前端和 sidecar，干净提交的完整生产包另行记录。
- 随后从干净提交 `6284dd44cbd663cb68c69613b6c668bf05fdb415` 完整运行 `pnpm tauri:build -- --bundles app`，前端生产门禁、Go sidecar 与原生宿主构建通过。对同一 `.app` 的严格本地 ad-hoc 签名校验、20 个原生窗口/Settings/关闭场景及原有两种档案的包级 smoke 全部通过；后者同时验证私有凭据身份、真实 macOS 通知授权只读查询、Global 工作区、鉴权拒绝及退出无残留。构建与验收后工作树保持干净；这不是正式 Developer ID 签名或 Apple 公证。
- 更新入口改为 Updates 信息对话框，移除没有实现依据的启动自动检查说明，提示 Preview 尚未提供更新检查并给出发行页面。这是能力说明纠正，E 的 updater 迁移仍未完成。
- 上述是程序化原生菜单动作与宿主事件验证，不包含真实鼠标/键盘操作、WebView 实际设置覆盖层渲染、编辑 responder、快捷键冲突或用户焦点。第二实例严格焦点门禁、托盘/通知交互、外接屏拔插及其他 macOS D 待办继续保留；D/E 尚未完成。

#### D：macOS 原生窗口、应用隐藏与关闭验收（2026-10-01）

- 新增 `tools/tauri/smoke-native-window.py` 和仅由环境变量显式开启的宿主验收入口，不新增 renderer 命令或权限。脚本拒绝启动已运行的同标识 Preview，使用临时 HOME/TMPDIR；默认托管和显式 core 档案各自连续重启，同一档案的凭据身份必须保持不变。
- 原生 API 分别操作真实 macOS 窗口并读取尺寸、位置、缩放、可见、最小化、最大化及应用隐藏状态。覆盖隐藏/显示、最小化/恢复、普通几何不被最大化覆盖、最大化退出后重启与取消最大化、普通窗口重启、应用隐藏/取消隐藏、CloseRequested 的继续运行/退出，以及退出偏好的下一次启动恢复。外部 runner 另检查真实 sidecar 就绪/未认证请求拒绝、实际继承的档案环境、正常退出与无残留。
- 共用 `show_main_window` 把恢复步骤放到主线程，并在 macOS 应用确实隐藏时先调用 Tauri `app.show()`；窗口显示和应用取消隐藏均有独立验收，避免只看窗口可见就认定应用已恢复。
- 本机 Stage Manager 已开启。首个靠近左边缘的测试位置在应用激活时从物理 x=80 移到 x=468；改用当前工作区中央的有效位置，仍严格要求恢复原始尺寸/位置。恢复断言没有放宽为“窗口可见即可”。在同一次应用隐藏/取消隐藏后立即最小化的组合试验中，曾观察到窗口没有进入最小化；独立场景通过不能证明该组合时序已解决，继续保留在交互验收中。
- 第二实例焦点为额外的严格 `--focus` 门禁：通过 LaunchServices 打开真实第二个进程，检查旧 sidecar 仍唯一、原窗口可见、普通几何正确且 `is_focused()` 为真。本机诊断中窗口可见但焦点为 false、应用未激活，门禁失败；该项仍是 macOS D 的未完成验收。按 [Apple 对 cooperative app activation 的说明](https://developer.apple.com/videos/play/wwdc2023/10054/)，激活请求可被系统拒绝；当前执行上下文可能有关，但这只是原因推断，不视为已排除应用缺陷或已通过真实用户唤起。
- 本机最终探测包的默认/显式档案各 7 个场景通过，共 14 个，包括关闭退出偏好的下一次启动恢复；严格 `--focus` 在第二实例阶段失败，失败状态已如实保留。使用本次真实 Go bridge 的 Rust 全量回归 189 项通过、2 项默认忽略；严格 clippy（含测试）、Rust 格式、Python 语法、CI YAML 与 macOS 窗口门禁配置检查通过。探测构建复用了已通过生产门禁的前端和 sidecar，仅重建原生宿主；干净提交的完整生产构建与包级回归仍需单独记录。
- 随后从干净提交 `91cbff49cbe10a0faab0a7570d880bad87ffc126` 完整运行 `pnpm tauri:build -- --bundles app`：前端生产门禁、Go sidecar 和原生宿主构建通过，包内 host/sidecar 严格本地 ad-hoc 签名校验通过。对同一 `.app` 的默认/显式临时档案原生窗口门禁 14 个场景再次全部通过；原有 `smoke-packaged-app.py` 两种档案均通过，真实 macOS 通知授权只读查询、私有凭据身份与 Global 工作区、鉴权拒绝及退出无残留同时通过。`pnpm test:tauri` 通过，构建与两套包级验收后工作树保持干净。这不是正式 Developer ID 签名或 Apple 公证。
- 本轮桌面自动化的 `getState()` 重新检查仍超时并重置工具会话，未取得可用于点击验收的桌面状态；没有把原生 API 门禁推广为菜单/托盘/编辑器的真实交互通过。
- 已接入 macOS CI 的默认窗口门禁，远端执行结果尚未取得。以上原生 API 验证不包含菜单/托盘点击、WebView 编辑、运行任务期间关闭、通知交互、钥匙串拒绝或真实显示器拔插；D/E 继续保持未完成。

#### D：Global 会话稳定工作区与原生打开（2026-09-30）

- Wails Global 对话使用 `ReasonixHomeDir/global-workspace`。Preview 此前未指定项目时把进程 cwd 交给 core，安装目录/启动位置可能漂移且顶部打开入口被隐藏；现在 rootless 会话使用当前 Preview 档案的 `global-workspace`，按需创建为 `0700`，文件与符号链接拒绝且不修改原目标。显式项目目录保留原选择，默认托管 Preview 不访问稳定 Wails 档案。
- 项目归属与实际工作区分离：Global 的会话/身份目录 `workspaceRoot` 仍为空，避免将内部实现路径写成项目。新增鉴权只读 `GET /v1/sessions/{id}/workspace-target` 与生成契约；runtime manager 持有 ownership 锁查询当前 controller 的规范路径，拒绝非当前、关闭或无 provider 的 runtime，不因查询创建/切换对话。
- 顶栏对所有已有会话查询能力，未发送草稿不显示。Rust 仍只接受会话 ID 与安装应用 ID，重新读取实际目录并验证响应版本/身份/绝对路径/边界与目录存在性；前端不推断 cwd 或传入任意 launch root。缺失目录或卸载应用失败时不给此前会话保留打开入口。
- 定向回归已通过：Go 的鉴权/所有权、不同档案、项目/Global 切换、内容保留与重启、默认目录文件/符号链接拒绝、关闭/不支持 provider；Rust 的不可信响应拒绝与真实 sidecar Global/项目/重启/旧会话拒绝；React 工作区的 Global 入口、仅身份跨 IPC、切换不可用目标后旧按钮消失。`pnpm test:tauri`、生产构建、Go 运行时/协议/sidecar 全量回归、Rust 全量（163 项通过、1 项原生钥匙串测试默认忽略）与严格 clippy 通过；Wails Global 目录能力基线、协议生成校验与 Go vet 通过；从干净提交 `065b1ae5f676e38a1f84a2a1bfab8f08a85bd7cc` 构建 `.app` 并通过本地严格 ad-hoc 签名校验，默认/显式档案包级 smoke 均通过，实际 sidecar 创建 Global 会话并解析当前档案内的 `0700` 目录，原生授权查询、鉴权拒绝及退出无残留同时通过。未启动外部系统应用。
- 此变更为没有显式 root 的会话设定稳定默认目录；旧 Preview 曾在启动 cwd 创建的文件不会自动搬迁，原文件保持原位。未记录实际旧目录的 rootless 会话需要单独做文件引用/附件/检查点兼容验收；不能把本轮新建/重启测试推广为旧会话完整迁移已通过。
- 真实系统应用启动及其目录/参数交互仍待现场验收；Linux 原生编译与桌面环境验收、Windows 原生 host 编译/注册表/图标/桌面环境验收继续保留。D/E 目标保持进行中。

#### D：Windows 应用发现、原生终端与图标（2026-09-30）

- 对照 Wails `external_opener_windows.go` 的 18 个编辑器/Explorer/终端身份，Rust 从绝对 PATH、[App Paths](https://learn.microsoft.com/en-us/windows/win32/shell/app-registration) 的 HKCU/HKLM 查询、默认安装目录及有界 JetBrains/Toolbox 版本目录发现应用。注册表只读，按用户优先查询 64/32 位视图；仅 `REG_SZ`/`REG_EXPAND_SZ`，限制读取体积，后者使用 Windows 原生环境变量展开。带引号的绝对 executable 路径可用，命令行尾缀、损坏/缺失值、脚本、非文件和相对路径拒绝。实际打开重新发现以处理卸载。
- Windows Terminal 通过直接进程参数 `-d` 打开实际目录，文件使用父目录；PowerShell/Command Prompt 使用 [ShellExecuteExW](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shellexecuteexw) 直接打开程序，`lpDirectory` 保留原目录且不传参数串。没有 `cmd /c start` 或提升权限 verb。独立 STA 线程平衡 COM 初始化，提交失败返回固定解决提示，启用 NO_UI 避免重复系统错误对话框；进程由后台回收，Shell 调用不索取进程句柄。提交成功不代表实际目录/窗口已验收。
- 沿用 Wails 的 Explorer 行为：目录以 `explore` 和尾部目录分隔符打开，避免同名 `.lnk` 抢先解析；文档保持系统关联打开。工作目录、固定程序路径和参数只留在宿主，前端仍仅传会话身份/已安装应用 ID，未增加通用 WebView launch/registry 权限。
- [SHGetFileInfoW](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shgetfileinfow) 获取原生图标，32×32 DIB 分别对黑/白背景绘制并 flush 后重建 RGBA，编码为最多 64 KiB 的 PNG。owned icon/DC/bitmap/selection 由 scope guard 释放并先恢复 GDI 选中对象。Windows Terminal 优先真实包图标，其次可渲染的 PowerShell 图标，再回退零字节 execution alias；保护目录不可读视为正常缺失。
- 新增 9 项共用逻辑回归，覆盖固定目录、PATH/注册表/安装目录顺序、卸载、32 位安装目录/Toolbox 版本树、字面根目录和扫描边界、坏注册值、终端别名图标回退、含空格/中文/`& % ^ ()` 的目录参数、目录与同名快捷方式、透明度重建及实际 PNG 解码。真实 bridge 的 Rust 全量回归 **181 项通过、1 项原生钥匙串测试默认忽略**；严格 clippy（含测试）、`pnpm test:tauri` 与 CI YAML 解析通过。独立临时 crate 在本机直接导入 Win32 实现及测试，以锁定的 windows-sys 0.61.2/png 0.18.1 做严格类型/lint 检查通过；该检查不链接或调用 Windows DLL，不证明 Windows host 或原生测试已通过。
- 从干净提交 `53c05ce5d9b588fc6178223a08ebb622355a8e57` 完成 macOS Preview `.app` 生产构建与严格本地 ad-hoc 签名校验；默认托管和显式临时档案包级 smoke 均通过，包括凭据身份、原生 macOS 通知授权查询、实际 Global 工作区、sidecar 就绪/鉴权拒绝及退出无残留。前端类型、分层、权限、滚动门禁和 bundle budget 同时通过。此包验证既有 macOS 行为，不包含 Windows DLL/原生 host/GUI 验收；正式签名/公证仍未完成。
- 新增 `desktop-tauri-windows` 门禁，配置真实 Go bridge、完整 native host 编译/严格 clippy/回归。Windows 专属回归包含实际 Explorer 图标、Shell 提交失败和独立临时 HKCU App Paths 的环境展开/删除恢复，不启动真实交互式控制台。**门禁尚未远端执行**；Windows 原生 host、SDK 运行、完整安装包/Explorer/终端/Unicode目录交互，以及 Linux 门禁和 GUI 验收仍待执行。D 尚未完整验收，E 按目标继续等待 D 稳定。

#### D：Linux 应用发现、终端目录与原生图标（2026-09-30）

- 对照 Wails `external_opener_linux.go`，Rust 增加固定编辑器/终端目录。按 [XDG 数据目录](https://specifications.freedesktop.org/basedir/latest/) 用户优先顺序查找 desktop entry，支持有界子目录扫描；只解析主段，动作段不能覆盖，`Hidden=true`、损坏条目和缺失 `TryExec` 拒绝。`NoDisplay` 不排除仍可打开文件的应用。PATH 只接受绝对目录中的可执行文件，实际打开重新发现以处理卸载。
- 编辑器优先直接 CLI；仅有 desktop entry 时交给 `gio launch` 处理 [Exec 字段扩展](https://specifications.freedesktop.org/desktop-entry/latest/exec-variables.html)，不自行拼接 shell。目录使用 `xdg-open` 或 `gio open`，GIO 默认目录关联提供实际名称与图标；无法解析时显示通用名称。Ghostty/GNOME Terminal、Konsole、Kitty、Alacritty 与系统 terminal 使用独立参数策略，打开文件时使用父目录，特殊字符作为单个参数。只接受已检测的固定应用 ID，原生程序、desktop 路径和参数不跨 IPC。
- GIO/目录启动器最长等待 3 秒，及时退出失败返回固定解决提示；持续运行的处理器和终端/编辑器由后台回收进程。成功表示启动已提交，不代表实际窗口、文件或目录已验收。
- 图标支持安装条目的绝对资源与有界主题/`pixmaps` 查找；含点的主题图标名称不误当扩展名，拒绝相对目录穿越、超大及无效内容。Linux 使用已锁定的 GdkPixbuf 0.18.5/GIO 0.18.4 原生加载器，把最多 1 MiB 输入转为最长边 64 像素、最多 64 KiB 的 PNG；不把 raw SVG、file URL 或 native 路径交给 WebView。本实现为固定尺寸/目录的有界查找，并非完整桌面主题继承引擎。
- 新增 9 项可在本机执行的共用逻辑回归，覆盖 XDG 优先级/默认值、用户覆盖、动作段隔离、TryExec、GIO 参数、卸载/执行权限、定制终端路径、特殊工作目录、图标点名/越界/体积及启动失败。真实 Go bridge 的 Rust 全量回归 **172 项通过、1 项原生钥匙串测试默认忽略**；严格 clippy（含测试）、`pnpm test:tauri` 与 CI YAML 解析通过。本机为 macOS，这些结果不包含 Linux 条件编译的 GIO/Pixbuf/native host 分支。
- 从干净提交 `b81abf555f63d2ef8fbd2280dfec74107af90c02` 完成 macOS Preview `.app` 构建及本地严格 ad-hoc 签名校验；默认托管/显式临时档案包级 smoke 均通过，包含凭据身份隔离、原生通知授权查询、实际 Global 工作区、sidecar readiness、未鉴权请求拒绝及正常退出无残留。生产前端构建的类型/分层/权限/滚动门禁与 bundle budget 同时通过。此包用于已有 macOS host 的回归验证，不覆盖 Linux native 分支或真实桌面应用启动；本地签名也不等同正式签名/公证。
- 新增 `desktop-tauri-linux` Ubuntu 24.04 门禁，配置 GTK/WebKitGTK/GIO/SVG loader 依赖、真实 bridge、严格 clippy 和 `dbus-run-session` 下的全量 Rust 测试；另有 Linux 专属 SVG 缩放/PNG 编码/坏输入回归。**门禁尚未远端执行**，不能声明 Linux 编译或原生图标测试已经通过。Linux GUI/包级验收、复杂 terminal wrapper/Flatpak 安装路径、完整主题继承仍待验证或完善；Windows 已在下一切片增加实现，原生 host 编译/注册表/图标/桌面环境验收继续保留。D/E 目标保持进行中。

#### D：Linux XDG 通知能力、运行中点击与退出清理（2026-09-30）

- 对照 Wails `internal/notify/sender_linux.go` 的 `notify-send` 基线，Linux 改为 Rust 直接调用 [XDG 通知协议](https://specifications.freedesktop.org/notification/latest/protocol.html) 的 `GetCapabilities` / `Notify`。未连接总线或无可用通知服务时报告 `unavailable`；服务可用时授权为 `unknown`，不把服务存在或 `actions` 能力当成用户已授权。Linux 没有协议内的标准授权/DND 查询或授权弹窗，此限制保持可见。
- `clickSupported` 取决于实际 `actions` 能力和已安装的回调。发送前先订阅通知信号，通知服务的唯一 owner、对象路径及接口均严格匹配；仅接受当前 ID 的 `default` 点击，忽略其他按钮、伪造发送者和坏消息。关闭、重复点击、意外 ID 复用、TTL 和容量上限均有保护；点击通过现有档案 token 映射、权威会话查询和主线程窗口恢复路径，不执行任务。后续能力查询或发送失败保留已成功通知的点击映射；服务 owner 替换或连接断开则清理旧 ID。
- 每个 host 懒创建一个通知 worker，不为每条通知创建阻塞等待线程。命令队列限 16 项，两类信号队列分别限 64 / 16 项，映射最多 256 项 / 7 天；单次 D-Bus 调用限 2 秒，整条命令含排队期限 5 秒、调用方等待最多 6 秒。过期命令不派发；应用退出先关闭队列、取消在途请求、释放回调并 join worker，连接关闭限 2 秒，再进入已有 sidecar 停止路径。
- 状态接口改为读取后端实际点击能力，macOS 未打包/不可用进程不再仅因编译目标而显示支持点击；macOS delegate 复用共享的点击恢复实现。前端五个专用通知命令、固定三语提示、权限范围与隐私字段限制继续沿用；Linux 不发送会话 ID、路径、凭据或任意正文，随机 token 仅保存在当前档案。
- 新增 7 项 XDG 单元回归和 1 项后端能力回归。另有 1 项显式联调，在 macOS 临时目录从[官方 D-Bus 1.14.10 源码](https://dbus.freedesktop.org/releases/dbus/dbus-1.14.10.tar.xz)构建测试 daemon 并启动独立私有总线，覆盖无服务、缺少 actions、真实 Notify 参数与静音 hint、回复前点击、伪造/重复/关闭后的点击、服务重启及 ID 复用、能力查询/提交失败不丢失已有点击、真实请求卡住后的超时恢复和退出取消。测试不连接用户的 session bus，也不显示系统通知。
- 真实 Go bridge 的全量 Rust 回归 **189 项通过、2 项默认忽略**（独立 D-Bus 联调已另行显式通过；原生钥匙串 smoke 前轮已显式执行）；严格 clippy（含测试）、`pnpm test:tauri`、CI YAML 解析通过。Linux CI 已增加显式独立总线联调，并将 clippy 扩展至测试代码；**远端门禁仍未执行**。
- 从干净提交 `f8aba01793814b7119c4de7829bf747e42a94d53` 完成 macOS Preview `.app` 生产构建及 `codesign --verify --deep --strict` 本地 ad-hoc 签名校验。默认托管和显式临时档案包级 smoke 均通过，覆盖独立凭据身份、真实 macOS 通知授权查询/点击能力、实际 Global 工作区、sidecar readiness、未鉴权请求拒绝和正常退出无残留；生产前端类型/分层/权限/滚动门禁与 bundle budget 同时通过。此包验证共享状态接口和既有 macOS host 生命周期，不覆盖 Linux native host 或真实系统通知点击；本地签名不等同正式发布签名/公证。
- 本轮仅完成协议与运行中点击实现/联调；Linux 完整 native host 编译、GNOME/KDE 横幅及通知中心操作、Wayland activation token/窗口焦点、安装包与冷启动点击仍待验收或补齐。Windows 原生授权与点击仍待改造，macOS 真实授权/横幅/点击仍待现场验收；D 尚未完成，E 继续等待 D 稳定。

#### D：原生通知授权、发送与点击定位（2026-09-30）

- 对照 Wails `internal/notify` 的固定事件提示，macOS 改用 [Apple UserNotifications](https://developer.apple.com/documentation/usernotifications/unusernotificationcenter) 读取真实授权状态并等待提交结果；仅 `not_determined` 请求授权，拒绝后不重复弹窗。设置页分别显示用户开关与系统授权；错误使用固定解决提示，不输出系统诊断。未打包开发进程报告不可用，不借用 Terminal 身份。Windows 使用 `notify-rust` 同步提交并返回错误，授权暂为 `unknown`、点击能力为 false；Linux 后续已接入 XDG 实际服务/能力查询和运行中点击，详见上一节，均不能据此声称已获系统授权。
- 删除旧通知插件的前后端依赖、注册及三个通用权限；五个专用命令均限定主窗口。发送只接受当前会话身份、事件类型、语言与失败布尔值，拒绝任意标题/正文。系统仅收到固定三语提示、应用标识及平台通知身份，不含问题、工具参数、原始错误、工作区路径或凭据；点击只导航，不审批、不回答、不提交任务。前端保留通知总开关、事件开关与声音偏好。
- token 到会话的映射在当前 core profile 内原子写入 `tauri-notification-targets.json`，Unix 权限 `0600`，与持久凭据档案身份匹配；最多 256 项 / 128 KiB / 7 天。重复 token、损坏、符号链接与外来档案拒绝读取且不覆盖原文件。提交结果不确定时保留映射，确认点击后才删除；持久写入失败保留队列供重试。
- 原生 delegate 在 WebView 就绪前安装并持有，点击队列限 32 项；前端先注册监听再读队列，忙碌/只读页面保留目标，恢复可用时再处理。host 依据当前权威目录及待删除清单解析工作区；缺失/待删除会话跳过且不创建，分页验证未完成则保留。复用现有会话切换流程，同会话只关闭面板并显示对话，卸载后不消费异步回复。
- 回归包括授权拒绝不重问、固定提示与隐私字段拒绝、发送不确定失败、档案重启与隔离、TTL/大小/损坏/权限/符号链接、确认失败回滚，以及真实 Go bridge 的权威目录解析与未知会话不创建。前端覆盖队列去重、忙碌推迟、目标不匹配、切换失败、卸载与确认失败；实际 React 工作区另测冷启动权威路径、已删除会话、同会话关闭设置、监听卸载且零审批/回答/提交。设置页另测只读查询、授权处理中防重复、拒绝状态与开关分别保存。
- 本轮五条新增三语提示使繁体中文 chunk 预算由 78.9 调整至 79.1 KiB，仅此文案上限增加 0.2 KiB；其他 JS/CSS 与简体预算保持原值。`pnpm test:tauri`、工作区通知集成测试、`pnpm test:tauri-build-contract`（36 项）、`pnpm build`、Rust 全量（161 项通过、1 项原生钥匙串测试默认忽略）与严格 clippy 通过；从干净提交 `cedc0c7d6d0a3925a5cd14fc1e7fbf4995dea310` 构建 `.app` 并通过严格本地 ad-hoc 签名校验，默认/显式档案的真实包级 smoke 均通过；只读通知查询返回有效原生授权状态，未申请权限、发送真实通知或执行 OS 点击。
- 真正的系统授权弹窗拒绝/重新启用、通知横幅/通知中心显示、前后台与冷启动的 OS 点击尚未执行；模拟回调和只读系统授权查询不替代这些验收。D 保持进行中，E 仍待 D 验收。

#### D：凭据档案隔离与显式迁移（2026-09-30）

- 新建 core profile 生成 128 位随机身份，保存在 `tauri-credential-profile.json`；原生服务名为 `com.reasonix.desktop.profile.<id>`。元数据仅含版本与身份，Unix 权限 `0600`，同目录无覆盖写入；并发首次创建采用同一胜出身份。可写档案创建 `.backup.json` 备份，备份创建失败不阻断已有有效主文件。主文件损坏时可读取有效备份，两个文件都损坏或两个有效身份冲突则禁用该档案凭据存储并提示恢复，不静默换身份。
- 重启、目录别名和移动保留身份；完整备份/复制包含两个身份文件时仍属于同一凭据档案。独立迁移实验须使用新目录并导入配置，不能把复制身份文件当作创建隔离档案。导入稳定版配置不会复制身份或自动读取稳定版 `reasonix` 钥匙串服务。
- 正常启动只读当前档案服务；旧 `com.reasonix.desktop` 服务和 app data 的 `keychain.dat` 不再自动迁移。设置页“迁移旧凭据”只传当前服务名给主窗口命令，由 host 验证该 Provider 已存在且需要密钥；优先读取旧 JSON 的同名 Provider，缺失时查询旧 Preview 原生服务。仅复制选中条目，保留旧原生服务和原文件，已有目标密钥则拒绝覆盖。回退旧 Preview 二进制时原凭据仍可用；新保存的密钥不会同步回旧服务。
- 旧文件限制 1 MiB / 256 项，拒绝符号链接、重复键、非法类型、控制字符及超长值；全文件验证后才写入选中项。原生写入可能已应用但响应失败时先恢复旧值，桥接响应失败时同时恢复原生存储并尽力对齐 sidecar 内存。恢复无法确认会报告重试/重启；不输出平台错误中的 credential debug 信息或密钥。命令限定 `main` 窗口并在 blocking worker 执行，迁移、保存、删除、bridge 重启共用凭据锁。
- 回归覆盖：不同档案/重启/移动/并发身份、元数据损坏和符号链接；迁移保留与拒绝覆盖、非法/缺失/不可用旧来源、原生不确定写入及删除、桥接失败回滚、迁移与手工写入并发、错误敏感信息过滤、旧版中文及含空格 Provider 名称兼容；前端防重复操作、仅 Provider 身份跨 IPC、未保存输入保留和状态刷新。
- 使用真实 Go bridge 的 Rust 全量 **154 项通过、1 项原生测试默认忽略**；额外显式运行 `native_keychain_profile_isolation_and_cleanup -- --ignored` **1 项通过**，仅使用随机临时命名空间和假密钥并清理条目。真实 bridge 验证导入/保存、重启恢复、删除不复活和文件无密钥。Go config/bridge 凭据定向测试、严格 clippy、`pnpm test:tauri`、`pnpm build` 通过，未放宽 bundle 门限。
- 原生钥匙串单元 smoke 不等同于授权弹窗拒绝、系统锁定或真实 WebView 迁移验收；这些状态及 Windows/Linux 后端继续保留待办。安装包 lifecycle smoke 另核对默认/显式档案的独立身份、私有权限和一致备份，不访问用户旧凭据。

#### D：共享外部链接入口（2026-09-30）

- 原 `bridge.openExternal` 只认识 Wails，Preview 中的 Markdown/Mermaid 会落到 `window.open`。现在保留原导出，转接共享平台 adapter：Tauri 优先调用 `open_external_link`，Wails 保留 `BrowserOpenURL`，普通浏览器保留独立 tab。原生拒绝不会走其他 transport 回退。
- Rust 对网页链接拒绝本地文件、任意应用 scheme、userinfo、控制字符和超长 URL；邮件链接仅允许收件人及 subject/body/cc/bcc，不允许附件参数、地址/标题头注入和 NUL。原有 OAuth `open_external_url` 仍仅接受 HTTP(S)，包括 loopback 回调 URL。
- Markdown 普通点击、中键和菜单使用同一路径；失败提示提供复制链接按钮，并复用现有本地化文案。Preview 入口补上共享 ToastProvider，使这些反馈实际可见。
- `pnpm test:external-links` 的 61 项断言覆盖 Wails、Tauri、浏览器及真实组件的点击/菜单行为，加入 `pnpm test:tauri`；完整前端构建通过且未调整体积门限。Rust 使用真实 bridge 的 129 项测试通过，其中 URL 校验覆盖合法邮件、中文网页、OAuth loopback 与禁止的 scheme/邮件字段。以上是组件与 host 校验测试，系统浏览器/邮件应用的真实打开操作仍需原生 UI 验收。
- 本切片的 `.app` 当时已构建并本地 ad-hoc 签名；同标识 release-candidate 实例曾阻塞独立 host smoke。后续应用发现及凭据隔离切片已在该实例退出后通过启动/退出 smoke，真实浏览器/邮件打开仍待验收。D/E 目标保持进行中。

#### D：macOS 应用发现、工作区打开与默认偏好（2026-09-30）

- `opener_catalog` 通过 [NSWorkspace](https://developer.apple.com/documentation/appkit/nsworkspace) 按固定 bundle ID 查询系统已注册的应用，支持改名与自定义安装位置；标准目录与 Spotlight 索引继续兜底。Spotlight 子进程限时 2 秒、结果限制 8 MiB 并回收进程，采用 NUL 分隔以正确保留含换行的路径。
- 使用 NSWorkspace 原生应用图标与 CoreGraphics 离屏渲染生成 64×64 PNG，避免全尺寸图标导致过大或丢失；图标输出不包含程序路径。展示目录缓存 15 秒；实际打开及保存偏好仍重新查询安装状态。
- Preview 项目会话顶部复用 Wails 的 `ExternalOpener` 选择器与并发偏好协调。工作区打开只传会话 ID 和安装应用 ID，由 Rust 获取 bridge 的 runtime 工作区查询（前轮使用 snapshot 的项目归属 root），拒绝会话身份不符、缺失目录和不可执行文档目标。普通 Markdown 默认打开仍遵循系统默认关联，指定应用打开可使用保存的偏好。
- `GET /v1/settings/desktop` 增加 `externalOpener`，`POST /v1/settings/desktop/external-opener` 使用已有鉴权、请求 ID 去重、共享配置锁及窄 TOML delta 写入。安装检测与原生进程启动留在 Rust；Go 仅保存稳定 ID。偏好写入失败不更新选择；卸载/复制自其他 OS 的 ID 在显示与默认打开时回退文件管理器，再回退首个应用，不重写原配置。
- 本机已验证 Finder/Terminal 真实目录与 64×64 PNG 图标、包含换行/中文的自定义索引路径、会话身份与目录保护。Go 定向测试验证未知配置项保留、坏输入/未鉴权不写入、去重冲突、写入失败保留原件及重启后读取；Rust 真实 sidecar 验证偏好往返保存及重启恢复。
- 前端原生 adapter/菜单/工作区选择器回归、共享链接 61 项、Wails 本地文档 20 项、共享应用选择器 32 项及跨会话偏好 4 项通过；`pnpm test:tauri`、完整 `pnpm build`、Wails 原生应用目录/图标/偏好定向测试、协议生成校验、Go vet、Rust 144 项与严格 clippy 通过。顶栏沿用固定高度且未修改 transcript viewport writer。
- 本切片 `.app` 构建及本地 ad-hoc 签名完成；此前另一份 Preview 已退出，安装包 smoke 已在默认托管与显式 profile 两种临时目录中独立通过：sidecar 就绪、继承环境清理、未鉴权 health 拒绝、正常退出与无残留。未操作用户原 profile，未将该启动测试当作应用菜单或系统程序的 UI 点击验收。
- 剩余：Linux 原生编译与桌面环境验收、Windows 原生 host 编译/注册表/图标/桌面环境验收；真实 WebView/系统应用交互，以及完整 D 现场验收。不能据此标记 D/E 完成。

#### D：本地文档打开、定位与另存为（2026-09-30）

- `app` Proxy 在回落 browser mock 前解析 Tauri 的五个文档 binding；Wails 的同名方法与调用形状保留。Preview 的 Markdown 点击/中键/菜单接入 `local_paths`，默认打开错误现在显示反馈。
- Rust 原生文档操作限定 `main` 窗口，路径必须绝对且存在；独立校验设备路径、Windows ADS/保留名称和 file URL 原始 authority。打开动作拒绝可执行后缀、Unix executable mode 及符号链接指向的可执行目标，定位/复制不执行目标。文件选择器的取消结果为空字符串，不显示成功。
- 另存为先持有源文件句柄，再由系统对话框选择目标；比较文件身份拒绝源文件、硬链接及符号链接别名，采用同目录临时文件、权限保留、sync 与原子替换。复制失败不截断原文件或原目标；未添加通用前端文件系统/任意命令权限。
- 引入官方 [opener 插件](https://v2.tauri.app/plugin/opener/)，替换外部链接的 deprecated shell open；关闭插件自动拦截 JS 链接，所有打开仍通过 host 的校验命令。shell 插件保留供 sidecar supervisor 使用，未开放其前端执行权限。
- 应用选择仅传 native catalog 的 ID，宿主重新检测应用是否存在；不接收 renderer 提供的程序路径或命令参数。macOS 标准目录中的常见编辑器/终端已检测，Ghostty 使用专门 working-directory 参数；完整 Wails 应用发现、图标、跨平台终端及偏好持久化仍待迁移，不能将这部分视为全部完成。
- 验证：Wails 本地文件打开/路径/复制定向 Go 测试、共享链接回归（61 项）、Wails Markdown 点击/菜单回归（20 项）、新增 Preview 原生文档菜单/adapter 回归、`pnpm test:tauri` 和完整前端构建通过；Rust 使用真实 bridge 的 139 项测试与严格 clippy 通过。本机无权限文件测试实际走拒绝分支。组件与文件操作测试未执行系统对话框、Finder 或编辑器的真实 UI 点击，仍保留安装包现场验收。
- 本切片 `.app` 已重新构建并完成本地 ad-hoc 签名。另一份同标识 release-candidate Preview 仍在运行，独立启动/退出 smoke 暂未执行；未退出或操作该运行中的实例。该构建结果不等于系统对话框与指定应用的现场验收，D/E 目标保持进行中。

#### D：配置/状态目录互斥与回退验证（2026-09-30）

- `profilegate.TryAcquireDesktop` 同时保护配置 home 和独立 state home；canonical/文件身份去重避免符号链接及不区分大小写卷上的重复取锁。竞争失败会释放先前取到的目录锁，正常退出与进程崩溃沿用 OS 锁释放机制。
- bridge 在身份库迁移、listener 和后台运行时启动前取锁；Wails 在诊断与主运行时初始化前取锁，持有至 `wails.Run` 返回。冲突启动移除绑定及 startup/shutdown 写入回调，保留原生 single-instance handoff 并在 DOM ready 退出。
- 使用本次构建的真实 bridge，验证 Wails 目录所有者能拒绝“共享配置、不同状态”的第二个进程，且未发布 readiness、未修改配置。使用本次构建的原生 Wails，验证目录被占用时自动退出，原配置不变，未初始化 sessions。
- Rust 导入回归覆盖 Preview 配置被修改后，正式版原件与时间戳备份仍保持原内容。回退方法是在退出 Preview 后重新打开 Wails 原目录；Preview 中的新数据不会自动回写，配置备份不是完整会话备份。
- 门禁：Go profilegate/bridge 启动测试、Wails profile/single-instance 测试（提供 `REASONIX_TAURI_BRIDGE_TEST_BIN` 与 `REASONIX_WAILS_PROFILE_TEST_BIN`），前端构建及 `pnpm test:tauri`，使用真实 bridge 的 Rust 测试（128 项通过）。旧版兼容与真实 UI 待验项继续保留，D/E 目标未完成。
- 当前改动的 `.app` 已重新构建并本地签名，打包 bridge 也已通过上述真实互斥测试；本切片当时因另一份 release-candidate Preview 正在运行而触发单实例转交、在 readiness 前正常退出，未通过该次 host smoke。脚本已在启动前检查同 bundle identifier 的其他 `.app` 副本；后续切片在该实例退出后重跑通过，结果见上方累计门禁。

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
