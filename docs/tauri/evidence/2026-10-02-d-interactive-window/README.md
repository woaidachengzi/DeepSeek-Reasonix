# 实际窗口操作与精确退出回执（2026-10-02）

本批只增加显式验收探针，没有改变生产窗口恢复、快捷键、权限或退出策略。
新包 host `9d58aae0c6b600de53355b6ee03574770fdd7027fa7b973730aadc22f0077ef6`，
sidecar `29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`，
DMG `e3ad67779c786331ac875064411981744f41be52517540c238c099ab2261757b`。
只读挂载后复制安装在 `/private/tmp/reasonix-d-interactive-installed-tks3iqss/Reasonix Tauri Preview.app`，严格 ad-hoc 签名核对通过；不代表 Developer ID 或公证通过。

`REASONIX_TAURI_NATIVE_WINDOW_SMOKE=interactive-observe` 在主线程只读窗口状态与已有原生事件计数，每 250ms 原子更新一个固定 JSON 文件，最多 1200 次。不会主动显示、聚焦、最小化、恢复或退出窗口。正常应用没有该环境变量时不启用。LaunchServices runner 新增 `--interactive --observe`，保留确切 host PID 的 kqueue 退出回执，最多等待 300 秒；退出后不再查询失效的 CUA app binding。

Clippy all-targets 在修正新代码的 needless-borrow 后通过，完整 `pnpm tauri:build`（前端门禁、Go、Rust、app、DMG）通过。`cargo fmt --check` 仍报告此前未提交 Rust 修改的格式差异，未称为通过。没有改 transcript 或新增渲染逻辑。

同包两种新私有档案经 LaunchServices 正常启动，实际目录、sidecar 身份、401 和独立凭据身份核对通过；两个 host 均有 kernel 正常退出码 0，sidecar/ready 无残留。见 package log 与 managed/explicit result。

实际 UI 档案 root `/private/tmp/reasonix-launch-services-lolc3af_`，host 78115、sidecar 78123、open 78113，主页面为私有 origin `cf7e8a0ab6398ed0af65a1b48bb144ce`，空会话。通过 CUA 操作：

- Cmd+M、黄色最小化按钮、Window → Minimize 三条路径均未观察到最小化。各快照 sequence 持续增加（146、208、303），`nativeMiniaturized/minimized=false`，Will/Did Miniaturize 均为 0。不判通过，不声称根因已定位。
- 全屏按钮后 native style mask 从 32783 变为 49167（FullScreen 位开启），几何从 2560×1640 变为 3840×2160。随后 Ctrl+Cmd+F 与 View → Toggle Full Screen 操作后的快照仍为 49167；退出全屏及稳定往返未验收。AX 工作台重新出现不能替代原生退出全屏状态。
- 实际 Cmd+Q 后 runner 收到该确切 PID 的正常退出码 0，open 返回 0，host/sidecar/ready 无残留。没有对已退出的 UI binding 再调用观察，也没有对这些正常退出的私有 PID 发终止信号。

原始状态快照、安装包摘要、启动/退出结果及验收探针源码已保存。既有 `4d2a6865` 全窗口 48/48、生命周期 8/8、双屏 8/8 结果仍只归属旧包，未转记本包；历史失败保留。D 未结案，E 未推进。物理最小化/恢复、完整全屏、托盘/Dock、IME/配置快捷键、通知拒绝/重启/点击、钥匙串拒绝/迁移、异缩放拔插拖动与广义旧数据回退仍待验。A/B/C 缺口继续沿用迁移清单；本批不发布、不切换默认下载。
