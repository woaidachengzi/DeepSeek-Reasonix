# Preview SQLite 会话事件库 RFC

状态：SQLite DAG 读写已接入受管 Preview，仍在验证期。旧 Wails/CLI 默认关闭此 capability；Tauri 显式自定义 profile 也关闭。没有对真实用户 profile 执行导入或写入。

已实现：identity SQLite schema v9 事件表、事件 JSONL 水位和 transcript checkpoint 水位；按 session 事务追加、读回、源指纹导入、导入前后的 legacy reducer 语义核验、generation/sequence CAS 轮转、payload 哈希与索引字段校验、精确批次重试幂等。导入源只有在 JSONL 与 SQLite reducer 的语义状态一致后才标记为已验证。`agent.Session.Save`、DAG reducer、解锁追加和轮转已走 managed Preview capability；JSONL 两个投影从 SQLite 重建。Tauri 只给私有 managed profile 设置 capability，自定义 profile 清除 capability。主要实现位于 `internal/sessionidentity/events.go`、`internal/agent/session_dag_sqlite.go` 和 `desktop/tauri/src/data_profile.rs`。

## 目标与边界

Reasonix 已经有 append-only 的 schema-2 会话 DAG：`<session>.events.jsonl` 保存消息、分支、重命名、回退、删改、turn 边界、压缩和 checkpoint 事件；`<session>.jsonl` 是为历史读取器保留的当前主分支投影。改造要把事件 DAG 的持久权威切到 Preview state home 的 SQLite，同时继续产出可由旧读取器打开的 JSONL 投影。消息和工具内容不能在数据库与文件间形成两个并列权威。

这不是“把现有 JSONL 再写一份 SQLite”。切换后 SQLite 事务提交才代表会话事件已持久化；JSONL 是带事件水位与摘要的重建投影。投影失败时保留已提交事件、记录待重建状态，不回退到 JSONL 写入。旧 Wails writer 不识别此协议，绝不允许与 SQLite writer 共用同一 profile。

长期记忆、checkpoint writer prompt 与记忆 FTS 属于独立 S3，不随事件库切换；沿用既有 Markdown 与 `session-context`。UI 协议与 Tauri `tauri-<id>` 身份不变。

## 当前写入链

`internal/agent.Session.Save` 生成 schema-2 `sessionDAGEntry`，经 `save_dag.go`、`session_dag_writer.go` 写入 `*.events.jsonl`；`session_dag_replay.go` 从事件日志重放 DAG；`session_dag_rotate.go` 对日志做 generation rotation；`save.go`/投影路径更新 `*.jsonl`。`*.turns.jsonl` 另存本地执行生命周期，`.meta`、recovery/context、approval/job 侧车继续作为独立 artifact。当前事件记录已有 JSON schema、writer ID、消息 DAG、哈希链、截断尾修复和字节/记录预算。

第一步必须保持现有事件语义和 JSON 编码，先替换持久化适配器；不重写 DAG reducer、turn-loop、工具审批协议或压缩语义。只把 JSONL 放进 SQLite BLOB 而不保留有类型的可索引字段不算最终结构；但投影器可先消费同一规范化事件流，之后再拆分 message/part 表。

## 目标表与不变量

SQLite 与 `session-state-v1.sqlite` 共用一个受 profile gate 保护的事务边界，避免跨两个数据库提交 identity 与事件。目标至少包含：

```sql
CREATE TABLE session_events (
  session_id TEXT NOT NULL REFERENCES sessions(id),
  sequence INTEGER NOT NULL CHECK (sequence > 0),
  event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  head_id TEXT NOT NULL DEFAULT '',
  parent_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  writer_id TEXT NOT NULL DEFAULT '',
  created_at_ms INTEGER NOT NULL,
  payload_json BLOB NOT NULL,
  payload_sha256 TEXT NOT NULL,
  PRIMARY KEY (session_id, sequence),
  UNIQUE (session_id, event_id)
);
CREATE INDEX session_events_head_sequence ON session_events(session_id, head_id, sequence);
```

再以 session-level revision/watermark 记录数据库最后序号、所选 head、projection watermark/hash、旧日志导入状态。后续版本可增加 `session_messages`/`session_parts` 的可查询投影，但不能要求它们复制一份可独立写入的真相。

- 每次追加一批事件、分配 sequence、推进 head/revision 和写入 message/part 投影在同一 SQLite 事务中；ID/sequence 唯一约束承担并发去重。
- SQLite commit 是唯一持久化确认点。未 commit 的事件对 reader 不可见；crash 后从已 commit 最大 sequence 继续。
- replay 保持 schema/version/type 检查、事件 hash、DAG parent、writer lease 与现有安全预算；不因切库放宽损坏日志的 fail-closed 行为。
- `.jsonl` 投影携带/侧存自己的 session ID、数据库 generation 和 sequence watermark。投影过期时从 SQLite 重建；不得把旧投影反向覆盖更新的数据库。
- 分支、rewind、redact、system patch、turn begin/end、compression、approval result 和 shutdown append 都必须经过同一 writer 接口。

