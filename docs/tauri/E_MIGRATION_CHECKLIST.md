# E 迁移清单

## 2026-10-08 原生远程订阅 review 提交

本批 review 提交原生事件投影、订阅/取消 IPC、包内 sidecar 退出撤销及 renderer 消费 helper。Review 修复异步 unlisten 的未处理拒绝、测试类型兼容及既存历史文案的构建预算漏计，并将 renderer/native adapter 回归接入远程检查。Rust 280 项、clippy、Go race、协议生成检查、前端远程回归/测试类型/生产构建通过，详情见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生远程订阅-review-提交)。尚未挂接实时 UI，也未重建 App；实际 WebView、窗口重建、统一归约和可写入口的门禁保持未完成。

## 2026-10-08 包内 sidecar 终止 observer（未提交增量）

明确 child 终止 receipt 已主动撤销对应 remote subscription owner；晚绑定立即撤销、旧 owner 通知不影响新 owner，取消关闭实际 TCP 读取。管道故障不作为进程退出证据；显式 developer binary 仍未接同等 observer。源码回归与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-bundled-sidecar-终止订阅撤销未提交增量)，尚未重建 App 或关闭窗口重建/live UI 门禁。

## 2026-10-08 原生事件载荷与订阅 IPC（未提交增量）

已接入 Go-derived 32-kind 载荷 contract、native typed projection 和主窗口订阅/取消命令；返回身份与 opening/ready/ended 分开，替换/取消后的旧 worker 不发布迟到结果。完整 Rust 277 passed / 0 failed / 6 ignored、clippy 与构建契约通过，范围与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生事件载荷投影与订阅命令未提交增量)。尚无此次实际 WebView IPC、新 App 或 live UI 验收；sidecar 意外退出即时撤销、窗口重建、快照/事件统一归约、恢复/发送/审批/模型归属和 bot 等门禁保持未完成。变更超过 30 个文件时先 review、验证，再提交；当前阶段不提前宣称完成或覆盖旧包。

## 2026-10-08 超过 30 文件的 review 提交

本批 review 提交共享原生历史图片验收修复、指定会话 SSE bridge/native transport 与 pending-open 取消、订阅 registry 核心。registry 防止旧 generation/worker 清理新订阅，限制 current/live/tombstone 数量；review 补 main Destroyed 之后的 admission 撤销、Supervisor stop 与 start 的 owner 锁归属、tombstone 上限准确恢复提示。四项 registry 回归、Go 五包 race/vet、完整前端 Tauri/测试类型、Python receipt 与协议镜像通过，详细范围见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-订阅注册表与超过-30-文件-review-提交)。订阅 IPC/载荷检查/实际 WebView 实时面仍未接入，本批源码提交不代表新 App 打包或整个 E 完成；已交付 App 不覆盖、不推送。下方未提交记录是各增量阶段当时状态。

## 2026-10-08 共享原生历史图片与会话事件传输（后续未提交源码）

