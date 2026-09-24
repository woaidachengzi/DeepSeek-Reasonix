# 审批评审稿 v2：MiMo 式会话存储改造（阶段 0–4）

> 状态：**待第三方评审**
> 日期：2026-09-23（v2：与 `SESSION_STORAGE_PLAN_V2` §13 决议索引对齐 + 问题清单）
> 分支：`experiment/tauri`
>
> **文档分工（按此顺序阅读）**：
>
> | 文档 | 角色 |
> | --- | --- |
> | [SESSION_STORAGE_PLAN_V2](./SESSION_STORAGE_PLAN_V2.md) | **主实施方案**（怎么做）· §13 声明对本稿的采纳/覆盖/补充 |
> | **本稿（APPROVAL v2）** | **审批边界**（A/B/C 编号）· **P1–P11 问题清单** · 门禁 |
> | [MiMo 源码对照](./MIMO_SOURCE_COMPARISON.md) | 为何阶段 1–3 不全量抄 SQLite 消息入库 |
> | [已有评审](./MIMO_STYLE_SESSION_STORAGE_REVIEW.md) | 前一轮评审（已被 PLAN_V2 §0 吸收） |
> | [原始提案](./MIMO_STYLE_SESSION_STORAGE_PROPOSAL.md) | 职责划分仍有效；落地部分被 PLAN_V2 取代 |
>
> **编号约定**：A/B/C/P/Q 仅在本稿定义；PLAN_V2 的 V1–V6 是其**新增**问题，与 Q 不重叠（见 PLAN_V2 §10）。PLAN_V2 §13.2 对 B2/Q2/Q3/Q7 的**覆盖以 §13 为准**；本稿 §3 已写入覆盖后的终态，避免两套结论。

---

## 1. 背景与问题

- **1.38.3** 路径身份脆弱；上游 1.38.4–1.38.10 会话存储约 5 天 41 提交、fix 约 40%（仅规模证据，不证因果）。
- 目标：稳定 ID + 记忆分层；**不**同迭代连换格式+身份+执行权威。
- 非目标（同 PLAN_V2 §1）：阶段 1–3 不把消息/工具事件搬进 SQLite；不迁 Wails；不改 Wails/CLI/Serve 身份语义。

---

## 2. 已核实事实（摘要）

| 主题 | 结论 | 出处 |
| --- | --- | --- |
| MiMo | SQLite 含 message/part 权威；`ses_` ID；记忆 Markdown+FTS；指纹只跳过未变文件 | SOURCE_COMPARISON |
| bridge 路径 | 恒为 `<sessionDir>/tauri-<id>.jsonl`，不看 workspace、无 `projects/` | PLAN_V2 §2.1–2.2 |
| 导入器 | `workspaceRoot≠""` 时错误拼 `projects/<slug>/sessions/` | `catalog.go:49-55` → **P1** |
| 身份库 | state home；**无调用方**；profile 尚无 sqlite | PLAN_V2 §2.3 |
| UI 数据源 | 仍是 `workbench-sessions.json` | Tauri workbench_catalog |

---

## 3. 审批结论（终态，已吸收 PLAN_V2 §13 覆盖）

### 3.1 建议批准

| 编号 | 决议 |
| --- | --- |
| **A1** | 阶段 0–1 边界：身份库在 Preview state home；JSONL 为执行真相；cache 只放可重建投影 |
| **A2** | 身份 ≠ 可删索引；身份库进备份；删 catalog 不得重分配 ID |
| **A3** | 沿用已有 `tauri-…` ID；改前缀须独立 Move 迁移 |
| **A4** | size+mtime 仅扫描跳过；歧义不自动绑定（证据序见 PLAN_V2 §6.4） |
| **A5** | Preview 与 Wails 隔离；稳定版导入 = 备份+复制+为副本登记 |
| **A6** | 阶段 4 默认不做；独立 RFC + 影子写 + 全量回放测试 |
| **A7** | 记忆 Markdown 树 + 可重建 FTS；注入复用 `session-context`；**旧 `REASONIX.md` 不迁，双轨写入需 RFC** |
| **A8** | `TranscriptPath` 唯一构造；导入器删除 `projects/<slug>/` 分支（PLAN_V2 §4） |
| **A9** | **终态（覆盖原 B2）：schema v1 即含 `session_path_alias`**；唯一键为 `(session_dir, transcript_name)` UNIQUE，废除 `path` 身份列（PLAN_V2 §5、§13.2） |

