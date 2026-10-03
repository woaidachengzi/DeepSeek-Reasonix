# 2026-10-02 当前包完整窗口：31/46，仍失败

实际安装包 host `b95b38c0e35d66c964bc370b621a46c9d69116b7408a7c7567f4c715abc17734`，来自 runtime 生命周期批次的完整 app/DMG，只读安装未修改。严格签名及每门禁后的摘要验证通过。

命令为串行 runner 的 `--gates window lifetime`。完整窗口使用原 `--focus --edit --dialogs`，断言没有放宽。托管档案 23/23 全部通过，包含正常/最大化几何、隐藏/最小化 Settings 恢复、实际后台任务的菜单退出、外观恢复/回退、原生剪贴板及编辑、四类面板取消、第二实例焦点和关闭退出偏好。

显式档案前 8 项通过；第 9 项 `menu-settings-minimized` 的真正最小化前提失败。共 31/46 成功，1 个阶段失败，余下 14 个窗口阶段没有执行。串行后续 lifetime 保持 `not-run`，另一个独立批次才补跑该门禁。

## 原生事实

请求前主页面完成、真实 active/key/main/visible 为 true，窗口与应用 occlusion Visible 为 true；允许最小化，正常 styleMask=32783，无 sheet/modal/live-resize，restore 请求/完成各 1，Reopen=0，两次外观请求均跳过。

请求后发生 resigned-key/became-key（614/650 ms）以及 1811 ms 的应用失活/失去 key；超时仍可见、非最小化，Will/Did/DidDeminiaturize 全为 0。这个结果说明当前候选仍有偶发失败，不能把托管成功推广为两档案窗口稳定，不能据失活推断根因或用户干扰已确认。

失败现场 `/private/tmp/reasonix-native-window-smoke-jym49jy0` 保留；`reasonix-native-window-trace.jsonl` 与 result 原始固定诊断已复制，未复制 core 档案/凭据。完整日志、脚本快照、签名及包摘要在本目录。源码补丁不含新生产适配器，其快照与包构建归属见 `../2026-10-02-d-runtime-lifecycle/`。

## UI 通道重查

在完整门禁结束并确认无其他同标识 Preview 后，再次使用既有私有档案启动同一包。CUA 应用清单可读，但重置观察会话后 `getApp` 仍报 ScreenCaptureKit -3811；未执行文件面板操作。向核对路径/父子关系的私有 host 发 SIGTERM 后无 host、sidecar、ready 文件残留；不属于 UI Quit 验收，见 `ui-retry-cleanup.json`。没有改变录屏、共享或 Stage Manager 系统设置，尚未确认外部环境为根因。

D 未验收，E 未推进。物理文件确认、最小化/托盘/Dock/IME、通知和钥匙串交互、不同缩放/实际拔插、完整历史数据及旧 Wails 生命周期互斥等未验事项，以及 A/B/C、正式签名/公证和发布授权缺口均保留。
