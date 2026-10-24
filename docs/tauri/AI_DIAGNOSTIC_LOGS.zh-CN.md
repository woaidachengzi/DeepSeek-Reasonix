# AI 诊断日志

在待分析会话中打开 **设置 → 诊断 → AI 诊断日志**，点击“导出当前会话日志”，选择本机 JSON 文件保存位置。无需再次执行失败操作，也不会自动上传日志。

“导出详细内容”默认开启，选择保存在本机。开启时包含用户和助手消息、模型实际返回的思考文本、工具参数、完整本地结果和模型可见结果。重复结果保留，便于核对。关闭后只导出简洁消息、调用标识、执行状态和电脑控制结果摘要。**这个选项只控制导出内容，不改变已有会话记录的保存方式。**

## 如何分析

- `entries[].modelReturnedReasoning` 是渠道返回的思考文本。有些模型不返回；不能据此认定工具实际完成了操作。
- 工具调用与结果用 `toolCalls[].id` / `toolCallId` 对应，`resolvedName` / `capabilityId` 标明能力代理实际调用的目标。
- 详细模式的工具 `content` 优先采用完整本地结果，`modelVisibleResult` 记录被截断或折叠后模型看到的内容。
- `computerUse.dispatched=false` 表示工具明确报告未派发输入；`blocked_reason`、`observation_age_ms`、`observation_ttl_ms`、`next_action` 可区分回执过期、目标改变等情况。字段不存在时是未知，不能当作 false。
- `dispatch_method=window_server_mouse` 标明坐标点击采用 WindowServer 前台路由。
- `dispatched=true` 表示派发输入，不保证目标控件接受输入或任务完成；`dispatch_unknown` 表示派发结果不确定，不能自动重试输入。`input_verification=confirmed` 才是字段内容匹配的验证结果。
- `toolRunState=completed` 只是工具执行结束，不能替代上述派发或结果验证。
- `createdAt`、`workDurationMs` 沿用现有会话元数据；`recentToolTimings` 提供当前本地服务内存事件窗口内的工具开始、结束与耗时。该窗口有界，服务重启后清空，早期事件可能被淘汰，缺少时间不能解释为耗时为零。
- 原生导出附加 `host` 中的程序版本、提交、构建是否有未提交改动、平台与会话标识，便于确认运行的是哪个安装包。

## 范围与边界

导出是当前会话分支的已保存消息快照，不是所有会话、所有历史分支或正在生成中的完整追踪。历史在压缩前已经丢失的信息不能恢复；进行中的请求可能尚未出现在快照中。消息和事件时间来自不同快照，不保证整体原子一致。

最多导出最近 400 条消息，消息 JSON 合计最多约 3 MiB；单文本字段最多 32 KiB，截断以 `truncated` 标记，省略消息计入 `omittedMessages`。JSON 格式化后的原生文件上限为 8 MiB。系统提示、截图及图片二进制不导出，仅保留图片数量。

导出会遮盖常见密钥、令牌和 Cookie 模式，不能保证识别任意形式的敏感信息。聊天正文、收件人等个人信息仍可能存在，分享前应检查。导出不修改模型历史或权限，也不会执行电脑控制操作。
