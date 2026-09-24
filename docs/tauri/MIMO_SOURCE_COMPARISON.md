# MiMo Code 源码对照（基于本地克隆）

> 源码路径：`/Users/jerry/temp/app/MiMo-Code-main`
> 日期：2026-09-23
> 用途：核实「MiMo 式会话存储」提案中的假设；结论以源码为准。
> 关联：[原始提案](./MIMO_STYLE_SESSION_STORAGE_PROPOSAL.md) · [评审](./MIMO_STYLE_SESSION_STORAGE_REVIEW.md)

---

## 1. 总判断：MiMo 不是「JSONL 真相源 + 旁路索引」

**原提案写错了方向。** MiMo 的架构是：

```text
历史形态（已迁移）：
  JSON 文件布局 storage/{session,message,part,todo,...}/*.json
        │
        ▼  json-migration.ts 一次性导入（只读源、事务批量）
  SQLite（Drizzle ORM）← 当前权威
        │
        ├── session / message / part / todo / permission 表
        ├── history_fts（part 级检索）
        └── memory_fts（Markdown 记忆检索，可重建）

Markdown 文件（仍是真相源，不是索引）：
  <data>/memory/
    projects/<projectID>/MEMORY.md      项目记忆
    global/MEMORY.md                    全局记忆（agent 只读）
    sessions/<sessionID>/checkpoint.md  会话检查点
    sessions/<sessionID>/notes.md       草稿
    sessions/<sessionID>/tasks/<tid>/progress.md
```

关键文件：

| 主题 | 源码 |
| --- | --- |
| 会话表结构 | `packages/opencode/src/session/session.sql.ts` |
| JSON→SQLite 迁移 | `packages/opencode/src/storage/json-migration.ts` |
| SessionID 生成 | `packages/opencode/src/session/schema.ts` + `src/id/id.ts` |
| 记忆路径约定 | `packages/opencode/src/session/checkpoint-paths.ts` |
| 记忆 FTS 与对账 | `packages/opencode/src/memory/service.ts`, `reconcile.ts`, `fts.sql.ts` |
| 历史 FTS | `packages/opencode/src/history/fts.sql.ts` |
| 外部只读导入 | `packages/opencode/src/session/external-import.ts` |

---

## 2. 逐条核实原提案假设

| 原提案说法 | 源码事实 | 判定 |
| --- | --- | --- |
| MiMo = JSONL 真相源 + SQLite 索引 | **SQLite 是消息与会话的权威**；JSON 是迁移前形态 | ❌ 不成立 |
| 会话身份用 UUID | **`ses_` + 26 字符，时间可排序**（v1/v2 编码，见 `id/id.ts`），brand 类型 `SessionID` | ❌ 应改为自有 ID 方案 |
| 旧历史只读扫描建旁路索引 | 迁移是 **JSON→SQLite 的一次性权威切换**（`onConflictDoNothing`、孤儿跳过、进度回调） | ⚠️ 更激进 |
| path 是 sessions 表里的可变列 | **session 表没有 transcript path**；`directory` 是**工作区目录**，不是会话文件路径 | ❌ 概念混淆 |
| 记忆层与会话迁移解耦 | ✅ **完全正确**：记忆是 Markdown + 独立 `memory_fts`，用 `sessionID` 定位，不绑路径 | ✅ 成立 |
| fingerprint（size+mtime）做增量 | ✅ `reconcile.ts` 用 `` `${size}-${mtimeMs}` `` 跳过未变文件 | ✅ 成立（仅作扫描跳过，见评审 #2） |
| 外部数据导入应只读 | ✅ `external-import.ts` 对 Claude/Codex/OpenCode 只读打开后写入本库 | ✅ 成立 |

---

## 3. MiMo 会话身份怎么做的（源码级）

### 3.1 ID 形态

```text
ses_<26 chars>    例：ses_f3A9...（descending：时间大的可字典序靠前）
msg_<26 chars>    ascending（消息按创建顺序可扫）
prt_<26 chars>
```

- 实现：`packages/opencode/src/id/id.ts`
- 前缀 brand + Zod/Effect schema 校验（`session/schema.ts`）
- **历史迁移时 ID 来自文件名 basename**，不读 JSON 内容里的字段  
  （`json-migration.ts` 注释：*Derive all IDs from file paths, not JSON content*）

