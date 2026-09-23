# 方案：MiMo 式会话存储改造（身份层 + 记忆层）

> 状态：提案，待评审
> 评审与修正：[MiMo 式会话存储：评审与分阶段落地](./MIMO_STYLE_SESSION_STORAGE_REVIEW.md)。本文件保留原始设想；其中的 bridge ID 形态、现有 SQLite 能力和“可重建索引中的永久 UUID”结论均以评审为准。
>
> 起因：Reasonix 1.38.4–1.38.10 在 `internal/session` 上以 5 天 41 次提交连续翻新存储格式与身份模型，fix 占比约 40%，bug 反馈集中。本方案主张采用**只加不改**的 MiMo 式分层，而非格式级重写。
>
> 基线：`v1.38.3`（当前 Tauri Preview 所钉版本）。
> 范围：会话身份与索引；不改变 jsonl 内容格式，不重写 Agent 执行权威。

---

## 1. 问题陈述

### 1.1 现状（v1.38.3）

```text
~/.reasonix/sessions/<workspace-derived>/<stamp>-<model>.jsonl   ← 真相源，追加写 + flock
<id>.events.jsonl / .turns.jsonl / .conflicts.jsonl / .guardian.jsonl   ← 辅助日志
<id>.recovery.json / .context.json / .pinned-context.json                ← JSON 侧车
historycatalog (SQLite, projectiondb)                                     ← 派生索引：roots/sources/FTS
```

关键事实：

- **会话身份 = 文件路径**（bot/Desktop/inbox 引用形如 `path:/sessions/....jsonl`）。
- 文件名含模型提示（`...-deepseek.jsonl`），但仅用于**新建**，不影响读取。
- SQLite 目录已存在，但**只做搜索与扫描状态**，不承担稳定身份。

### 1.2 路径身份的弱点（真实影响面）

| 情景 | 是否受影响 |
| --- | --- |
| 同一会话中途换模型 | ✅ 不受影响（路径不变） |
| 换 API 账号 | ✅ 不受影响（本地文件） |
| 文件移动 / 工作区路径变化 / 重命名 | ❌ 所有外部引用断裂 |
| 复制项目产生第二份 jsonl | ❌ 视为两个会话，无法去重 |
| 归档 / 回收站 / 恢复 | ❌ 引用易断（1.38.10 一串修复即源于此） |
| 跨设备同步后路径变化 | ❌ 身份漂移 |

类比：用"家庭住址"当身份证号 — 搬家即改身份。

### 1.3 上游 1.38.4–1.38.10 的教训

- `internal/session`：**89 文件，+22,543 行**，41 个提交（约 1/3 的 core 提交）。
- 同期叠加：v4 framed codec、streaming content store、SessionID-only、Harness turn-loop、归档/回收站统一。
- **fix/repair 提交约 57/143 ≈ 40%**，修复链显示"推倒 → 再推倒 → 再修"。

结论：架构方向（稳定身份、快速恢复）合理，但**在同一迭代内同时换格式 + 身份 + 执行权威**导致回归面失控。本方案吸取该教训：**格式冻结，只加身份与索引层**。

---

## 2. 设计目标与非目标

### 2.1 目标

1. **稳定会话 ID（UUID）** 与路径解耦；外部引用只持 ID。
2. **路径降级为可变投影**；移动/改名仅更新索引，不断引用。
3. **旧 jsonl 只读兼容迁移**：不重写、不移动源文件，可重跑、可回滚。
4. **索引可全量重建**：jsonl 仍是唯一真相源。
5. **记忆层与会话迁移解耦**：项目级记忆不依赖单个会话文件存活。
6. **与当前 Tauri bridge 兼容**：迁移期 host 不感知内部格式变化。

### 2.2 非目标（本方案明确不做）

- 不把 jsonl 改为 v4/分帧/内容寻址等新二进制或重分块格式。
- 不改 turn-loop / Harness 执行权威 / provider 请求格式。
- 不在同一 PR 内切换 Desktop 身份引用（分阶段）。
- 不引入需要写权限的"迁移即转换"对源文件的批量重写。
- 不替代上游 1.38.10+ 的会话重构；本方案是**并行可落地的增量层**。

