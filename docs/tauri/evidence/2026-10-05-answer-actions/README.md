# 回答复制、时间与本轮用量

用户确认按 Harness 回答底部操作行优化：先实现复制、时间及本轮用量详情。本地 Harness revision `5badb15009` 的 MessageIconActions、TurnTailNodeView、TurnUsagePanel 为只读参考：操作行在回答结束后，复制整个回答，时间在末尾，用量可点击展开逐轮明细。本轮不增加分叉或赞/踩空按钮。

## 实现与 review

- Tauri 最终回答下方新增复制、紧凑用量和日期，沿用共享 CopyButton 的成功/失败语义；复制显示原文（包含 Markdown），中间过程与流式片段不重复显示操作行。时间来自保存的 assistant 完成时间，不给旧记录伪造时间。
- 用量弹层包含输入、输出、总 tokens、思考（输出子集）、缓存命中/未命中与请求次数；支持 Escape/关闭返回触发按钮焦点、点击外部关闭、页面滚动/resize 关闭，portal 在宽/窄视口中夹紧位置。操作行可换行，不溢出；换会话采用会话+绝对历史 index 的子组件身份，换文本/时间关闭旧弹层。
- healthy model completion 将 immutable RequestUsage 副本和时间作为可选本地字段保存到原会话消息；DAG/event/checkpoint 的既有写入/读取路径保留它们，不增数据 schema 或另建用量数据库。ModelMessages 移除用量，ProjectionMessages 保留；实际模型请求中没有本地用量/时间。
- bridge 在分页之前扫描原历史，遇真实用户提问重置，从所有 assistant 请求累加本轮数据，包括不可见工具请求；每个可见 assistant 得到独立累计快照。只公开安全整数/布尔数值，不公开 reasoning 文本、system、工具参数、API Key 等。输入/输出/总量独立保留真实计数，思考不再次加入总量。
- 旧记录无用量时显示“用量未记录”；新请求缺失 terminal usage 用 Unknown 标记，不冒充精确零消耗；部分记录显示“已记录”，Estimated 显示约数并解释。缓存 split 在采样 billable-input 归一化之前记录 unknown 来源，缺失时不显示假的零命中。负数/超过 JS 安全整数/累加溢出不发布该请求，保留 partial 标记。
- Go/TypeScript/Rust schema 添加可选 turnUsage，版本1及旧客户端保持兼容。DTO 门禁新增 optional omitempty pointer 的实际编码类型支持，required pointer 的 null 仍拒绝，独立测试覆盖；并非删除协议检查。
- Review 核对同轮多请求/工具不可见消息、跨轮重置、不可变快照、无 usage、estimated、缓存未知、请求边界、旧数据重载、弹层身份/焦点。仍沿用原 Tauri visible-history 最后 assistant 为回答的分组，不声称已迁移全部 turn endReason / 多通道协议。取消/失败片段没有完整 accounting 时保持缺失/部分提示。

## 回归与实际进程

Go race 全量 provider、desktopbridge、protocolgen、bridge command 通过；agent race 专项覆盖 Run 真实工具循环、本地字段保存/重载、DAG 保存、usage/sampling/stream 汇总。初次沙箱内 Go 编译缓存无权限，获准沙箱外后运行；首轮协议门禁发现可选 pointer 类型不被 checker 支持，修复并加 required/null 保护回归后全量通过。

前端完整 pnpm test:tauri（含新实际组件的复制/焦点/缺失/估算测试）、pnpm test:transcript、pnpm build 和 tauri build contract 通过。Rust 带当前真实 sidecar 完整测试 232 passed / 5 ignored，系统交互 ignored 保留未验。所有原始输出在本目录。

source-smoke.json 使用当前实际 sidecar、一次性 profile、仅本机 OpenAI SSE mock：第一轮225/第二轮450 tokens、各自输入输出缓存与思考、完成时间、重启字节同值、旧包读取新历史再回新包保留统计、无 terminal usage unknown，以及 provider 请求不泄漏本地元数据均通过。4次进程退出0，ready文件清理，3次模型请求都在本机。不是外部 MiMo/DeepSeek 服务验收。

