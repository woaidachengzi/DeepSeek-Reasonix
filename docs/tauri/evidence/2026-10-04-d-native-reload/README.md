# D：原生 Reload 与当前候选验收（2026-10-04）

生产菜单 Reload 从 renderer `eval("location.reload()")` 改为 Tauri `window.reload()`，使用原生 WKWebView 加载链路；失败打印固定恢复说明。遗留 `force_reload` handler 与其共用实现，当前菜单未安装该项，不宣称强制绕过缓存。未增加快捷键或扩大权限。其余修改限于 opt-in 探针及验收工具：新增正常/隐藏 Reload 门禁、固定 WK 错误域/编号诊断、重开时 composer 尚未挂载的空值保护；没有放宽验收条件。

## 候选与来源

基于 `d0516cefdab3a754690df7cc61eac5c923f96022` 的未提交源码构建；最终源码在 `source/` 和 `source.patch`，各队列独立冻结源码/工具。两个候选分别列证，不能互相继承验收。

| 候选 | host SHA-256 | DMG SHA-256 |
| --- | --- | --- |
| 首轮 dda | dda2b3c7fa2a016628df7fa0b0bf90f3f9050be4acf6f240574b568de13d0237 | 782fa75b00e23c1d5a48c9ee48a91b297b8cddb8630a64ec28ec2fdc40df94bf |
| 最终 f4 | f4c2a60db15259131e2b7b4fd334d1a6746f443f6b21ae092707fa46adba1e98 | 3d8ca0d157c08fd43aa9d1ba51e7fd7faafa1c546bbcbe46510dba5b1db90fad |

共同 sidecar：`7352e79707cdcb83d7f55348afe251e5065c335c27a3b9c1e6506d6d3f403649`。首轮后仅补探针错误分类与 composer 空值保护，最终重建/DMG 实际挂载安装；安装回执在根目录。生产前端、依赖与权限未改。两轮 Rust 各 231 passed / 0 failed / 5 ignored，strict clippy 和 app/DMG 构建通过；日志原样保存，忽略项不计通过。

## 最终 f4 实际结果

- `final-process/result.json`：独立 reload、package、failure-exit、lifetime 四组通过。Reload 在 managed/explicit 两种档案各执行正常及隐藏两轮，实际 NSMenu 调用生产 handler；验证新 JS realm、页面渲染、私有 localStorage canary 保留、可信主页面 origin、原生窗口身份/几何/可见性及同一 sidecar instance。成功夹具清理前打印两份结果及八条原生 trace，见 `reload.log`。
- 此门禁起点为已加载健康页面。未证明 never-Finished、页面 JS 故障恢复或物理 Reload 点击；也不证明所有窗口状态稳定。
- `final-backup/`：真实备份应用到独立 0700 HOME，0600 配置；固定摘要的官方 1.38.3 内嵌 CLI 回读成功/exit0，备份与恢复摘要一致，Preview 停止后原树、备份及恢复树不变。只验配置回退，本 f4 未跑官方 GUI 全历史/附件/检查点回退。
- `final-native/result.json` 原队列保持 selection-send failed，后续 reload/package/failure-exit/lifetime not-run；不因独立四组通过而改写。失败在“旧键未 preventDefault 且选区仍未折叠”的联合断言，kernel2。没有分别记录两个子条件，不能认定旧键仍接管或选区折叠为原因。尚未进入重开阶段，composer 空值保护未获安装包覆盖。失败夹具 `/private/tmp/reasonix-native-message-copy-jqnsltnj` 的安全观察/退出文件在 `selection-final/`。
- 本 f4 完整窗口矩阵及 fullscreen 未运行，不继承首轮或历史候选结果；其他 D 门禁亦未在本包全部重验。

## 首轮 dda 的通过及失败

`native/` 四组同样通过；`window/` 完整窗口矩阵首个 managed exercise failed/kernel2，外层 0 阶段接受，后续 explicit/selection/fullscreen 未运行。`window-first/` 原始 trace 显示已 Finish、两个 scale2 显示器；Show 后曾 key/focused，随后失焦；Will/DidMin 均 0、nativeMini=false。不归因锁屏、setter 队列或已修复的历史根因。

`selection/` managed 实际设置录键→旧默认不接管/去重→第二次发送→provider 引用边界响应及首次 host exit0 已过；随后普通重开 kernel2，通用 WK evaluation error，explicit 未运行。`selection-first/` 在修补前冻结原探针及安全结果。空值保护修复确定的 nullable-read 风险，但未证实首轮异常根因。

物理 CUA 本轮可读取 UI 和截图：确切私有 app 绑定、实际菜单输入按序执行。第一夹具 180s timeout/exit1，自身清理；button、Cmd+M、Window→Minimize 未得到 native mini 事件，Reload 点击后 AX 页面可读取，但没有独立 realm 回执。第二新夹具实际 Window→Show 后 nativeKey/appActive=true；随后最小化仍未通过，实际 Reasonix→Quit 后 kernel exit0、opener exit0、sidecar/ready 无残留（`physical-show-quit/`）。退出后的 CUA connectionInvalid 是失效绑定，不重绑/启动用户档案。上述 Show/Quit 仅属 dda 显式 idle 夹具，不继承到 f4。动作转录在 `reasonix-native-reload-physical-actions.json`；截图只在工具中展示，未保存图片文件，不宣称已有像素归档。

最初普通 sandbox CG count0 与原生两个显示器不一致；同权限上下文复查 count2（`reasonix-native-reload-displays-confirmed.json`）。不能用 sandbox 的 count0 代表门禁时显示器状态，也不能据此认定仍锁屏；此前 43 候选原生 monitors=[] 的失败证据仍保留。

## Review、清理与剩余验收

固定 schema 错误诊断不输出 localizedDescription/页面内容。复核生产 Reload handler、probe 的可信 origin/独立 realm/同 sidecar 断言与失败保留机制；Python AST、冻结 helper SHA（含 `.c`）及源码 patch 一致性检查通过。未改前端，不重复浏览器 canary：用户已确认打开成功，该事实不代替其余链接场景。`reasonix-native-reload-postcheck.json` 核对两安装包 strict signature/精确摘要；无运行的 Preview，相应五个失败/物理夹具 sidecar 和 ready 均无残留。

D 未完成，E 尚不满足推进条件。仍待原生快捷键/IME/选区重开稳定性、物理最小化/全屏与多屏恢复、真实文件/目录确认保存、通知横幅/点击与拒绝重授权/冷启动、钥匙串人工授权取消及旧 Wails service 边界、mailto/OAuth 来源点击、旧自定义共享根互斥，以及页面 never-Finished 可见恢复。不得将本次 Reload 门禁视为这些项目通过。

A 的协议/事件/错误，B 的旧 Global/本地图像/大历史，C 的 terminal/browser/worktree/MCP/plugin 真实进程生命周期仍有缺口；E remote host/bot/updater/复杂管理页尚未逐项完整验收。仅 Windows/Linux 是用户已确认延期的范围。当前 ad-hoc Preview 可构建并有配置回退路径，但未具备正式 Developer ID 签名/公证、完整数据回退及所有发布门禁。正式发布、push 和默认下载切换未获本轮授权。

只归档代码、固定夹具元数据/结果及日志；不复制私有 HOME/core、原始剪贴板、keychain 或安装包二进制。`SHA256SUMS` 覆盖本目录其余全部文件。目标继续 active。
