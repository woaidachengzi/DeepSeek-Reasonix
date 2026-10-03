# c71ab66c Settings 启动方式对照

同一已安装 app、未改生产恢复逻辑，使用 LaunchServices `open -n -W` 携带明确的私有 HOME/TMPDIR/core 参数启动，实际 sidecar 父 PID 必须是目标安装宿主，kqueue 取得真实宿主退出码；不能把 open 进程退出当作宿主成功。

托管/显式档案分别执行 `menu-settings-minimized` 与 `menu-settings-native-minimized`，四次实际 exit0、原生phase成功、sidecar/readiness无残留、档案身份隔离。前者保持active/key窗口前提、实际最小化、Settings菜单恢复、事件恰好一次；后者额外要求AppKit Minimize角色、will/did-miniaturize及did-deminiaturize事件。[四次回执](receipts.json)和分目录host/trace/exit记录归档。

对照之前[裸宿主启动失败](../2026-10-03-d-wails-env-package-continuation/README.md)，本次原生条件没有放宽，仅改变实际安装包的启动方式。这证明该候选在LaunchServices环境可完成此链路，不证明所有历史失败由同一原因造成。正式完整窗口另有独立门禁，不将这四项代替当前工具的48阶段。

据当前锁定Tao源码，set_focus调用makeKeyAndOrderFront与activateIgnoringOtherApps；Apple的[activateIgnoringOtherApps说明](https://developer.apple.com/documentation/appkit/nsapplication/activate%28ignoringotherapps%3A%29?language=objc)提示激活可能延迟，并建议新的activate接口；[WWDC23 AppKit激活说明](https://developer-rno.apple.com/videos/play/wwdc2023/10054/)介绍系统的协作激活上下文。没有据文档推断测试通过，也没有修改用户其他应用的激活/权限状态来强迫通过。

工具补齐有限native-phase入口和完整窗口的LaunchServices宿主适配器。原有裸宿主模式保留为`window-direct`诊断；`window`使用真实.app启动，所有既有native断言、系统剪贴板完整恢复、原件/备份/持久身份、后台任务、第二实例和退出检查保留。没有测试浏览器链接，没有发布或切换默认下载项。