- 实际 release App 的 managed/explicit 两种隔离档案已通过共享历史图片、可见 ImageViewer、Escape 保留底层历史、关闭清理和无未捕获异常门禁。修复 SSH acronym wire、Serve 首帧 admission、Rust 缺省字段省略、预览 Escape 冒泡。图片阶段证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-共享历史的真实原生图片验收0ebda4fd9-后续未提交源码)；已交付 `0ebda4fd9` App 未覆盖。
- 新增 shared controller `SessionEvents`，按明确 catalogue member 路径接收后台/前台会话事件，使用原 SSH owner/operation 的取消与 cookie transport；不把未标记或其他会话帧归给当前 surface。帧/行/UTF-8/序号/关联 ID 有界，未知字段通过 typed event 投影剔除；取消和关闭拒绝已缓冲结果，EOF 要求显式重开并补快照，不自动重试、接管或授予写入。
- shared client 五项顶层回归及生产 Serve token gate/真实 Controller/会话文件/broadcaster 路由用例通过；完整 controller/Serve race 和 vet 通过，日志 `/private/tmp/reasonix-native-history-ui.POmxwA/events-complete-race.log` / `events-vet.log`。生产 broadcaster 事件为自有 fixture，不声称已执行真实模型 turn、SSH/SSE 原生界面或审批恢复。
- 后续增加受监督 handle 的窄 `session-events` bridge route，实际自有 SSH tunnel + cookie HTTP + bridge HTTP 回归通过；验证路径过滤/typed projection、撤销清理、字段/鉴权/重放拒绝、每 handle 两订阅和取消释放，另用故障 writer 验证 request 尚未取消时的首写/事件输出失败清理。完整 bridge/controller/protocolgen race、vet、DTO/schema、镜像、前端类型和 Rust 编译检查通过。输出有写入预算，不进入本地 EventLedger；schema event 仍是 opaque，原生 payload 检查和 subscription generation fence 尚待接入，不能算实际包实时对话验收。证据 `events-bridge-final-race.log` / `events-bridge-complete-race.log`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程会话-sse-bridge-路由未提交增量)。
- 原生内部 transport 核心已接固定 HTTP/1.0 SSE、catalogue membership、controller/path fence、有界 header/frame、socket shutdown 取消和 Drop 清理，五项 transport/完整 remote-controller 15 项通过；实际 bridge HTTP/1.0 framing + socket 关闭清理 race 通过。尚未注册 command 或向 renderer emit opaque event；pending-open 取消、subscription registry、payload/capability 检查、generation/window/sidecar fence 仍待实现。不能把 TCP fixture 当作 WKWebView/App 联调。证据 `events-native-controller-final.log` / `events-native-http10.log`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生指定会话事件传输核心未提交增量)。
- 后续补 pending-open cancellation：同一 operation 的 socket lease 覆盖 catalogue 响应头/JSON body 和 SSE 响应头，取消主动 shutdown 而非等 I/O timeout；预先取消零 TCP dispatch，失败打开释放 lease，迟到打开不返回。两项顶层回归与完整 Rust 266 passed / 0 failed / 6 ignored 通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生会话事件-pending-open-取消未提交增量)。TCP connect 仍有 1 秒预算；subscription registry、载荷检查和窗口/sidecar/generation fence 尚待接入，不算 actual App 订阅验收。
- 下一步接原生订阅与 generation/surface fence，再接快照/实时事件统一归约、显式恢复/发送/审批/模型归属和重连。现有只读历史 handle 不能隐式升级为可写会话。系统剪贴板、bot Desktop、三平台专项、复杂管理页与 SQLite 等其余目标保持未完成；本批没有新的 App 重建或提交/推送。

## 2026-10-08 远程图片正向原生验收与 review 提交

真实 release App 的 managed/explicit 两种私有档案均通过 WKWebView → 注册 IPC → 包内 bridge → 临时密钥认证 SSH/SFTP → 生产 shell bootstrap → 实际 CLI Serve → saved workspace PNG 解码。验收没有 mock 图片响应或 controller factory，没有真实账号、模型请求、系统剪贴板操作、下载或远端安装；会话及 workspace 元数据保持逐字节不变。review 补正向收据严格布尔校验、fixture 输入/XOR 回归和 Serve 进程身份/清理确认；只有清理成功才生成最终收据。证据 `/private/tmp/reasonix-remote-image-positive.78LOTy/`、`/private/tmp/reasonix-native-ssh-2976458357/integration-receipt.json`。本批 review 后提交，再从干净 HEAD 重建 arm64 ad-hoc `.app`，以最终包复验记录为准。共享 Transcript 的原生完整界面、剪贴板粘贴发送、真实远端平台/代理/安装及完整 controller/bot 等门禁仍保留；原生 standalone 图片节点验收不能代替共享对话界面验收。下方“未提交/未执行”是各阶段当时状态。

## 2026-10-08 原生远程图片边界实际 App 验收（后续源码）

独立 release App 在 managed/explicit 两种临时档案中通过真实 WKWebView 图片 IPC 注册与 unknown field/空 source/未知 handle 拒绝、正常退出及无 sidecar/readiness 残留；没有操作系统剪贴板或真实账号。新增正向 PNG 探针仍需真实自有 SSH/Serve fixture，尚未执行，不把拒绝检查算作远程图片成功验收。首次 debug origin 拒绝保留失败证据；构建脚本已遵循 Cargo 独立 target 输出并验证实际候选签名，73 项构建契约及 Rust 257 passed/6 ignored、clippy、receipt verifier 通过。证据 `/private/tmp/reasonix-native-image-release.eUtbs7/`、`/private/tmp/reasonix-native-remote-image-boundary-bkdb2gjw/`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生远程图片边界探针与独立-release-候选后续源码)。本增量未提交，候选在独立临时目录；已交付 `eefdb4e55` App 未覆盖。完整目标继续保持未完成。

## 2026-10-08 远程图片预览与渲染证据（后续源码）