含义：MiMo 旧 JSON 时代 **文件名就是 `ses_…`**，路径是 `session/<projectID>/<sesID>.json`。  
**路径结构变化不影响身份，因为身份从来不是完整路径，而是路径末段的 ID。**

这正是 Reasonix 1.38.3 的差距：

```text
Reasonix 1.38.3：  身份 = 完整路径 …/20260614-120000-deepseek.jsonl
MiMo（JSON 时代）： 身份 = 文件名 ses_xxx；路径 = session/<project>/<ses_xxx>.json
MiMo（现在）：      身份 = SQLite 主键 ses_xxx；消息也在同一库
```

### 3.2 session 表（摘录字段）

`session.sql.ts`：`id` PK、`project_id` FK、`parent_id`、`context_from`、`slug`、`directory`（工作区）、`title`、`version`、归档/压缩时间戳、`last_checkpoint_message_id`、权限与 prompt 配置 JSON。

**没有**「当前 transcript 文件路径」列 — 因为 message/part 已在库内。

### 3.3 迁移工程特征（可借鉴）

- 预扫描全部文件 → 分批 1000 → `SAVEPOINT` + `onConflictDoNothing`
- `PRAGMA journal_mode=WAL`、`synchronous=OFF`（仅迁移期）
- 孤儿（父 project/session 不存在）跳过并计数
- 进度回调（Desktop 显示 `sqlite_waiting`）
- **幂等**：重复插入靠 conflict 忽略

---

## 4. MiMo 记忆层（与提案一致，可直接借鉴）

`checkpoint-paths.ts` 约定：

```text
<data>/memory/
  projects/<projectID>/MEMORY.md     ← 项目长期记忆（原 memory.md 已原子改名为 MEMORY.md）
  global/MEMORY.md                   ← 全局偏好，agent 侧只读、不自动创建
  sessions/<sessionID>/
    checkpoint.md                    ← v5 单文件检查点
    notes.md                         ← 主 agent 草稿
    tasks/<taskID>/progress.md       ← 子任务日志
```

检索：`memory_fts`（path UNIQUE、scope/type 索引、fingerprint）；**搜索前 lazy reconcile**（`checkpoint.memory_reconcile_on_search` 默认 true）。

对 Reasonix 的映射：

| MiMo | Reasonix 现有/建议 |
| --- | --- |
| `projects/<id>/MEMORY.md` | 已有 `REASONIX.md` / 记忆体系 — 可对齐命名与注入时机 |
| `sessions/<sid>/checkpoint.md` | 已有 `session-context` / `.context.json` — 复用注入入口（评审 #5） |
| `notes.md` / `tasks/*/progress.md` | 缺口，属 S5 记忆层 |
| `memory_fts` + reconcile | 新增可重建索引（阶段 3） |

**所有记忆路径只含 `sessionID`，不含绝对路径** — 这是「身份与位置解耦」在记忆层的直接体现。

---

## 5. 对实施方案的修正建议

结合 [评审文档](./MIMO_STYLE_SESSION_STORAGE_REVIEW.md) 与源码：

### 5.1 保持评审结论的阶段 1–3

评审已正确划定边界：

1. **持久身份库**（Preview state home SQLite）+ 只读扫描 JSONL  
2. **双轨 resolver**（打开/改名/删除/切换走同一入口）  
3. **检索与记忆**（复用 history FTS5 + memory Markdown + 预算注入）

这与 MiMo 的「身份进库、记忆走 Markdown、FTS 可重建」一致，且比 MiMo 的全量消息入库更保守 — **符合「吸取 1.38.10 教训」的初衷**。

### 5.2 身份 ID 建议改用自有方案（非 UUID、非路径）

参照 MiMo：

```text
推荐：ses_ + 时间可排序后缀（或 tauri- 前缀保持现有 host 兼容）
禁止：完整绝对路径
禁止：从「可删索引」里生成且无持久身份库的随机 UUID（评审 #1）
```

