# 保留真实退出码与清理（2026-10-02）

旧安装包 `9d58aae0` 在新的托管私有档案中收到固定、不支持的 native 验收 phase，
写出 `ok=false` 并请求 `app.exit(2)`，但 kernel 回执为 0。runner 按预期拒绝，
显式档案未运行；失败 marker、回执和日志保留，未把 0 改记为通过。
锁定的 tauri-runtime-wry RequestExit 分支设置 `ControlFlow::Exit`，没有传播请求码。

生产 host 改为 `run_return`，记录首个明确 ExitRequested 请求码，完成原 Exit
回调（原生观察器注销、通知 shutdown、窗口状态保存、sidecar stop）及 Tauri 清理后，
把请求码报告给系统；没有明确请求时保留 runtime 返回码。正常 Quit 仍为 0。
没有新增 renderer command、权限、持久化格式或默认数据目录变化。已有 restart
交给 Tauri 的 run_return 生命周期处理，本批未验证 updater/restart UI。

新 host SHA256 `e0fc4d124080bc9e337b009611d48240982e8ad14121ed24248bcf90ed868c76`，
sidecar `29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`，
DMG `c24fe61e32b98399f723cf9bfc440d3897985bef45e365761f04f5c0117dd367`。
安装副本 `/private/tmp/reasonix-d-host-exit-installed-xg2eq0h_/Reasonix Tauri Preview.app`。
完整 app/DMG、Clippy all-targets `-D warnings` 与安装后 strict ad-hoc 签名核对通过。
尚未 Developer ID 签名或公证，不代表正式发布候选验收完成。

同包验收见 [统一门禁原始证据](../2026-10-02-d-host-exit-code-run/result.json)：

- 托管与显式档案的固定 deliberate failure 都有 `ok=false` marker、实际 kernel
  **2**、open=0，host/sidecar/ready 无残留；不是实际窗口通过。
- 正常 package 两档案 **0**，原身份/目录/401/Global/通知门禁与清理通过。
- 托管/显式 × SIGTERM/SIGKILL × idle/真实流式任务，生命周期 **8/8**，实际
  sidecar kernel 正常退出、原件/ready/锁与同档案重启门禁通过。
- 新显式 UI 档案 `/private/tmp/reasonix-launch-services-eo1vjyg4`，私有 origin
  `0de7389f38f3f16aed80a45ac9fd58fb` 与当前空会话主页面相符；实际 CUA Cmd+Q 后
  host 84743、sidecar 84751、open 84741 全退出，kernel/open=0，ready=0。
  中断后原 exec session 已消失，但存档 kernel 结果及只读 PID 核对确认终态，未重跑，
  未在 Quit 后读取失效 UI binding 或对这些正常退出 PID 发终止信号。

`probe-launch-services-profile.py --failure-exit` 只在私有验收中启用固定错误 phase，
与 interactive 互斥；保留实际/期望退出码后严格核对 marker 和清理。
新 `failure-exit` 已加入 `verify-installed-d.py` 默认门禁，runner 的成功 0 与
被测错误 host 的 2 分别记录，不把 launcher 的 0 当成 host 成功。

本批没有再次运行完整窗口、双屏、通知/钥匙串拒绝交互或官方广义历史回退；先前
结果保留各自包归属。当前最小化/完整全屏仍未通过；[主队列对照](../2026-10-02-d-mainqueue-window/README.md)
也没有提供可发布的窗口修复。D 未验收、E 未推进，A/B/C、其余 D 物理/权限/旧数据
与正式签名门禁继续按迁移清单保留。没有正式发布、切换默认下载或新增延期。
