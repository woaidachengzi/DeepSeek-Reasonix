# 当前环境独立 Cocoa 激活对照

从已保存固定 Cocoa/WKWebView 源码重新编译 arm64、macOS minos 11.0（SDK 15.2），签名后验证有效 entitlements 与当前生产包相等。无产品配置、sidecar 或网络。隔离 HOME/TMPDIR，LaunchServices 启动，确切 PID 13495 的 kernel exit 0、open exit 0。

原生激活/key 前提成立；miniaturize 收到 will-mini/did-mini、原生 minimized=true；deminiaturize 收到 did-demini，原生 minimized=false/visible=true/key=true。结果通过。见 result.json、host.log、build.json 和固定源码/runner。

此对照说明当前环境允许普通 Cocoa 窗口完成该链路，不证明 Tauri 已修复，亦不代替实际菜单/快捷键或多显示器验收。后续 Tauri 样本需在当前环境重新核对前提。
