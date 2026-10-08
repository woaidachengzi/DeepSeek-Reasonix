# Reasonix Desktop Bridge Protocol v1（草案）

## 原生只读远程订阅增量（2026-10-08）

主窗口可调用 `bridge_remote_controller_subscribe`，request 仅含 controllerId、sessionPath、surfaceId、generation；host 返回 protocolVersion 与受监督 owner 绑定的随机 subscription 身份。`bridge_remote_controller_unsubscribe` 只接受 subscriptionId。收据不等于流已就绪；native 分别发送 `bridge:remote-session-state`（opening/ready/ended）和 `bridge:remote-session-event`（身份与投影 frame）。renderer 必须先监听再调用，逐帧检查当前身份/generation，结束后显式补快照，不自动重放或获得写入权限。

v1 envelope 的 event 仍为 opaque object；进入 renderer 前 native 依据从权威 Go eventwire 生成的 `remote_event_contract.generated.json` 校验/投影。协议生成检查同时覆盖该第三份 host-only artifact 与现有 TS/Rust 镜像。此源码入口尚不代表实际 App/live UI 或完整窗口/sidecar 生命周期验收。

## 范围

本协议连接 Tauri host 与本地 Go sidecar。它不是公网 API，sidecar 只允许本机
Tauri host 访问。v1 首批覆盖：健康检查、会话快照、创建/打开会话、工作区附件、
提交消息、取消、审批/提问/MCP 交互、Agent 流事件与正常关闭。

## 安全与启动

1. Rust 在每次启动生成 256-bit 随机 token，使用受限继承环境变量传给 sidecar。
2. sidecar 只监听 `127.0.0.1` 随机端口；它把端口和协议版本写到 Rust 已创建且
   权限为 owner-only 的 ready 文件，或通过受监督的 stdout ready frame 返回。
3. 所有 HTTP request 和 stream upgrade 都必须带 `Authorization: Bearer <token>`。
4. token、端口、prompt、Provider key、会话文本不可写日志；诊断仅记录脱敏版本、
   PID、请求 ID 与错误分类。
5. Rust 只终止自己启动且 PID/启动 nonce 匹配的 child，禁止按进程名杀进程。

## 传输与端点

初始实现使用 loopback HTTP/1.1 + JSON，事件使用 Server-Sent Events（SSE）。
token 是认证边界。若平台部署验证否定 loopback 方案，可改 Unix socket/Windows
named pipe，但必须保留相同 JSON envelope、认证、sequence 与重连语义。

