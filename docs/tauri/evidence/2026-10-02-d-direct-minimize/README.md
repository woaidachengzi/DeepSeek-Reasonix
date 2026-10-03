# macOS D：直接 NSWindow 与最小 Cocoa 对照

2026-10-02。目标进行中，D 未验收、E 未推进。没有修改生产窗口恢复策略、放宽
原完整窗口门禁或新增延期。本轮只增加 opt-in 直接原生调用实验。

## 实验与结果

`--direct-only` 在 Preview 的实际主线程和 live NSWindow 调用 Tao 使用的
`NSWindow.miniaturize(window)`，绕过 Tauri 消息调度和菜单 responder chain。
仍要求实际主 key 窗口、最小化状态、Will/Did Miniaturize、Settings 恢复及
Did Deminiaturize。它不替代原 API、菜单或完整窗口验收。

新 app/DMG 构建、严格 clippy、DMG 校验/只读临时复制/签名/卸载镜像通过。
host 摘要 `de4f88c88dc80f3757b577315dbe9d42b8f75134e914851065a70bae34d814f5`；
路径、完整摘要、源码补丁和逐项日志见 [result.json](result.json)。源码仍未提交。

- **Preview direct-only 失败**：托管首次最小化未完成，显式及后续阶段未运行。
  Will/Did Miniaturize 为 0，窗口从 key 状态退出；超时 applicationActive=true、
  applicationResignedActive=0，因此该样本不能完全解释为应用失活。私有夹具保留，
  没有重试覆盖。正常两档案 package smoke 另行通过。
- **最小 Cocoa/WKWebView 对照通过**：私有、独立 bundle/HOME，固定 HTML，使用
  相同主要窗口 style、页面完成后 hide/show、真实 active/key 前提和直接 miniaturize。
  299 ms 请求，834 ms 已最小化且观察到 Will/Did，1504 ms 恢复并观察到 Did Deminiaturize；
  最后退出码 0。源代码、固定状态日志和私有路径在 `cocoa-control.*`。
  该程序不启动 sidecar，不读取用户凭据或其他应用内容；运行与 Preview 实验串行。

这排除了“当前会话完全不能执行原生最小化”，且直接 Preview 调用仍失败，不能
只归因于 Tauri 消息调度或菜单 action。最小程序与 Preview 的 bundle、delegate、
事件循环、内容和其他宿主上下文仍有差异，尚未证明根因或代码/环境归属。

只读 `defaults read com.apple.WindowManager GloballyEnabled` 返回 1，当前 Stage Manager
开启；本轮未修改该设置，也未把它当作已确认原因。已询问检查期间是否存在用户输入
或其他应用交互，答案未收到时保持未知。不能据此擅自调整系统偏好或豁免窗口验收。

## 复跑与剩余范围

```sh
python3 -B tools/tauri/smoke-native-menu-window.py '/path/to/Reasonix Tauri Preview.app' --direct-only
```

`--direct-only` 不可与其他实验选项组合；独立私有档案失败时保留证据，拒绝操作
已运行的普通 Preview。原完整窗口门禁及历史失败继续保留。
当前诊断包未重复全量 Rust、目录拒绝/恢复、强制退出、双屏、通知、钥匙串和官方回退；
这些先前证据分别保留原包来源。物理托盘/Dock/IME、不同缩放/拔插、通知和钥匙串
权限交互、旧宿主目录互斥、A/B/C/E 以及正式签名/公证仍是发布缺口。
