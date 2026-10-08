# E 迁移清单

2026-10-06 用户要求直接推进 E，明确不需要 updater。本轮不再以 D 全部验收为 E 的启动条件；D、A/B/C 未关闭项仍保留为正式发布门禁。正式发布、替换正式应用、切换默认下载均未授权。updater 从本轮范围排除。

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