## 导入与上线

1. **隔离导入器**：只从受控快照读取 `*.events.jsonl`/`*.jsonl`，保留源文件。记录源大小与 SHA-256；逐事件验证 schema/type、JSON、顺序和可重放性；整个 session 在一个事务里导入。重复导入以 source fingerprint 幂等；变更的来源拒绝覆盖已导入事件。若只有 transcript checkpoint，则明确标记 legacy checkpoint import，不能伪造原始 DAG 分支。
2. **影子回放**：同一导入数据分别走现有 JSONL reducer 与 SQLite reducer；比较每个 head、selected head、消息 ID/顺序/内容、turn 边界、redaction、compaction 和 checkpoint watermarks。差异写 path-free 报告并禁止切换。
3. **Preview writer gate**：新增受管 profile capability。当前 Tauri bridge 显式启用；自定义 `REASONIX_HOME`、Wails、CLI 默认关闭。事务提交后同步尝试更新 JSONL 兼容投影；失败保留 `projection_pending`，从 DB 重建；下一次打开先恢复投影再交给旧 controller reader。
4. **故障注入与回退**：测试 commit 前/后 kill、projection replace 中断、磁盘满、WAL 损坏、duplicate writer、同 ID 重试、SQLite future schema、旧版本读 checkpoint、所有 DAG 操作回放、导出/重导。降级先停 writer、从 DB 导出 checkpoint 与事件快照、hash 校验后才切换；禁止自动把旧 JSONL 合并回较新的 SQLite。
5. **profile promotion**：只在新的隔离 Preview profile 做完整恢复演练。真实稳定版/Wails 数据导入需要单独的用户数据迁移流程、确认所有旧 writer 已停止、备份和恢复演练；本 RFC 不授权其执行。

## 验收门槛

- 隔离快照导入重复运行稳定幂等，源 JSONL/侧车字节不变；损坏、hash 不符和不支持 schema 均不部分写入。
- SQLite 与当前 reducer 的全量 DAG 结果逐字段相同；100k event/128 MiB 边界、长消息、多工具、图片、分支、rewind、redaction 与 compaction 有确定性测试。
- 对所有提交/投影步骤注入 crash；恢复后没有丢失已确认事件、没有重复事件，也没有回退到旧 checkpoint。
- 整个写路径没有 SQLite/JSONL 双写权威窗口；旧 writer 同 profile 启动必须被停写操作规程挡住。受管 Preview 以外的 backend 路径保持原样。
- bridge、Tauri UI、导出/恢复和旧 host 兼容矩阵验证通过后才宣称 SQLite 会话存储完成。仅身份目录使用 SQLite 不能算此项完成。

## 当前实施切片与剩余验证

