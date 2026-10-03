# 2026-10-02 托盘命令的真实 IPC caller 边界

仅扩展既有 opt-in 原生剪贴板 probe；正常产品 handler、前端语言同步、菜单标题、权限和窗口策略未改动。新 probe 从实际主 WKWebView 的 `window.__TAURI_INTERNALS__.invoke` 调用已注册 `set_tray_locale`，要求成功；从真实隐藏第二 WKWebView 发出正常 zh 请求及伪造 `window: "main"` 的 en 请求，要求二者均返回 custom handler 的主窗口拒绝文案。拒绝来自实际 Tauri webview identity，不是 Rust 直接函数调用、无效语言校验或伪造返回值。

## 新实际包与结果

严格 all-targets clippy、完整 app/DMG 构建通过；DMG 校验、只读私有安装、严格 ad-hoc 签名及镜像卸载通过。host `317d96bb0df1806bf23a09ac83439084a36604fd8fb35fa072e23c62ec5f39fb`，sidecar `29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`；DMG 摘要/实际路径见 install.json。

两档案各菜单、clipboard-native、tray-language，共 6/6 阶段通过。clipboard-native 包含主 WebView 新命令成功、第二 WebView 新命令与伪造 caller 拒绝（各档案三次真实新命令 IPC），以及原有文本读写、图片/第二窗口插件拒绝、完整系统剪贴板原件恢复。tray-language 仍核对 Tauri 菜单项缓存、无效输入拒绝及 bridge 已保存语言值不变，不是实际弹出托盘画面。正常 package 两档案通过，退出无残留、身份/鉴权检查与逐门禁包摘要/签名保持。

这是新命令的 caller 验收，不是全部会话/资源权限或 iframe 边界验收。新增检测代码仅在显式 native smoke 时执行，没有新生产 IPC 或 capability。

前一同生产逻辑包 `4d2a6865` 在 `../2026-10-02-d-tray-current-window/` 完整窗口 48/48、异常退出 8/8、双屏 8/8 通过。这些结果保留各自包归属，新包未重跑这些组，不转记为新包已过；旧偶发最小化失败根因未解释，实际交互仍待验收。

## 下一项验证

当前普通阶段 runner 通过直接启动安装包内 executable 验证；第二实例已使用 LaunchServices。后续核对正常用户启动 .app 的 LaunchServices 路径与私有环境/实际 PID 绑定，作为不同启动上下文的验收，不替换原失败门禁或放宽窗口状态断言。

已核对本机 Apple SDK：-3811 对应 SCStreamErrorInternalError，权限拒绝另有 -3801；[Apple 官方定义](https://developer.apple.com/documentation/screencapturekit/scstreamerror/internalerror)仅表明框架内部无法启动捕捉。它不能证明录屏/共享、权限拒绝、用户输入或最小化根因，未改系统设置。物理 UI、D/A/B/C/E 和正式签名/公证、发布授权缺口保留；D 未验收，E 未推进。
