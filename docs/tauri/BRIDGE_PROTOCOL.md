# Reasonix Desktop Bridge Protocol v1（草案）

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
| 提交 | `POST /v1/sessions/{sessionId}:submit` | `X-Reasonix-Request-ID` 去重 |
| 取消 | `POST /v1/sessions/{sessionId}:cancel` | 是 |
| 工具审批 | `POST /v1/sessions/{sessionId}:approve` | 可安全重试 |
| 回答 `ask` | `POST /v1/sessions/{sessionId}:answer` | 可安全重试 |
| 回答 MCP 交互 | `POST /v1/sessions/{sessionId}:mcp` | 可安全重试 |
| 重放待处理提示 | `POST /v1/sessions/{sessionId}:replay-prompts` | 是 |
| 流订阅 | `GET /v1/events?afterSequence=N` | 可重连 |
| 正常关闭 | `POST /v1:shutdown` | 是 |

当前 bridge 已实现 health、脱敏 Provider 摘要、建/开会话、空闲会话的显式切换、重命名与删除、快照、可见历史、工作区附件、逐层工作区目录、受限工作区文件预览、Git 变更列表与受限 diff、submit、cancel、审批/ask/MCP 提示回答、断线后的待处理提示重放、SSE 事件与正常关闭。工作区目录只返回当前会话工作区下的一层，隐藏常见构建产物、依赖目录和 `.git`，每层最多 200 项；返回值只有相对路径，`..` 越界会被拒绝。文件预览只读取有限大小的常规文件；二进制或非法 UTF-8 文件仅返回 `binary: true`，不会把原始字节交给 renderer。Git 变更查询只执行固定的只读 Git 子命令，diff 输出限制为 2 MiB；非 Git 工作区会返回 `gitAvailable: false`，不会伪造“干净”。会话自定义标题写入 core 的 `.jsonl.meta`，不改动 transcript；标题为空、含控制字符或超过 120 个 Unicode 字符时会被拒绝。删除走 core 自身的产物清理（transcript、事件日志与 sidecar、guardian、inbox、checkpoint、子 agent 记录、cleanup 标记），只允许删除当前持有且空闲的会话，避免 host 误删另一个 host 正在使用的会话；删除成功后 bridge 释放该控制器且不再写回快照，否则会把刚删掉的文件重新创建出来。对已不存在的会话重复执行删除返回 `not_found`，而带同一 `X-Reasonix-Request-ID` 的传输重试重放首次响应。

单文件恢复沿用 core 的两阶段检查点事务。预览不会写入文件；提交会重新校验方案与文件指纹，旧方案和会话运行中的提交会失败。会话首次修改后文件再次变化时，界面须显示覆盖风险，并让用户在最终操作中显式选择 `overwrite_checkpoint`。`keep_current` 由界面取消表示。恢复成功后若 core 返回可撤销事务 ID，界面提供一次撤销入口；撤销前重新核对文件指纹，后续手工修改会使撤销失败。bridge 只回传相对路径、冲突原因和恢复数量，不回传原始文件内容或绝对路径。此功能只恢复一个由当前会话检查点持有的文件，不回退对话历史。

检查点页接入 core 的 `RewindCode` 事务。预览列出该轮及以后被捕获的文件，最多展示 60 条路径，同时返回完整计数；发现文件冲突时禁用提交，不提供多文件强制覆盖。项目覆盖缺口允许提交前必须勾选风险确认，bridge 再次检查该标志；没有确认时即使绕开前端也不能提交。成功的代码回滚可经前述撤销事务入口恢复操作前文件，且不改变对话历史。对话分叉或对话与文件组合回滚仍未接入 Preview。

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