| 操作 | 方法与路径 | 幂等性 |
| --- | --- | --- |
| 健康检查 | `GET /v1/health` | 是 |
| 脱敏 Provider 摘要 | `GET /v1/providers` | 是 |
| Preview Provider 设置与模型发现 | `GET/POST /v1/settings/provider-configs`、`POST /v1/settings/provider-configs/discover-models` | 保存使用 request ID 去重；模型发现只读 |
| Preview 敏感信息保护 | `GET/POST /v1/settings/secrets` | 保存使用 request ID 去重；同步更新当前 sidecar 的运行时保护开关 |
| Preview 桌面费用展示币种 | `GET /v1/settings/desktop`、`POST /v1/settings/desktop/currency` | 保存使用 request ID 去重；仅改变新费用报价的展示币种，不改供应商价表 |
| Preview 远程 SSH 主机与别名 | `GET /v1/settings/remote`、`POST /v1/settings/remote/scan`、`POST /v1/settings/remote/hosts` | 读取与别名扫描只读；主机变更使用 request ID 去重 |
| Preview 远程 SSH 连接与断开 | `POST /v1/settings/remote/connect`、`POST /v1/settings/remote/disconnect` | sidecar 生命周期内保留受监督 SSH 连接；未知主机密钥只有在调用方提交与当前握手一致的指纹后才会写入 Reasonix 管理的 `known_hosts`；sidecar 关闭时断开 |
| Remote controller 只读连接 | `POST /v1/remote/controllers` | 同一已认证 SSH 底层连接与 host/workspace 复用健康 handle；新附着可取消旧的未完成附着；只接受 `name/workspace`，不接受 URL/token |
| Remote controller 会话清单 | `GET /v1/remote/controllers/{controllerID}/sessions` | 只读；远程路径不转换为本地会话/文件权限；断线/重连/关闭后旧 handle 拒绝读取 |
| Remote controller 指定会话视图 | `POST /v1/remote/controllers/{controllerID}/session-view` | 只读；body 只有 `sessionPath`，必须匹配远程清单；不恢复/接管/切换会话，不调用本地 RuntimeManager |
| Remote controller 指定会话事件 | `POST /v1/remote/controllers/{controllerID}/session-events` | 只读、非重放 SSE；body 只有 `sessionPath`，必须匹配清单；独立于本地 EventLedger，不授予发送/恢复权限 |
| 关闭 Remote controller | `DELETE /v1/remote/controllers/{controllerID}` | 合法 handle 可重复关闭；只清理自有 HTTP owner，不停止远程 Serve、SSH 或其他工作区 |
| Preview 远程工作区浏览 | `POST /v1/settings/remote/browse` | 只通过已验证连接执行 SFTP 目录读取；每页最多返回 500 项，不执行远程 shell 命令 |
| Preview 权限规则 | `GET/POST /v1/settings/permissions` | GET 可用 `workspaceRoot` 读取项目有效值；POST 的 `scope=global|project` 选择用户配置或当前项目 `reasonix.toml`，项目首改按字段继承对应全局值；保存使用 request ID 去重 |
| 设置新会话默认模型 | `POST /v1/settings/default-model` | `X-Reasonix-Request-ID` 去重 |
| 切换当前会话模型 | `POST /v1/sessions/{sessionId}/model` | `X-Reasonix-Request-ID` 去重；只允许空闲会话，目标模型必须在当前工作区已配置；先耐久快照再重建同一会话，失败时恢复旧模型 |
| 设置 Preview Agent 语言/压缩阈值 | `POST /v1/settings/agent-preferences` | 每次仅更新一个字段；压缩阈值支持 30–85%，精度 0.1%；`X-Reasonix-Request-ID` 去重 |
| 建/开会话 | `POST /v1/sessions:open` | `X-Reasonix-Request-ID` 去重 |
| 显式切换会话 | `POST /v1/sessions:switch` | `X-Reasonix-Request-ID` 去重；仅空闲会话 |
| 旧会话标题摘要 | `POST /v1/sessions:previews` | 只读；最多 50 个 ID，仅返回已有标题和首条可见用户消息，不切换当前会话 |
| 重命名会话 | `PATCH /v1/sessions/{sessionId}/title` | `X-Reasonix-Request-ID` 去重；仅空闲会话 |
| 删除会话 | `DELETE /v1/sessions/{sessionId}` | `X-Reasonix-Request-ID` 去重；仅空闲且为当前持有的会话 |
| 会话快照 | `GET /v1/sessions/{sessionId}/snapshot` | 是 |
| 当前会话余额 | `GET /v1/sessions/{sessionId}/balance` | 是；仅返回当前控制器格式化后的余额，不配置时为 `null`；错误详情不会回传 |
| 当前会话 MCP 连接 | `POST /v1/sessions/{sessionId}/mcp/runtime` | `X-Reasonix-Request-ID` 去重；只作用于指定的空闲会话，连接前检查服务器已启用；不改变持久启用设置 |
| 清除当前会话 MCP 凭据 | `POST /v1/sessions/{sessionId}/mcp/auth/clear` | `X-Reasonix-Request-ID` 去重；仅空闲会话；按真实配置来源清除所选服务器的静态凭据和私有 OAuth 状态，并断开该会话中的连接 |
| 启动 MCP OAuth | `POST /v1/sessions/{sessionId}/mcp/oauth` | `X-Reasonix-Request-ID` 去重；只作用于指定空闲会话；返回随机 flow ID，不返回授权 URL 或令牌 |
| 查询/取消 MCP OAuth | `GET/DELETE /v1/sessions/{sessionId}/mcp/oauth/{flowId}` | 绑定发起流程的当前会话；状态仅为 pending/complete/failed/canceled；流程最长 5 分钟 |
| 只读会话清单 | `GET /v1/sessions/inventory?catalog=<path>` | 是；只读，不建库、不认领 |
| 保存的项目文件夹 | `GET /v1/projects` | 是；只读旧版项目目录的 workspace root 与自定义标题 |
| MCP 服务器列表 | `GET /v1/mcp/servers?workspaceRoot=<path>` | 是；只返回凭据键名 |
| 新增/编辑 MCP 服务器 | `POST /v1/mcp/servers?workspaceRoot=<path>` | `X-Reasonix-Request-ID` 去重；字段省略即保留原值 |
| 删除 MCP 服务器 | `DELETE /v1/mcp/servers?workspaceRoot=<path>` | 否；不存在返回 `not_found` |
| 可见历史 | `GET /v1/sessions/{sessionId}/history` | 是 |
| 附加文件 | `POST /v1/sessions/{sessionId}:attach` | `X-Reasonix-Request-ID` 去重 |
| 当前会话本地工作区 | `GET /v1/sessions/{sessionId}/workspace-target` | 只读且仅允许当前会话；返回 runtime 实际目录供 Rust 原生打开，Global 项目归属仍为空 |
| 内置终端清单 | `GET /v1/sessions/{sessionId}/terminal` | 只读；仅当前 session workspace，同会话 controller 重建保留终端，返回终端/shell ID，不返回执行参数或环境 |
| 创建内置终端 | `POST /v1/sessions/{sessionId}/terminal` | 必须带 `X-Reasonix-Request-ID`；相同请求重试不创建第二个 PTY |
| 内置终端输入 | `POST /v1/sessions/{sessionId}/terminal/{terminalId}/input` | 必须带 `X-Reasonix-Request-ID`；base64 原始字节，有界串行队列；重试不重复执行 |
| 内置终端输出快照 | `GET /v1/sessions/{sessionId}/terminal/{terminalId}/output` | 只读；至多 128 KiB 原始字节，base64 与 byte offset |
| 终端尺寸/名称/关闭 | `POST .../terminal/{terminalId}/resize`、`PATCH .../terminal/{terminalId}/title`、`DELETE .../terminal/{terminalId}` | 支持请求 ID 去重；仅归属当前会话的终端，不能操作外部 Terminal |
| 工作区目录（逐层） | `POST /v1/sessions/{sessionId}:workspace` | 是 |
| 工作区文件预览 | `POST /v1/sessions/{sessionId}:workspace-file` | 是；仅返回受限文本或二进制标记 |
| 工作区变更（Git + 本轮会话检查点） | `POST /v1/sessions/{sessionId}:workspace-changes` | 是；来源显式标记 |
| 工作区变更详情 | `POST /v1/sessions/{sessionId}:workspace-change-detail` | 是；仅返回受限 diff，支持 Git 或会话检查点 |
| 单文件恢复预览 | `POST /v1/sessions/{sessionId}:workspace-file-revert-preview` | 只读；返回会话首次修改前的恢复方案、可用性及冲突原因 |
| 单文件恢复提交 | `POST /v1/sessions/{sessionId}:workspace-file-revert-commit` | `X-Reasonix-Request-ID` 去重；仅接受刚预览的 plan ID；冲突需显式 `overwrite_checkpoint` |
| 撤销上次工作区恢复 | `POST /v1/sessions/{sessionId}:workspace-file-revert-undo` | `X-Reasonix-Request-ID` 去重；只接受上次恢复返回的 transaction ID；文件再次变化时拒绝撤销 |
| 检查点列表 | `POST /v1/sessions/{sessionId}:checkpoints` | 只读；最多返回最近 50 个检查点，提示词预览有长度限制 |
| 多文件代码回滚预览 | `POST /v1/sessions/{sessionId}:code-rewind-preview` | 只读；返回该轮及以后受影响的文件、冲突与覆盖缺口 |
| 多文件代码回滚提交 | `POST /v1/sessions/{sessionId}:code-rewind-commit` | `X-Reasonix-Request-ID` 去重；只接受 `code` 方案；项目覆盖缺口须显式确认，提交重新校验文件 |
| 对话回滚预览 | `POST /v1/sessions/{sessionId}:conversation-rewind-preview` | 只读；只允许同一 transcript 内可切换 head 的会话，旧格式返回禁用原因 |
| 对话回滚提交 | `POST /v1/sessions/{sessionId}:conversation-rewind-commit` | `X-Reasonix-Request-ID` 去重；只接受 `conversation` 方案，在同一 transcript 创建新 head，文件不变 |
| 旧格式对话分叉预览 | `POST /v1/sessions/{sessionId}:legacy-fork-preview` | 只读；仅接受旧格式会话的检查点对话边界 |
| 旧格式对话分叉提交 | `POST /v1/sessions/{sessionId}:legacy-fork-commit` | `X-Reasonix-Request-ID` 去重；创建新会话文件并登记独立身份，原会话和工作区文件不变 |
| 返回原对话 | `POST /v1/sessions/{sessionId}:conversation-rewind-undo` | `X-Reasonix-Request-ID` 去重；只接受当前尚未追加消息的 rewind head ID |
| 对话版本列表 | `POST /v1/sessions/{sessionId}:session-heads` | 只读；返回当前 transcript 的 schema-2 活跃 head，旧格式为空列表；限制展示数量和预览长度 |
| 切换对话版本 | `POST /v1/sessions/{sessionId}:session-head-switch` | `X-Reasonix-Request-ID` 去重；只接受当前 transcript 内的活跃 head ID，路径和工作区文件不变 |
| 组合回滚预览 | `POST /v1/sessions/{sessionId}:combined-rewind-preview` | 只读；同时检查会话边界、文件冲突及覆盖缺口，旧格式会话返回禁用原因 |
| 组合回滚提交 | `POST /v1/sessions/{sessionId}:combined-rewind-commit` | `X-Reasonix-Request-ID` 去重；只接受 `both` 方案及所需覆盖确认；结果区分全部成功与文件事务未完成的部分成功 |
| 提交 | `POST /v1/sessions/{sessionId}:submit` | `X-Reasonix-Request-ID` 去重 |
| 取消 | `POST /v1/sessions/{sessionId}:cancel` | 是 |
| 工具审批 | `POST /v1/sessions/{sessionId}:approve` | 可安全重试 |
| 回答 `ask` | `POST /v1/sessions/{sessionId}:answer` | 可安全重试 |
| 回答 MCP 交互 | `POST /v1/sessions/{sessionId}:mcp` | 可安全重试 |
| 重放待处理提示 | `POST /v1/sessions/{sessionId}:replay-prompts` | 是 |
| 流订阅 | `GET /v1/events?afterSequence=N` | 可重连 |
| 正常关闭 | `POST /v1:shutdown` | 是 |

