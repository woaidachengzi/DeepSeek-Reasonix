# macOS D：宿主事件收敛与 DMG 安装验收

2026-10-02。D 尚未结案，E 未推进；没有收到本轮物理验收延期确认。
本轮修复和验收可分别审阅，不能把安装包的原生 API 操作推广为实际鼠标/键盘操作。

## 改动与基线

对照 `reasonix-wails-1.38.3/desktop/frontend/src/components/DesktopCloseBehaviorHint.tsx`，
共享消费者存在相同的异步状态查询覆盖新托盘事件问题。受控回归在改动前失败，改动后通过。
现在先订阅宿主状态，再发起查询；其间收到有效事件时不接纳旧查询结果。
未收到事件时仍接纳初始查询，失败时保留原有提示策略。

设置菜单与关闭行为消费者的 `window.runtime.EventsOn` 移入 `bridge.ts` 的两个固定事件接口。
解除订阅幂等，已排队回调在解除后失效；浏览器没有宿主事件时返回空清理函数。
这是共享 Wails 消费者与 adapter 的收敛，Tauri 独立启动图继续使用现有 `host:open-settings`
入口，未注入 Wails shim，也没有改动 Rust 最小化行为或新增 renderer 权限。

测试类型检查还发现既有快捷键 JSON 可选 modifier 被推断为 unknown；测试改为严格比较
`=== true`，保留原生组合占用断言。生产快捷键行为未变化。

源 HEAD、未提交源码补丁、补丁 SHA-256、DMG/安装后 host/sidecar SHA-256、门禁退出码
见 [result.json](result.json) 与 [source.patch](source.patch)。源码修复仍在工作树中。

## 真实安装结果

使用项目要求的本地缓存 pnpm 10.34.5 与 Node 26.10.0 完成生产构建。
测试类型检查、专项回归、`test:app-lifecycle`、生产前端门禁及完整 Tauri `.app`/DMG 构建均通过。
本机 ad-hoc 签名严格校验通过，未进行 Developer ID 签名或公证。

DMG 经 `hdiutil verify`、只读挂载、`ditto` 复制至私有临时安装目录、严格签名校验后卸载。
安装后 host/sidecar 摘要与工作区构建一致。以下七组门禁在复制出的应用上顺序运行；
各脚本使用自己的私有 HOME/core，失败不重试，也不替换用户的安装或档案。

| 门禁 | 结果 | 证据与边界 |
| --- | --- | --- |
| 包级启动/退出 | 两档案通过 | [package.log](package.log)：环境隔离、未认证请求拒绝、readiness 与 sidecar 清理 |
| 完整窗口 | 两档案各 23/23 | [window.log](window.log)：原生窗口 API、菜单、剪贴板权限、编辑角色、取消面板、第二实例焦点、背景任务与退出 |
| 宿主异常退出 | 8/8 | [lifetime.log](lifetime.log)：SIGTERM/SIGKILL × 空闲/流式 × 两档案，sidecar 内核退出码 0、目录锁释放、原件和同档案重启 |
| 双屏 | 8/8 | [displays.log](displays.log)：两块 2× 屏幕，副屏定位、重启、跨屏恢复与再次重启；不证明不同缩放、实际拔插或物理拖动 |
| 配置导入 | 3 阶段通过 | [profile.log](profile.log)：实际 host/bridge 导入、修改后重启、显式档案拒绝导入及旧目录字节/权限/mtime 原件保护；未传旧 CLI，不证明旧 GUI 或全量数据回退 |
| 外链 | 两档案通过 | [links.log](links.log)：主 WKWebView URL 拒绝、默认浏览器真实请求回执与清理；不证明邮件/OAuth/指定编辑器实际交互 |
| 通知 | 6 次系统送达通过 | [notifications.log](notifications.log)：固定标题/正文、隐藏发送、精确清理与同档案重启；两档案 initial active 均 false，不算前台或横幅视觉/点击/拒绝恢复验收 |

此前未改产品的候选也通过一轮 46 项完整窗口门禁，以及两档案独立菜单/剪贴板/编辑/取消面板
8 项门禁，见 `previous-bundle-window-pass.log` 与 `previous-bundle-independent.log`。
`previous-window-failure.log` 保留更早托管 23/23、显式首项最小化失败的证据，属于旧候选。
本轮没有定位该偶发失败根因；两次成功不是原问题已修复或长期稳定的证明。

## 未关闭的发布门禁

- D：历史偶发最小化失败；实际消息/失败反馈、IME/自定义组合、托盘/Dock、系统明暗切换、
  邮件/OAuth/指定应用、通知权限拒绝及点击、钥匙串拒绝与设置迁移、其余 UI 偏好冲突路径。
  原生 UI 工具三次 `cua.getState` 均在 30 秒超时并重置；未执行的操作未记为通过。
- D 数据：当前 lock-aware Wails/bridge 使用 `profilegate`，1.38.3 基线没有该协议。
  独立默认目录及配置导入原件保护不证明任意旧二进制在显式共享目录下互斥。
  旧 GUI 单实例、全量历史/附件/检查点及完整回退继续使用清单中各自证据边界。
- A：真实包启动与 bridge 生命周期已补验；其他 API/event 面与异常 UI 仍须按清单逐项认证。
- B：旧 Global/Markdown 图片 resolver 与模型图片输入、完整历史/附件/检查点恢复，以及大目录、
  diff 和跨会话的完整 Preview 交互证据仍不足。
- C：现有终端 cwd、任务退出及 MCP/插件切片不等于 shell/terminal、Browser、worktree 和全部
  MCP/插件进程、资源、权限边界已完成。
- E：remote host、bot、updater、复杂管理页的完整配置、连接/进程、恢复和权限验收尚未完成。
  updater 插件注册与 Updates 说明入口不等于可用更新流程。
- 本轮候选具备构建和独立档案/配置原件保护路径；全量数据回退尚未认证。正式签名、公证、
  发布和默认下载切换仍是独立门禁。Windows/Linux 延期沿用既有用户确认；本轮不扩大延期范围。

重跑：以当前 DMG 只读挂载复制出的 `.app` 为参数，依次执行 `tools/tauri/` 下
`smoke-packaged-app.py`、`smoke-native-window.py --focus --edit --dialogs`、
`smoke-host-lifetime.py`、`smoke-native-displays.py --straddle`、`smoke-profile-import.py`、
`smoke-native-links.py`、`smoke-native-notifications.py`。先确认同 bundle identifier 无普通实例。