补远程历史图片的 shared ImageViewer 与 scoped resolver 生命周期，保持只读权限、不打开本机文件。新增 effect/旧请求/ABA/预览/清理回归，以及真实共享 Transcript 浏览器 desktop/narrow 图片解码、预览、reader refresh 与迟到图片验证，完整 Tauri/Transcript、类型及构建门禁通过。日志和截图在 `/private/tmp/reasonix-remote-image-ui.TknC1t/`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程图片共享预览与浏览器验收后续源码)。native invoke 使用模拟 adapter，不能当成原生 App/SSH/Serve 联调。当前交付 App 仍为 `eefdb4e55`；本增量未提交/未打包。下一步继续实际隔离原生链路，系统剪贴板 runner 未获授权，未执行；其余完整目标保持未完成。

## 2026-10-08 远程图片 IPC/UI review 与 macOS App

本轮接入 main-only 图片 IPC、remote lease 与共享历史图片 resolver，保持 backend workspace、只读会话归属和迟到结果 fence，不提供本机 opener/文件回退。已构建 arm64 ad-hoc 可运行 App，签名及实际 managed/explicit 临时档案启动和退出通过；review 补图片 scope、释放/迟到结果及私有字段回归。日志与上版 App 备份在 `/private/tmp/reasonix-package-review.QZYtvf/`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程图片-nativeui-review-与可运行-app)。尚无新增图片 command 的实际 WKWebView/SSH/Serve 联调，不能把启动 smoke 当作该功能验收。真实剪贴板、完整 controller/bot Desktop、跨平台、管理页和 SQLite 门禁保持未完成；不发布、不替换正式应用。

2026-10-06 用户要求直接推进 E，明确不需要 updater。本轮不再以 D 全部验收为 E 的启动条件；D、A/B/C 未关闭项仍保留为正式发布门禁。正式发布、替换正式应用、切换默认下载均未授权。updater 从本轮范围排除。

2026-10-08 最新可运行候选与 review 记录见本文末尾及 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程历史增量-review-与-macos-可运行候选)。下列“未提交/未打包”保留各阶段当时状态，不覆盖后续交付记录。

| 功能面 | Wails 基线 | Tauri 起点 | 当前工作及验收 |
| --- | --- | --- | --- |
| remote 配置与认证 | desktop/remote_hosts.go、remote_prefs.go；SSH config、显式主机指纹、临时/持久凭据 | 读取/修改/删除、SSH alias 扫描、指纹确认和密码恢复已有 | 本批补连接请求归属、断开/改配置/退出取消、重开设置时的权威连接状态；源码 Go race / UI 回归、新包本机 SSH 信任/持久化/退出验收已通过 |
| remote 文件 | remote_listing.go；浏览、读取/写入及路径操作 | SFTP 浏览、文本预览、revision 保存已有 | 本批补目录/文件异步请求归属、取消/切换后旧结果丢弃；新包真实 SFTP 浏览/预览/revision 保存及旧版本冲突通过。本轮接入 mkdir、同目录 rename、删除（目录需显式递归确认），侧车拒绝根目录/登录主目录删除；Go race、Tauri 组件回归通过。此轮未重建 `.app`，不能把源码回归视为包内证据。 |
| remote tunnel / Serve | remote_serve.go、remote_server_stop.go；forward、bootstrap、状态/日志 | Serve 生命周期与 controller 浏览器入口已接；Tauri 内嵌 remote tab 未接 | 设置页可按已认证 SSH 会话读取状态、启动/复用、停止及读取最多 500 行、256 KiB 日志；可由原生宿主将fragment 登录 controller URL 直接交给系统浏览器，renderer 不接收 token/URL。tunnel 固定 `127.0.0.1:0`，断开 SSH 会随 client forward 一并关闭。源码 Go/UI/Rust 回归通过，但未用真实远端 Serve 或重建 `.app` 验收。自动启动可复用现有兼容 CLI，或按策略在远端 npm 安装；Preview 未携带可上传 CLI/发布下载回退，`local-proxy` 凭据显式拒绝。配置转发自动应用与 Tauri 原生 remote tab 后续批次，不把 SSH/SFTP 验证算作 Serve/controller 验收。 |
| remote 对话与工作区 | remote_tab*.go、remote_projects.go；恢复、事件/审批、模型归属、断线重连 | 可启动 remote controller 浏览器页；本地单会话 bridge 尚未完整对接远端 controller | 系统浏览器 controller 使用 Serve 的现有页面/会话能力；Tauri 内的会话恢复、事件/审批、模型归属与断线重连仍未迁移，不能借用本地会话或放宽模型/写入权限代替。 |
| bot 配置和账号 | bot_connection_app.go、bot_runtime_app.go；多账号、凭据、启停、状态、诊断 | 旧渠道配置/凭据、路由/队列/权限、适配器运行已有 | 本批补手动多账号创建/删除、受保护凭据、路由/订阅清理、运行时重启和配置应用状态；源码 Go race / UI、新包默认关闭/私有凭据/重启持久化/旧包读取回退通过；未连接真实 IM |
| bot 配对和管理 | internal/bot/pairing.go、desktop/bot_bridge*.go | 可开启配对，但没有待申请审批入口；Desktop=nil | 本批补本档案申请列表/批准/拒绝、失效连接拒绝、配置写失败保护；源码失败恢复及新包鉴权/连接内授权通过；桌面对话订阅/远程审批/接管仍待迁移 |
| bot 扫码安装与诊断 | Start/PollBotConnectionInstall、Diagnose/TestBotConnection | 尚无完整 Preview 安装/诊断流程 | 后续批次；真实 IM 平台连接须在可用账号环境验收 |
| 复杂管理页 | permissions/secrets/sandbox/network、skills/plugins/MCP、hooks/memory/subagents、data/diagnostics | 多数已有独立 Tauri 页面和 bridge | 本批修复 bot 管理轮询覆盖未保存路由草稿；其余页面逐项审计配置/异步归属/失败恢复/权限边界，不据页面存在判定完成 |
| updater | Wails 自动更新 | Preview 说明/手动下载 | 用户明确排除，本轮不迁移 |