桌面偏好读取 `GET /v1/settings/desktop` 现在包含 `externalOpener` 稳定应用 ID；`POST /v1/settings/desktop/external-opener` 接收 `{ "id": "vscode" }`，复用鉴权、请求 ID 去重和窄用户配置写入。该接口只保存 UI 偏好，不执行应用程序。Rust 原生宿主校验本机安装目录后才调用写入，工作区打开使用会话 ID 读取权威 snapshot；应用卸载后的选择回退不会改写保存的 ID。此字段可被旧客户端忽略，旧配置没有该字段时默认为空。

`remote_controller_sessions_v1` 目前只标识只读 catalogue 阶段；Tauri 主窗口源码已接窄 IPC 和设置页列表，不表示完整 remote tab 可用。附着返回 `{protocolVersion, controller: {id, name, workspace, readOnly: true}}`；ID 是随机 16 bytes 的规范无 padding base64url（22 字符）。清单追加 `sessions`，每项固定 `name/path/title/turns/current/running/takenOver/mtimeMilli`，不返回 Serve 的额外 URL、token 或配置。认证 cookie 仅在 Go 持有。每个 handle 同时绑定当前受监督 client 与底层 SSH 连接；连接替换、断线、停服、退出均有撤销边界。列表操作不恢复/切换远程当前会话，也不访问本地 RuntimeManager。读写/流事件/审批/模型归属须另行接入，不因 catalogue 返回而开放权限。此源码增量未重建 App。

