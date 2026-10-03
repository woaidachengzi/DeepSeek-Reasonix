# Tauri 真实聊天入口：选区引用与同包回归

2026-10-03，基线 acd42f558 加 native/source.patch、source-untracked 和本目录source快照。Wails baseline snippets与selectedTextContext源码保存来源/hash：基线将选区添加为引用卡片，normalize/去重、按会话保留并发送quoted context，并非覆盖或直接追加输入框指令。

## 实现与review

TauriChatWorkspace接入共享lazy TranscriptSelectionMenu回调，复用ComposerContextCard及normalizeSelectedText/formatSelectedTextContext。引用草稿按session ID隔离，切回可恢复；重复内容只留一份，12,000字符限制/截断提示沿用基线，可移除、聚焦composer。输入文字不被替换，引用可单独发送；使用共享JSON引用上下文后缀，其中特殊字符转义，引用保持quoted-context语义。已接受发送只消费当次引用ID，失败保留引用和文字；发送期间新加的其他引用不会被旧提交清除。

用户历史和乐观消息复用共享context拆分，不显示内部JSON；普通正文、引用详情和文件附件分开呈现，先拆引用后解析附件，以免引用里的文件字符串被作为当前附件剥离。普通消息的内容保持。新增React入口回归已登记test:tauri；无新增native command/capability/dependency或Wails runtime直连。

## 当前真实安装身份

host b4cbb6be88ca74cc00fa6d350225c4b967905b10660fda092962cffcdce724ae，sidecar cb1b462ad6172758401674c8620515814663a3dc284b55c3e1334d61aea348a0，DMG 321f64dff58d5d02d87824d68cd0dc80eaab9beaf90b58af32c90175da3e3bea。app/DMG实际构建、只读复制安装及strict ad-hoc签名通过，目录见install.json。不是干净HEAD构建，未替换用户安装，未公证。调用方脚本在构建后补充读取新增原生回执，不影响已构建产品源码。

## 验证范围

- 原test:tauri全套通过；当次进程启动时尚未登记新测试，新入口用例随后单独运行通过，包含真实TauriSessionApp挂载、选区按钮、引用/草稿、去重/移除、session切换隔离与恢复、发送失败保留、成功发送消费及provider输入同一quoted-context格式。它使用stub bridge，**不是实际provider端到端验收**。完整test:transcript通过（含选区菜单88项），strict clippy通过；当前新安装sidecar对应Rust **231 passed / 0 failed / 5 ignored**。构建内类型/契约/lint/CSS/预算通过。
- 当前包 `message-copy/package/fullscreen/failure-exit/lifetime` **5/5**，见native/result.json。两档案实际聊天入口均提交一次私有nonce问题，本机固定provider回复，真实Copy写入精确OS文字并完整恢复原剪贴板；随后实际选区按钮和synthetic Cmd+L生成一份引用，输入draft保留、composer聚焦、selection清理、重复引用去重，**剪贴板文字与generation均保持**。八项原生布尔回执见selection-native-receipts.json。无重试菜单/按键动作；各provider只调用一次，所以第二轮quoted-input的真实provider接收仍待验。
- 同包双档案完整全屏五阶段往返各一次通过，nativeFullscreen/Tauri位与did-enter/exit通知、精确普通框和普通存档保护/重启保持；退出失败边界、lifetime八项cleanup/原件/重启通过。范围见各原日志与工具，不把这些通过作为物理按键或授权证明。
- **原窗口矩阵48/48**、双档案各24、kernel0，原断言/超时/容差不变，不重试。逐阶段receipt/原生trace/NSWorkspace时序/本阶段kernel保存，window-summary.json和window.log汇总。归档省略各阶段重复携带的之前阶段kernel副本；每个阶段自身kernel保留，完整临时输出仍在原路径。单次成功不关闭历史偶发窗口问题。

## 保留的失败与未验项

首个新测试命令工作目录错误，第二次夹具未激活会话，随后重复选区嵌套act未提交新overlay状态；各原失败日志保留。补全真实入口的激活/分阶段act后通过，没有改产品判据。external-opener的缺失Tauri fixture警告单独保存，补全其invoke stub后消失；SVG stub的empty-src警告为测试夹具。完整transcript第一次因沙箱拒绝tsx IPC未启动，原日志保留；在允许IPC环境相同命令通过。这些不记为产品通过前的真实安装包失败。

D仍未完整验收：第二轮引用请求的真实provider接收/重开历史显示、物理选区/Cmd+L/IME及Tauri快捷键设置中的selection action配置、页面未Finished的可见恢复、实际文件目录选择/保存、通知交互/重新授权、钥匙串人工授权/取消/旧服务、混合缩放/拔屏/无活动屏幕恢复、任意官方旧版自定义共享目录互斥和历史窗口稳定性仍待验。当前b4候选其余native权限/.env/显示器/官方备份回退不继承ecac或e7等旧包通过，只保留旧证据作为对照。

E remote/bot/updater/复杂管理页未开启；A协议/事件/错误覆盖、B旧Global图片/大历史、C实际工具/进程生命周期及正式签名公证仍阻碍发布。仅Windows/Linux沿用用户延期，无新增延期。未push、发布或改默认下载。归档不含私有HOME/core、钥匙串、原剪贴板快照或二进制，SHA256SUMS验证完整性。