## 浏览器

Browser plugin not available，使用已有 Playwright + 安装的 headless Chrome；本机 http://127.0.0.1:5196/chat-qa 渲染实际 TauriSessionApp / CopyButton / TauriAnswerActions / Markdown / CSS，只有 bridge与宿主响应模拟，native clipboard write 捕获在页面内。before 固定 revision a52c4ddfd；深色/浅色 1280/768/390px 页面身份/内容非空/无框架 overlay/console error-warning 为空、行布局无横向溢出通过。实际展开/收起过程保留回答、复制整个 Markdown 回答、用量精确值和思考子集、宽/窄弹层位于视口内、Escape还原焦点、reload后重新打开会话得到fixture用量均通过。真实重启持久化由source/package smoke单独证明。截图已查看。首次 browser reload 未重新打开 mock 会话导致等待超时；修正夹具流程后通过，不修改产品来迎合夹具。

## 打包及入口清理

按用户已有 review/提交/打包授权，干净源码构建直接 macOS arm64 .app，不生成 ZIP/DMG。用户新增要求清理搜索入口并仅保留现有正式版：只读 Spotlight 确认 /Applications/Reasonix.app 与三份 Preview（Applications、macos、portable）。正式版保留不替换；旧 Preview 程序移动到不索引的回退位置，保留用户数据；本轮新包仍为 Preview，不冒充正式发布。

构建脚本将实际 macOS 包放入 macos.noindex，保留 macos 相对链接供现有 Tauri/验收路径使用；冲突目录/错误链接失败关闭，不覆盖未知目录。实际 app 构建将验证此路径、签名与嵌入源码。包回执、包内 smoke和精确清理路径在本目录构建后追加。当前数据字段为 additive，旧包能读原历史；正式发布/默认下载项未切换。

## 实际新包完成

干净源码 `3f4f0a7b0` app-only 构建成功，macos 相对链接确实指向 macos.noindex；严格 deep codesign、host 嵌入源 revision、包内 Go vcs.revision 与 vcs.modified=false 核对通过。package-receipt.json 记录实际路径、摘要及前包备份。strict clippy 通过。包内实际 sidecar 的 health/401/shutdown/exit0/readiness清理通过。

package-smoke.json 在新包实际进程中重复本轮用量/时间、两个独立回合、重启、缺失 terminal usage、provider本地字段隔离和前包回读全部通过；4次正常退出、3次本机 SSE 请求，未调用真实模型或打开原生窗口。程序仍为 ad-hoc Preview/未公证，本轮未正式发布或替换正式版。

final-ui-tests.log 的旧 SVG/原生 opener 缺失警告来自原 JSDOM 夹具；真实浏览器 console 检查为空，未把这些测试夹具当作原生 UI 验收。

## 重复入口已清理

Spotlight 初始发现正式版和三份 Preview，清理后精确查询仅返回 `/Applications/Reasonix.app`。Applications 的旧 Preview 和 portable 旧包已注销并移到 `desktop/tauri/target/preview-backups.noindex` 下的 `.app.rollback`，逐份主程序摘要核对不变；本次新包保存在 macos.noindex，并注销兼容路径/新路径注册。正式版1.38.3原主程序 SHA256 前后相同，未替换/发布任何正式版，未操作用户 profiles。详细原路径、备份路径和搜索结果见 app-entry-cleanup.json。没有重置整个 Spotlight/LaunchServices、删除其他应用或重启 Dock。

首次注销新包 canonical noindex 路径返回 -10814（没有注册/Spotlight找不到该包）而中断了后续核对；旧包已经安全移动。第二次核对记录这些已存在备份，仅将该未找到状态视为已无入口，其他错误仍拒绝；最终搜索只有正式版。公开清理脚本的 beforeSpotlight 是第二次运行时的即时状态，initialDiscovery 是首次只读核对的四条真实路径，保留这一区别。原生 Apps/Launchpad 搜索面板没有主动操作，旧面板需要重新搜索才能显示更新。
