# 选区快捷键配置接入：源码通过，实际新包选区失败保留

2026-10-03，基线 HEAD `9de426cf4`。D继续进行中，E未开启；未授权正式发布/默认下载切换。本批不能称选区快捷键已通过真实安装包验收。

## Wails 对照与生产改动

Wails 1.38.3 `keyboardShortcuts.ts` 的 selection.addToChat 默认 Cmd+L，菜单标签与触发都读共享自定义配置；Tauri此前已有引用卡片/发送但只配置43动作，菜单仍读Wails配置。新增第44项 `add_selection`，默认Cmd+L，加入设置录键、冲突拒绝、重置和快捷键帮助；继续使用独立 `reasonix.tauri.shortcuts.v1`。

共享选区菜单通过可选 combo/matcher 注入使用宿主配置；未注入的Wails默认路径保持原解析与监听。Tauri菜单标签与触发均读相同配置，matcher稳定于模块级，IME/keyCode229与原生菜单冲突继续拒绝；只有可见选区且无设置/诊断/工作区/命令面板/快捷键帮助覆盖时启用。不扩宽原生权限或增加Wails runtime直连。

## 源码验证

完整Tauri回归通过（tauri-rerun.log）；首次完整回归因帮助页仍断言43行失败保留（tauri.log），更新为44后重跑通过。完整transcript回归通过（含共享菜单88项与其他原契约），strict clippy、实际build的lint/类型/契约/bundle门禁通过。当前安装sidecar的Rust **231 passed / 0 failed / 5 ignored**。

真实Tauri组件（bridge stub）覆盖设置页打开/实际录键/关闭/重新选区、改键立即生效、旧默认与Wails绑定隔离、IME不触发、持久化写入、重置不改Wails、引用去重/跨会话/失败保留/成功消费及quoted-context。最终设置流单独通过 settings-flow.log；先前完整Tauri通过后仅把同一个测试由直接setter加强为实际设置流，生产源码未再改。该测试不证明原生键盘/重启或实际provider请求。

## 新包身份

安装于 `/private/tmp/reasonix-d-selection-shortcuts-installed-yl6bd48p/Reasonix Tauri Preview.app`。本次源码差异及installer冻结；first-failure/source.patch为构建/首次门禁时源，final-source.patch/source为最终测试加强后的源。生产源码一致，不能把后来测试脚本的差异称包重新构建。

| 对象 | SHA-256 |
| --- | --- |
| host | `b1f7f210931a9523459e2d90530ca950d39d35de4fe22c44cd537cb6418749e1` |
| sidecar | `d3439788d0c0fb0841795e723eb0b29113968d4ff9a3cb8cb45abb4a786f56e4` |
| DMG | `836f127bdb7d0d650bb57e1b8aabde2a52601965f795c2816298161367bff9d1` |

实际app/DMG构建、DMG临时安装及strict ad-hoc签名通过；不是正式Developer ID签名/公证。末次真实plist ID查询两app无运行实例，新包摘要/签名保持，选区失败夹具无sidecar/readiness。

## 实际选区门禁失败

native/result.json：selection-send exit1/failed，managed ui-selection-send kernel2，explicit未执行；原队列 package/failure-exit/lifetime均not-run。失败原件、安装身份、执行源码已冻结，失败私有夹具未重用。

新探针实际打开快捷键设置、点击 add_selection 录键按钮、发送 synthetic Cmd+Shift+L、确认本机存储与录键结束并关闭设置，这些前置检查完成。随后首次 Add to Chat action 等待超时，未到引用第二轮发送、旧绑定停用验证或重启配置/历史检查。因此源码回归通过不代替本包这条失败。

当前失败没有原生窗口时序/帧时钟文件；别从不存在的trace推断原因。display-observation.json在**失败以后**真实CG查询status0/count0；原Mac锁定尚无用户解锁答复。选区按钮依赖requestAnimationFrame调度，但未记录失败时是否发生帧回调，不能确认无活跃显示器/锁屏为原因，不能直接判产品恢复缺陷已修复。下一步需补当时的只读调度/原生状态诊断，或在用户解锁并显示器活跃后完成当前候选同路径验收；不放宽断言、不重试直到通过。

失败完整剪贴板原格式恢复通过，日志保留；不归档剪贴板快照或私有HOME/core。

## 独立生命周期门禁

process/result.json：package/failure-exit/lifetime三项独立通过；原队列not-run保持不改。lifetime两档案idle/streaming × SIGTERM/SIGKILL共8项，kernel sidecar正常退出、原件保持与重启身份通过。不称D整组通过，不继承b6的17项、完整窗口或回退结果。

## 待验与发布缺口

本包选区实际第二轮/配置重启、完整窗口/全屏、显示器保护/恢复、其他D权限和当前包官方回退仍需对应新包证据。物理菜单/快捷键/IME、文件目录确认保存、通知权限/点击、钥匙串授权取消/旧服务、混合缩放/拔屏、页面未Finished异常恢复、任意旧共享目录互斥及历史间歇窗口问题保持未完成。A协议/事件/错误面、B旧Global/Markdown图片/大历史、C复杂进程生命周期及正式签名公证仍阻碍发布。没有自行延期或提前推进E。

归档源码、日志与非秘密JSON；`SHA256SUMS`覆盖全部冻结文件。仅源/证据检查点，不是验收完成的发布候选。
