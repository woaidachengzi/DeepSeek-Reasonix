# macOS D：遗留运行时边界与窗口保存生命周期

2026-10-02。D 未验收，E 未推进；最小化失败继续是发布阻塞，没有新增延期或发布授权。

## 基线与改动

核对 Wails 1.38.3 的 `lib/windowState.ts`：窗口几何由前端按 resize、定时轮询和
beforeunload 报告，Go 退出时只保存最后报告，不在关闭路径查询原生窗口。当前工作树
已为单个保存器串行化捕获/保存，但 hook 卸载后，等待中的读取仍会继续提交。
确定性回归先复现旧观察覆盖新 mount 状态、继续读原生位置及排队请求继续捕获的三项失败。

现在保存器接受可选 AbortSignal；在捕获开始及每个异步原生读取后检查生命周期，
hook cleanup 先 abort 再移除 timer/listener。卸载后的尚未提交观察和排队捕获不再
调用 SaveWindowState。既有顺序、去重及 beforeunload 最佳努力语义保留。
**不能取消已发送的原生 IPC，也不能撤回已发送给 Go 的保存请求**；不据此宣称所有
跨 mount 持久化顺序或真实 Wails UI 交互已完全验收。

新增轻量 `wailsDesktopRuntime.ts` 收敛窗口几何和遗留文本剪贴板原生调用。
在使用时取得运行时，完整能力才提供窗口适配器，调用保留宿主 receiver；不构造
浏览器 mock，也不引入整个 Wails bridge。`clipboard.ts`、窗口保存 hook 不再直接
调用这些 runtime 方法。Tauri 原生文本通道仍优先；拒绝时仍不转到 Wails、browser
或 execCommand。Wails/browser 的已有回退顺序保持，未扩大权限。

## 验证及当前包

- 窗口保存回归：旧逻辑明确失败，新逻辑全部通过；覆盖未提交旧观察、排队捕获、
  无/不完整宿主能力、延迟运行时注入及方法 receiver。
- 原有 native/browser/Wails 剪贴板与复制反馈回归通过，包含 Tauri 拒绝后禁止其他通道。
- 完整测试类型检查通过；生产 build（绑定、lint、类型、bundle 门禁）及 app/DMG 通过。
- 真实 DMG 校验、只读临时安装、严格 ad-hoc 签名和卸载镜像通过。
- 当前包独立原生菜单/剪贴板切片：两档案各 2 项、共 4/4 通过。它没有完整窗口
  几何/隐藏/最小化前提，也未选择原生编辑或对话框项，不能算完整窗口门禁通过。
- 正常 package smoke 两档案通过：真实 bridge、私有档案/身份、通知授权查询、
  Global 工作区和退出清理。

当前 host SHA-256：`b95b38c0e35d66c964bc370b621a46c9d69116b7408a7c7567f4c715abc17734`。
安装路径、DMG/sidecar 摘要、源码补丁及日志见 [result.json](result.json)。源码未提交；
补丁不包含未跟踪文件，因此另附新生产适配器快照及摘要，重建时需同时恢复它。

Tauri 独立启动图不运行 Wails 几何轮询，本次生命周期修复不是 Tauri 最小化修复。
本轮未重新运行全量 Rust、完整窗口、启动中断、就绪后强制退出、双屏、通知点击、
钥匙串或官方数据回退；此前证据保留各自包来源。实际 Wails 卸载 IPC 交互、D 物理
验收及 A/B/C/E、正式签名/公证仍未完成。最小化根因、Stage Manager 与输入干扰
仍未确认，没有修改系统偏好或把未回复当作“没有输入”。

```sh
pnpm test:window-state
pnpm test:clipboard
pnpm test:typecheck
python3 -B tools/tauri/smoke-native-window.py '/path/to/Reasonix Tauri Preview.app' --independent
```
