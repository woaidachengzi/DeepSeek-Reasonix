# 2026-10-03 提交前代码 review

范围：当前 experiment/tauri 全部未提交的 D 原生能力、Wails runtime 适配、凭据来源迁移、验收工具及历史证据。提交仅保存本地改造；D 未完成，E 未开启，不发布或切换默认下载。

## 已修复发现

- 旧 Preview/Wails keyring 来源迁移可能覆盖目标已有文件/runtime 凭据。三个来源统一在 provider mutation 锁内检查有效目标配置，先拒绝覆盖，再读取来源；真实 sidecar 回归验证三个来源拒绝且原件保持，更新中英繁提示。
- 全局 .env 来源的 lstat/open 间隙允许 FIFO 替换造成阻塞。复用 OpenFileBeneath 的 Unix 非阻塞打开与目录约束，并核对句柄 regular/inode、限制读取大小。
- 归档源码中的 .go 文件被主模块 ./... 发现。证据目录新增独立 go.mod；go list ./... 成功且不含证据包。保留历史原始快照。
- Python 字节码目录仅属本地缓存，加入 .gitignore，不进入提交。

## 当前源码验证

- go test ./cmd/reasonix-desktop-bridge ./internal/config ./internal/fileutil：通过；随后构建 /private/tmp/reasonix-review-bridge 用于 Rust 集成回归。
- pnpm test:tauri：最终全套通过（含设置凭据、runtime 适配、通知与外链）。首轮 locale 文案断言失配已修复，失败日志保留。
- pnpm test:window-state、pnpm test:clipboard、pnpm typecheck：通过。窗口状态首轮被沙箱拒绝 tsx 本地管道，允许环境下补跑通过。
- Python 原生剪贴板 fixture：4 项通过。
- Rust 当前源码/真实新编译 bridge：227 passed, 0 failed, 5 ignored。沙箱重跑曾出现 19 个本地 TCP/sidecar 权限失败；允许本地监听环境下完整补跑通过，失败日志仍保留。
- cargo clippy --all-targets -- -D warnings：通过。
- pnpm build：通过，含绑定契约、lint、类型与 bundle 门禁。
- 活动源码和文档（排除原始 evidence 快照）的 git diff --cached --check：通过；验收 Python 工具 AST 解析通过。原始 source.patch/log 保留自身空格与终端输出，不为消除历史快照 whitespace 提示而改写证据。

## 验收边界

新增 native_wails_env_import_restart_delete_preserves_original 为显式 ignored 原生 fixture 测试：已编译，本轮未运行系统钥匙串导入/授权，不能记为 OS 验收通过。旧钥匙串来源读取失败、GUI 与授权取消待验仍保留。

本轮 review 后生产代码已改变；已安装 c71ab66c 的 15 组程序门禁和 LaunchServices 48 阶段属于其固定旧包身份，不覆盖本轮修复。当前源码前端可构建且回归通过，新的 app/DMG 构建、安装 smoke 和原生权限验收待继续。用户已确认的浏览器到达验收页不重复运行。

D 仍缺实际系统凭据迁移/授权取消、部分物理 UI/多显示器 pending 恢复等验收；E remote/bot/updater/复杂管理页尚未整组迁移验收。A/B/C 的协议覆盖、Markdown/旧 Global 图片和真实工具进程，以及签名公证等发布缺口继续按 API_SURFACE_AUDIT 清单保留。现有配置备份与 Wails 原件回退证据保留；新修复包须复验。