---

## 3. 目标架构（MiMo 式分层）

```text
┌─────────────────────────────────────────────────────────┐
│ 记忆层（跨会话，与文件位置无关）                          │
│   项目 MEMORY.md / notes.md / tasks/<id>/progress.md     │
│   （MiMo 式分层；与历史迁移解耦，可先落地）               │
└─────────────────────────────────────────────────────────┘
                          │ 注入 / 写入
┌─────────────────────────────────────────────────────────┐
│ 身份 + 索引层（新增，旁路）                               │
│   sessions 表：                                          │
│     id UUID PK, path UNIQUE, fingerprint,                │
│     title, model_hint, created_at, last_activity_at,     │
│     message_count, source_kind                           │
│   FTS 表：标题/预览/正文（可从 jsonl 重建）               │
│   path → id / id → path 双向查询                         │
└─────────────────────────────────────────────────────────┘
                          │ 只读扫描 / 增量 fingerprint
┌─────────────────────────────────────────────────────────┐
│ 真相源（保持 1.38.3 格式，只读对待）                      │
│   <stamp>-<model>.jsonl（O_APPEND + flock）             │
│   既有侧车（.events/.context/.recovery…）原样保留         │
│   historycatalog / projectiondb 视为可重建的派生索引       │
└─────────────────────────────────────────────────────────┘
```

### 3.1 与现状的对应关系

| 层 | 1.38.3 现状 | 本方案 |
| --- | --- | --- |
| 真相源 jsonl | ✅ 有 | ✅ 不动（格式冻结） |
| 侧车 | ✅ 有 | ✅ 不动 |
| historycatalog | ✅ 有（搜索/扫描） | ✅ 保留或演进为 sessions 表宿主 |
| 稳定 UUID 身份 | ❌ 无（路径即身份） | ✅ 新增 |
| 跨会话记忆文件 | 部分（REASONIX.md 等） | ✅ 对齐 MiMo 分层，独立演进 |

---

## 4. 数据模型

### 4.1 sessions 表（核心）

```sql
CREATE TABLE sessions (
  id            TEXT PRIMARY KEY,          -- UUID v4，首次扫描分配，永不改变
  path          TEXT NOT NULL UNIQUE,      -- 当前 jsonl 绝对路径；可 UPDATE
  fingerprint   TEXT NOT NULL,             -- 内容指纹（大小+mtime 或 partial hash），用于增量
  title         TEXT NOT NULL DEFAULT '',
  model_hint    TEXT NOT NULL DEFAULT '',  -- 仅来自文件名，展示用
  workspace_root TEXT NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL DEFAULT 0,
  last_activity_at INTEGER NOT NULL DEFAULT 0,
  message_count INTEGER NOT NULL DEFAULT 0,
  source_kind   TEXT NOT NULL DEFAULT 'jsonl',
  indexed_at    INTEGER NOT NULL DEFAULT 0,
  missing       INTEGER NOT NULL DEFAULT 0 -- 路径暂不可达时置 1，不删行
);

CREATE UNIQUE INDEX idx_sessions_path ON sessions(path);
CREATE INDEX idx_sessions_activity ON sessions(last_activity_at DESC);
```

设计要点：

- **`id` 永不因移动而变**；`path` 允许变更。
- **`missing` 标记**：文件暂时不可达（外置盘/同步中）不删身份，避免引用二次断裂。
- **`fingerprint` 幂等**：重扫只 upsert，中断可重跑。
- `model_hint` 不参与任何业务判定 — 回答"换模型是否丢历史"：不丢。

### 4.2 分配 UUID 的规则

- 源 jsonl **不写入** UUID（避免改真相源）。
- UUID 只存在于索引层；`path → id` 映射持久化。
- 若同一 path 被删除后重新出现且 fingerprint 变化：视为**新会话**（新 UUID），旧 id 保留 `missing=1` 或归档（可配置）。
- 同一 content fingerprint 出现在两个 path：**允许两条会话**（不自动合并，避免误伤）；去重作为后续可选功能，依赖内容闭包比对。

