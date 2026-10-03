# 7ac27142 原生服务四组回归

同一实际安装候选，原 dialog-cancel、tray-language、links、notifications 四组全部exit0；严格签名、包摘要、脚本依赖摘要逐阶段保持，源码快照见 result.json。运行结束后新增的隐藏 IPC 断言不属于本包执行范围，另建新包证据。

dialog-cancel：双档案原菜单契约、主窗口真实文本剪贴板 IPC/原生精确值、隐藏调用方文本读写及tray拒绝、主窗口图片权限拒绝；完整原剪贴板有序 item/type/bytes 独立恢复。文档Save As、诊断导出、主题导入及目录选择器实际AppKit取消，生产回调/返回值/原件保持通过；目录取消采用同生产callback接收函数，不是目录选择成功/持久化证明。

tray-language：双档案原菜单/剪贴板切片、中文/繁中/英文native菜单标签及无效locale拒绝/持久语言不变通过。links：双档案两条原生打开入口、URL拒绝和默认浏览器canary请求通过。notifications：双档案三次实际OS投递/移除，其中隐藏应用两次和同档案重启身份保持，共6项；授权已有，无新授权请求。

不覆盖全部物理菜单/托盘/Dock、真实选择保存与重启持久化、实际外链来源控件点击/mailto/OAuth、通知点击/banner/授权拒绝恢复、全部剪贴板GUI或窗口稳定性。D未完成/E未开启。
