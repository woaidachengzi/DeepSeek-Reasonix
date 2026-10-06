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
| `cargo clippy --all-targets -- -D warnings` | 通过；移除遗留的未使用同步管理方法 | clippy.log |
| 真实组件 headless Chrome（模拟 bridge） | 宽/窄屏、深/浅色、失败重试、配对确认、健康轮询、重启、转发和中文按需加载通过；console/page errors 为空 | browser/report.json 与截图；脚本 desktop/frontend/bench/tauri-e-management.mjs |

审查验证曾发现旧思考强度功能漏测试导出、HTML pattern 的 Unicode v 字符类错误，均已修复。中文语言包因新增 E 文案超过预算，改为 E 页面按需加载，未扩大预算。Browser plugin 不可用，使用已安装的 Playwright 和无界面 Chrome；没有打开用户可见浏览器或使用联系人测试。

打包前完整保留旧 `.app` 的回退副本与二进制摘要，见 rollback-before-build.json。直接可运行新包从干净源码提交 `7e6645bdc8f9011a4cf289d6885e998503e65651` 构建，80 MB，无 ZIP/DMG；Go 包内 stamp 为该提交且 `vcs.modified=false`。严格 deep codesign 验证通过，仍为 ad-hoc 签名，未公证。完整收据见 [package-receipt.json](package-receipt.json)。

实际包证据：

| 检查 | 结果 | 记录 |
| --- | --- | --- |
| 包内 sidecar E 集成 | 通过：无鉴权拒绝、显式 SSH 指纹/重启后信任、真实 SFTP 浏览与 revision 保存冲突、loopback tunnel 传输/退出释放、bot 默认关闭/凭据保存、连接内配对授权、文件私有权限、重启持久化、旧包读取新配置后新包继续移除账号 | package-smoke.log |
| 原生宿主（managed/explicit 两个新建档案） | 通过：屏幕 2 内侧物理坐标恢复并由 native capture 保存为相同几何，独立凭据身份、只读通知权限查询、Global workspace、sidecar 鉴权与正常退出清理 | native-package-smoke.log、left-window-template.json |
| 正式应用与搜索入口 | 正式二进制 SHA256 未变；仅注销本次 Preview 实际路径/别名，mdfind 只剩 `/Applications/Reasonix.app` | package-receipt.json |

第一次位置验收在屏幕 2 边缘得到 x 从 -3600 移到 -3372，失败证据保留于 native-package-placement-failure.log。改用屏幕内侧 x=-3000，两个档案均严格保持相同几何；未放宽断言，也未修改产品窗口代码。这只证明本机指定位置的 package-smoke 正常退出，不代表完整 D 多显示器/焦点/托盘矩阵。通知只查询权限，未验证发送/点击；bot 没有向真实 IM 联系人发送任何消息。smoke 工具的错误诊断增强发生在构建之后，不影响已记录的产品二进制源码身份。

剩余发布阻碍：E 的 remote Serve/远端对话与工作区、远端路径操作、配置转发自动应用、bot Desktop 控制/订阅/审批接管、扫码安装/完整连接诊断与管理页实际运行矩阵尚未完成。A 的剩余 Wails runtime 入口收敛与当前包完整回归；B 的工作区/diff、大目录/异常/跨会话原生矩阵与历史资源引用完整兼容；C 的 shell/terminal、Browser、worktree Preview 入口及 MCP/插件真实进程完整矩阵，均未在此批关闭。D 当前包原生显示器/窗口焦点、通知交互及官方 Wails 完整档案互斥/回退等剩余门禁不继承旧包通过；正式 Developer ID 签名、公证和正式发布未执行。Windows/Linux 按之前用户确认延期；updater 按本次用户要求排除。