### 4.3 记忆层（与会话表解耦）

```text
REASONIX_STATE_HOME/           ← 或项目根
  MEMORY.md                    ← 项目常驻指令 / 架构决策（已有体系）
  memory/
    notes.md                   ← 草稿区
    tasks/<id>/progress.md     ← 任务进度
```

- 记忆**不引用 session path**；任务进度用 task id。
- 会话索引迁移失败不影响记忆层读写。

---

## 5. 迁移方案（安全迁移旧历史）

### 5.1 原则

1. **jsonl 只读**：不重写、不移动、不改名。
2. **备份先行**：迁移前整树快照。
3. **幂等可重跑**：按 `path` upsert，`fingerprint` 未变则跳过解析。
4. **旁路上线**：新表对旧代码不可见时，旧代码仍可工作。
5. **双轨期**：外部引用同时可解析 `id` 与 `path`。
6. **可验证**：抽样逐条对比 jsonl 与索引投影。

### 5.2 步骤

```text
Phase A — 备份与盘点
  1. cp -R ~/.reasonix ~/.reasonix.bak-<timestamp>
  2. 用 IsSessionTranscriptName 扫描，输出：文件数、总字节、损坏清单
  3. 对损坏文件：只记录、不修复、不删除（沿用 guardian/recovery 现状）

Phase B — 首次索引（只读）
  4. 对每个 jsonl：解析行数、首末时间、标题预览、fingerprint
  5. INSERT sessions(id=uuid4, path=..., ...)
  6. 可选：填充 FTS（从 jsonl 投影 user/assistant 可见文本，
     复用 bridgeHistoryMessageVisible 的过滤规则）

Phase C — 双轨
  7. 读路径：resolve(path)→id 与 resolve(id)→path 均可用
  8. 写路径：新会话创建时同时写 sessions 行（id 预分配）
  9. 文件系统事件（rename/move）：UPDATE sessions.path WHERE old_path
     （找不到则标 missing=1，不删行）

Phase D — 切换引用
  10. Desktop 最近列表 / bot 映射 / inbox：改为只存 id
  11. bridge 响应同时携带 sessionId 与 path（host 用 sessionId）

Phase E — 收尾
  12. 旧 path 引用代码路径打日志，确认零命中后移除
  13. 备份保留 N 天后清理
```

### 5.3 回滚

- 索引层可整库删除，jsonl 原样可用（旧代码不读新表则完全兼容）。
- 双轨期间随时可停用 sessions 表。
- 禁止：为"迁移"目的批量 rename 或重写 jsonl。

### 5.4 验证清单

- [ ] 抽样 ≥20 个旧会话：索引 message_count/首尾消息与 jsonl 一致
- [ ] 移动一个测试会话文件：外部引用仍可通过 id 打开
- [ ] 中断迁移后重跑：结果幂等、无重复行
- [ ] 删除索引库后：1.38.3 旧行为完全恢复
- [ ] bridge snapshot/history 回归：与迁移前逐字段对比
- [ ] 换模型新建会话 + 打开旧会话：历史均完整

---

## 6. 与 Tauri bridge 的接口影响

### 6.1 协议增量（非破坏）

在 `docs/tauri/protocol/v1.schema.json` 中，会话对象增加（可选字段，旧 host 忽略）：

```json
{
  "sessionId": "0192f0c0-...",
  "id": "path:/sessions/....jsonl"
}
```

约定：

- **新字段 `sessionId` = UUID**，host 与 workbench catalog 逐步只用它。
- 旧字段 `id`（路径形态）在双轨期保留；`protocolVersion` 仍为 1（加可选字段不算破坏）；若需强约束再升 v2。
- `ready.json` 可增 `sessionSchema: "jsonl-path-v1" | "jsonl-uuid-index-v1"`，host 据此拒绝不兼容 sidecar（防止混用未迁移与已迁移环境）。

### 6.2 bridge 命令

现有 `open/switch/submit/history/snapshot` 语义不变：

