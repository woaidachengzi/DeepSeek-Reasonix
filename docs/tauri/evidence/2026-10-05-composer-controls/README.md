# 对话输入区、审批模式与模型选择

按用户截图参考：更宽松的圆角输入框，左下附件 + 工具审批选择，右下当前模型 + 圆形发送/停止。保留草稿、附件、选中文本、发送快捷键与原审批卡片。顶部原“新对话默认模型”选择移入输入区；当前对话使用现有会话模型切换 API，新对话仍设置默认模型，工作区配置可能覆盖。模型菜单按提供商分组、可搜索并标记当前项，只列出已配置凭据的模型，提供设置入口。

截图的只读/工作区写入模式不等于 Reasonix 已有的 ask/auto/yolo。本次使用“需要审批 / 自动审批 / 完全权限 · Yolo”及明确说明，不宣称工作区写入或只读隔离；原拒绝规则、显式询问和沙箱约束保持有效。运行中/等待审批时禁改。

新增仅限当前桥接会话的 GET/POST approval-mode API：令牌认证、幂等 POST、归属核对、模式白名单与 idle 门禁；RuntimeManager 串行化。元数据先写成功再更新控制器 gate，失败不改 live posture；首条消息前保存的模式重开时也优先恢复。新对话只修改 desktop 默认，不修改其他会话或全局 permission/sandbox 规则。

## 验证

- Go 两组完整回归通过；覆盖真实 Controller 首轮前持久化/重开、错误模式、其他会话、running/paused 拒绝、写入失败、HTTP 未认证和幂等。首轮测试发现首条消息前模式被默认值覆盖，已修复并重新通过。
- 目标 UI 单测通过：当前会话/默认 scope、写入拒绝保留原显示、模型 ref、未配置服务不展示、禁用和 Escape。完整 test:tauri 退出 0；存在已有 fixture 的空 logo src / ExternalOpener invoke 缺失警告，不宣称整套日志无警告。
- cargo check 与严格 all-targets clippy 通过；完整前端 build 门禁通过，不调高本次任何预算。
- 浏览器使用统一 CUA in-app browser（Browser 专用插件未提供），临时 mock IPC fixture 复用实际组件与样式。页面 http://127.0.0.1:5197/composer-qa.html / Reasonix Composer QA，默认桌面与 390×844 检查；内容非空、无 overlay、console error/warn 为空。审批切换显示自动、搜索 reasoner 只留匹配项、选择后当前按钮更新；窄窗 scrollWidth=390，菜单范围 x29..359 在视口内。临时服务器/fixture/tab 已关闭移除，实际截图见 browser-mock.png。与参考区别：使用现有主题配色及准确审批语义，没有虚构 High 推理档位。

## 待验边界

mock 浏览器不是实际 Preview 安装包；真实服务模型切换、原生权限与审批交互端到端 smoke 仍待。新包需另行记录来源与哈希，不能继承旧包验收。D/E 和 A/B/C 发布缺口保持，未正式发布或切换默认下载项。