验收分开记录：源码回归、真实组件浏览器、实际 `.app` 包内进程与本机协议服务、外部平台/原生窗口用户体验。旧包通过不自动升级为新包证据。测试不得使用真实凭据、发送测试消息到用户联系人或改动正式版数据。

复杂管理页首轮对照（Wails worktree `/Users/jerry/temp/app/reasonix-wails-1.38.3`，当前 HEAD `2185b8e8ff8abb2166bf8e55df0694d05a0f9d76`；本地基线包含既有补丁，不冒充官方归档身份）：

| 页面 | Wails 对照 | Tauri 当前实现与本次回归 | 仍需验收 |
| --- | --- | --- | --- |
| 权限/敏感信息 | app.go 的配置读写与权限规则 | 独立 permission/secrets bridge，项目覆盖与全局规则保持；补全局→项目读取乱序保护；Go 持久化、规则隔离与 UI 迟到响应回归 | 原生窗口实际工具执行与拒绝/恢复 |
| sandbox/network | app.go 配置、网络代理与 sandbox 策略 | 独立桥接；Go 配置保存、凭据脱敏和其他字段保护已回归 | 真实平台策略/代理切换后的工具与子进程行为 |
| skills/plugins | app.go SkillsSettings、plugin_packages_app.go | 来源、安装计划、revision、启停、归档/恢复、诊断；Go 计划/安装/拒绝风险缺失/项目隔离回归，已有 UI 回归 | 实际包中第三方安装、取消/重启/失败回退；无真实第三方执行证据 |
| MCP | app.go MCPServers/配置/认证与 mcp_* | 配置、运行时、认证、目录与设置 UI 回归已有；本次工作区/会话切换重新挂载管理页 | 实际打包子进程退出、OAuth/超时/异常重启与权限完整矩阵 |
| hooks | hooks_settings_app.go | 项目信任与配置范围、插件 hooks；已有请求归属保护；本次保持工作区/会话隔离 | 实际工具触发、取消和 hook 子进程权限 |
| memory | app.go Memory、memory_suggestions.go | 文档/fact/revision/建议、工作区边界；当前会话应用须空闲；本次避免工作区切换沿用旧页面状态 | 原生编辑、建议接受与真实模型召回 |
| subagents | subagents_app.go | profile CRUD/revision、试运行/取消及模型默认；本次页面归属跟随工作区/会话 | 真实模型试运行、取消/子进程退出/失败恢复 |
| data/diagnostics | app.go 档案与诊断 | 档案隔离、导入审阅与只读诊断；storage bridge 有有效目录回归 | 不继承旧包备份/导入证据，当前候选完整矩阵仍待验 |

本批源码提交 `7e6645bdc8f9011a4cf289d6885e998503e65651` 已构建直接可运行 `.app`，包内 sidecar 在临时档案与本机 SSH/SFTP 服务通过集成验收，旧包读取新配置并回到新包通过。原生宿主在屏幕 2 内侧通过两种档案的退出清理和只读通知权限查询；这不替代 D 的完整原生窗口矩阵。证据见 [本批验收记录](evidence/2026-10-06-e-lifecycle/README.md) 和 [包收据](evidence/2026-10-06-e-lifecycle/package-receipt.json)。

