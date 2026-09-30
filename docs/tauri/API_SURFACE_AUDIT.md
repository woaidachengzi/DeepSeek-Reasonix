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
| D：对话框与链接 | 现有 Tauri 选择器保留；共享外部链接及本地文档 adapter 已接入 Rust host。Markdown 默认打开、定位、另存为及指定已安装应用均走原生入口，错误不退回 browser mock。文档可执行目标拒绝、特殊路径、取消保存、源文件别名保护及权限拒绝已有回归。外部链接支持 HTTP(S)/受限 mailto，OAuth 入口仅接受 HTTP(S)。 | macOS 已增加系统应用注册查询、Spotlight 自定义安装位置和原生 64×64 图标；项目会话顶部选择器已接入配置偏好与卸载回退。Windows/Linux 仍只检测 PATH 编辑器，尚需补齐 App Paths、desktop entry、终端启动策略和原生图标；无显式工作区的 Global 会话打开能力仍待接入。系统对话框、浏览器/邮件、指定应用的真实 UI 交互及 OAuth 仍需验收。 |
| D：通知与钥匙串 | macOS 通知改为原生 UserNotifications：读取实际授权、报告发送失败、点击恢复对应会话；冷启动队列、档案隔离、失效会话与重复点击已有回归。凭据按持久档案身份隔离；设置页显式迁移旧 Preview 凭据，保留原件并拒绝覆盖。迁移/保存/删除与重启串行，写入及桥接同步失败回滚；macOS 原生隔离读写、真实 bridge 迁移/重启/删除及不落盘回归通过。 | 真实系统通知授权拒绝、通知显示与前后台/冷启动点击、钥匙串锁定/授权拒绝、原生设置页迁移操作仍待验收。Windows/Linux 通知授权读取和点击回调、凭据后端仍待补齐/验收。 |
| D：单实例与数据保护 | Tauri 单实例及独立默认 Preview 数据目录已存在。当前 Wails 与 bridge 启动均持有配置/状态两处目录锁；共享任一目录都会拒绝第二个写入宿主，目录别名去重，失败释放已取锁。真实 bridge/Wails 拒绝启动测试及配置原件/备份回退回归通过。导入页在操作前展示来源、目标目录与回退说明。 | 未参与目录锁协议的旧稳定版仍需兼容性验收；不能将当前两个宿主的测试推广为所有历史二进制互斥。真实 Wails 单实例通知/唤起、完整安装包导入与回退操作仍待验收。 |
| E：remote host / bot / updater / 管理页 | remote host 与 bot 已有部分设置/bridge 接口；updater 插件已注册。 | D 验收后对照 Wails 逐项审计和补齐；特别是当前“检查更新”仍是说明对话框，插件注册不能视为更新流程完成。 |

累计门禁：`pnpm test:clipboard`、输入框剪贴板回归、terminal selection、`pnpm test:tauri`、`pnpm build`，以及使用真实 Go bridge 的 Rust 测试（最新通知切片 161 项通过，另有前轮 1 项显式 macOS 原生钥匙串测试通过）。此前凭据隔离切片已通过 `pnpm tauri:build -- --bundles app` 构建与本地 ad-hoc 签名；`tools/tauri/smoke-packaged-app.py` 在临时 HOME 分别验证默认和显式数据目录、sidecar 就绪、未认证请求拒绝以及退出无残留。此 smoke 没有执行菜单/托盘等 UI 点击；两次桌面自动化分别超时和报 ScreenCaptureKit `SCStreamErrorDomain -3811`，所以真实 UI 验收保留待办。本地 ad-hoc 签名不是正式发布签名/公证。

