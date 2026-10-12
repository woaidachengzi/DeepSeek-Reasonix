# Tauri 直接接管安装版

用户于 2026-10-12 明确选择直接接管原 Wails 数据目录，而非复制到 Preview 独立目录。短期目标为 macOS 安装包、备份／回退、配置与原历史兼容、互斥保护和隔离／真实资料验收；每批最多 15 个文件，测试及 review 后提交。接管版已构建，默认 Preview 的隔离保护保留。

## 第一批：原路径身份与备份

- 只读检查确认原 profile 为 `/Users/jerry/.reasonix`，含全局、项目 JSONL 及 `desktop-sessions-v5/by-id`。当前 Preview 只支持自身扁平路径；旧记录不可通过单纯改变 home 得到完整支持。
- 新增 `wails-<sha256>` 导入身份，摘要绑定 profile 相对 transcript 路径；原项目 JSONL 保持原位置，搬移完整 profile 时 ID 不变。仅 takeover 模式且已注册、路径匹配的 ID 可打开，未知旧 ID 不会被创建为空对话。普通 Preview 会话路径规则保持不变。
- Inventory 在身份与路径摘要匹配时核验原文件，避免打开后记入 workbench catalogue 产生假路径差异；改变注册路径仍触发差异，物理唯一性仍由身份库约束。
- 现有 offline snapshot 拒绝 MCP npm 缓存中的符号链接。补充只排除 `mcp-state/<server>/cache` 和 `mcp-state/<workspace>/<server>/cache` 的可重建缓存，服务器配置与项目内名为 cache 的目录仍保留，不跟随符号链接。
- 未发现正在运行的 Reasonix writer 后，通过现有 `reasonix-session-import snapshot --writers-stopped` 做只读备份。首轮在 MCP cache 停止，未发布 manifest；修复后已创建并验证 `/Users/jerry/temp/app/reasonix-tauri/desktop/takeover-backups.noindex/reasonix-session-snapshot-3019110576`，25,788 个文件、676.5 MiB 文件内容，权限私有，逐文件 hash 验证。该目录独立于 target 并被 Git 忽略，含配置／凭据，不得打进安装包或提交。
- `internal/desktopbridge/sessionpath`、`internal/sessionidentity`、完整 bridge 回归及新路径／inventory／MCP snapshot 专项 race 通过。临时项目旧历史打开后路径仍为原 JSONL，未产生扁平副本；Preview 与未注册旧 ID 拒绝通过。

## 后续仍需完成

1. 兼容 Wails 的 authoritative workspace-state 与 v5 存储，读取选中投影及实际 workspace／title／archive 状态，避免从旧源文件猜测或展示重复历史。只在完整备份之后接管，保留原 v5 产物供回退。
2. 独立安装模式选择正式 profile，保留 Preview 默认隔离。接管模式与身份导入只能在原 profile ownership gate 内启用；旧版本不参与同一 gate 时须防止同时写入。
3. 配置／凭据来源、项目清单与历史分页，安装后的首次启动、重启恢复和回退验证。
4. 生成 macOS 安装包，先隔离验收，再检查真实资料。不因备份或单个项目打开测试通过宣布整项接管完成。

## 第二批：实际使用格式核对与副本演练

进一步只读比较发现，v5 的 29 个 frames 文件停留在 2026-09-20；项目 JSONL 最新写入为 2026-10-11，当前 tab 路径也不在旧 v5 source mapping 中。因此当前接管默认采用 `desktop-projects.json`／`desktop-tabs.json`／topic SQLite 与项目 JSONL 的组合，不能将旧 workspace-state 当作唯一当前清单。

新只读 exporter 直接使用旧源 `internal/session` 的 codec、完整 batch 校验、内容引用和 Project 语义，从私有备份导出 v5；不构造 writer。当前 JSONL 话题优先使用原路径；open tab 保留其明确选择的版本，缺少 JSONL 的已列出话题才以匹配 topic／workspace 的 v5 投影补充。旧 v5 未列出的 active 项、当前 deleted topic 不恢复。工作区 state v1–v3 只作兼容补充。