不将表中“已有”能力计作整个 E 已完成。下一批优先 Tauri 原生 remote tab/controller 与 bot Desktop 桥接；配置转发自动应用、扫码安装、完整管理页实际包矩阵仍保持未完成。仅 updater 属于用户明确排除。

## 2026-10-08 远程 controller 共用 HTTP 层（未提交源码增量）

- 将 Wails 的回环 Serve client / token handshake 提取到 `internal/remote/controller`；`desktop/remote_listing.go` 通过薄 adapter 使用它。共用层不依赖 Wails/CGO，不把 renderer URL、代理或本地模型配置用作远程 controller 归属。
- 仅接受显式端口的数字 loopback HTTP origin；拒绝 DNS 主机、userinfo、base query/fragment/path。独立 transport 禁用环境代理和自动重定向，并在每次请求前检查原始 IP/端口与 Host。Cookie jar 不隔离端口，因此跨端口请求必须在发送前拒绝，不能只依赖 cookie domain 或重定向保护。
- token 只在 `/auth/token` 的受限 JSON body 中交换，后续 API/SSE 走后端 cookie jar。握手有 10 秒取消预算，响应只丢弃最多 4 KiB，错误不返回响应正文、token 或 URL；JSON 转义后也符合 Serve 的 8 KiB 入站预算。SSE 没有 whole-client Timeout，stream body 由 owner context 取消。
- 共用层 7 项顶层回归覆盖地址拒绝、token/body→cookie API/SSE、环境代理关闭、307 和直接跨端口泄漏拒绝、Host 伪装、错误字节预算/脱敏、握手及 SSE 取消、无效 token/client 的零请求。另用 `internal/serve` 的生产 token middleware 验证未认证 401、错误 token 拒绝及认证后的 sessions/status/history/events；这些 API 响应是 fixture，不冒充实际 controller 生命周期。
- root 定向 race 与 `go vet` 通过，Wails `TestServe|TestRemote|Test.*CredentialProxy` race 回归通过（真实主机 smoke 开关显式禁用）。测试只访问自有回环服务，没有连接真实 SSH、模型或 IM 平台。证据保存在 `/private/tmp/reasonix-controller-transport.CywPLr/`。
- 此批未增加 Tauri remote tab/HTTP endpoint 或 bot Desktop 桥接，也未重建 App。下一步仍需 backend-owned host/workspace/connection generation、会话/模型归属、事件与审批恢复、断线重连和真正包级验证；共用 transport 通过不关闭 E 或图片原生粘贴门禁。

## 2026-10-08 远程 controller 连接归属与只读清单（后续未提交增量）

- 在上述共用 transport 基础上增加 bridge 内的 attach/list/close 接口，health 仅新增 `remote_controller_sessions_v1`，不声明远程对话完整可用。入站字段只有 saved host 与 workspace；地址与 token 来自后端 EnsureServe 和已认证 SSH tunnel，renderer 只得到随机 128-bit handle、主机/工作区及 `readOnly: true`。
- controller 绑定受监督 SSH client **及其当前底层 SSH 连接**，不能仅凭同一个 client 指针跨自动重连复用。附着 HTTP request 的结束不结束连接；断开、主机配置变更、替换连接、关闭 controller、sidecar 关闭会撤销 owner 并取消认证/读取。断线回调只取消 owner，迟到附着/清单再次检查当前归属，不能复活旧 handle。
- 同 host/workspace 的健康连接复用 handle，不同工作区隔离；已发布与正在附着的总数限制 16。附着预算 45 秒、等候 Serve 锁可取消；清单整个读取预算 15 秒、body 至多 8 MiB、条目至多 10,000。固定 DTO 丢弃未知字段，拒绝非法结构、重复路径、多 current、控制字符与非 JS-safe 计数；保留合法多行备用标题。401/403 cookie 失效废弃旧连接，后续需重新认证。
- 成功停止 Serve 后按请求或解析后的 workspace 撤销已发布 handle；同主机尚未完成解析的附着保守取消，防止路径别名导致停服后迟到发布，其他已发布 workspace 不受影响。停服完成还须通过当前 SSH 归属检查，不能撤销替换连接的新 controller。
- 新增 shared client 4 项、bridge controller 7 项顶层回归，使用自有 loopback SSH（真实 direct-tcpip）和 HTTP cookie 服务，覆盖复用、工作区隔离、鉴权/字段拒绝、上限、关闭/退出、断线取消、同 client 自动重连、迟到附着与可取消锁。另读取生产 Serve `/sessions`（真实 `control.Controller` + 临时 transcript + token gate）验证 current、计数、多行备用标题且不切换控制器；不再只有伪造 catalogue body 的兼容证据。未进行远端安装或模型调用。
- 最终定向 Go race、Go vet、Go DTO/schema、协议镜像一致性、前端 typecheck 和 Rust `cargo check --locked` 通过。证据在 `/private/tmp/reasonix-controller-ownership.aFopli/`，最终 race 为 `ownership-fenced-final.log`，静态检查为 `ownership-final-vet.log`。早期 fixture 失败和一次自动权限审核超时保留，不列作通过；修复夹具未读取 POST body、ID 格式后回归通过。
- 此批仍没有原生 IPC 命令、Tauri remote tab、远程历史/流/发送/审批/模型配置归属或 bot Desktop 联动；未重建 App、未提交/推送，也未操作系统剪贴板。清单中的 remote session path 是展示数据，不是本地 RuntimeManager、工作区或文件授权。下一步接原生窄 IPC 与只读 UI，再逐项迁移 controller 读写/事件与审批；E、真实 Serve 和包级门禁保持未关闭。

