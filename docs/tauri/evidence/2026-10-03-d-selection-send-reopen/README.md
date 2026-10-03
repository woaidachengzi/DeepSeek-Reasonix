# 当前安装包：选区引用发送、历史重开及 D 补验

2026-10-03，本地 review/提交检查点；D 仍进行中，E 尚未开启。未授权正式发布或切换默认下载。

## 构建与代码范围

构建基线 `fc9c968285c65c6695522254709005582c69b8f2` 加本目录冻结的 opt-in 原生探针及验收工具改动。生产前端、权限、capabilities、依赖未改；本轮不重复上一提交的前端组件测试。实际构建包含契约、lint、类型与 bundle 门禁；strict clippy 通过，当前真实 sidecar 的 Rust 测试 **231 passed / 0 failed / 5 ignored**。1 项 Python provider 边界回归含 7 种拒绝子例通过。

安装包：`/private/tmp/reasonix-d-selection-send-installed-17bg3tz9/Reasonix Tauri Preview.app`。

| 对象 | SHA-256 |
| --- | --- |
| host | `b6f7003fb08ab9a6cf8229eafefebb0764ffac3b12fadcfe30367209b96df613` |
| sidecar | `28a5c67da9208fb809fe601f35444436bd2fd835ed342b46b5f3b3bd51c3cf18` |
| DMG | `64ba30c24ffcb33c2a4531050a05b9257bde9296d047874d3269bdb444529ed1` |

实际 DMG 安装与两可执行文件 strict ad-hoc 签名通过。末次核对摘要保持，真实 plist ID `io.reasonix.desktop.preview` 与 `com.wails.reasonix-desktop` 无运行实例，失败物理夹具自身 sidecar 无残留。ad-hoc 签名不等于 Developer ID 签名或公证。

## 当前包实际通过

`native/result.json` 6/6：selection-send、package、fullscreen、dialog-cancel、failure-exit、lifetime；`boundaries/result.json` 前 5 项通过：app-data-boundary、boundary、explicit-boundary、parent-resolution、identity；`services/result.json` 3/3：profile、notifications、displays。合计 **14 个不同程序门禁通过**，不称 D 整组通过。

- 两种档案各实际渲染首轮响应→Copy 到系统剪贴板→Add to Chat→synthetic Cmd+L 去重→点击发送第二轮引用→真实 Go bridge/本机 fake provider 接收完整共享 quoted-context 格式→唯一回复→普通退出→新进程点击持久化会话重开。每档案恰好 2 次 provider 请求，重开没有额外调用；历史引用可见、内部 JSON/指令说明隐藏、草稿消费且不重放。八个原始回执见 `selection-receipts.json` 和 `native/selection-send.log`。完整剪贴板原件已恢复；没有归档原件。程序按钮及 synthetic 快捷键不代表物理键盘/鼠标验收。
- 两档案各 5 阶段全屏与普通几何恢复、AppKit 状态/通知、普通窗口存档字节及重启保持通过。lifetime 覆盖 idle/streaming 的 SIGTERM/SIGKILL 共 8 项及 sidecar 清理/重启。dialog-cancel 含程序菜单/剪贴板及 WebView/命令/plugin 权限边界，不代表实际文件确认/保存交互。
- profile 独立导入、编辑、重启、显式拒绝及原来源/备份保持通过。notifications 在已有 granted/provisional 授权下实际 OS 投递 6 条并清理，不代表 banner/click/拒绝后重授或冷启动 UI。displays 两个真实 scale=2 显示器，两档案共 8 阶段；不代表混合缩放/拔屏/零显示器恢复。

## 两条实际回退路径

`backup/` 与 `backup.log`：当前包导入的配置备份实际应用到另一独立 0700 私有 legacy HOME 的 0600 config；自己的 host/sidecar 已停止，官方 1.38.3 内嵌 CLI 回读默认模型 native-import/alpha、currency CNY，正常退出。恢复配置/来源/备份的字节、摘要、模式与 mtime 保持，恢复配置 SHA-256 `30c2e6c6771f68e26613c91ca621e07471dc369998ec7419de1013ea0e61b36b`。

`legacy.log`：官方 Wails GUI/内嵌 CLI 在私有 fake provider 档案创建历史、Global 文本/图片附件及编辑检查点；Preview 导入/编辑/重启/显式拒绝不改变原树；官方 GUI 前后回读历史、活动旧写入者拒绝、正常退出通过。官方继续会话和第二检查点后，最新检查点实际恢复前像，更早冲突回滚拒绝；历史/附件保留且回滚无 provider 调用。不是全部历史/资料或 Preview 图片渲染验收。

官方 GUI SHA-256 `869b02d8f8a92f5847c1728923fde7153d931e105f26fd2926bfa8feeb863650`；内嵌 CLI `5b1ab31424d45c8bf3cfe6a60b11da527df2252aee963d1c0c56352f57945962`。

## 未通过与未执行必须保留

`boundaries` 原队列在 inactive-display-state 前置条件失败，原日志明确 `zero active CoreGraphics displays required; conditional gate is not applicable`；未启动该测试夹具。Mac 锁定不代表零活跃显示器。原队列 profile 为 not-run，后续 services 的独立 profile 通过不改写原队列状态。

物理私有夹具被 CUA 成功定位；请求点击最小化后 AX 无变化，原生观察 mini=false/Will=0/Did=0。后续 Raise/截图明确返回 Mac locked。输入送达无法确认，不判产品最小化失败，也不判物理验收通过。180 秒监控超时退出 1，自己的终止/清理完成，不是物理 Quit 成功。`physical-ui-actions.json` 是忠实的工具动作转录；无屏幕截图。仍待用户解锁后新的私有夹具验收，失败夹具未重用。

本候选 **未跑完整原窗口 48 阶段**，不继承 b4 的 48/48；未继承旧包 .env/钥匙串授权证据。物理菜单/快捷键及配置映射、IME、文件与目录确认/保存、通知权限交互、钥匙串授权取消/旧服务、混合缩放/拔屏/零显示器恢复、未 Finished 页面的可见异常恢复、任意旧共享目录互斥及历史间歇窗口问题仍待完成。已获用户确认的浏览器 canary 不重复；mailto/OAuth 等其他入口仍需对应验收。

A 的协议/事件/错误面、B 的旧 Global/本地 Markdown 图片及大历史、C 的 terminal/browser/worktree/MCP/plugin 实际生命周期仍阻碍发布。E remote host/bot/updater/复杂管理页尚未完整验收，按 D 验收且 Preview 稳定后推进。未将未验项目自行延期。

## 证据校验

仅冻结源码、文本日志及 JSON，不包含私有 HOME/core、原剪贴板、钥匙串数据库或二进制。`summary.json` 保留分队列状态，`install.json`/`postcheck.json` 保留包身份与清理，`source/` 保留当前改动，各门禁 source.patch/scripts 为执行时快照。在本目录执行 `shasum -a 256 -c SHA256SUMS` 检查归档；冻结 patch 的空白不参与工作树 diff-check。
