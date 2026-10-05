# Tauri 对话归档与恢复

本轮接入单条对话归档、已归档列表和恢复并打开。侧栏鼠标移入/键盘聚焦对话行后显示归档按钮，内联确认后从普通会话/项目/快捷切换列表隐藏；“已归档对话”保留标题、原工作区与归档日期，恢复打开原 ID，可继续聊天。归档与永久删除有独立按钮、确认文案及接口，不调用删除 sweep。

## 基线与数据兼容

只读核对 Wails `desktop/app.go` 的 `ListTrashedSessions`/`RestoreSession` 与 `projectTreeArchive.ts`：原版归档进入可恢复回收站，移动完整会话工件，并在恢复时检查已有 writer/目标冲突。Tauri 本轮采用独立可见性状态文件 `desktop/session-archives-v1.json`，保留原 transcript、metadata、检查点、附件与会话身份数据库，不迁移 Wails 回收站、不改变身份 schema 9。回退旧包仍可读原历史，但旧包不识别新归档隐藏状态；该差异明确保留，不能称 Wails 回收站、批量归档、归档内预览和永久清空已迁移。

空对话可归档，reserved 身份与标题 metadata 保留，不生成假 transcript。已知 missing/deleting/deleted、待恢复改名、外部 writer、运行中/等待审批或有后台任务时拒绝；当前对话有未发送草稿/附件/引用时也拒绝。普通目录协议仍返回完整身份集用于目录核验与分页，UI 按权威归档集合筛选，Go 工厂也拒绝未恢复的 ID。

## 提交前 review 与回归

- 修复归档动作复用删除/设置 CSS 标识造成的既有入口测试歧义，独立动作标识并共享样式。运行全套 `pnpm test:tauri` 通过；测试夹具补齐既有 API Key adapter 导出。JSDOM SVG 空资源与缺失原生 opener 的已有夹具警告不计作原生界面验收。
- 归档事务在 manager mutex 下阻止 submit/open/switch/shutdown 插入。保存前 flush；提交失败保留 controller，成功后正常 Close 结束会话并释放 lease。inactive 归档不切换当前 controller。拒绝 archived/corrupt 的 Switch 在旧 controller 关闭前做 admission。专项测试含确定性并发提交、写入失败、准备失败、paused、旧 owner 保留。
- 元数据有限大小/条数、ID 验证、普通文件/目录检查；只读不创建目录，损坏/链接拒绝并保留原文件。临时文件 0600、sync/close 后原子 rename；重试保留原归档日期。HTTP 只用 profile 派生路径，Bearer 与 request-ID 门禁沿用现有机制；主窗口之外不能调用新增原生命令，不增加 shell/filesystem 权限。
- `go test -race ./internal/sessionarchive ./internal/desktopbridge ./cmd/reasonix-desktop-bridge` 三包通过；Rust 带当前真实 bridge 的全量测试 **232 passed / 5 ignored**，空对话归档、metadata 保留、退出/重启/恢复原 path 均通过。ignored 的系统交互项未执行。
- `pnpm build` 全部生产契约、hooks、CSS/主题、类型和 bundle 门禁通过。Transcript 滚动算法、writer 与 kernel 没有变更；归档当前对话沿用会话替换 generation 取消过期请求。

## 浏览器与真实进程证据

Browser plugin not available，使用项目已有 Playwright 与已安装的 headless Google Chrome，没有操作用户浏览器或弹出原生窗口。临时 `http://127.0.0.1:5199/archive-qa` 渲染实际 `TauriSessionApp`/`SessionRow`/CSS，仅 bridge/history、宿主 invoke/event 与持久化响应模拟。1280×900、900×760、390×900 页面身份、非空、无框架覆盖层、console error/warning 空、面板位于视口内/无横向溢出均通过；确认归档→普通列表隐藏→reload→已归档列表→恢复原 ID 与历史的交互通过，且未调用永久删除。截图及几何在 `browser-results.json`，脚本 `browser-qa.mjs`；截图已逐张查看宽屏和窄屏。

`source-smoke.json` 使用新构建的真实 sidecar、临时 profile 与本机 SSE 模拟模型验证历史保留、运行中409、重试日期不变、工件路径/字节保留、归档直开409、0600/schema9、重启恢复、继续发送、inactive 归档保留当前 owner、空对话标题保留、损坏状态失败关闭且不覆盖、修复重试与配置原字节不变。两次退出0、ready文件移除，4次模型请求全部在本机。附件/检查点为隔离 marker，只证明归档不删除/搬动它们，不冒充图片加载或代码回滚验收。

直接 `.app` 的构建、签名、包内同范围 smoke 和前一版回退回读证据追加在本目录回执；不生成 ZIP/DMG，不替换 Applications，不发布或切换默认下载项。原生 WebKit 鼠标/键盘交互和真实模型网络请求尚未执行，D 稳定验收/E 全面迁移、A/B/C 与正式签名/公证缺口不因本功能关闭。
