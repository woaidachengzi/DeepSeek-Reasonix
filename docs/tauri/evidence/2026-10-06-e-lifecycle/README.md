# E 首批：连接生命周期、bot 管理与权限作用域

用户 2026-10-06 授权直接推进 E，明确不需要 updater。此批完成可独立验收的基础能力，不把整个 E 标记为完成。完整范围及未实现功能见 [E 清单](../../E_MIGRATION_CHECKLIST.md)。正式版未替换、默认下载未切换；发布候选尚未达到正式发布门禁。

审查后修复：

- SSH 请求有归属：断开、更改连接配置、替换请求、退出会取消未完成的连接；迟到结果不能复活连接。重新打开设置读取当前连接状态，目录与文件请求丢弃旧结果。Rust 连接/SFTP/转发调用使用后台 worker，不阻塞 AppKit 事件线程。
- local forward 对齐 Wails 会话内转发，只绑定本机 127.0.0.1；新增/读取/移除需要主窗口调用和 sidecar 鉴权，修改有请求去重。断线重连由既有 forward registry 恢复，主动断开/退出清除；不会自动持久化为下一次连接的规则。
- bot 手动多账号创建/移除、运行时重启、配对请求批准/拒绝接通。新账号默认关闭，配对开启且不默认放行所有人；凭据保护保存，不回显。移除仅清理指定连接、关联路由/订阅及无人引用的自有凭据。配置更新使用宿主寿命 context，重建状态可见，旧适配器退出。
- 配对只授权申请所属连接，连接已删除时拒绝，配置损坏/提交失败时保留申请码。修复共享 core 的静默默认配置覆盖与申请丢失，含 Wails/CLI 调用路径；不是跨两个文件的崩溃原子事务。
- bot 健康轮询不重新载入设置，不覆盖未保存路由草稿；写入失败保持输入供重试。权限页全局/项目读取和修改按请求归属发布，有工作区作用域的管理页随工作区/会话变化重新挂载。

源码证据：

| 检查 | 结果 | 记录 |
| --- | --- | --- |
| Go race：bridge、bot、botruntime、remote 及 bootstrap/forward/SFTP | 通过；新增 late dial、CRUD/shared-secret、orphan pairing、corrupt config、adapter replacement、真实 SSH forward 数据/端口释放回归 | go-race.log |
| `pnpm test:tauri` | 通过；E 实际组件、权限读取乱序、草稿/凭据保护加入常规 suite | frontend-tests.log、management-ui.log |
| `pnpm build` | 通过；保留绑定/分层/类型/CSS/主题及原体积预算 | frontend-build.log |
| `REASONIX_TAURI_BRIDGE_TEST_BIN=<本批 bridge> cargo test` | 234 passed，5 ignored；指定当前实际 sidecar，不用无 binary 的静默跳过 | rust-tests.log |
| 真实组件 headless Chrome（模拟 bridge） | 宽/窄屏、深/浅色、失败重试、配对确认、健康轮询、重启、转发和中文按需加载通过；console/page errors 为空 | browser/report.json 与截图；脚本 desktop/frontend/bench/tauri-e-management.mjs |

审查验证曾发现旧思考强度功能漏测试导出、HTML pattern 的 Unicode v 字符类错误，均已修复。中文语言包因新增 E 文案超过预算，改为 E 页面按需加载，未扩大预算。Browser plugin 不可用，使用已安装的 Playwright 和无界面 Chrome；没有打开用户可见浏览器或使用联系人测试。

打包前完整保留旧 `.app` 的回退副本与二进制摘要，见 rollback-before-build.json。新包、严格签名及包内进程验证将单独写入 package-receipt.json；源码/模拟组件不替代其证据。

剩余发布阻碍：E 的 remote Serve/远端对话与工作区、远端路径操作、配置转发自动应用、bot Desktop 控制/订阅/审批接管、扫码安装/完整连接诊断与管理页实际运行矩阵尚未完成。A 的剩余 Wails runtime 入口收敛与当前包完整回归；B 的工作区/diff、大目录/异常/跨会话原生矩阵与历史资源引用完整兼容；C 的 shell/terminal、Browser、worktree Preview 入口及 MCP/插件真实进程完整矩阵，均未在此批关闭。D 当前包原生显示器/窗口焦点、通知交互及官方 Wails 完整档案互斥/回退等剩余门禁不继承旧包通过；正式 Developer ID 签名、公证和正式发布未执行。Windows/Linux 按之前用户确认延期；updater 按本次用户要求排除。
