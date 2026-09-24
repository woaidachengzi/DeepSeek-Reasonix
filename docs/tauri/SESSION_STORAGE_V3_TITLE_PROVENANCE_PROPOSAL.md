# 提案：V3 §3.2 增加标题溯源列（`title_source` / `title_revision`）

> 提出方：DeepSeek
> 对象：`docs/tauri/SESSION_STORAGE_IMPLEMENTATION_V3.md` §3.2 的 schema 骨架
> 性质：**只增列，不改变 V3 已确定的任何结论**（路径、状态机、动作分离 API 均不变）
> 状态：待 Codex 裁决。V3 正文未被本提案改动。

## 1. 为什么现在提：项目里已经有两条标题写入路径

这不是"将来接自动标题才需要考虑"。当前代码里已经存在：

| 写入路径 | 代码位置 | 语义 |
| --- | --- | --- |
| 用户手工重命名 | `agent.RenameSession` / bridge 的 `PATCH /v1/sessions/{id}/title` | 用户意图 |
| AI 生成标题 | `desktop/session_ai_title.go:22-70`（`AIRenameSession`），底层 `control.Controller.GenerateSessionTitle`（`internal/control/session_title.go:30`） | 机器生成 |

`AIRenameSession` 已经在用"读取-比较-写入"：它先读 `meta.CustomTitle` 作为
`expectedTitle`，生成后调用 `renameSessionInDirIfTitleUnchanged`，并在
`ErrSessionTitleChanged` 时放弃（`desktop/session_ai_title.go:43-66`）。

也就是说：**"不覆盖用户已改的标题"这条规则今天是被调用方各自实现的**。
一旦 Tauri Preview 也接入自动标题，就会有第二处实现；而身份库如果分不出标题来源，
任何"仅当标题仍是自动生成时才允许覆盖"的判断都只能靠调用方自觉。

## 2. 建议的改动（精确到可评审的 diff）

在 V3 §3.2 的 `CREATE TABLE sessions` 中，`title` 之后增加两列：

```sql
  title          TEXT NOT NULL DEFAULT '',
  title_source   TEXT NOT NULL DEFAULT 'user'
                 CHECK (title_source IN ('user','generated','fallback')),
  title_revision INTEGER NOT NULL DEFAULT 0,
```

即 V3 现有骨架从

```sql
  title TEXT NOT NULL DEFAULT '',
```

改为上面三行。**其余字段、索引与动作分离 API 全部不动。**

取值语义（三态，与 MiMo 一致，见 §4）：

| `title_source` | 何时写入 | 含义 |
| --- | --- | --- |
| `user` | 用户手工重命名（bridge 的 rename、`agent.RenameSession`） | 人工意图，**不可被机器覆盖** |
| `generated` | AI 生成（`AIRenameSession` 之类的路径） | 机器生成，可被更新，但不可覆盖 `user` |
| `fallback` | 从未有过标题、由 UI 显示派生名（如 `对话 <id 前 7 位>`） | 占位，可被任何来源替换 |

`title_revision` 是 CAS 版本号：每次标题成功写入 +1。

## 3. 建议的写入规则（4 条守卫）

入库时以 `UPDATE ... WHERE id=? AND title_revision=?` 做 CAS；`RowsAffected()==0`
即冲突，**放弃写入并重新读取**，不得重试覆盖。除 CAS 外还要守这三条：

1. `user` 的标题不可被降级为 `generated`（用户改过，机器不得覆盖）。
2. `fallback` 不可从非 `fallback` 回退（已生成/已手改的标题不得被重置为占位名）。
3. `title_revision` 在任何成功写入后必须递增，不允许"值不变但内容变了"。

这三条不是我的发明，是 MiMo 现行的显式守卫（见 §4 出处），我们照抄语义、不照抄实现。

## 4. 依据：MiMo 源码里的同一职责

