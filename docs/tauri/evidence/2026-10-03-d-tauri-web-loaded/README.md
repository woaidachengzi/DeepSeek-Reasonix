# Tauri 独立 Cocoa 窗口补齐 WKWebView 对照

只改私有 example。创建持有的真实 WKWebView，固定 inline HTML、nonPersistentDataStore，不读取产品数据；主线程采样 isLoading=false 且 estimatedProgress=1.0 作为内容完成前提。新增 loaded 模式仅在完成后执行一次 orderOut/show/activate，并记录是否执行。Observer/main-thread 生命周期保持不变。离线 release build 与 Clippy -D warnings 通过。

同一二进制 `7bf660c4c8a642ad2ad84b7335b0fc78143ddead799a0fe575b69bed56a6d8dc` 顺序运行两份隔离样本：none PID16685、loaded PID16721。两份前提均满足 key/active/visible/Tauri pageFinished/Cocoa pageComplete；loaded 的重新显示确实执行。mini/demini 各调用一次，但 nativeMiniaturized 全部 false、无 will-mini/did-mini/did-demini，kernel2/open0；runner0 仅表示成功采集预期失败。

另外保持此前通过的 Swift 源码/身份，仅改1280×820尺寸，PID16791 kernel0/open0，通过完整原生 mini/demini 事件，故尺寸不是该组合失败的必要条件。仅增加1.25秒等待的 Swift 样本 PID16815 failed-key/kernel1，未执行最小化，不能比较时机因果。

页面完成后的重新显示不足以修复当前 Tauri 对照；不将此实验移入生产。当前来源仍不足以定位具体根因，下一步需要实际菜单/按钮操作与程序调用对照。D 窗口、全屏和稳定性仍未验收。