- 请求参数接受 `sessionId`（UUID）**或**旧 `id`（path 形态），bridge 内部解析。
- 归一化后一律按 UUID 调 runtime。

### 6.3 workbench catalog（Tauri host）

`WorkbenchSession.sessionId` 字段含义从"path 派生 id"切换为"UUID"；迁移期存 UUID，`title/workspaceRoot` 不变。

---

## 7. 分阶段落地（吸取 1.38.10 教训）

> 每阶段独立可合并、可回滚；禁止单版本内同时动"格式 + 身份 + 执行"。

| 阶段 | 内容 | 退出标准 | 预估 |
| --- | --- | --- | --- |
| **S0** | 本方案评审 + sessions 表 schema RFC 冻结 | 另一模型/评审通过 schema | — |
| **S1** | 只读扫描器 + 幂等 upsert + 备份脚本；不接任何 UI | 抽样验证清单全绿 | 1–2 周 |
| **S2** | 双轨：新会话预分配 UUID；path/id 双向查询 | 新旧会话均可打开；中断重跑幂等 | 1–2 周 |
| **S3** | bridge 协议加 `sessionId`（可选字段） | 契约测试 + host 兼容 | 1 周 |
| **S4** | Tauri host / workbench 切换到 UUID 引用 | 移动文件后最近列表仍可用 | 1 周 |
| **S5** | 记忆层（MEMORY.md 分层对齐）——可与 S1 并行 | 记忆读写不依赖会话路径 | 并行 |
| **S6** | 可选：FTS 增强、跨 path 去重、归档统一 | 单独立项 | 以后 |

**对比上游节奏**：S1–S4 是"只加旁路"，任何一步失败都可停在上一步；上游 5 天 41 次提交属于反模式，本方案显式禁止该节奏。

---

## 8. 风险与缓解

| 风险 | 等级 | 缓解 |
| --- | --- | --- |
| 扫描损坏 jsonl 导致索引错误 | 中 | 坏文件标记 `missing/error`，跳过不中断；清单人工处理 |
| path UNIQUE 与大小写/符号链接 | 中 | 存储规范化后的绝对路径 + 记录 `realpath`；macOS/Windows 分别测 |
| 双轨期 path 与 id 不一致 | 中 | 唯一写者：文件事件与会话打开都走同一 resolver；不变量测试 |
| 与上游 1.38.10+ 会话重构冲突 | 高 | 本方案钉 1.38.3 格式；升级 sidecar 前跑 bridge 回归；`sessionSchema` 握手 |
| 迁移误伤用户数据 | 高 | 只读源文件 + 强制备份 + 禁止 rename 式迁移 |
| UUID 分配后 path 丢失（文件真没了） | 低 | `missing=1` 保留行；导出工具从 jsonl 恢复 |
| 外部引用遗漏（bot/inbox） | 中 | Phase D 前 grep 全部 `path:` 引用点，列入切换清单 |

---

## 9. 评审请求（给另一模型/评审者的问题）

1. **是否同意"格式冻结 + 只加身份/索引层"优于追平 1.38.10 的存储格式重写？**
2. **UUID 不写入 jsonl、仅存索引表**，是否可接受？（备选：迁移时在 jsonl 追加一条 `session_header` 行 — 但这违反"只读真相源"，需要权衡）
3. **path UNIQUE + missing 标记**是否足以覆盖外置盘/同步盘场景？
4. **双轨期长度**建议以"旧引用零命中"还是"固定版本数"为退出条件？
5. **bridge `sessionId` 可选字段**是否应在 protocol v1 内加，还是直接发 v2？
6. 记忆层与会话索引**完全解耦**是否过度，或应共享同一 SQLite 库？

---

## 10. 一句话提案

**保留 1.38.3 的 jsonl 真相源不变，在其上旁路增加 MiMo 式 sessions 身份表与可选记忆层；旧历史通过只读扫描一次性入索引；所有外部引用逐步从路径切换为 UUID —— 以分阶段可回滚的方式，解决路径身份弱点，同时避开上游会话存储连环重构的覆辙。**
