# Tauri 直接接管安装版

用户于 2026-10-12 明确选择直接接管原 Wails 数据目录，而非复制到 Preview 独立目录。短期目标为 macOS 安装包、备份／回退、配置与原历史兼容、互斥保护和隔离／真实资料验收；每批最多 15 个文件，测试及 review 后提交。尚未完成安装包，不将 Preview 的隔离保护直接解除。

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