### 3.2 批准但附带条件（终态）

| 编号 | 条件 |
| --- | --- |
| **B1** | 阶段 1 完成前：**修 P1**；workbench 导入 + **孤儿只读扫描（PLAN_V2 §6.6）**；只读导入清单（§7.1，含未认领文件）；SHA-256 前后不变；失败回退；此前 workbench 仍是 UI 源 |
| **B2** | ~~阶段 2 前冻结 alias schema~~ **→ 已由 A9/PLAN_V2 覆盖：阶段 1 即带 alias；pragma/schema/Move/证据序以 PLAN_V2 §5–6 冻结** |
| **B3** | 备份：停写或 backup API；禁止 WAL 活跃拷单 .sqlite |
| **B4** | bridge 仅可选字段；破坏性变更升 `protocolVersion` |
| **B5** | 退出条件不放宽：jsonl 字节不变 / 重扫幂等 / ID 不变 / 缺文件非空会话 / 未认领文件必须列出 |
| **B6** | 删除：先 `RemoveSessionArtifacts` 成功再删行（先文件后行） |

### 3.3 明确不批准（除非新 RFC）

| 编号 | 否决项 |
| --- | --- |
| **C1** | 阶段 1–3 重写 JSONL 编解码 |
| **C2** | 同迭代连改：存储格式 + 身份模型 + turn 执行权威 |
| **C3** | 以迁移为名批量 rename/重写用户 jsonl |
| **C4** | 身份库放 cache 或可全量重建的库 |
| **C5** | 阶段 4 前宣称"已完成全量 SQLite 迁移" |
| **C6** | **P1 修复前**接入启动流程或切换 UI 数据源 |

---

## 4. 发现的问题清单（P1–P11）

| ID | 严重度 | 问题 | 修复归属 | 状态 |
| --- | --- | --- | --- | --- |
| **P1** | **致命** | 导入器 `catalog.go:49-55` 在有 workspace 时拼 `projects/<slug>/sessions/`，与 bridge 扁平路径不一致 → 带工作区会话会被误标 missing 或写错 path | PLAN_V2 §4 `TranscriptPath` + 删 projects 分支 + §4.3 同源测试 | **未修** |
| **P2** | **高** | `ErrPathChanged` 死路：登记错误/合法移动后无法纠正 | PLAN_V2 §5 alias + §6.3 Move（A9） | **设计已定，代码未改** |
| **P3** | **高** | `path TEXT NOT NULL UNIQUE` 把路径升格为身份 | PLAN_V2 §5 `(session_dir, transcript_name)` UNIQUE | **代码未改** |
| **P4** | 中 | 路径构造双实现（bridge vs 导入器），校验各写一份 → 再漂移 | PLAN_V2 §4.1 唯一构造 + §4.3 | **未修** |
| **P5** | 中 | `sessionidentity` 无调用方；勿称"阶段 1 完成" | 流程：C6/B1；阶段 2 才接线 | 文档已声明 |
| **P6** | 中 | 仅 workbench 导入扫不到 catalog 外 jsonl | **PLAN_V2 §6.6 已补规则**（确定性 ID 推导，拒绝随机新 ID） | 规则已有，**代码未写** |
| **P7** | 中 | `position` 越小越新，易与直觉相反 | PLAN_V2 V2：保持现状 + 注释锁方向 | 待拍板（V2） |
| **P8** | 中 | 删除顺序（先文件后行）未在 store/resolver 强制 | PLAN_V2 §5 删除语义 + B6；阶段 2 实施 | 阶段 2 风险 |
| **P9** | 低 | pragma 顺序与 PLAN_V2 §6.2 不一致（WAL/quick_check 先后） | PLAN_V2 §8 实施清单 | 未对齐 |
| **P10** | 低 | `store_test.go:167` 可能把错误路径写成期望 | PLAN_V2 §4.3/§8 | 未改 |
| **P11** | 低 | 历史文档曾不一致（`path:/` 误解、`current_path UNIQUE` 边界图） | 本 v2 + PLAN_V2 §13.3 | **本版已对齐** |

