# 原生隐藏窗口 IPC 权限验收扩展

生产统一 invoke handler 已校验真实 WebView label，只允许 main 调用所有自定义命令；插件仍走 capability，未发现主题/诊断导出的生产缺口，无重复守卫或权限放宽。初始仅看函数签名的判断已纠正。

扩展 opt-in native clipboard acceptance 的真实隐藏 WebView JS 调用：tray locale、主题导入、主题导出、诊断导出、notification_permission、Wails keychain_import_legacy，共6命令×普通/伪造 window=main 两种载荷。必须返回统一 fixed main-window 拒绝，其他业务验证错误不算通过；守卫先于系统对话框、provider/source读取和凭据写入。隐藏 dialog插件 open 与文本读/写必须按capability拒绝；main继续文本精确值读写，图片读取拒绝，原剪贴板全部 item/type/order/bytes 独立恢复不变。

这是验收代码加强，不改变产品调用/授权行为。clippy通过，完整app/DMG构建、只读复制安装和strict签名通过；新host a62f8d6dbf7c1692d57e6ca34dfca6028f1e63adcbfb55e4ea60a49f5d485077，sidecar cd726201，DMG62fef152。[实际新包八组门禁](../2026-10-03-d-native-permission-package-run/README.md)全部通过，包含双档案6命令×普通/伪造main共24次拒绝、隐藏dialog插件拒绝、完整剪贴板恢复与AppKit取消及原七组基础回归。

不是实际主窗口所有权限功能成功/系统授权取消、托盘点击或完整D/E验收。前7ac包原生服务四组通过另存，不能继承为本包新增断言通过。