主窗口命令 `bridge_remote_controller_attach/sessions/close` 仅接受 `{request}`；
attach 的 request 只有 `name/workspace`，另两项只有 `controllerId`，拒绝未知字段。
宿主固定 HTTP method/path，先检查 main label，再在 blocking worker 执行；
网络等待不占 supervisor lock，错误不透传网络/解码器自由文本。响应再次检查版本、
canonical handle、只读标志、目录字段/计数预算及重复路径。renderer 不能指定 URL/token。

前端共享 lease 按 host/workspace 去重；最后一个消费者卸载才关闭。迟到 attach 与 close
完成之前，同 scope 的新 attach 等待 predecessor，防止旧关闭清理新 UI 的复用 handle。
关闭投递不确定时保留拒绝 barrier，直到成功的显式 SSH 重连/断开或真实配置撤销；
保存仍为 connected 的未改变配置不清空池。旧清单和 finally 不可写入新 scope。
列表由用户显式打开才懒加载，每页最多挂载 50 条，路径仅展示；关闭/停服收起列表。
Rust HTTP fixture、绑定与组件回归、浏览器模拟原生调用的验证不等于 App 原生调用验收。

`remote_controller_view_v1` 为后续指定会话读取能力，不声明远程发送或审批权限。
Go 先在当前 owner 的 catalogue 中验证 path，再访问 Serve 新增的
`GET /desktop/session-view?session=<encoded-path>`，整个操作最多 20 秒。
新 Serve 路由要求一个显式 canonical path，非法/不存在/越界/已删路径失败，不回退前台。
它不使用旧 `/status` 的自动 reclaim 或 `/history` 的默认会话回退；旧 Serve 缺少路由时
返回可操作错误，不用旧路由猜测兼容。

结果是 `{protocolVersion, controller, view}`，view 含版本、sessionPath、readOnly=true、
ownership=serve/saved/external、current、modelRef、label、可选 runtimeState 和 typed history。
只有实际 Serve-owned foreground/detached controller 提供自己的 modelRef/label/runtime；
saved/external 从指定文件加载，不借用前台模型，runtime 缺省且模型字段为空。
外部 writer 的最新磁盘历史可读取，但不会自动接管；retiring 的后台 owner 返回冲突。
bindMu 防止 controller 替换时串会话；history 与随后采样的 runtime 是观察值，不是原子
事件事务或未来发送授权。事件恢复仍须单独接入。

后续源码增加指定会话 `session-events`，认证仍由 bridge bearer 与 backend cookie
完成；禁止 query、`Last-Event-ID`、URL/token/workspace/afterSequence 等入站字段。
每个受监督 controller 最多两个订阅，关闭订阅不关闭共享 controller。后端读取
`/events?all=1`，但只发布精确匹配指定路径的 typed event，未标记和其他会话帧丢弃。
输出 `data: {protocolVersion, controller, sessionPath, event}`；controller 保持只读。
schema 中 event 是 opaque object，Go 用共享 eventwire 类型剔除未知字段，但不表示
各 native payload 已验证或 UI 可直接消费。native 接入还须验证 payload/capability，
并用 controller + session + subscription generation 拒绝迟到帧；已写入 TCP 的字节
不能由后来的 SSH 撤销收回。本流不进入本地 session EventLedger，也不分配重放 ID。
输入帧（含 metadata/comments）最多 8 MiB，输出 envelope 最多 9 MiB；原 transport
限制响应头等待 15 秒，输出写入/flush 等待最多 5 秒，空闲每 10 秒发送 comment。
操作取消、断线/重连/关闭 controller 均关闭 upstream body 并释放槽位；EOF/损坏
结束连接，必须显式重开并 reconcile 快照/待处理提示，不自动重试、切前台或接管。
后续已增加内部 Rust host transport：读取 catalogue 后固定 HTTP/1.0 POST，要求
close-delimited SSE（拒绝 chunked/Content-Length），有界 header/frame 与 socket
shutdown 取消已打开的 reader。typed envelope 对 controller/host/workspace/path 再次
核对；event 仍 opaque，不能直接 emit 到 renderer。订阅 registry、
native payload 检查、generation/window/sidecar fence 尚待接入。后续 pending-open 已
补 operation socket lease，覆盖 catalogue HTTP 响应头/body 和 SSE 响应头，取消主动
shutdown 已建立 socket；连接阶段仍有 1 秒预算，完成后已取消则不派发请求。
registry 须在打开前分配独立 operation，在关闭/替换时取消，不复用旧取消句柄。
后续内部 registry 核心为每次 reserve 分配 native 随机 subscription ID，按 surface
单调 generation 拒绝旧请求；sidecar identity 只由 Supervisor 绑定。16 current /
32 live / 64 generation tombstones 有界，旧 worker Drop 不能删除替换 entry，
main Destroyed/stop 撤销 owner admission。尚无注册 IPC 或未经校验的 payload emit，
queued event 仍须 renderer 用完整 identity fence 拒绝，不能以 native gate 代替。
当前没有注册原生命令、
live UI 或恢复/发送/审批链路，transport fixture 不是实际 App 命令验收。

