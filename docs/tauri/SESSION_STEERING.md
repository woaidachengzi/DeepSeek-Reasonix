# 运行中引导会话

用户要求：运行中追加指令，引导当前任务调整执行方向；参考 MiMo 和 DeepSeek Harness 最新源码，按每批约 15 个文件 review、测试并提交。

## 参考源码

2026-10-14 获取官方源码：DeepSeek Harness 现有干净仓库执行 `pull --ff-only`，更新到 `d743267388641bc76f17c45ce8b4c231aed1d32c`；MiMo 本地目录没有 Git 信息，保留原快照，另行浅克隆官方 main 至临时目录，HEAD 为 `e27e87de81a74db5475bd93540f8889dfbc70ec6`。首次直接连接超时，使用已有本地代理后成功。

- [Harness 提交策略](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/packages/client/ui-conversation/src/client/input/submission-policy.ts)将 queue／steer 区分，实际窗口由 Host／Agent 判定；[原生 Web 场景](https://github.com/deepseek-ai/deepseek-harness/blob/d743267388641bc76f17c45ce8b4c231aed1d32c/apps/web/tests/steering.e2e.ts)检查暂存到持久引导消息的交接。
- [MiMo TUI prompt](https://github.com/XiaomiMiMo/MiMo-Code/blob/e27e87de81a74db5475bd93540f8889dfbc70ec6/packages/cli/src/cli/cmd/tui/component/prompt/index.tsx)及 [session prompt](https://github.com/XiaomiMiMo/MiMo-Code/blob/e27e87de81a74db5475bd93540f8889dfbc70ec6/packages/cli/src/session/prompt.ts)分别管理输入／中断及当前用户身份。当前源码没有可直接复用的 `TrySteer` 接口；不声称其与 Harness 具有完全相同的行为。

## 本轮行为

运行或等待审批时，输入框可提交文字“引导当前任务”，独立的停止按钮保留。发送快捷键使用同一行为。等待审批仍需处理审批，引导不会自动批准工具。附件保留为草稿，当前引导仅接受文字及选中文本。

新 `:steer` 命令按精确会话 ID 调用现有 Controller.TrySteer，不重启模型，不打开新回合。输入上限 16 KiB，幂等请求 ID 防止重复注入。回合已经结束、引导窗口关闭或没有可接受的 Agent 时返回脱敏 409；输入保留供用户重新发送，不采用历史兼容 fallback（可能造成误执行）。

UI 区分“引导已提交”和内核 `steer` 消费事件后的“引导已应用”，未应用 notice 给出明确错误。引导消费后由已有内核持久化为用户来源的中途指导，历史和模型上下文沿用已有机制。异步成功响应不覆盖会话运行状态，避免已经完成的任务被显示为仍运行。

本轮暂未实现 Harness 的可编辑 FIFO 队列面板／QueueDock。首次改造重点为正在执行任务的直接引导、精确会话隔离和反馈。

## 验证与 review

实现批次 15 个文件。完整 Go race、Rust 320 项（6 项原有忽略）及 Tauri 前端回归通过；补充组件验证拒绝引导保留输入、文字引导保留附件、等待审批时不会绕过提示。真实 Go 控制器在工具边界将 A 的指令交给模型，重复请求仅应用一次，B 的 scope 与请求保持不变；已完成任务拒绝引导且没有新模型请求。

Browser 插件不可用，使用项目已有 Playwright／Chrome。真实 React 界面深浅主题检查引导与停止按钮分离、输出保留、双会话切换和侧栏状态，console／框架错误为 0。review 修正成功响应覆盖已完成状态的风险，并使测试夹具在发布请求之前确定调用顺序，避免并发调度改变工具边界归属。

原资料接管候选版本更新为 0.1.6，沿用原应用身份和资料准备规则。
