# d295 当前包实际窗口操作

通过 CUA 操作当前 d295 安装包的显式私有普通档案，PID48922、sidecar48929、NSWindow5479。真实页面 URL 为私有 reasonix-preview origin，主页面已加载，未操作既有用户档案。

1. Window → Minimize 实际点击后，AX 标准窗口仍在；只读宿主采样 nativeMiniaturized=false、will/didMiniaturize=0、visible=true。
2. Window → Show Reasonix 后 focused/nativeKeyWindow=true、restoreRequests/Completions=1。
3. 点击黄色 minimize 按钮后，focused/nativeKeyWindow=false，但 nativeMiniaturized=false、will/didMiniaturize=0、visible=true。CUA 截图呈缩小变形中的窗口。未据截图或无事件判断具体根因；保留为实际操作未完成最小化证据。
4. 再次 Show Reasonix 后，View → Toggle Full Screen；实际观察 geometry3840×2160、style49167，AX 标题栏按钮消失。实际 ctrl+super+f 退出，geometry2560×1640/x640/y142、style32783、nativeKeyWindow=true，AX 标题栏按钮恢复。
5. 实际 Quit Reasonix：CUA 最后读取报告 procNotFound，独立 LaunchServices runner 核对 kernel exitCode=0、openExitCode=0、无 sidecar/readiness 残留，exit0；没有把读取已退出进程错误当成产品失败。

采样文件保留。全屏/恢复/正常退出仅覆盖本显式档案操作；最小化未通过，托管物理窗口、长期稳定、多屏拔插等仍待验收。没有把四项几何对照程序成功覆盖实际失败。