历史保留 reasoning、工具参数/结果、搜索结果和恢复提示；DTO 不含 provider replay/raw
或额外配置/凭据字段。响应最多 30 MiB（为宿主 32 MiB body gate 留 envelope 空间）、
最多 100,000 条；saved/external 源文件先检查普通文件和 64 MiB 上限，超限失败而不截断。
bridge 输入按最多 200 KiB JSON 与解码后的 32768 字节 path 双重限制；不接受 URL/token。
读取结束再次检查 SSH owner，关闭/断线迟到结果不发布。原生
`bridge_remote_controller_session_view` 仅 main label，typed request 为 controllerId/sessionPath，
固定 POST 路由、25 秒 worker 等待，响应再次校验归属、只读、字段预算和版本并脱敏错误。
搜索来源状态保留原 wire key `sources_status`；Rust 生成镜像显式 serde rename，不能由
camelCase 默认规则改变该字段。native HTTP fixture 同时检查读取与再次输出的字段名。
指定会话的每条 history 现在必须带唯一非空 `id`（最多 4096 UTF-8 字节，无控制字符）。
该 ID 直接来自 Controller/存储 entry；过滤消耗后的 recovery、追加及重排不按位置重新编号。
旧格式日志由现有 agent loader 确定性赋予兼容 ID，spectator 不修改文件；已有重复或非法
显示身份明确失败。旧 `/history` wire 保持不变。尚不含 entry IDs 的早期 session-view
原型不按数组下标兜底，需要升级远端 Serve；不把该失败转成本地恢复或发送授权。
前端 native binding 与共享 lease 已可读取该 DTO，读取前后均核对 consumer/SSH owner、
controller、resolved workspace、指定 path 和只读标志，并拒绝重复 ID 与迟到结果。
后续源码已连接设置页的显式只读历史面板，复用共享 Transcript/TimelineProjection/Kernel。
消息/工具/搜索身份由 controller、resolved workspace、session path 与 backend entry ID
共同命名；刷新不重建同一 surface，旧请求完成不发布到替换后的会话。
快照新增问题不是用户发送：共享 reducer 只在客户端明确提交时标记临时
`tailFollowRequested`，不从 history DTO 或 provider 事件赋予，收据确认不撤销该显示意图。
共享问题导航仅以新的明确提交意图恢复 tail follow，普通历史刷新保持 reader 锚点。
此面板没有发送/编辑/恢复/审批/模型变更能力；远程图片 resolver 显式拒绝，
附件与 file/source 链接不得回落到本地 AttachmentDataURL、文件打开器或路径菜单。
这不是完整 remote tab 或远程文件预览；本批已进入 macOS 候选构建，review/交付门禁见 API audit，
native client fixture、模拟宿主浏览器和普通包启动均不等于新增命令的 WKWebView 联调。

当前 bridge 已实现 health、脱敏 Provider 摘要、建/开会话、空闲会话的显式切换、重命名与删除、快照、可见历史、工作区附件、逐层工作区目录、受限工作区文件预览、Git 变更列表与受限 diff、submit、cancel、审批/ask/MCP 提示回答、断线后的待处理提示重放、SSE 事件与正常关闭。工作区目录只返回当前会话工作区下的一层，隐藏常见构建产物、依赖目录和 `.git`，每层最多 200 项；返回值只有相对路径，`..` 越界会被拒绝。文件预览只读取有限大小的常规文件；二进制或非法 UTF-8 文件仅返回 `binary: true`，不会把原始字节交给 renderer。Git 变更查询只执行固定的只读 Git 子命令，diff 输出限制为 2 MiB；非 Git 工作区会返回 `gitAvailable: false`，不会伪造“干净”。会话自定义标题写入 core 的 `.jsonl.meta`，不改动 transcript；标题为空、含控制字符或超过 120 个 Unicode 字符时会被拒绝。删除走 core 自身的产物清理（transcript、事件日志与 sidecar、guardian、inbox、checkpoint、子 agent 记录、cleanup 标记），只允许删除当前持有且空闲的会话，避免 host 误删另一个 host 正在使用的会话；删除成功后 bridge 释放该控制器且不再写回快照，否则会把刚删掉的文件重新创建出来。对已不存在的会话重复执行删除返回 `not_found`，而带同一 `X-Reasonix-Request-ID` 的传输重试重放首次响应。