**关闭优先级**：阶段 0–1 必须关 **P1–P4、P6（规则→代码）、P9、P10**；P5 流程约束；P7 拍板；P8 阶段 2。

---

## 5. 存储边界（与 PLAN_V2 §5 对齐）

```text
【持久 · state home · 备份范围】
  session-state-v1.sqlite
    sessions(id PK, session_dir, transcript_name, workspace_root,
             title, position, missing, created_at_ms, updated_at_ms)
      UNIQUE(session_dir, transcript_name)
    session_path_alias(path PK → session_id FK)
  workbench-sessions.json（过渡期导入输入，不删，留回退）

【执行真相源 · Preview sessions · 格式冻结 1.38.3】
  <session_dir>/tauri-<sessionID>.jsonl + sidecars
  唯一构造：sessionpath.TranscriptPath

【可重建 · cache · 可删】
  session-catalog · history FTS5 ·（阶段 3）memory FTS5

【记忆真相源 · Markdown】
  memory/projects/<pid>/MEMORY.md
  memory/sessions/<sid>/{checkpoint,notes}.md
  memory/sessions/<sid>/tasks/<tid>/progress.md
  memory/global/MEMORY.md

写入权威：
  jsonl/sidecars → Go Agent/controller（删除经 RemoveSessionArtifacts）
  身份库         → Go sidecar 唯一写；host 只读经 bridge
  记忆 Markdown  → agent 工具 + 用户手工
  可重建索引     → 扫描/reconcile，可整库删除
```

---

## 6. 开放问题（Q1–Q8 + 指向 PLAN_V2 V1–V6）

### 6.1 本稿 Q（编号稳定，勿与 V 混用）

| # | 问题 | 终态倾向（含 §13 覆盖） |
| --- | --- | --- |
| **Q1** | ID 前缀 | 沿用 `tauri-…`（A3） |
| **Q2** | alias 形态 | **独立 `session_path_alias` 表**（PLAN_V2 §13.2 覆盖） |
| **Q3** | 孤儿 jsonl | **确定性推导 ID，不分配新 ID**；无法推导 → 未认领清单（PLAN_V2 §6.6/§13.2 **覆盖**原"分配新 ID"倾向） |
| **Q4** | 记忆目录 | 新文件 MiMo 树；旧 `REASONIX.md` 不迁（A7） |
| **Q5** | versioned FTS cursor | 阶段 3 建议要 |
| **Q6** | Wails 导入协议 | 对齐只读+进度+幂等 + B3 |
| **Q7** | 库损坏恢复 | 主备份；次新 ID 须**用户可见**放弃原 ID 提示（§13.2 补充） |
| **Q8** | 阶段 4 准入 | 独立 RFC，不预批 |

### 6.2 PLAN_V2 新增（请在其 §10 表内批注，勿写进 Q）

| # | 问题 |
| --- | --- |
| **V1** | 存 `session_dir` vs 只存文件名 |
| **V2** | `position` 是否改为越大越新 |
| **V3** | 导入单条失败：整体失败 vs 跳过（倾向整体失败） |
| **V4** | workbench 回退窗口与停读条件 |
| **V5** | 关闭时自检身份行 vs 磁盘 |
| **V6** | 孤儿扫描只扫一层 vs 递归旧 projects 目录 |

### 6.3 PLAN_V2 §13.4 仍待裁定的冲突