现有 Tauri `tauri-…` ID 可保留为**已登记 ID**（评审身份约束），新 ID 建议同前缀规范，避免两套生成器。

### 5.3 迁移姿态选「旁路」而非「权威切换」（阶段 1–3）

| | MiMo json-migration | 本项目阶段 1–3 |
| --- | --- | --- |
| 源数据 | JSON（将废弃） | **JSONL（继续为执行真相源）** |
| 目标 | SQLite 成为唯一权威 | SQLite 只做身份/工作台元数据 |
| 消息体 | 导入 message/part 表 | **不导入**（阶段 4 才考虑） |
| 源文件 | 迁移后可不再读 | **持续读写**（Agent 仍追加 jsonl） |

### 5.4 阶段 4（可选）才对标 MiMo 全量 SQLite

若未来做消息入库，必须单独 RFC，且具备：

- 回放/崩溃/审批/撤销/分支/压缩的完整测试（评审阶段 4）
- 影子写校验
- 旧 JSONL 保持只读可导出

**在阶段 4 完成前，不得对外宣称「已完成 MiMo 式全量迁移」。**

### 5.5 可直接抄的工程细节

1. **迁移 PRAGMA + SAVEPOINT 批处理 + 孤儿跳过 + 进度事件**（`json-migration.ts`）  
2. **memory fingerprint 跳过未变文件 + 搜索前 lazy reconcile**（`reconcile.ts`）  
3. **历史索引带 version/cursor/phase 的迁移状态表**（`history/fts.sql.ts` → `history_index_migration`）— 阶段 3 可借鉴  
4. **外部只读导入协议**（`external-import.ts`）— 若需从 Wails 稳定版导入，对齐此模式  
5. **记忆路径全部 sessionID 化**（`checkpoint-paths.ts`）

---

## 6. 修正后的一页架构（阶段 1–3）

```text
┌─ 持久（state home，备份范围内）────────────────────────────┐
│ session-state-v1.sqlite                                  │
│   session_identity(id PK, current_path, workspace,        │
│                    title, missing, created_at, …)         │
│   session_path_alias(old_path → id)  ← 显式移动才写        │
│ workbench / 运行态（过渡期）                               │
└───────────────────────────────────────────────────────────┘
              ▲ 写：唯一 Go sidecar resolver
              │ 读：bridge 查询
┌─ 执行真相源（可清理 cache 外的 Preview sessions）──────────┐
│ *.jsonl + sidecars   Agent 追加写（格式冻结 1.38.3）       │
└───────────────────────────────────────────────────────────┘
┌─ 可重建索引（cache）──────────────────────────────────────┐
│ session-catalog · history FTS5 ·（后续）memory FTS5        │
└───────────────────────────────────────────────────────────┘
┌─ 记忆真相源（Markdown，人工可编辑）───────────────────────┐
│ memory/projects/<pid>/MEMORY.md                           │
│ memory/sessions/<sid>/{checkpoint,notes}.md               │
│ memory/sessions/<sid>/tasks/<tid>/progress.md             │
│ memory/global/MEMORY.md                                   │
└───────────────────────────────────────────────────────────┘
```

---

## 7. 给评审者的问题（更新）

1. 阶段 1 身份 ID：沿用 `tauri-…` 并扩展规范，还是引入 `ses_` 风格新前缀？  
2. 是否需要 `session_path_alias` 表（评审已建议）还是单表 `current_path` 即可？  
3. 记忆目录布局：直接采用 MiMo 的 `memory/projects|sessions|global` 树，还是贴合 Reasonix 现有 `REASONIX.md`/`session-context` 命名？  
4. 阶段 3 是否引入 `history_index_migration` 式的 versioned cursor，还是继续现有 `historycatalog` rebuild？  
5. 从 Wails 稳定版导入是否对齐 MiMo `external-import` 的只读+进度+幂等协议？

---

## 8. 一句话

**MiMo 的真实做法是「自有可排序 ID + SQLite 权威存储 + Markdown 记忆 + 可重建 FTS」；对 Reasonix 阶段 1–3，应抄的是身份与记忆的职责划分和迁移工程细节，而不是把 JSONL 权威一次性切到 SQLite — 后者属于评审中的阶段 4，必须单独立项。**
