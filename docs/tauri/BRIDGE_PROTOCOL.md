# Reasonix Desktop Bridge Protocol v1（草案）

## 范围

本协议连接 Tauri host 与本地 Go sidecar。它不是公网 API，sidecar 只允许本机
Tauri host 访问。v1 首批覆盖：健康检查、会话快照、创建/打开会话、工作区附件、
提交消息、取消、Agent 流事件与正常关闭。

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
| 设置新会话默认模型 | `POST /v1/settings/default-model` | `X-Reasonix-Request-ID` 去重 |
| 建/开会话 | `POST /v1/sessions:open` | `X-Reasonix-Request-ID` 去重 |
| 显式切换会话 | `POST /v1/sessions:switch` | `X-Reasonix-Request-ID` 去重；仅空闲会话 |
| 重命名会话 | `PATCH /v1/sessions/{sessionId}/title` | `X-Reasonix-Request-ID` 去重；仅空闲会话 |
| 删除会话 | `DELETE /v1/sessions/{sessionId}` | `X-Reasonix-Request-ID` 去重；仅空闲且为当前持有的会话 |
| 会话快照 | `GET /v1/sessions/{sessionId}/snapshot` | 是 |
| 可见历史 | `GET /v1/sessions/{sessionId}/history` | 是 |
| 附加文件 | `POST /v1/sessions/{sessionId}:attach` | `X-Reasonix-Request-ID` 去重 |
| 提交 | `POST /v1/sessions/{sessionId}:submit` | `X-Reasonix-Request-ID` 去重 |
| 取消 | `POST /v1/sessions/{sessionId}:cancel` | 是 |
| 流订阅 | `GET /v1/events?afterSequence=N` | 可重连 |
| 正常关闭 | `POST /v1:shutdown` | 是 |

当前 bridge 已实现 health、脱敏 Provider 摘要、建/开会话、空闲会话的显式切换、重命名与删除、快照、可见历史、工作区附件、submit、cancel、SSE 事件与正常关闭。会话自定义标题写入 core 的 `.jsonl.meta`，不改动 transcript；标题为空、含控制字符或超过 120 个 Unicode 字符时会被拒绝。删除走 core 自身的产物清理（transcript、事件日志与 sidecar、guardian、inbox、checkpoint、子 agent 记录、cleanup 标记），只允许删除当前持有且空闲的会话，避免 host 误删另一个 host 正在使用的会话；删除成功后 bridge 释放该控制器且不再写回快照，否则会把刚删掉的文件重新创建出来。对已不存在的会话重复执行删除返回 `not_found`，而带同一 `X-Reasonix-Request-ID` 的传输重试重放首次响应。
Provider 摘要仅包含配置名称、类型、已配置模型 ID、模型数量、是否需要凭据、凭据是否已配置及用户默认模型；不返回 endpoint、凭据变量名、密钥或请求 headers。模型 ID 仅用于本地下拉选择。
默认模型修改复用 `internal/config` 的选择校验、用户配置锁和窄写入，只改变新会话默认值，不重建或改写当前会话；选项只包括已启用且凭据可用的模型。工作区 `reasonix.toml` 仍可覆盖用户默认值。
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
状态返回 `conflict`，不会停止或替换用户的活动回合。

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
