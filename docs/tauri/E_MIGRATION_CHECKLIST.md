# E 迁移清单

2026-10-06 用户要求直接推进 E，明确不需要 updater。本轮不再以 D 全部验收为 E 的启动条件；D、A/B/C 未关闭项仍保留为正式发布门禁。正式发布、替换正式应用、切换默认下载均未授权。updater 从本轮范围排除。

| 功能面 | Wails 基线 | Tauri 起点 | 当前工作及验收 |
| --- | --- | --- | --- |
| remote 配置与认证 | desktop/remote_hosts.go、remote_prefs.go；SSH config、显式主机指纹、临时/持久凭据 | 读取/修改/删除、SSH alias 扫描、指纹确认和密码恢复已有 | 本批补连接请求归属、断开/改配置/退出取消、重开设置时的权威连接状态；源码 Go race / UI 回归、新包本机 SSH 信任/持久化/退出验收已通过 |
| remote 文件 | remote_listing.go；浏览、读取/写入及路径操作 | SFTP 浏览、文本预览、revision 保存已有 | 本批补目录/文件异步请求归属、取消/切换后旧结果丢弃；新包真实 SFTP 浏览/预览/revision 保存及旧版本冲突通过；mkdir/rename/delete 尚未接入 |
| remote tunnel / Serve | remote_serve.go、remote_server_stop.go；forward、bootstrap、状态/日志 | 本批新增会话内 local forward；Serve 入口未接 | 本地转发默认且只允许 loopback；源码真实 SSH 传输、重复请求拒绝、断开释放端口及新包转发/退出释放通过；配置转发自动应用、Serve 后续批次，不把 SSH/SFTP 验证算作 Serve 验收 |
| remote 对话与工作区 | remote_tab*.go、remote_projects.go；恢复、事件/审批、模型归属、断线重连 | 本地单会话 bridge，尚未完整对接远端 controller | 后续批次；不能借用正式版档案或放宽模型/写入权限代替 |
| bot 配置和账号 | bot_connection_app.go、bot_runtime_app.go；多账号、凭据、启停、状态、诊断 | 旧渠道配置/凭据、路由/队列/权限、适配器运行已有 | 本批补手动多账号创建/删除、受保护凭据、路由/订阅清理、运行时重启和配置应用状态；源码 Go race / UI、新包默认关闭/私有凭据/重启持久化/旧包读取回退通过；未连接真实 IM |
| bot 配对和管理 | internal/bot/pairing.go、desktop/bot_bridge*.go | 可开启配对，但没有待申请审批入口；Desktop=nil | 本批补本档案申请列表/批准/拒绝、失效连接拒绝、配置写失败保护；源码失败恢复及新包鉴权/连接内授权通过；桌面对话订阅/远程审批/接管仍待迁移 |
| bot 扫码安装与诊断 | Start/PollBotConnectionInstall、Diagnose/TestBotConnection | 尚无完整 Preview 安装/诊断流程 | 后续批次；真实 IM 平台连接须在可用账号环境验收 |
| 复杂管理页 | permissions/secrets/sandbox/network、skills/plugins/MCP、hooks/memory/subagents、data/diagnostics | 多数已有独立 Tauri 页面和 bridge | 本批修复 bot 管理轮询覆盖未保存路由草稿；其余页面逐项审计配置/异步归属/失败恢复/权限边界，不据页面存在判定完成 |
| updater | Wails 自动更新 | Preview 说明/手动下载 | 用户明确排除，本轮不迁移 |

验收分开记录：源码回归、真实组件浏览器、实际 `.app` 包内进程与本机协议服务、外部平台/原生窗口用户体验。旧包通过不自动升级为新包证据。测试不得使用真实凭据、发送测试消息到用户联系人或改动正式版数据。

复杂管理页首轮对照（Wails worktree `/Users/jerry/temp/app/reasonix-wails-1.38.3`，当前 HEAD `2185b8e8ff8abb2166bf8e55df0694d05a0f9d76`；本地基线包含既有补丁，不冒充官方归档身份）：

| 页面 | Wails 对照 | Tauri 当前实现与本次回归 | 仍需验收 |
| --- | --- | --- | --- |
| 权限/敏感信息 | app.go 的配置读写与权限规则 | 独立 permission/secrets bridge，项目覆盖与全局规则保持；补全局→项目读取乱序保护；Go 持久化、规则隔离与 UI 迟到响应回归 | 原生窗口实际工具执行与拒绝/恢复 |
| sandbox/network | app.go 配置、网络代理与 sandbox 策略 | 独立桥接；Go 配置保存、凭据脱敏和其他字段保护已回归 | 真实平台策略/代理切换后的工具与子进程行为 |
| skills/plugins | app.go SkillsSettings、plugin_packages_app.go | 来源、安装计划、revision、启停、归档/恢复、诊断；Go 计划/安装/拒绝风险缺失/项目隔离回归，已有 UI 回归 | 实际包中第三方安装、取消/重启/失败回退；无真实第三方执行证据 |
| MCP | app.go MCPServers/配置/认证与 mcp_* | 配置、运行时、认证、目录与设置 UI 回归已有；本次工作区/会话切换重新挂载管理页 | 实际打包子进程退出、OAuth/超时/异常重启与权限完整矩阵 |
| hooks | hooks_settings_app.go | 项目信任与配置范围、插件 hooks；已有请求归属保护；本次保持工作区/会话隔离 | 实际工具触发、取消和 hook 子进程权限 |
| memory | app.go Memory、memory_suggestions.go | 文档/fact/revision/建议、工作区边界；当前会话应用须空闲；本次避免工作区切换沿用旧页面状态 | 原生编辑、建议接受与真实模型召回 |
| subagents | subagents_app.go | profile CRUD/revision、试运行/取消及模型默认；本次页面归属跟随工作区/会话 | 真实模型试运行、取消/子进程退出/失败恢复 |
| data/diagnostics | app.go 档案与诊断 | 档案隔离、导入审阅与只读诊断；storage bridge 有有效目录回归 | 不继承旧包备份/导入证据，当前候选完整矩阵仍待验 |

本批源码提交 `7e6645bdc8f9011a4cf289d6885e998503e65651` 已构建直接可运行 `.app`，包内 sidecar 在临时档案与本机 SSH/SFTP 服务通过集成验收，旧包读取新配置并回到新包通过。原生宿主在屏幕 2 内侧通过两种档案的退出清理和只读通知权限查询；这不替代 D 的完整原生窗口矩阵。证据见 [本批验收记录](evidence/2026-10-06-e-lifecycle/README.md) 和 [包收据](evidence/2026-10-06-e-lifecycle/package-receipt.json)。

不将表中“已有”能力计作整个 E 已完成。下一批优先 remote Serve/controller 与 bot Desktop 桥接；文件路径操作、扫码安装、完整管理页实际包矩阵仍保持未完成。仅 updater 属于用户明确排除。