新离线准备命令在 profile gate 内重验与备份的源文件 hash，原 JSONL 不复制／改写，只有补充项写新 Tauri 文件。完整显示历史与旧模型 workset 分别保存到 canonical／有覆盖 hash、pinned provenance 和 model lineage 的 context checkpoint，保留已压缩上下文；完成 receipt 最后发布，既有 Tauri 资料拒绝覆盖。原 v5、配置和旧 topic metadata 保留。

`/private/tmp/reasonix-session-recovery-1270784220/profile` 副本已完成 25 个当前话题：21 个原路径、4 个 v5 补充；当前清单中没有仍应隐藏的 archive，旧 v5 archived 标记不覆盖当前已列出的话题。25,788 个备份文件中属于原 profile 的成员在演练后 hash 全部不变。真实 `/Users/jerry/.reasonix` 尚未发布接管 receipt 或新身份库，安装包尚未生成。

## 第三批：接管安装模式与真实历史验收

- 独立 `io.reasonix.desktop.takeover` 安装标识，直接选择默认原 profile。先检查已完成的接管 receipt、目录一致性与备份存在，再启动 bridge；Preview 默认隔离规则保留。安装包不包含用户配置、凭据或备份。
- 新原生验收阶段通过 packaged host 的实际 supervisor 检查历史列表可操作来源、全部当前历史、最近目录同步与 sidecar 重启恢复；运行器仅允许 `/private/tmp/` 内准备好的 HOME，不发送模型请求。保留可选择的 Go 私有资料验收测试，默认跳过。
- 副本全部 25 个话题打开成功，重启前后历史消息数一致，列表来源为 `identity`。原生 WebView 在正式原目录显示 4 个项目和当前对话；实际点击历史后发现最近目录同步仍按扁平路径核验，已补充 hash 绑定旧路径的支持和路径篡改拒绝测试。
- 原 `/Users/jerry/.reasonix` 已在 gate 内重验备份并完成离线准备，25 个话题（21 原路径、4 补充），没有覆盖原配置或迁移原 JSONL。日常打开／继续对话后，原目录将按新版正常使用生命周期写入。完整接管前备份仍在上述独立目录。
- 本地 Apple Silicon 安装包版本 0.1.1，ad hoc 签名，未配置 Apple notarization。只运行一个桌面版本；旧二进制是否参与 profile gate 取决于其编译版本，不能依赖新 gate 约束不支持它的旧程序。

### 回退

先完全退出 Tauri 和 Wails。原 Wails 文件仍保留，可启动原版；若要恢复接管前完整状态，先另存接管后的原目录（避免丢失新聊天），再将已验证 snapshot 的 `profile` 恢复为 `.reasonix`。不要把历史恢复覆盖到正在运行的目录，也不要删掉备份。原 receipt 指向备份目录，接管版启动要求该目录继续可用。

实际界面复验还覆盖了第二次启动的最近列表重放：已登记的 hash 绑定旧路径身份不再被误当作扁平 Preview 文件重复导入。重放只跳过已核验的旧身份，未知／改路径身份仍拒绝；新扁平会话的原导入规则保持不变。原目录历史点击后的最近登记错误已消失，消息旁的分叉按钮可用。

最终验证：安装包原生副本 25 条历史／重启／identity 列表／最近登记通过；真实 HOME 的 WKWebView 二次启动没有旧目录重复导入错误，打开旧对话正常、分叉入口可用，模型为原 deepseek/deepseek-flash。Rust profile 专项 22 项通过；sessionidentity 完整回归通过；bridge 完整回归在已有异步 Ask ticket 测试一次失败后复跑通过（17.283s）。App deep/strict 签名验证通过，DMG 中 App 标识和版本核验为 io.reasonix.desktop.takeover / 0.1.1。