| 事实 | 出处（`/Users/jerry/temp/app/MiMo-Code-main`） |
| --- | --- |
| `title_source ∈ {fallback, generated, user}` + `title_revision` 是会话表列 | `packages/opencode/src/session/session.sql.ts:30-31` |
| CAS：`previousRevision` 必须等于当前 revision，且新 revision = 当前 + 1 | `packages/opencode/src/session/projectors.ts:95` |
| 守卫：`user` 不可被解锁为其他来源 | `packages/opencode/src/session/projectors.ts:97`（"Protected title cannot be unlocked"） |
| 守卫：`fallback` 不可从非 `fallback` 回退 | `packages/opencode/src/session/projectors.ts:98`（"Generated title cannot return to fallback"） |
| 迁移导入时按"标题是否为空"定源 | `packages/opencode/src/storage/json-migration.ts`（`title_source: 标题非空 ? "user" : "fallback"`） |

## 5. 迁移与兼容

- 本提案把 V3 §3.2 的 schema 从"V3 骨架"变成"V3 骨架 + 2 列"。按 V3 §3.2 自己的
  规定：**若已有真实身份库或备份，不能原地改定义**，必须走带版本号的事务迁移
  （V2 §6.2 建议迁移用带时间戳的有序目录 `migration/<timestamp>_<name>.sql`，
  而不是把 DDL 写死在代码里）；若确认从未真实使用，可在首次接线前直接改尚未生效的
  schema 与测试。**本提案不预设哪一种，取决于 V3 §3.2 要求的只读检查结果。**
- 默认值与回填：`title_source DEFAULT 'user'`、`title_revision DEFAULT 0`。
  对已存在行，`title` 非空 → `user`；`title` 为空 → `fallback`（与 MiMo 迁移同规则）。

## 6. 建议的测试（3 条，都是可失败的）

```text
1. CAS 冲突：读 revision=3 → 并发把标题改成别的并 bump 到 4 →
   用 revision=3 写入必须失败（RowsAffected==0），且库里标题仍是并发写入的值。
2. 守卫：source=user 的行，用 generated 写入必须被拒绝；库里标题与 source 不变。
3. 守卫：source=generated 的行，用 fallback 写入必须被拒绝。
```

## 7. 影响面

| 项 | 影响 |
| --- | --- |
| V3 §3.2 的动作分离 API | **不变**。建议在 `ResolveForOpen` 返回的视图里带上 `title_source`，供 host 决定是否显示"AI 重命名"入口 |
| V3 §3.3 / §5 S1–S2 | **不变**。导入时按 §5 的回填规则定源即可 |
| bridge 契约 | **按本提案的建议不需扩协议**：bridge 的 rename 端点当前只收 `title`（`cmd/reasonix-desktop-bridge/main.go:590-592`），来源由端点决定（用户端点写 `user`）。若评审要求 host 能指定来源，则触发 V3/APPROVAL 的 B4（新增字段仅允许可选，破坏性变更才升 protocolVersion） |
| `internal/agent` | **不改**。`agent.RenameSessionIfTitleUnchanged` 已提供文件侧 CAS；身份库这层是库内 CAS，两者互补 |

## 8. 需要 Codex 明确表态的点

1. **接受/拒绝这两列。** 若拒绝，请说明"如何在不记录来源的前提下阻止生成标题覆盖用户标题"——
   因为这正是当前两处调用方各自实现、且将来 Preview 会成为第三处的那条规则。
2. **`title_source` 由谁决定。** 我建议**由 bridge 按端点决定**（不扩协议、host 不能指定），
   而不是让 host 传值。
3. **`fallback` 的落库时机。** 我建议"从未有标题时行内即 `fallback`"，
   而不是"留空再由 UI 派生"——否则库里无法区分"没标题"与"标题就是空字符串"。
4. **是否同时约束 `position`。** 与标题无关，但同一类问题：V3 §3.2 已定 `position`
   维持"越小越新"，本提案不涉及。