## 2026-10-08 只读 controller 原生 IPC 与设置页清单（后续未提交增量）

- 上述后端快照之后，接入 main-window-only 的 attach/sessions/close 三个 typed command。只有 saved host/workspace 或 canonical opaque handle 可入站；固定 HTTP 路由、后台 worker、严格字段/版本/响应预算及错误脱敏，不向 renderer 提供 Serve 地址/token，不持有 supervisor lock 等待网络。
- 设置页新增显式“查看远程会话”，懒加载、每页 50 条、显示当前/运行/接管状态与多行标题；目录仅展示，不链接到本地文件或本地 RuntimeManager，没有发送、恢复或模型/审批操作。React effect 依赖原始 host/workspace，切换后旧响应/错误/finally 均丢弃。
- 共享 lease 处理多个挂载与 StrictMode：最后消费者释放才关闭，同 scope 新附着等待旧附着与关闭完成；旧 finally 不删除新 owner。关闭投递不确定时保留 barrier，直到显式 SSH 操作已撤销后端 owner；未改变、仍保持 connected 的配置保存不清空池。
- Rust 新增 3 项 typed HTTP fixture 单测和 clippy 通过；pool/绑定/组件、完整 Tauri 前端回归、生产/测试类型检查、build 的现有门限通过。Browser plugin 不可用，使用已有 Playwright + 自有 headless Chrome，桌面/390px 窄屏的非空/overlay/console/分页/关闭重开/错误隐私/长路径布局通过；原生调用是 mock，不能算 `.app` 或真实远端验收。详细日志与截图见 [API audit](API_SURFACE_AUDIT.md#后续源码窄原生-ipc-与只读列表)。
- 未重建当前阶段的 App，未提交或推送，未操作真实 profile/SSH/模型/IM/系统剪贴板。下一步继续 controller 读写/事件与审批/模型归属及隔离包内验证；只读列表不关闭 remote tab、bot Desktop、图片原生粘贴或整个 E 的门禁。

## 2026-10-08 指定远程会话历史与 runtime 读取（后续未提交增量）

此前图片/终端/只读列表改造现已进入当前提交 `a9a9728fa`；本段是该提交后的独立源码增量，不把旧 App 或旧快照测试升级为新代码的包级通过。

- 新增 Serve 的显式 `desktop/session-view`，修正新远程读取链路会误用旧 status 自动 reclaim、旧 history 默认前台回退的问题；旧 API 保持原行为不变。新路由只读取选中的 canonical 会话：foreground/detached 使用各自 runtime/model，saved/external 使用各自文件，缺失/越界/退休不回退。
- shared client、认证 bridge 的 `session-view` 和 main-only native typed command 已接；远程清单是输入白名单，URL/token 不入站，30 MiB/100,000 条预算，未知配置及 replay 字段不出站，owner 关闭取消在途读取并拒绝迟到发布。只读视图不开放本地 RuntimeManager 或远程发送权限。
- 新增 Go client 3 项、生产 Serve 3 项、bridge 2 项顶层回归，以及 Rust 2 项 native client fixture；包含自有真实 SSH direct-tcpip/cookie、生产 Controller/磁盘历史兼容、工具/reasoning/search、后台模型隔离、不自动接管、缺失/symlink/未知字段拒绝、取消/脱敏/版本/预算。未调用真实模型或外部账号。
- 最终定向 4 包 Go race、shared client 全量 race、Go vet、DTO/schema 与生成镜像检查、前端 typecheck、原生 remote client 的 5 项 Rust 测试（其余 254 filtered）及 clippy 通过，证据 `/private/tmp/reasonix-remote-view.v5p1wr/`。最初 compile 因 sandbox 禁止写 Go cache 失败，获审核后最终编译/回归通过；不将最初失败记为成功。
- 未接前端远程历史面板，未增加稳定消息/turn 展示身份、发送、SSE/重连/审批或远程模型修改；未重建或启动 App，未提交/推送本批。接下来把视图接入独立 remote surface，保留生成代次与 transcript 单写入者，再补完整 controller 的事件和写入权限。bot Desktop、原生图片粘贴、Windows/Linux 和其余包级门禁保持未关闭。

## 2026-10-08 本批 review 与 macOS App 交付

- 用户要求先生成可运行 App，再 review 并提交；此前大批图片/终端/只读列表已在 `a9a9728fa`，本轮审阅剩余 20 项增量并加入生成器修复与回归。发现并修复 Rust 将 `sources_status` 误作驼峰字段而丢失搜索来源状态的问题，复现与修复后验证分开记录。
- 完整四包 Go race、Rust 253 passed/6 ignored、完整前端 Tauri 回归、typecheck、vet、clippy、协议镜像门禁通过。首次构建的 arm64 `.app` 与两种临时档案启动/退出 smoke 通过，交付包从本次干净提交重建，最终日志位于 `/private/tmp/reasonix-macos-review.J5DA05/`。
- 新增远程历史 UI、稳定消息身份、事件/发送/审批/模型改动仍未实现；完整 E 目标继续。未连接真实外部账号、改动正式数据或触碰系统剪贴板，本地 ad-hoc 签名不是正式发布签名/公证。

## 2026-10-08 远程历史稳定身份与共享读取（后续源码增量）

- 当前已提交/交付的 macOS App 基线是 `d0106adb8`；本段为其后的未提交源码，未覆盖该 App，也未升级旧包级证据。
- 指定会话 DTO 增加必需的 backend entry `id`。普通、steer/recovery 显示投影保留各自源身份；可见条目缺失/重复/越界 ID 时失败，不由数组位置生成。旧 `/history` 不变，旧格式存储通过现有 loader 确定性兼容且不回写；早期缺少 entry IDs 的远端快照明确要求升级 Serve。
- native binding 和 RemoteControllerPool lease 接入 `sessionView(path)`：派发前检查未释放/退役，完成后核对 controller、host、resolved workspace、path、只读标志和 ID 唯一性。共享消费者、release 到派发间隙、迟到读取、SSH 重置/新 owner、不同身份/路径、拒绝无本地/Wails fallback 均有回归。
- 源码验证：4 包定向 Go race、生产 Serve 的重复读取/追加/legacy 不回写/损坏身份/后台隔离回归、Rust 6 项筛选回归、完整 `test:tauri`、生产/测试 typecheck、vet、clippy、协议生成检查通过。证据 `/private/tmp/reasonix-remote-identity.8uASVb/`；补充 fixture 首次误用 Save 被 DAG 合法去重，修正为损坏旧格式后同一拒绝断言通过，失败日志保留。
- 尚未把远程历史接到共享 Transcript；下一步是独立 remote surface、远程文件/图片解析权限和 generation 归属，不复制位置键或另建滚动写入者。之后继续完整 controller 事件/恢复/发送/审批/模型与 bot Desktop、异平台/复杂管理页/SQLite 门禁；本次不是总体完成，也不是新增 native command 的 App/WKWebView 验收。

## 2026-10-08 只读远程历史界面与共享渲染安全（后续源码增量）

- 继上一阶段，远程列表现可显式打开独立只读历史 surface，懒加载并复用共享 Transcript/TimelineProjection/Kernel。所有展示键按 controller/workspace/path/backend entry 命名；刷新保留同一 surface，选中替换、读取失败和卸载拒绝旧结果。没有 composer、编辑/恢复、审批或模型修改入口，不将 runtime 快照冒充实时进展。
- 修补共享入口：UserMessage 附件遵从 scoped resolver，拒绝/异常不回落本地；远程 file URL/路径/代码行引用统一拦截点击、中键与本地路径菜单。远程文件/图片尚未接入，界面明确提示；现阶段不是媒体或完整 controller 完成。
- 浏览器复现了共享问题导航把“快照新增问题”误当“用户发送”抢回底部的真实问题。仅客户端明确提交才产生临时 `tailFollowRequested`，不会由历史读取赋予；共享 hook 与确定性 Kernel 回归覆盖普通刷新 reader 零写入、稳定锚点、主动提交恢复和补丁/前插不重放，未新增滚动写入者或重试时钟。
- 隔离浏览器最终验收通过：250 轮打开末尾、思考/工具展开、远程媒体/文件零本地调用与零直接图片请求、刷新可见锚点和 scrollTop 不变、旧请求切换拒绝、390px 无横溢出、错误隐私/空态/卸载释放。原生 invoke 为 mock，不替代 App/SSH/Serve 包级联调。证据 `/private/tmp/reasonix-remote-history.Aw6GtM/`，详细场景、失败日志和截图见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-只读远程历史-ui-与共享权限阅读意图修复)。
- 完整 `test:transcript`、完整 `test:tauri`、生产/测试 typecheck、hooks lint、single-writer 与 build 体积门禁最终通过；新投影/媒体/异步归属/提交意图回归均纳入测试脚本。初稿撤掉所有被动跟随导致普通 tail 批量增长的挂载预算回归，修正为仅保留已有、非 native lease 的 tail 归属后原 53 项 viewport 断言通过，不调整 ≤40 或 4px 门限。失败与最终通过日志分开保留。
- 已交付 App 仍是 `d0106adb8`，本批未提交/推送、未重建 App。下一步继续远程媒体授权与实际 IPC 只读链路，再接 controller 事件/恢复/发送/审批/模型与 bot Desktop；原生图片粘贴、Windows/Linux、复杂管理页和 SQLite 门禁未关闭。