后续源码增加 `POST /v1/remote/controllers/{controllerID}/session-image`，只接受
`{sessionPath, source}`（24 MiB body，source 至多 22370645 UTF-8 bytes）。workspace
由 backend-owned handle 注入 Serve `POST /desktop/session-image`，renderer 不提供
workspace、URL、token 或本地会话 ID。返回 `{protocolVersion, controller, view}`，
view 固定 `protocolVersion/sessionPath/workspace/image`；image 仅含重新编码的 PNG
data URI、filename、mime、size 或白名单 errorCode，不含 openHref。client 严格检查
版本/路径/workspace、12 MiB response、8 MiB PNG、1200px 与完整可解码 raster。
saved/external 必须有归属当前 Serve workspace 的持久化元数据，缺失不回写/迁移。
活跃/后台使用各自 runtime root；读取前后均检查 owner、目录/会话文件 inode、alias
及取消。锁忙、owner 改变为固定错误，不 fallback 本地。两端最多两项 admission，
Serve/bridge 分别有 25/32 秒预算，client 整个 catalogue+读取为 30 秒。
后续 review 已接 main-only `bridge_remote_controller_session_image`；入站窄 DTO
只有 `controllerId/sessionPath/source`，拒绝其他字段。Rust 在 typed 输出前再次完整
解码有界 PNG；remote lease 核对 handle/path/workspace 并剔除 opener/private 字段，
共享历史 resolver 在派发/返回时检查同一布局提交 scope。实际包启动 smoke 已通过，
但不等于图片命令 WKWebView/SSH/Serve 联调通过，feature capability 未新增。

此后端阶段尚未新增原生 command、前端 resolver 或 feature capability，当时已交付 App 仍是
`de2c6050c`；typed 后端/SSH fixture 不等于包内远程媒体 UI 验收。

单文件恢复沿用 core 的两阶段检查点事务。预览不会写入文件；提交会重新校验方案与文件指纹，旧方案和会话运行中的提交会失败。会话首次修改后文件再次变化时，界面须显示覆盖风险，并让用户在最终操作中显式选择 `overwrite_checkpoint`。`keep_current` 由界面取消表示。恢复成功后若 core 返回可撤销事务 ID，界面提供一次撤销入口；撤销前重新核对文件指纹，后续手工修改会使撤销失败。bridge 只回传相对路径、冲突原因和恢复数量，不回传原始文件内容或绝对路径。此功能只恢复一个由当前会话检查点持有的文件，不回退对话历史。

检查点页接入 core 的 `RewindCode` 事务。预览列出该轮及以后被捕获的文件，最多展示 60 条路径，同时返回完整计数；发现文件冲突时禁用提交，不提供多文件强制覆盖。项目覆盖缺口允许提交前必须勾选风险确认，bridge 再次检查该标志；没有确认时即使绕开前端也不能提交。成功的代码回滚可经前述撤销事务入口恢复操作前文件，且不改变对话历史。对话回滚、组合回滚和旧格式独立会话分叉均已接入 Preview。

MCP 服务器管理按“列表 → 新增 → 编辑/删除”拆分。`GET /v1/mcp/servers` 返回每个服务器的 `name / type / source / scope / configPath / command / args / url / envKeys / headerKeys / autoStart / tier / managedByPackage / nativeOAuthEligible / authenticationSaved`：**凭据只写不回传**——`envKeys` 与 `headerKeys` 只给出该服务器期望的键名，任何值都不会进入响应。`scope=project` 的写入由 `workspaceRoot` 选定的 `reasonix.toml`，`scope=global` 写入用户配置；已有条目按内核记录的真实来源写回，不会把项目级服务器提升为全局。`POST` 的 `args` / `env` / `headers` 用“字段省略 = 保留原值、显式空对象 = 清空”的语义，因此编辑不需要重输密钥；由已安装插件包管理的服务器会被拒绝修改。删除会从拥有该声明的配置文件移除，名称不存在时返回 `not_found`。当前会话连接动作支持已启用服务器的重连与断开：请求必须带当前会话 ID，空闲检查在宿主和 runtime 两层执行；操作不更改持久启用项，断开只影响当前会话，连接返回工具数量而不返回工具 schema 或凭据。符合内核资格检查的 Streamable HTTP MCP 可启动系统浏览器 OAuth PKCE 授权，流程由当前 runtime 管理，令牌仅写入 Reasonix 私有状态；Tauri 查询到的只有流程 ID、服务器名和脱敏状态。完成授权后需要用户显式连接 MCP，不会自动改变当前会话工具集。设置页可清除已保存的静态凭据和私有 OAuth 状态，此操作要求当前会话空闲，按真实配置来源写入并断开当前 Host 中该服务器。

