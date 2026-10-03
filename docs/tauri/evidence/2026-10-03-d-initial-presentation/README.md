# macOS 隐藏启动与首次呈现：review/安装包验收

2026-10-03，源码基线 e5270c1dd 加 boundary/source.patch 和新增模块快照。按 Wails StartHidden/domReady 基线收敛生产启动行为：主窗口初始 hidden/unfocused，可信主页面 Finished 与宿主 Ready 同时满足后呈现；早期显式 Show 合并，页面重复完成不重开窗口。自动启动呈现尊重应用 Hide，显式 Show 可解除 Hide。只接受当前档案私有 origin；debug 另接受配置的 dev origin，不增加 release 权限。

Review 修正已消费但尚在主线程队列中的显示动作与退出竞态：所有 ExitRequested/Exit 取消 gate，原生动作执行前再次检查。关闭到后台仍走既有 CloseRequested/prevent_close/Hide。四项状态回归覆盖就绪顺序、早期请求合并、未就绪退出和已排队动作取消。无依赖/capability 扩张。

## 当前候选与结果

安装目录、完整 SHA 见 install.json。host ecac5751dde271a1c2d4c5cb758e76ad80da5be991d98a348f365e979b1f7248，sidecar db15d5210d7008e8bb38baab27d06fdd5e7462a47d9dceea9fd9a390cf952805，DMG 665e04655f7b960946e257ca35a0ada856bc06c40c55a2896fc0cb40f207fc09。真实 app/DMG 构建、只读复制安装及严格 ad-hoc 签名通过；不替换用户安装，不是干净 HEAD 构建，未公证。

- strict clippy 通过；Rust **231 passed / 0 failed / 5 ignored**，测试 sidecar 使用此前 f6bc 候选实际二进制，当前新 sidecar 的验收由安装包门禁覆盖。前端构建内契约、类型、lint 与预算通过。
- 同包 package/startup/failure-exit/lifetime **4/4 门禁通过**；启动故障边界 12 项与运行期 idle/streaming、SIGTERM/SIGKILL 8 项保留 kernel、清理与同档案重启证据，详见 boundary 日志及 result.json。
- **原窗口矩阵 48/48**：managed/explicit 各 24 阶段一次通过，包括精确几何、最小化/恢复、应用 Hide/关闭到后台、菜单/快捷键、剪贴板、对话框取消、托盘、单实例、关闭退出和重启恢复。未改断言/容差、未重试，每阶段 kernel exit0。window-summary.json、window.log 与逐阶段原始回执/只读原生时序齐全。
- 实际 exercise-ready 两档案页面未完成时 nativeVisible=false；exercise-shown 时 Finished=true 且恢复为 2000×1400/x920/y344。这确认新的隐藏启动流程，不能证明 React 完成渲染或历史最小化根因。

## 保留的验收边界

这是当前候选一次程序化成功，历史偶发最小化/焦点/2px 问题不能仅凭一次通过关闭。未再运行用户已确认的浏览器 canary。当前包全屏、混合缩放/拔屏/无活动屏幕恢复、物理菜单/IME/实际目录选择、通知交互/重新授权、钥匙串人工授权/旧 Wails 服务与当前包实际官方备份回退仍需对应验收，不继承旧候选通过。若可信页面始终不产生 Finished，窗口仍隐藏；页面加载失败的可见恢复未验收，不以超时绕过 readiness。

D 未完整验收，E 未开启，无新增延期。A 协议/事件/错误覆盖、B 旧 Global 图片和大历史兼容、C 真实工具/进程生命周期及正式签名公证等仍阻碍发布，remote/bot/updater/复杂页待 D 稳定后推进。回退可使用既有官方 Wails 安装与档案备份流程，但本包实际回退验收尚未执行。没有 push、正式发布或默认下载切换。

归档只含源码、工具、日志和回执，不含私有 HOME/core、钥匙串数据库、原剪贴板快照或构建二进制。SHA256SUMS 保存文件完整性。