## 2026-10-08 远程历史批次 review 与可运行 App

- 已收敛上一阶段 42 项源码/测试/文档；尚未接通的远程图片读取入口不进入此次提交/包。共享阅读位置、稳定 ID 和本地/远程权限分流的回归保留，未新增滚动写入者、降低预算或开放远程写入。
- 当前 arm64 App 候选构建及 deep/strict ad-hoc 签名通过；实际 App 用独立临时 HOME 分别验证 managed/explicit 档案、私有身份、Global workspace、认证与 sidecar readiness、正常退出无残留通过。旧 App 可恢复副本与全部日志位于 `/private/tmp/reasonix-history-review.Ubhzkf/`。
- 本轮完整 Go 四包 race/vet、Rust 253 passed/6 ignored 与 clippy、完整前端 Transcript/Tauri、测试类型、协议生成与生产构建门禁通过。交付要求 review 提交后从干净 HEAD 重建，再运行相同包 smoke，最终收据是 `build-final.log` 与 `package-smoke-final.log`；不把 dirty 候选当作已提交版本。
- 未推送/正式安装或发布，不使用真实账号、用户数据/剪贴板。包启动不替代新增远程命令实际 WKWebView/SSH/Serve 联调，原生图片粘贴及跨平台等其余目标保持未完成。

