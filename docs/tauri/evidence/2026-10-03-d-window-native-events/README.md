# 独立窗口原生事件采样

仅修改独立 example，注册以目标 NSWindow 为 object filter 的 NotificationCenter 监听，主 operation queue 回调并在 run_return 后移除 token；未改生产 host。保存首次 observer 类型编译错误、修复后的离线 release build 和 Clippy -D warnings 通过记录。

二进制 `8cac98fe15c0ff57fe01241515805836daf0d1ff6ef99229509894b79c22bb7e`；PID 15078，kernel exit 2/open exit 0。激活/key/visible/pageFinished 前提成立，mini/demini 各执行一次；原生 minimized 始终 false。最终事件为 resign-key、occlusion、key、occlusion，未见 will-mini、did-mini、did-demini。焦点事件正样本证明监听器实际收到目标窗口事件。Runner exit 0 只证明失败回执采集与校验成功。

[Apple willMiniaturize 文档](https://developer.apple.com/documentation/appkit/nswindow/willminiaturizenotification)与[didMiniaturize 文档](https://developer.apple.com/documentation/appkit/nswindow/didminiaturizenotification)用于解释事件含义。

样本差异仍存在：Tauri 事件循环内独立 Cocoa 窗口无 WKWebView 内容，而 Swift 对照有 WKWebView，且激活/调度时机不同。不能只凭当前对照认定 Tauri 事件循环是根因。下一步控制内容与调用时机；D 最小化、全屏、实际快捷键、多显示器门禁仍未关闭。
