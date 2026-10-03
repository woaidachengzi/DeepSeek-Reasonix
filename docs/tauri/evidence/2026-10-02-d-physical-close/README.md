# macOS 实际关闭按钮与退出策略

同一真实安装包 `e0fc4d124080bc9e337b009611d48240982e8ad14121ed24248bcf90ed868c76`，
sidecar `29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`。
本轮不改生产二进制，仅补交互验收工具的显式等待参数。未发布或切换默认下载。

## Wails 基线核对

冻结仓库 `reasonix-wails-1.38.3/desktop/app.go::beforeClose`：系统/强制退出优先；
后台关闭确认可恢复路径、保存窗口与会话后隐藏；quit 允许退出。macOS 使用 application
hide，Tauri 当前使用 window hide，不能从“窗口不可见”推导两种 application hide 语义相同。
`settings_preferences.go::SetCloseBehavior` 只持久化配置，不重建运行时。
远程子窗口、活动回合、恢复路径丢失等分支不由本次空闲主窗口验收覆盖。

Tauri `main.rs::on_window_event` 主窗口 CloseRequested 按 HostPreferences 执行 hide 或 exit(0)；
`host_preferences.rs` 默认 keep_running。设置页的实际 radio 与 Rust 落盘状态相符。

## 实际操作结果

- 后台档案 `ckjb8lp0`：实际原生关闭按钮后，先读取原生观察文件，visible/nativeVisible=false、
  restoreRequests=0，sidecar ready 仍存在。默认后台 radio=1/quit=0。
- 随后 CUA AX 读取触发 Reopen，原生记录 reopenEvents/restoreRequests/restoreCompletions=1，
  窗口恢复可见。这是工具观察导致的恢复，**不是手动 Dock/托盘恢复通过**。
- 首轮 launcher 等待 300 秒超时，下一次 radio 操作未执行；不能把此轮标为 quit 通过。
  超时日志与三个时间点原生快照保留，确切进程已终止。
- 新档案 `6i6k6g9y` 用显式 `--interactive --observe --wait-seconds 900`：实际 Cmd+, 打开设置，
  点击 radio 115，AX 确认后台=0/quit=1；私有 `host-preferences.json` 确认 closeBehavior=quit。
  返回工作区后点击原生关闭按钮 49，之后不再读取失效 UI。
- 该 PID 的 kqueue kernel 退出码=0，open=0；宿主、sidecar、ready 均无残留，runner=0。
  详见 `quit-exit-receipt.json`、`quit-result.json`、`cleanup.json`。

## 工具和回归

新增 `--wait-seconds`，仅允许 interactive+observe 自定义，范围 1..900；默认仍 300，
非交互门禁仍 25 秒。**原生只读观察采样仍最多 300 秒**，900 秒只是进程退出等待。
无效 0/901、脱离交互观察的 900、单独 observe 均在启动前拒绝；新参数真实包操作通过。

Rust host_preferences 10 项通过，覆盖 close 保存、重新读取、损坏配置、旧配置及保存失败。
Wails 8 个命名关闭/恢复基线测试通过。第一次在根模块运行失败，第二次缓存权限失败，
随后在 desktop 模块获准访问已有 Go 缓存后通过；所有日志保留。未调整验收条件。

本轮未操作系统剪贴板，也未更改用户默认档案。此前剪贴板首轮原内容未恢复的已知影响
仍见上一轮证据；本次验收不代表该影响被修复。

D 仍未结案：手动 Dock/托盘恢复、实际活动回合关闭、安装包同档案重启、最小化/全屏退出、
多显示器实际恢复及通知/钥匙串完整权限验收仍需推进。E 保持门禁，A/B/C 发布缺口继续保留。