剪贴板插件接入与文本权限参考 [Tauri 官方文档](https://v2.tauri.app/plugin/clipboard/)。

#### D：原生通知授权、发送与点击定位（2026-09-30）

- 对照 Wails `internal/notify` 的固定事件提示，macOS 改用 [Apple UserNotifications](https://developer.apple.com/documentation/usernotifications/unusernotificationcenter) 读取真实授权状态并等待提交结果；仅 `not_determined` 请求授权，拒绝后不重复弹窗。设置页分别显示用户开关与系统授权；错误使用固定解决提示，不输出系统诊断。未打包开发进程报告不可用，不借用 Terminal 身份。Windows/Linux 使用 `notify-rust` 同步提交并返回错误，授权暂为 `unknown`、点击能力为 false，不能据此声称已获系统授权。
- 删除旧通知插件的前后端依赖、注册及三个通用权限；五个专用命令均限定主窗口。发送只接受当前会话身份、事件类型、语言与失败布尔值，拒绝任意标题/正文。系统仅收到固定三语提示和随机 token，不含问题、工具参数、原始错误、工作区路径或凭据；点击只导航，不审批、不回答、不提交任务。前端保留通知总开关、事件开关与声音偏好。
- token 到会话的映射在当前 core profile 内原子写入 `tauri-notification-targets.json`，Unix 权限 `0600`，与持久凭据档案身份匹配；最多 256 项 / 128 KiB / 7 天。重复 token、损坏、符号链接与外来档案拒绝读取且不覆盖原文件。提交结果不确定时保留映射，确认点击后才删除；持久写入失败保留队列供重试。
- 原生 delegate 在 WebView 就绪前安装并持有，点击队列限 32 项；前端先注册监听再读队列，忙碌/只读页面保留目标，恢复可用时再处理。host 依据当前权威目录及待删除清单解析工作区；缺失/待删除会话跳过且不创建，分页验证未完成则保留。复用现有会话切换流程，同会话只关闭面板并显示对话，卸载后不消费异步回复。
- 回归包括授权拒绝不重问、固定提示与隐私字段拒绝、发送不确定失败、档案重启与隔离、TTL/大小/损坏/权限/符号链接、确认失败回滚，以及真实 Go bridge 的权威目录解析与未知会话不创建。前端覆盖队列去重、忙碌推迟、目标不匹配、切换失败、卸载与确认失败；实际 React 工作区另测冷启动权威路径、已删除会话、同会话关闭设置、监听卸载且零审批/回答/提交。设置页另测只读查询、授权处理中防重复、拒绝状态与开关分别保存。
- 本轮五条新增三语提示使繁体中文 chunk 预算由 78.9 调整至 79.1 KiB，仅此文案上限增加 0.2 KiB；其他 JS/CSS 与简体预算保持原值。`pnpm test:tauri`、工作区通知集成测试、`pnpm test:tauri-build-contract`（36 项）、`pnpm build`、Rust 全量（161 项通过、1 项原生钥匙串测试默认忽略）与严格 clippy 通过；安装包验证证据将在构建完成后记录。
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
- Preview 项目会话顶部复用 Wails 的 `ExternalOpener` 选择器与并发偏好协调。工作区打开只传会话 ID 和安装应用 ID，由 Rust 获取 bridge snapshot 的权威 root，拒绝会话身份不符、缺失目录和不可执行文档目标。普通 Markdown 默认打开仍遵循系统默认关联，指定应用打开可使用保存的偏好。
- `GET /v1/settings/desktop` 增加 `externalOpener`，`POST /v1/settings/desktop/external-opener` 使用已有鉴权、请求 ID 去重、共享配置锁及窄 TOML delta 写入。安装检测与原生进程启动留在 Rust；Go 仅保存稳定 ID。偏好写入失败不更新选择；卸载/复制自其他 OS 的 ID 在显示与默认打开时回退文件管理器，再回退首个应用，不重写原配置。
- 本机已验证 Finder/Terminal 真实目录与 64×64 PNG 图标、包含换行/中文的自定义索引路径、会话身份与目录保护。Go 定向测试验证未知配置项保留、坏输入/未鉴权不写入、去重冲突、写入失败保留原件及重启后读取；Rust 真实 sidecar 验证偏好往返保存及重启恢复。
- 前端原生 adapter/菜单/工作区选择器回归、共享链接 61 项、Wails 本地文档 20 项、共享应用选择器 32 项及跨会话偏好 4 项通过；`pnpm test:tauri`、完整 `pnpm build`、Wails 原生应用目录/图标/偏好定向测试、协议生成校验、Go vet、Rust 144 项与严格 clippy 通过。顶栏沿用固定高度且未修改 transcript viewport writer。
- 本切片 `.app` 构建及本地 ad-hoc 签名完成；此前另一份 Preview 已退出，安装包 smoke 已在默认托管与显式 profile 两种临时目录中独立通过：sidecar 就绪、继承环境清理、未鉴权 health 拒绝、正常退出与无残留。未操作用户原 profile，未将该启动测试当作应用菜单或系统程序的 UI 点击验收。
- 剩余：Windows/Linux 完整应用发现、原生图标与终端策略；没有显式工作区 root 的 Global 会话能力；真实 WebView/系统应用交互，以及完整 D 现场验收。不能据此标记 D/E 完成。

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