`GET /v1/sessions/inventory` 是迁移前的只读盘点：返回 `sessionDir`、`identityPath`（由 sidecar 自身解析，**不接受调用方指定**）以及每行 `id / path / exists / registered / workspaceRoot / title / source / claim / detail`，并单列 `unclaimed`（磁盘上没有任何 catalog 或身份行解释的 transcript）与 `errors`（逐行问题）。`source` 取 `identity` / `workbench` / `scan`，`claim` 取 `registered`、`claimable`、`missing_file`、`path_conflict`、`path_changed`、`unclaimed_file`、`invalid_file`、`unreadable_file`。它**不创建身份库、不登记任何候选、不改动 transcript**；旧文件只作为候选列出，未在 catalog 中的文件不会被自动认领。会话 sidecar（`.events/.turns/.conflicts/.guardian`）不计入清单。`catalog` 参数可选，由宿主持有的 `workbench-sessions.json` 路径传入；它是只读输入。
Provider 摘要仅包含配置名称、类型、已配置模型 ID、模型数量、是否需要凭据、凭据是否已配置及用户默认模型；不返回 endpoint、凭据变量名、密钥或请求 headers。模型 ID 仅用于本地下拉选择。
默认模型修改复用 `internal/config` 的选择校验、用户配置锁和窄写入，只改变新会话默认值；选项只包括已启用且凭据可用的模型。工作区 `reasonix.toml` 仍可覆盖用户默认值。当前会话模型切换是独立操作，不写入新会话默认值；它先关闭并耐久快照空闲控制器，再用所选模型恢复同一 transcript，构建失败会重新打开原模型。
`submit` 仅确认既有 Go Controller 已接收输入（HTTP 202）；它不会等待 Agent 生成结束，
SSE 事件携带进度和最终结果。事件 replay 使用有界 ledger；落在窗口之前的 sequence 会
得到 `resync_required`，host 必须请求快照。Tauri host 为建/开会话和提交生成高熵
`X-Reasonix-Request-ID`；bridge 以请求方法、路径和 body 的 SHA-256 指纹在当前 sidecar
生命周期内有界缓存 256 个完成响应。相同 ID+相同请求会重放原响应，不同请求复用同一 ID
返回 `conflict`。缓存只保存响应与摘要，不保存 prompt 原文；host 仅在传输失败后用相同 ID
重试一次。历史端点只返回 user/assistant 的 `content`：不返回 system prompt、推理内容、工具
参数/结果、图片引用或本地执行元数据。单条正文最多 16,000 个 Unicode 字符，单次最多返回最近
200 条可见消息；`startIndex` 与 `totalMessages` 让 host 明确提示未加载的更早内容。

附件请求只接受由 Tauri 原生文件选择器返回的绝对路径。Go core 会拒绝符号链接、
目录、空文件和超过 25 MB 的普通文件（图像上限 64 MB，并校验实际图像格式），
随后将副本写入当前工作区的 `.reasonix/attachments/`。响应只返回工作区相对引用、
展示名称、大小与图像标记，不返回源文件绝对路径；附加操作使用 request ID 去重，
避免传输重试时重复创建副本。

`switch_session` 是为工作台导航准备的受限交接：bridge 首版仍只拥有一个 Go
Controller。目标会话与当前会话不同且当前状态为 `idle` 时，bridge 先调用旧
Controller 的 durable shutdown，再创建/恢复目标 Controller；`running` 或 `paused`
状态返回 `conflict`，不会停止或替换用户的活动回合。若旧 Controller 已正常关闭但
目标创建失败，bridge 会尝试重新打开原会话；恢复成功时仍返回目标切换错误，调用方
保留原选择。若原会话也无法恢复，则同时报告两项失败，不伪称切换成功。

审批请求只允许一次性“允许”或“拒绝”，Tauri 不直接暴露原始工具参数；`ask` 请求携带
结构化问题和选项，回答可为空表示跳过；MCP 交互只转发 `accept`、`decline`、`cancel`
及表单内容。所有三类回答都先由 Controller 持久化 `prompt_answered` 再释放阻塞的 Agent
回合。重新打开处于 `paused` 的会话后，host 在建立 SSE 监听后调用
`replay-prompts`，因此审批卡片不会因窗口重启而丢失。

## 内置终端增量（2026-10-08，未提交）

创建体为 `{ "path": ".", "shellId": "default" }`，两个字段都可省略。
`path` 必须是实际 controller 工作区内的相对目录/文件，文件使用父目录；绝对路径、
越界和外部链接拒绝，不接受 renderer 的 executable/args/env。
shell ID 只选择后端批准且已安装的解释器，默认遵循用户设置/现有 Wails 发现规则。
同一 session workspace 最多 10 个终端，创建中与关闭中也占 reservation。

输入体只有 `{ "data": "<base64>" }`，解码后 1..65536 字节，不能换用 URL 或命令字段。
HTTP 202 表示输入已复制进入最多四项的串行队列，不表示命令完成；队列满返回
`terminal_busy`，客户端不得自动更换 request ID 重发已被确认的输入。
尺寸体为 `{ "cols": 80, "rows": 24 }`，分别限制 1..1000 / 1..500；
名称体只有 `title`，最多 80 个 Unicode 字符，拒绝控制字符。

输出快照包含 `id / data / start / end`。`data` 是 base64 原始终端字节，最多 128 KiB；
`start/end` 是累计 raw byte offset，不是 Unicode 字符数。`terminal_output` SSE payload
使用同一结构，每帧最多 8 KiB，`terminal_exit` 为 `id / exitCode / removed`。
客户端必须将它们与 Agent 事件分流，不能作为 assistant text；replay 缺口需要用终端快照
替换旧缓存。输出不持久化到 core History，也不自动进入模型上下文。

