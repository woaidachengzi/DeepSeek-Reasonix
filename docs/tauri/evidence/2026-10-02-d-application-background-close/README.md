# macOS 后台关闭对齐 Wails 的应用隐藏

冻结 Wails `desktop/app.go::hideForBackground` 在 darwin 调用 `runtime.Hide`，其他平台
调用 `WindowHide`。此前 Tauri 所有平台仅 window.hide；实际关闭快照 applicationHidden=false。
本轮 macOS 改为 AppHandle.hide，保留后台宿主与主窗口；现有共同恢复路径先取消应用隐藏，
再显示/取消最小化/聚焦窗口。Windows/Linux 路径不改。

原生 background-close 与 second-instance smoke 现在明确要求关闭后 NSApplication.isHidden，
恢复后清除该状态；不将仅 window visibility 当作 macOS 应用隐藏验收。
该检查编入最终包，但完整窗口程序门禁仍受既有最小化失败阻挡，本次不宣称完整门禁通过。

## 当前真实候选

- host：`15fcd43baa4fc08225838a3b8bdd2441a10f823dacfd514375d081239347a370`
- sidecar：`29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`
- DMG：`bbe82994c298bc08091fd69fd3de1a38b596ae87ce2329f648e6b4a72fdf1a43`
- 安装：`/private/tmp/reasonix-d-app-hide-installed-ipo2n156/Reasonix Tauri Preview.app`

最终 build（绑定/lint/类型/CSS/bundle、Go、Rust、app/DMG）与 all-target Clippy 通过。
只读挂载 DMG、ditto 独立安装、卸载、strict ad-hoc 签名通过，详见 install.json/log。
不具备 Developer ID/notarization，未正式发布或切换默认下载。

首次 DMG 打包失败、未记录具体底层原因；直接 frontend CLI 诊断因错误工作目录识别失败；
正确仓库 wrapper 的 verbose 重试成功。随后 native smoke 强化后再次最终构建成功。
保留所有日志；不把重试成功当作首次失败原因已查明。

## 实际交互与原生状态

私有档案 vpvsvlno，宿主 93423、sidecar 93441、open 93421，页面 origin
44bd85a2c38bdc6413fdbbad0bd32875。未使用默认档案或系统剪贴板。

1. CUA 实际关闭按钮 49，先读取原生观察文件：applicationHidden=true，visible/nativeVisible=false，
   restoreRequests/completions/reopenEvents=0，sidecar ready=1。
2. AX 读取和确认原 PID 存活后的重新绑定均没有恢复应用。首次“期待 Reopen 恢复”的断言失败，
   after-reopen/after-rebind 快照仍 hidden=true；不宣称 Reopen 或 Dock 恢复通过。
3. 实际 View 菜单检查后 Cancel，再打开 Window。点击 Show Reasonix 前快照仍 hidden=true；
   点击该菜单项后 hidden=false，visible/nativeVisible=true，restoreRequests/completions=1，
   reopenEvents=0。这证明实际菜单选择经过共同恢复路径取消隐藏，而不是 AX 观察预先恢复。
4. Cmd+Q，确切 PID 的 kernel/open=0，宿主/sidecar/ready 无残留；退出后未读取失效 UI。

在旧 e0fc4d12 独立样本先尝试 Dock/SystemUIServer 绑定，两者均 -10005 timeoutReached，
未关闭窗口或执行恢复；随后 Cmd+Q 干净退出。manual-entry 文件保留。
**手动 Dock/托盘点击仍未验收，不由 Show 菜单成功替代。**

同最终包的程序门禁见相邻 `2026-10-02-d-app-hide-installed-run/result.json`：failure-exit
两档案 deliberate failure marker 与 kernel=2 通过；package 两档案正常启动/退出通过；
lifetime 8/8（两档案 × TERM/KILL × 空闲/实际假 Provider 流式任务）清理、原档案文件及
同档案重启通过。三个 runner 门禁均 exit 0，源码/脚本/包签名和哈希一致。
之前候选的窗口/权限结果不转移为新包结论。

D 未结案。最小化与全屏退出失败、手动 Dock/托盘、多显示器实际恢复、活动回合关闭、
通知/钥匙串完整权限与重启链路等缺口继续保留；E 未推进，A/B/C 也仍阻碍正式发布。