1. **B2 时机**：本稿终态已接受"阶段 1 带 alias"；若评审坚持推迟，须接受阶段 1 仍是 `ErrPathChanged` 死路。
2. **Q3**：若主张分配新 ID，必须给出可持久化的文件→ID 绑定，否则重扫幂等破产。
3. **schema 等价**：`(session_dir, transcript_name) UNIQUE` 是否完整覆盖"可见路径不重复"语义。

---

## 7. 验证门禁

### 7.1 阶段 0–1

- [ ] **P1**：带 workspace 的 catalog 条目 path **逐字符串等于** `bridgeSessionPath`
- [ ] **P2/P3**：新 schema + alias + Move 语义落地；`ErrPathChanged` 不再是唯一出路
- [ ] **P4**：TranscriptPath 同源回归（PLAN_V2 §4.3）
- [ ] **P6**：扫描器按 §6.6；未认领文件进清单不入库
- [ ] **P9/P10**：pragma 顺序与测试期望修正
- [ ] 只读清单命令；抽样与磁盘一致
- [ ] 重扫幂等；kill -9 无重复行
- [ ] JSONL SHA-256 不变
- [ ] 越界/非主转写/非常规文件拒绝
- [ ] 无身份库时旧 catalog 可用；与 Wails 零交叉

### 7.2 阶段 2

- [ ] 重启 ID 不变（含改题）
- [ ] 缺文件 → missing，不建空 jsonl
- [ ] Move：alias 可查；隐式漂移拒绝
- [ ] 删除：先 artifacts 后行（B6）
- [ ] host 最近列表切 bridge；workbench 回退可解释（V4）
- [ ] bridge 可选字段双向兼容

### 7.3 阶段 3

- [ ] 手工改 md → FTS 命中
- [ ] 注入预算与顺序；诊断无密钥/完整消息
- [ ] 删 memory FTS → 可重建

### 7.4 每阶段

- [ ] `go test ./internal/sessionidentity/...` + bridge 契约
- [ ] `cargo test` / 前端 tauri 契约（若触及）
- [ ] REASONIX.md 要求的 PR 门禁字段

---

## 8. 风险（增补）

| 风险 | 等级 | 控制 |
| --- | --- | --- |
| **带着 P1 接入启动** | 致命 | C6 |
| 身份库与 jsonl 不一致 | 高 | 单写者；resolver；missing 不猜 |
| 双实现路径再漂移 | 高 | P4 + 同源测试 |
| 扫描误收辅助 jsonl | 中 | `IsSessionTranscriptName` |
| 与上游 1.38.10+ 冲突 | 高 | 钉 1.38.3；升级前门禁 |
| 备份/WAL | 中 | B3 + 恢复演练 |
| 记忆注入泄密 | 中 | 预算+过滤+脱敏 |
| 范围蔓延阶段 4 | 高 | A6/C1–C5 |
| 删除顺序留孤儿 | 中 | B6 |

---

## 9. 评审请求（给第三方模型）

请回复：

1. **A1–A9、B1–B6、C1–C6：同意/反对？**（B2/A9 已按 PLAN_V2 覆盖写终态。）
2. **P1–P11：遗漏或误判？严重度是否认可？**
3. **Q1–Q8 逐项** + **PLAN_V2 V1–V6 逐项** + **§13.4 三个冲突如何裁。**
4. **P1 未修前能否合入现有 `sessionidentity`？**  
   （本稿：可合为库+导入增量；**禁止接线与宣称阶段 1 完成**（C6）。）
5. 是否有**遗漏的存储/并发/备份/删除不变量**？

---

## 10. 一句话

**批准 PLAN_V2 为实施规格、本稿为 A/B/C 门禁与 P 清单：阶段 0–1 先关 P1–P4/P6/P9/P10（路径同源 + v1 schema 含 alias + 孤儿扫描规则落码 + 只读清单），JSONL 格式冻结、阶段 4 不预批；待第三方批注 A/B/C、P1–P11、Q1–Q8、V1–V6 与 §13.4 冲突后进入阶段 2 接线。**
