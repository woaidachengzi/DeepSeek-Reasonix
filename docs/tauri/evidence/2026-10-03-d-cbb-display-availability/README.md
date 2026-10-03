# 原生显示器可用性独立对照

screens.swift 仅调用 NSScreen/CGGetActiveDisplayList 查询，无窗口、激活或操作系统设置修改，无 Tauri/sidecar。编译通过，正常进程环境与保留失败夹具的私有 HOME/TMPDIR 两次都 exit0、NSScreen=2（1920×1080 logical、scale2）、CoreGraphics status0/activeCount0。

Tao available_monitors 使用 CGDisplay::active_displays，空列表不能等同于 NSScreen 缓存为空。此对照不是最小化原因证明、屏幕解锁或热插拔验收。所有日志保留，不据此把本次完整窗口失败改为通过。
