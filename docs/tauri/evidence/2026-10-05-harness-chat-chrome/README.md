# Harness 风格对话显示

用户要求参考本地最新 `/Users/jerry/temp/app/deepseek-harness`，重点是用户消息下方的日期/复制，以及回答上方的“已完成，用时”折叠行。只读核对源 revision `5badb15009`（根 LICENSE 为 MIT / Copyright 2026 DeepSeek），涉及 `packages/client/ui-chat/src/client/chat/{TurnProcessNodeView.tsx,TurnProcessNodeView.module.css,MessageIconActions.tsx,MessageIconActions.module.css,message-chrome.ts,MessageItem.module.css}`。在 Reasonix 已有组件和桥接数据上独立实现，没有引入 Harness 的运行时或复制整个项目。

## 结果与 review

- 用户消息采用右对齐的中性圆角气泡，下一行时间和复制按钮；当天显示时分、往日显示月日、跨年带年份。缺失/非法历史时间省略，发送中的新消息一次记录本地时间，随后以历史权威时间替换。复制沿用共享 CopyButton，仅写入成功才显示成功反馈。日期相对当前本地日期在渲染时计算。
- 过程行改为透明背景、底部分隔线，文字“已完成，用时 47秒”和右侧展开箭头；进行中仍显示“正在处理”。最终回答独立在行下方左对齐，不因折叠隐藏，不再重复显示头像和 Reasonix 名称。真实已存耗时精确到秒/分/小时，不凭空给旧记录补耗时。
- 单条有耗时的回答也显示完成行，但没有过程内容时不提供箭头或空折叠；同耗时的不同回合用首条历史的绝对 index 保持独立身份。原先运行中尾部、默认折叠/深度展示和手动展开逻辑保持。
- Review 核对历史缺失时间、绝对 index、同耗时身份、活动尾部与单答案。Bridge 尚未暴露 Harness 式显式 commentary/final 和持久化 turn endReason，仍沿用“最后一条 assistant 为回答”的已有分组；未把本次样式修改声称为错误/停止等全部终态协议迁移。没有修改滚动 writer/kernel、权限、模型配置或 API Key。

## 验证

`pnpm test:tauri` 全套通过；`pnpm test:transcript` 通过（含 single-writer 静态门禁、kernel/视口/分页竞态与70项 projection 断言）；`pnpm build` 生产契约/hooks/类型/CSS/主题/bundle 门禁通过。原始日志在本目录。Transcript 首次在沙箱内遇到 tsx IPC `listen EPERM`，在获准的沙箱外运行通过；Tauri 测试末尾已有 SVG 空资源夹具警告不是本次浏览器结果，未隐藏。

Browser plugin not available，使用已有 Playwright 和已安装的 headless Google Chrome。实际 `TauriSessionApp`、history renderer、共享 CopyButton 和生产 CSS；仅 bridge/history/native invoke-event 响应模拟，剪贴板 write 捕获在页面内，不操作系统剪贴板、用户账号或真实模型。临时页面 `http://127.0.0.1:5197/chat-qa`，before 为修改前 HEAD 文件，after 为本次工作区源文件。

深色/浅色 × 1280/768/390px 六组检查均通过：页面 title/非空实际内容、无框架 overlay、console error/warning 为空、元信息/状态行在 transcript 内且没有横向溢出。额外验证展开/收起中间输出、最终回答始终可见、单回答完成行无空箭头、往日/跨年时间、复制原文与成功图标。宽屏/窄屏截图已人工查看，原始几何和交互结果在 before.json/after.json，脚本为 browser-qa.mjs。

## 包与范围

按已有用户授权 review/提交后构建直接 `.app`，不生成 ZIP/DMG、不替换 Applications。保存前一包为 `.app.rollback`，避免新增应用搜索图标；包的 source/hash/signature/回退位置另见 package-receipt.json（构建后追加）。当前证据不包含原生 WebKit GUI 和真实模型网络验收。用户接受后续通过日常使用反馈；原 D/E/A/B/C 及正式签名/公证缺口保持，未正式发布或切换默认下载项。