现有测试覆盖临时 profile 的 Session.Save、SQLite writer capability 缺省关闭且拒绝 profile 外或非 Tauri session 路径、schema-2 导入与已验证水位、损坏 legacy log 导入时不写入部分事件且源字节不变、仅有 schema-1 transcript checkpoint 时升级为标记 `upgraded_from` 的线性 SQLite 主分支、不伪造旧分支，并切回旧 writer 后由 schema-1 reader 成功读取 checkpoint；导入提交后崩溃且源日志消失时 fail-closed、JSONL/SQLite shadow replay 对比完整 reducer 状态（head、message/time 链、system/patch、redaction、compaction、turn、writer 与 orphan）、分支/rewind/redaction/压缩标记/turn 边界/writer 元数据、轮转 generation。事件存储现在以独立子进程 `os.Exit` 实测提交后、投影临时文件写好但原子替换前、投影发布后水位更新前三个崩溃点；每次重启均从 SQLite 恢复事件和投影水位。另有 event-log 投影损坏恢复、checkpoint 恢复、SQLite 回放预算拒绝，以及从含压缩摘要的 SQLite 当前选中 head 导出 schema-1、由旧 reader 重载，再将 schema-2 流导入另一会话并比较 reducer 状态；另在隔离临时 profile 导出 Preview 会话，再由旧 Wails 仓库中的 `LoadSession` 直接逐字段读取，覆盖 system/user/assistant、原文、图像、reasoning、工具参数、写意图、diff、工具结果关联及 assistant 上的 `allow_once` 审批回执，跨仓库回读通过。另有 SQLite `max_page_count` 耗尽故障注入，确认追加失败时整批事件回滚且已提交序号不变；此外有 1200 条批次的 SQLite 绑定参数边界测试、100,000 条事件的实际读回/回放测试、128 MiB 字节限额的精确算术边界，以及 120 MiB 消息在 SQLite 写入、JSONL 投影和 DAG 重放中的真实往返测试。新增的工具调用、结果和图像测试同时比对 SQLite reducer 与旧 schema-1 导出再加载的结构化消息（包括工具参数、写意图、diff、结果 ID/关联、图像 data URL、原始文本、reasoning 字段和审批回执）；审批 allow_once/deny receipt、取消状态在独立 turns ledger 的重开测试也已覆盖。identity store 的 abrupt-exit 测试同时核实 WAL 恢复后已提交事件仍在；新增故障注入破坏已提交帧校验和，确认打开前拒绝且主库/WAL 字节不变，并覆盖 schema 页之后的帧损坏。真实 Rust supervisor + Go sidecar 的临时 Preview profile 测试现已覆盖两条链路：旧 schema-2 导入后重启并由 SQLite 修复事件投影；全新会话提交真实 bridge turn，在 sidecar 关停前确认兼容 checkpoint 和 SQLite 事件投影都含有回答，再重启、损坏投影并从 SQLite 恢复，随后从重启后的 controller 历史读回回答。bridge runtime 增加了 settled-turn snapshot monitor，并把取消、审批和 prompt 答复后的变更也标记为待保存。存储相关 Go 包已通过完整测试；`sessionidentity`、`agent`、`fileutil` 完整 Go 测试通过，当前 sidecar 重新构建后，managed Preview 真实导入、turn 写入、sidecar 重启及损坏投影修复集成测试通过，完整 Tauri suite 90 项通过。授权边界测试确认同会话 controller rebuild 保留 session grants，而新会话 controller 不继承旧会话的工具授权、Plan 命令信任或写根。managed Preview gate 已打开，但 promotion 和“完成”结论仍需全面兼容矩阵及恢复演练。

两个独立 SQLite Store 连接的并发测试同时追加不同事件并重试同一事件批次，连续运行 10 次均得到连续唯一序号；同 ID 重试只生成一条事件。并发 generation 轮转也经双连接竞态测试，确保 CAS 只允许一个 writer 提交完整替换。

Preview SQLite 追加现会在单个事务中检查最终事件条数与 canonical JSONL projection 字节数，拒绝超限批次而不提交任何事件；exact retry 只在事件 ID、payload hash 和原始连续序号顺序均一致时幂等成功，重排或部分重试返回冲突。generation 轮转在替换提交前检查整份新 projection 的条数与字节预算。记录数/字节数拒绝测试确认 SQLite sequence、generation 和既有 JSONL projection 均不变；`sessionidentity`、`agent`、`fileutil` 完整 Go 测试在该改动后通过。

追加与 generation 轮转现在还会在同一 SQLite 事务里先顺序扫描并校验当前 event stream 的连续 sequence、索引列和 payload SHA-256；发现已有行损坏时拒绝追加或删除替换。故障注入确认损坏 payload 下两种 writer 都保留原 generation、sequence 和行数。该检查与事务内预算门禁一起防止“返回失败前已提交”及“已知损坏流被继续扩写/轮转”。

离线 snapshot/restore 的隔离测试现还覆盖 SQLite 会话事件与水位：在 Preview 临时 profile 写入身份、事件流、event-log 投影、transcript 和 event/checkpoint watermarks，生成并校验离线快照，恢复到不同的临时 profile 后核对事件内容及水位，并从恢复库继续追加事件。该测试证明受管快照 staging 能带回会话事件数据库状态；它不证明旧 writer 已停写，也不构成真实 profile 的恢复演练。

2026-09-26 对当前工作树执行 `GOCACHE=/private/tmp/reasonix-go-cache go test ./... -count=1`，仓库全部 Go 包通过；Tauri `cargo test --locked --manifest-path desktop/tauri/Cargo.toml` 的 90 项也通过。旧 Wails writer 停写、真实发布二进制认证与真实 profile 恢复仍是独立发布门禁，不由源码测试替代。

旧 Tauri host 的隔离兼容脚本也已修正为测试当前 bridge 工作树：从 bridge `HEAD` 归档后叠加已跟踪差异与非忽略未跟踪文件，再构建 sidecar。修正后的候选旧 host 完整 28 项测试通过；本次 bridge 和 host SHA-256、隔离产物位置记入 `SESSION_STORAGE_IMPLEMENTATION_V3.md`。这仍不等于实际发布旧 host 或旧 Wails writer 的停写认证。