## 2026-10-08 远程图片 Serve/client/bridge（后续源码增量）

- 上批已提交/交付基线 `de2c6050c` 保持不变；本批未提交/未重建 App。新增 scoped PNG 读取接口、共用 client、authenticated bridge、Go DTO/schema 与生成镜像；尚无原生 command 或 UI resolver，不将其记为完整远程图片功能。
- 全局 session catalogue 不等于 workspace grant。saved/external 按持久化 workspace（有界、无回写、root confinement）校验，owned runtime 按自身 workspace 校验；只允许同 Serve/SSH workspace，逻辑 alias 必须同 inode。旧元数据缺失、其他工作区、退休/替换、路径/文件 inode 改变、取消明确失败；不借本地工作区/cwd/原 opener，不自动恢复/接管历史。
- 入站 body/source、图片 bytes/pixels、输出 PNG/尺寸和两层 admission slots 有界；解码不持有 bind lock，发布再校验全部归属。公共图片复用现有专用 SSRF/proxy transport；controller Cookie、remote path/openHref、私有配置/诊断不交给 renderer。
- 当前回归：生产 Serve 实际 token/Controller/磁盘附件、客户端 PNG/字段/预算/错误/privacy、真实 loopback SSH bridge handle 生命周期与取消、held root/alias 替换及 metadata/admission 门禁通过。完整四包 race、最终新增用例两次 race、vet、协议 DTO/schema/镜像、Rust check/clippy、生产/测试 typecheck 通过；证据 `/private/tmp/reasonix-remote-image.rKqpbO/`，最终完整 race 为 `four-packages-final-race.log`。仅后端 seam 测试归属 race，不冒充真实网络图片或 native UI。
- 下一步：main-window-only IPC → remote lease scoped image resolver → 图片界面/滚动与隔离实际 App 验收；同时保留没有历史 workspace 元数据的兼容恢复检查。不关闭真实外部服务、原生剪贴板、完整 controller/bot Desktop、Windows/Linux/复杂管理页/SQLite 门禁。