所有入口复用 bridge 认证和当前 session 归属；切换、删除和退出关闭旧 generation 的
输入/创建 gate，迟到回包不能暴露到新会话（包括同 ID 的新 generation）。关闭中仍有进程
或创建未完成返回失败，不伪造清理成功。同一会话的 model/settings/effort rebuild 使用
独占 backend retention lease 保留原 PTY、名称、尺寸和输出缓存；校验实际工作区、会话、
transcript path 与事件流身份，不接收 renderer 的归属重绑定。模型失败恢复仍交接同一 PTY；
无法恢复则关闭 lease。创建尚未完成最终归属确认时替换返回 `terminal_busy`，需等待创建完成。
所有替换递增 owner epoch，迟到回包不得越过 generation；旧 provider 不能再写已交接终端。
RuntimeManager 在 factory 重建期间持有 lease，退出立即清理，不依赖候选构建返回。
设置/推理候选只有发布后才提交暂存配置，交接失败恢复旧 gate 并回滚选择。
Rust 主窗口已提供 `bridge_terminal_workspace/create/output/input/resize/rename/close`，严格
typed request 的 mutation 都需 `requestId`，只允许 `main` window；请求由后台 worker 执行。
输入是严格 base64 原始字节；host 检查响应归属/offset/预算/版本，同 ID 丢响应重试不会换
body。退出码有符号，允许 Unix `-1` 与 Windows DWORD，生成镜像为 `i64`。
`terminal_output` / `terminal_exit` 经 host 校验后仅对 `main` label 发 `bridge:terminal-event`，不混入
`bridge:event` 的 Agent stream；无效 terminal frame 丢弃。前端源码已接共用 TerminalPanel/xterm
懒加载入口、单一订阅、同终端串行输入和 byte offset 去重/缺口快照替换，不按消息次数猜测。
renderer 输入待发送和在途合计最多 256 KiB（单次 64 KiB），最多 1024 个待执行操作；
断线/失去 owner/失败时取消尚未提交的输入，不换 request ID 自动重发。恢复期间暂存最多
16 × 8 KiB live frame，快照最多一次附加重试，快照缺口用 revision + queued RIS 替换旧显示。
主动“添加输出到聊天”才转为去除 ANSI/控制序列的文本上下文，仍不自动提交或写入 History。
浏览器宿主模拟与真实 Rust client → Go → PTY 的验证是分别执行的。独立 ARM64 候选 App
现已构建，并通过 managed/explicit 私有 profile 的真实 WKWebView/xterm InputEvent → 已登记
IPC → 打包 Go → PTY 验收，包含中文/ANSI、显式上下文暂存但不发送、收起不重建与关闭/
切换/正常退出后的自有 PID 清理。证据与候选 hash 见 [API_SURFACE_AUDIT.md](API_SURFACE_AUDIT.md)。
它不覆盖物理键盘/输入法或系统剪贴板，也不表示旧已交付 Preview 已更新或 C 全部完成；
Windows/Linux 实际运行、远程 controller/bot、图片原生粘贴与正式发布门禁继续保留。

## Envelope

目标协议的变更请求带 `X-Reasonix-Request-ID`，用于重放去重；成功结果、错误和事件都
显式带 `protocolVersion: 1`。请求 ID 去重仅在一个 sidecar 生命周期内有效；sidecar 重启后
host 必须先恢复健康状态和快照，不能复用旧 ID 假定提交已完成。未知 major version将返回
`protocol_version_unsupported`，不进行猜测或部分兼容。

事件 `sequence` 在一个 sidecar 生命周期内严格递增。客户端在断线后使用最后已
确认 sequence 重连；若缓存不再覆盖请求位置，sidecar 返回 `resync_required`，
客户端必须请求 session snapshot 并刷新投影。

v1 的 `payload` 保持为有类型对象但 Schema 暂允许附加字段，以便先固定传输和
生命周期语义。将来每一种 `eventKind` 会在不改变 v1 已发布字段含义的前提下收紧
为独立 Schema。

## 关闭与恢复

- Tauri 关闭先停止接收新提交，再请求 graceful shutdown，等待有限时间。
- 若超时，仅终止已验证的 child PID，并在下次启动前查询会话快照；不得把未完成
  的内存事件当成已持久化结果。
- sidecar 崩溃时，Tauri 显示可操作错误并允许受控重启；重启后一定先 health、
  snapshot，再允许提交。
- 同一 state root 由跨进程锁保护；Wails 或另一个 Tauri 实例持锁时拒绝启动。

机器可校验的 v1 envelope 位于 `docs/tauri/protocol/v1.schema.json`。该 Schema 是
源文件：`go run ./cmd/desktop-bridge-protocol-gen` 从它生成 TypeScript 与 Rust 的
wire 镜像，`-check` 在 CI 中拒绝漂移。Go 侧 DTO 由其自身持有：Go 是 wire 格式的
生产者，Schema 描述它而非反向生成它。host 自身的 command 载荷（`BridgeStatus`、
`BridgeSnapshot`）不属于 wire 协议，仍手写。

## Global 会话本地工作区

未指定 `workspaceRoot` 的 Preview 会话使用当前 `REASONIX_HOME/global-workspace`，
目录按需创建为 `0700`，已有普通目录保留，文件或符号链接拒绝作为隐式默认目录。
与 Wails Global 工作区约定一致，默认托管档案仍位于 Preview 私有数据根；
不会把进程启动目录或安装目录作为默认项目。显式项目的目录保持原选择。

`session.workspaceRoot` 与会话目录中的 root 仍表示项目归属；Global 保持空值，
不把此实现目录写进侧栏项目、会话身份记录或导入元数据。
原生打开采用新只读 `workspace-target`，在 runtime ownership 锁内查询实际目录，
拒绝未知、已切换、关闭或未实现此能力的 runtime，且不创建/切换会话。
响应为 `protocolVersion/sessionId/workspaceRoot`；Rust 校验版本、身份、绝对路径和
当前目录存在性，WebView 只提交会话 ID 与固定安装应用 ID。
