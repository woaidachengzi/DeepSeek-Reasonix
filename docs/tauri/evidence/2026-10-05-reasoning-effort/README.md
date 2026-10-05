# 思考强度选择与官方 MiMo 协议验收（2026-10-05）

## 实现与 review

Wails 基线 `SetEffortForTab` 使用 `config.NormalizeEffort`、同对话 runtime 重建、idle 校验和失败保留；Tauri 此前仅提供审批与模型选择，没有把已有思考能力接到输入框。参考本机最新 Harness `packages/client/ui-model-selection/src/client/ModelSelect.tsx` 的精确模型能力与 Host-owned effort ID 设计，复用 Reasonix 适配器 capability，而未导入第二套 runtime 或客户端硬编码所有模型的档位。

- 模型旁添加“思考强度”，实际选项来自解析后的模型及 per-model override；新对话的显式选择通过首次 Send 的 open payload 传入，现有对话对后续消息生效。新对话草稿选择不写全局 provider 配置；重新打开应用前的未发送草稿不保证保留。发送中、待审批、附件在途时沿用对话门禁禁止重建。
- 同 session mutex 保护 ownership、idle 与准确 modelRef；复用 settings Build/migrate/publish，保留 transcript、审批姿态和 grants。保存元数据在 replacement publication 前进行，构建失败条件回退所改的 metadata；旧 controller 不用 shutdown 覆盖新历史。模型切换默认重置 effort，失败恢复携带旧 effort。
- session `.meta` 新增可选 model-bound reasoning selection；snapshot/meta 合并保留该字段；重启、凭据刷新和会话重开读取原选择。协议 v1 添加可选字段，Go DTO/TS/Rust 门禁保持启用。provider summary 仅包含 ID、档位、默认和当前配置选择，不含 endpoint/key/header。
- 重点修正 MiMo：Chat 只发 `thinking.type=enabled|disabled`，去掉无效 `reasoning_effort`；Responses 展示 `enabled|none`。保存的 legacy low/medium/high 保持“开启”效果，新界面不提供无效深度。自动/关闭/推理档位仍由各适配器序列化。
- review 发现并修复：模型切换携带旧 effort；disabled 按钮导致选完焦点无法恢复；runtime publish 后再写元数据的失败顺序；快照覆盖新增 preference。最终测试同时覆盖失败保留、无能力、catalog 错误、disabled 和 Escape。

官方资料核对（2026-10-05）：[MiMo Chat](https://mimo.mi.com/docs/zh-CN/api/chat/openai-api) 仅 `thinking.type` 开关；[MiMo Responses](https://mimo.mi.com/docs/zh-CN/api/chat/responses) 说明 none 关闭，其余级别行为相同；[DeepSeek](https://api-docs.deepseek.com/guides/thinking_mode/) 支持 low/high/max。第三方 compatible gateway 的自定义能力以其配置声明为准。

## 验证

- Go race 完整 openai/responses/config/desktopbridge/protocolgen/bridge command 回归通过；新 runtime effect test 验证实际 HTTP 请求 low/max/disabled、历史保存、重启、真实 `.env` key 刷新、构建失败 metadata 回退。agent 的 BranchMeta、保存与 accounting 专项 race 通过。
- 完整 `pnpm test:tauri`、仓库要求的 `pnpm test:transcript`、`pnpm build` 与 build-contract 36项通过。既有 jsdom 空 SVG src 警告保留在原始日志，实际浏览器没有 console/page errors。
- Rust 使用当前实际 sidecar：233 passed / 5 ignored；严格 clippy `--all-targets -- -D warnings` 通过。未把系统交互 ignored 算作已验收。
- Browser plugin 不可用，使用既有 Playwright + 隐藏的本机 Chrome。实际 TauriSessionApp/ComposerControls，模拟 bridge：深/浅主题 × 1280/768/390，无页面/console error、无 Vite overlay，菜单未越界，Escape 与选择恢复焦点；首次 Send 传递准确等级，切到 MiMo 只显示开关，不支持的模型显示默认说明。该浏览器夹具不是 native WebView 验收。
- source-smoke 用真实 sidecar、一次性 profile、file credentials 的假 `.env` Key、本机 SSE 服务：capability authentication、首次 max、当前 low、非法等级拒绝与历史不变、重启恢复、模型切换重置、MiMo binary payload，通过。5个本机 provider 请求，2次退出0、ready 文件移除。首轮夹具失败因误写 API 路径及仅设置进程环境密钥；按 Reasonix 正常 credential 来源改用隔离 `.env` 后通过，没有操作真实凭据。

## 包与边界

旧已验证应用已复制到 `rollback.json` 的 `.app.rollback` 路径，未改用户 profile。待本提交的 direct `.app` 构建与包内 sidecar smoke，随后补 package receipt。产物继续放在 `macos.noindex`，不制作 ZIP/DMG，不新增安装入口。

系统钥匙串/弹窗/通知及真实外部 provider 调用未触发。本次属于用户日常验收反馈修复，不新增 D/E 完成声明，不替代 A/B/C、签名/公证、跨平台与其他发布门禁。正式发布和默认下载切换仍未获授权。
