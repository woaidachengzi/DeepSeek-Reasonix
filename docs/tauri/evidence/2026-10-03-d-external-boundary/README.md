# 外链 Wails runtime 入口收敛

`externalLinks.ts` 的 BrowserOpenURL 直连移入 `wailsDesktopRuntime.ts` 的窄 adapter。
Tauri 仍先使用主窗口 Rust `open_external_link`，拒绝后直接返回失败；不进入 legacy
adapter 或 window.open。Wails 保留原调用对象/URL；浏览器开发仍使用外部新标签。
这是 D 外链边界收敛，不代表所有 Wails runtime 直连已迁完。

既有 `pnpm test:external-links` 通过，涵盖 Markdown/菜单/邮件/中键、原生拒绝不回退、
复制恢复及本地文档入口；`pnpm build`（绑定契约、lint、类型、CSS 和 bundle 门禁）通过。
没有新增镜像实现的测试。

首次两条检查未进入测试，临时工具链已消失，系统 pnpm 在联网签名校验失败后拒绝
自动切换项目版本。保留拒绝日志；随后直接使用本机缓存的 pnpm 10.34.5 和现有 Node
26.10.0 完成验证，不关闭签名检查、不修改配置、不联网安装依赖。

## 当前候选

- host `bfb47fd84156acba8e9b530889d6a215145291d0a0722ab5f2f0518f2a17b4fa`
- sidecar `29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39`
- DMG `c070fd175cc649ec8649920a965775a5a40ae4c11e7ae7eafc60d37ff521e09e`
- 私有安装 `/private/tmp/reasonix-d-external-boundary-installed-_h6wuc93/Reasonix Tauri Preview.app`

完整 app/DMG 构建、只读挂载复制/卸载、strict ad-hoc 签名通过。不是 Developer ID/
notarization，不发布或切换默认下载。包内 Go sidecar 未改变。

同包 gate 终态、包/源码/脚本身份和各阶段原始日志见相邻
`2026-10-03-d-external-boundary-run/result.json`：failure-exit 与 package 通过；links 在
首个托管档案失败，原生结果 ok=false，错误为 actual browser loopback receipt timed out；
显式档案未开始，lifetime 因顺序门禁停止而未运行。没有把原生打开请求当作浏览器送达成功。
随后单独运行同包 lifetime，终态见 `2026-10-03-d-external-boundary-lifetime/result.json`。
独立 lifetime 8/8 通过（两档案 × TERM/KILL × 空闲/实际假 Provider 流式任务），包含
sidecar/上游清理、档案原件保护与同档案重启；runner=0，候选和脚本身份保持一致。
外链失败原因未确认，之前候选的外链通过不能替代本包失败。这是程序化包级验收；不代替未完成
的真实窗口、手动 Dock/托盘、权限交互和稳定性验收，历史包通过不转移为本包结果。

实际激活尝试的 UI 超时、诊断退出字段修复和无法保存 kernel 回执见相邻
`2026-10-03-d-manual-activation`。没有把这些失败改记为通过，也没有延期窗口要求。
D 未结案，E 未推进，A/B/C 与回退、正式签名等发布缺口继续保留。
