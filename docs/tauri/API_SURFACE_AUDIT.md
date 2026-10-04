# Wails API 与事件面盘点

## 结论

当前前端不是直接遍历调用 Wails binding：
`desktop/frontend/src/lib/bridge.ts` 已是主 React-to-Go adapter。

- `AppBindings` 本体声明 **466** 个方法；其组合的子 binding 接口另有 **79** 个
  方法，即至少 **545** 个 host API 入口。
- `bridge.ts` 通过 `app` Proxy 执行调用，并集中实现 `agent:event`、terminal、
  updater、runtime、tab、session 和 remote 等主要订阅。
- `bridge.ts` 是 Tauri adapter 的正确替换点；不应把 545 个方法逐一变成 Tauri
  command，也不应让 React 组件直接调用 Tauri `invoke`。

## 现有分层

```text
React components/hooks
       │
       ▼
lib/bridge.ts: AppBindings + app Proxy + event subscription helpers
       │                         │
       │                         └── browser mock
       ▼
window.go.main.App + window.runtime (Wails generated/runtime API)
       ▼
desktop.App (Go) + Go core
```

迁移后的形态应为：

```text
React components/hooks
       │
       ▼
desktop API contract (保留 app/event helper 的调用形状)
       ├── Wails adapter（稳定版与回归对照）
       └── Tauri adapter（invoke + bridge stream）
                 │
                 ├── Rust host：窗口、菜单、系统能力、sidecar 生命周期
                 └── Go bridge：会话、Agent、Provider、MCP、工具
```

## 按迁移顺序分组

| 组别 | 首轮处理 | 示例 |
| --- | --- | --- |
| A：PoC 必需 | 建立精简 bridge | health、session snapshot、open/create、submit、cancel、event subscription、shutdown。 |
| B：核心稳定性 | PoC 通过后 | settings/provider、会话历史、workspace、文件与 diff。当前 Tauri 已有 provider/历史/附件、逐层 workspace 文件引用、受限文件预览，以及 Git 与本轮 session checkpoint 变更/diff；已接入单文件恢复、撤销、检查点多文件代码回滚、同日志会话头的对话回滚与版本导航、文件和对话组合回滚，以及旧格式独立会话分叉。 |
| C：进程与工具 | 需单独生命周期测试 | shell/terminal、MCP、Browser、plugins、worktree。 |
| D：host 平台能力 | 由 Rust 实现，不进 Go core | 窗口、文件对话框、外部链接、菜单、托盘、通知、钥匙串。 |
| E：后置 | Preview 稳定后 | remote host、bot、updater、复杂管理页。 |

### A→B→C 当前执行状态（2026-09-30）

| 组别 | 当前已验证 | 下一个缺口 |
| --- | --- | --- |
| A：PoC 必需 | 精简 bridge、会话/事件链路已存在；构建从 Go `App` 源码核对 587 个方法与 TypeScript `AppBindings`，不再依赖本地生成的 Wails 类型；Rust 测试已用本次构建的真实 bridge 验证启动、退出、重启和会话读取。 | 当前提交的真实安装包回归；其余 Wails runtime 直连按功能面迁移。 |
| B：核心稳定性 | Provider、历史、工作区预览/变更和回滚切片已接入；检查点与 Git 变更面板现于回合空闲确认后自动刷新。 | 继续补齐工作区文件与 diff 的交互覆盖，并在真实 Preview 窗口验证跨会话、异常和大目录状态。2026-10-01 源码核对发现 Markdown 图片 resolver 尚未接入 Tauri，旧 Global 文件引用/附件图片仍需迁移与验收。 |
| C：进程与工具 | MCP 服务器/运行时及插件设置已有 bridge 和 Preview 界面；Go MCP/插件测试、前端 MCP/工作区测试以及使用真实 bridge 的 Rust 测试通过。 | shell/terminal、Browser、worktree 的 Preview 入口与生命周期测试尚未接通；MCP/插件仍需真实打包进程验证。 |

本轮 A、B、C 验证不代表整组完成；下列 D→E 改造继续保留这些发布缺口。

本次验证：`pnpm build`（含绑定契约、lint、类型检查和 bundle 门禁）、`pnpm test:tauri`、`pnpm test:mcp-app`、`pnpm test:workspace`、`go test ./cmd/reasonix-desktop-bridge -run 'Test.*(MCP|Plugin)' -count=1`，以及设置 `REASONIX_TAURI_BRIDGE_TEST_BIN` 后的 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml`（124 项通过）。这些门禁验证源码和测试进程，尚未代替真实安装包验收。

### D→E 改造与验收目标（2026-09-30，进行中）

按 D→E 顺序推进。D 验收且 Preview 稳定后再推进 E；正式发布和默认下载项切换另行授权。

#### 当前候选验收索引（2026-10-04）

**当前46包实际保存与取消恢复：** [左屏诊断保存证据](evidence/2026-10-04-d-current-save-recovery/README.md)。一份explicit档案中文/空格命名保存、原生覆盖Cancel保持hash/mtime/mode、备份后Replace有效JSON0600、保存面板Cancel不写且保留5事件、重试导出和普通Quit清理通过。Wails此入口ZIP/工作区默认目录与当前JSON/目录策略存在差异，未宣布资料格式等价；其他档案/过滤/写入故障/保存入口仍待。D稳定/E待验缺口保持。

**当前46包左屏首次全屏键往返：** [新建私有档案与真实原生完成证据](evidence/2026-10-04-d-screen2-template-fullscreen/README.md)。启动器新增受限geometry模板且先确认native恢复，边界回归2项通过；当前一份explicit左屏2档案Ctrl+Cmd+F首次进入/退出完成各1、精确恢复与正常Quit清理通过。最小化按钮/键仍失败；此前其他现场首次全屏失败保持，不称间歇根因或D稳定已解决。

**当前46包目录、多文件与默认工作区重启：** [一份explicit私有档案的实际CUA验收](evidence/2026-10-04-d-current-directory-multifile/README.md)。真实目录Open/Cancel、两份附件同时返回（中文/空格路径）、附件Cancel保留/逐项移除、普通重启同工作区通过；两次正常Quit及自有sidecar/ready清理、安装SHA/strict signature保持。源码未改，不继承其他档案/入口/保存过滤错误场景；Escape/Cmd+A自动化未奏效记录保留。D稳定性缺口与E待验保持。

**最新 46c2c9da 选区普通重开通过：** [失败stage、就绪前提修正与当前包七gate](evidence/2026-10-04-d-selection-reopen-ready/README.md)。首轮feb在首个storage配置检查code4/kernel2，检查前可信URL但mainFinished=false/hidden；保留失败。修正opt-in探针在可信Finished/渲染后读取storage，原配置/历史断言不变；当前两档案实际录键、旧键、引用二次发送/恰两provider/普通重开持久化/协议隐藏/无draft回放通过。Rust231/5ignored、strict clippy、真实app/DMG安装签名、七程序gate（含48/48、双档案fullscreen）和本包配置备份官方CLI回读通过。实际Cmd+M两个事件分发时key和查询target均main但无Mini完成，不证明role调用；物理全屏菜单进/键退/精确恢复和Quit通过，首次键进/物理最小化仍未通过。D未完成，E逐项验收仍待D稳定；其他D/官方GUI全资料回退不继承旧包，A/B/C与正式签名公证缺口保留。

**最新 041949c6 原生输入/选区原子观察：** [源码与真实当前包证据](evidence/2026-10-04-d-native-input-observation/README.md)。仅opt-in诊断改动：本主窗口固定M/F+modifier及点击计数、事件原样放行/退出移除；旧键同次dispatch的两个条件独立保存，原断言保持。当前真实包Rust231/5ignored、strict clippy、构建/DMG安装签名、六程序gate（含window48/48及双档案fullscreen）和配置备份官方CLI回读通过。实际Cmd+M有2条匹配local事件而Will/DidMin=0，不证明role调用或根因；实际全屏菜单进/键退/精确恢复与Quit通过，首次键进仍未通过。选区managed旧键两条件false、发送/provider通过，重开WKErrorDomain code4/kernel2，explicit未运行，整组failed。还需check stage/初始化前提定位；其他D/官方GUI全资料回退不继承旧包。D未完成/E未开启，A/B/C/正式签名公证缺口保持。

**当前 f4c2a60d 窗口补验：** [原生路径/完整48阶段/全屏及物理差异](evidence/2026-10-04-d-f4-window-current/README.md)。生产和工具未改，当前包完整窗口managed24/explicit24共48/48通过、双档案原生全屏进入退出/精确存档/重启通过；API/AppKit/menu六例最小化恢复通过，两个occlusion前提失败且未请求最小化，保留exit1。实际CUA全屏菜单进入、Ctrl+Cmd+F退出/原几何与正常Quit清理通过，但快捷键首次进入、物理menu/button/Cmd+M最小化未通过；输入/窗口状态根因未证实，不称间歇问题已修复。前后strict签名/SHA/自有进程清理保持，当前配置回退沿用同一f4上一轮实测；选区失败、其他D与官方GUI全资料回退仍待。D未完成/E未开启，A/B/C/正式签名公证缺口保留。

**最新 f4c2a60d 原生 Reload：** [实现、真实安装包及失败证据](evidence/2026-10-04-d-native-reload/README.md)。Reload 从页面 eval 改为原生 WK 加载；两档案正常/隐藏四轮及 package/failure-exit/lifetime、当前包配置备份应用/官方 CLI 回读通过。Rust231/5ignored、strict clippy、实际 app/DMG 构建安装签名及清理通过。首轮 dda 实际 Show/Quit 通过，但完整窗口最小化失败；最终 f4 选区门禁联合断言失败，不能据此认定旧键接管，重开空值保护未获安装包验证。健康页面 Reload 不证明 never-Finished/坏JS恢复；最终完整窗口和官方GUI全数据回退未运行，其他D不继承旧包。当前CUA可读UI；D未完成/E未开启，A/B/C及正式签名公证缺口保留。

**最新43dc4305选区终态诊断：** [真实帧/焦点/可见性证据](evidence/2026-10-03-d-selection-frame-observation/README.md)。生产未改、仅opt-in固定结构观察；当前失败时页面hidden/无焦点、monitors=[]、独立rAF未执行、选区collapsed且按钮closed/disabled，设置已关闭。原选区断言保持failed/kernel2，不归因为配置失效或证明历史根因，不在不可见环境重复尝试。新包Rust231/5ignored、修正unused import后strict clippy、实际构建/DMG安装签名通过；独立package/failure-exit/lifetime三组及实际配置备份应用/官方CLI回读通过，摘要/进程清理保持。其他D/官方GUI回退不继承旧包，D未完成/E未开启，A/B/C及正式签名公证缺口保留。

**最新 b1f7f210 选区配置生产接入/失败保留：** [源码与实际新包证据](evidence/2026-10-03-d-selection-shortcuts/README.md)。新增第44项add_selection，Tauri独立配置同时驱动共享菜单标签/触发，原生冲突/IME/遮盖保护；Wails默认路径保持。完整Tauri/transcript、实际设置流组件回归、Rust231/5ignored、strict clippy及构建/DMG安装签名通过。新包实际设置录键前置完成后Add action超时，kernel2；未执行第二轮/配置重启，不称已验收。失败后CG观察count0，但缺当时帧时序，不归因锁屏。独立package/failure-exit/lifetime三组通过、签名与清理保持；其他D/当前包回退不继承b6，D未完成/E未开启，A/B/C与正式签名公证缺口保留。

**最新 b6f7003f 完整窗口失败/零显示器补验：** [原始失败及条件证据](evidence/2026-10-03-d-zero-display-window/README.md)。完整矩阵在 managed exercise 后因缺失窗口存档失败（外层0阶段接受，explicit未启动），原生最小化Will/Did各1、kernel0，时序monitors=[]；不改断言、不记48/48。随后真实CG零活跃显示器条件下两档案旧几何保护/清理通过，独立tray-language共6阶段、startup共12项通过；同候选累计17个程序门禁，但显示器重新活跃恢复及物理UI仍待验。签名/摘要与自身进程清理保持；D未完成/E未开启，历史窗口与A/B/C/签名公证缺口保留。

**最新 b6f7003f 选区实际发送/历史重开与 D 补验：** [当前包证据与 review 检查点](evidence/2026-10-03-d-selection-send-reopen/README.md)。两档案实际引用第二轮由本机 fake provider 严格验证共享 quoted-context，恰好两请求；普通退出后重开持久化历史，内部协议隐藏、草稿消费且不重放。当前包 14 个不同程序门禁、实际配置备份应用/官方 CLI 回读及官方 Wails 历史/Global 附件/检查点回退通过；Rust231/5ignored、strict clippy、构建/DMG安装与签名/清理通过。零显示器门禁前置条件失败，原队列 profile 未运行，后续独立 profile 通过；物理输入未确认、CUA 报 Mac 锁定，监控超时与清理已保留。完整48阶段、钥匙串/权限交互等不继承旧包。D 未完成/E 未开启，A/B/C 和正式签名公证缺口保留，未授权发布或默认下载切换。下列为历史候选证据，各自通过范围不自动继承。

**当前 b4cbb6be 选区引用生产接入：** [实际入口与同包回归](evidence/2026-10-03-d-selection-reference/README.md)对照Wails引用卡片而非追加指令，接入共享normalize/quoted-context格式、去重/移除、会话隔离/切回和失败保留；历史UI隐藏内部JSON。React真实入口stub-submit回归、完整Tauri及transcript、Rust231/5ignored、strict clippy和实际构建/安装签名通过。同包message-copy/package/fullscreen/failure-exit/lifetime五组及原窗口 **48/48** 一次通过；两档案实际按钮/synthetic Cmd+L与OS clipboard generation保持、原件恢复通过。实际provider只接收首个nonce问题，第二轮引用provider接收/历史重开和物理/配置快捷键仍待验；其他native权限/通知/钥匙串/显示器及当前包实际官方回退不继承旧包。D未完成/E未开启，历史窗口稳定性及A/B/C/正式签名公证仍待完成。

**当前 ecac5751 全屏/回退补验：** [同包新证据](evidence/2026-10-03-d-initial-presentation-fullscreen-rollback/README.md)两档案各5阶段一次通过，真实AppKit全屏进入/退出did各1、2000×1400/x920/y344精确往返，普通存档原字节/重启/身份保持。当前包实际配置备份应用与官方CLI回读、官方Wails历史/Global附件/检查点回退实际通过，原树与两包摘要/签名保持；错误Preview ID的初次退出检查已保留并从真实plist纠正复验。新增可重跑备份门禁和失败夹具保留，1项Python回归/真实三阶段通过。当前已覆盖回退范围有验证路径，但不代替完整资料/物理UI、任意旧目录互斥或Preview图片兼容；其他D/历史间歇稳定性及A/B/C/正式签名公证仍待完成，D未完成/E未开启。

**隐藏启动生产改造 / review 检查点：** [ecac5751 首次呈现](evidence/2026-10-03-d-initial-presentation/README.md)按 Wails 基线初始隐藏，可信主页面 Finished 与宿主 Ready 联合呈现；早期 Show 合并、重复完成不重开，退出取消排队动作并在执行前再检查。四项状态回归加入，Rust231/5ignored、strict clippy、构建/DMG安装签名通过。同包 package/startup/failure-exit/lifetime 四组与原窗口矩阵 **48/48（两档案各24）** 一次通过，原断言/容差未变、kernel0，原始回执/时序完整归档。单次通过不关闭历史偶发窗口问题；页面未 Finished 的可见恢复、物理 UI/其他 D 与当前包实际官方回退仍待验，全屏不继承旧包。D 未完成/E 未开启，无发布或默认下载授权。

**原exercise两因素对照：** [f6bc6f89页面/Show组合](evidence/2026-10-03-d-minimize-exercise-factors/README.md)新增opt-in分支，原路径/断言不变；八例6过2失败，managed单Show未等页面与explicit原双Show路径均最小化失败/kernel2、Will/Did=0，原始回执完整保留。等页面四例通过，不等也两例通过；不能证明页面根因，重复Show不是必要失败条件。Wails隐藏启动/domReady呈现与当前页面未完成已可操作窗口的差异已核对，下一项收敛可信页面/初始化完成的一次性呈现和启动期请求归属，不仅改测试。当前包Rust227/5ignored、strict clippy、构建/安装签名和package/failure-exit/lifetime三组通过；历史最小化、物理UI/其他D及当前包官方回退仍未关闭，E未开启。

**当前最小化路径对照：** [22ee同包8例](evidence/2026-10-03-d-minimize-path-control/README.md)两档案各一次API/直接AppKit/原生菜单/呈现前提，8/8、nativeMiniaturized=true、Will/Did=1、恢复和kernel0，原始时序/回执与固定包签名保持。成功前提均页面Finished与一次Show；原失败exercise页面未Finished且两次Show，仍混有初始化轨迹，不能归因或判修复；成功样本occlusion可为false。生产未改，新增可重跑路径工具仅help/语法验（真实执行固定控制脚本另存）。下一项按页面就绪与首次Show分别控制原exercise，不延时/重试/改断言；历史最小化、物理UI及其他D仍待验，E未开启。

**当前全屏探针包：** [22ee53cb全屏完成与精确恢复](evidence/2026-10-03-d-fullscreen-completion/README.md)新增opt-in真实AppKit角色、FullScreen位/四通知/普通存档保护门禁，不改生产行为/权限。首轮托管五阶段通过，显式exercise最小化失败/kernel2、will/did=0，显式全屏当时未执行，整组保持failed；同包同保留档案的独立显式restore/fullscreen/restore三项通过，两档案进入3840×2160后精确回2000×1400/x920/y344、did-enter/exit各1、存档原字节/重启保持。同包package/failure-exit/lifetime另三组通过；Rust227/5ignored、strict clippy及app/DMG安装签名通过。首次宏递归编译失败保留；失败fixture复用前漏复制result/trace的证据限制明确记录，原日志/kernel保留，工具新增失败快照及回归。最小化/历史稳定性、物理UI、其他D及当前包回退仍未通过，不继承旧候选，E未开启。

**当前候选窗口补验：** [f259ddc9完整矩阵与前置中断](evidence/2026-10-03-d-message-candidate-window/README.md)原矩阵托管24/24、显式前20项通过（44阶段kernel0），menu-editing前剪贴板完整保存保护拒绝；剩余4项当时未执行，不记完整48/48。当前只读capture确认不可读取格式，但原次具体helper状态未保存，不能事后归因。固定包/保留显式fixture的单实例与close/restore三项独立3/3通过，精确几何/身份和签名保持；menu-editing仍待安全前置。捕获错误新增固定分类及5项Python回归，不改保护或生产窗口代码。CUA再次30秒超时；现有矩阵不含全屏，下一步单独补完整fullscreen状态/结束通知/精确恢复，物理UI与历史稳定性、其他D及当前包回退仍未完成，E未开启。

**最新 review/本地提交检查点：** [f259ddc9 实际消息选区复制](evidence/2026-10-03-d-message-selection-copy/README.md)已接入真实 Tauri 聊天入口的 lazy 共享菜单与正文选择边界；原生拒绝保留选区且无浏览器回退，Tauri Add to Chat 尚未接入。完整 transcript（菜单88项）、Tauri、Rust227项/5ignored与strict clippy、app/DMG构建安装签名通过。首包探针过早读取异步复制结果导致kernel2，原源码/失败/身份保留；修正等待后全新包双档案实际消息→菜单→OS剪贴板及原件恢复通过，当前包message-copy/package/startup/lifetime四组通过（启动12、生命周期8）。不继承e7的其余门禁/官方回退，完整窗口/其他服务/现场UI与授权仍待验；D未完成/E未开启，A/B/C和正式签名公证缺口保留。

**当前候选补验：** [e7f5f9ae边界/原生fixture/官方回退](evidence/2026-10-03-d-restore-queue-continuation/README.md)新增8组程序门禁通过，同包共13组（前批窗口48/48）；双屏8项、通知OS六条送达与精确清理通过。原生全局.env迁移使用当前源码测试二进制+当前安装sidecar实际1/1通过；初次fixture前缀保护拒绝保留，修正前缀后全新fixture通过。当前包官方历史/附件/检查点/code rewind与实际配置备份应用回读通过，两app签名/无运行实例。权限/物理UI、钥匙串授权取消/旧服务、任意旧目录互斥及窗口历史失败仍未关闭，D未完成/E未开启，A/B/C及正式签名公证仍阻碍发布。

**当前恢复修复候选：** [e7f5f9ae恢复队列边界](evidence/2026-10-03-d-restore-queue-boundary/README.md)保留pending至主队列setter后完成，阻止重入并提前注册状态；真实断言捕获过渡2560×1640/x640/y142后目标2000×1400/x920/y344，旧缓存保持。源码227项/clippy及app/DMG安装签名通过；定向8/8、当前包package/startup/failure-exit/window/lifetime五组通过，完整窗口48/48、生命周期8项。历史间歇失焦/2px未关闭，不继承74其他门禁和回退；新包其余D、备份回退/授权/物理UI待继续，D未完成/E未开启。

**2px坐标定位补充：** [固定历史种子坐标对照](evidence/2026-10-03-d-persistence-coordinate-control/README.md)在原74安装包上分别运行managed/explicit的restore-normal与second-instance，4/4、kernel exit0，精确保存y344不变；只读窗口边界与scale2一致。锁定Tao的尺寸/位置设置异步投递，当前restore立即解除pending存在过渡捕获可能，尚未证明因果；未补偿偏移，历史y342/失焦未关闭，D未完成/E未开启。

**完整窗口时序补充：** [74fefe10逐阶段只读时序](evidence/2026-10-03-d-full-window-timeline/README.md)使用原完整矩阵与精确断言，managed24/24、explicit24/24，本次48/48与全部kernel exit0，签名和进程清理通过。两种单实例阶段均记录第二退出后主激活。生产源码未改，历史失焦/2px漂移与失败实验仍保留，不能称根因修复或稳定性达标；D未完成/E未开启。

**最新 review 检查点：** [单实例退出协调实验](evidence/2026-10-03-d-singleton-exit-review/README.md)存在跨档案同 Bundle ID 常驻实例阻塞恢复的 P1 缺陷，生产改动已撤回，保留完整源码。实验 f120f29d 构建/安装签名、源码回归、独立8阶段及 package/startup/failure-exit通过；完整窗口managed完成21阶段后second-instance失败/kernel2、restoreRequests/Completions=0，explicit与lifetime未运行。未证明故障根因或2px问题修复；74仍为此前候选，D未完成/E未开启。

**同包时序观测：** [74fefe10前台/单实例时序](evidence/2026-10-03-d-activation-timeline/README.md)未改生产代码；首版managed四阶段通过，explicit单实例复现短暂key约15ms后失焦/kernel2。首版getter因未服务run loop可能陈旧，不作归因。修正只读事件观察器后explicit四阶段通过，收到第二实例退出及主实例随后激活；尚缺修正版失败时序，不能视为修复/48阶段通过。下一项检验退出事件协调，2px问题仍待定位，D未完成/E未开启。

**单实例定位补充：** [协作激活实验](evidence/2026-10-03-d-singleton-handoff-control/README.md)源码回归/clippy及构建安装通过，但实验cb85c6b5在managed前三阶段通过后second-instance仍失焦/kernel2，explicit未运行；无效生产改动已撤回，源码恢复此前提交，实验包不晋升候选。CUA超时；只读最终控制台已登录/onConsole，锁定字段缺失，测试后前台为另一应用，不能推定失败因果。下一步按失败PID/时序捕获前台切换及2px持久化差异；当前74的验收范围不变，D未完成/E未开启。

**最新真实安装候选：** [提交后 74fefe10 验收](evidence/2026-10-03-d-post-review-candidate/README.md)，源码371a731af，sidecar16e3761d/DMGb3a4555f，构建、只读复制安装、严格签名通过；12个不同程序门禁、双屏8项、当前包官方历史/附件/检查点及实际配置备份回退通过。全局 `.env` 迁移在当前安装sidecar与原生私有钥匙串fixture中实际1/1通过，系统授权/设置页/跨档案仍待验。完整window托管24/24、显式完成21阶段后second-instance失焦/kernel2；独立同阶段managed通过，explicit原生exit0但精确持久化y344→342拒绝，均保留。下一项优先定位这两个窗口观测，不继承旧c71的48/48。D未完成/E未开启。

**提交前 review 更新：** [代码审查与当前源码回归](evidence/2026-10-03-d-code-review/README.md)修复三个凭据来源拒绝覆盖的一致性、来源 FIFO 竞态，并隔离归档 Go 包；前端构建、Go/前端回归、真实 bridge Rust 227 项与 strict clippy 通过。新增原生 .env 测试已编译但 ignored/未执行。以下 c71 已安装包证据属于旧固定身份，不能覆盖本轮修改；新包构建/安装 smoke 待继续，D 未完成/E 未开启。

上一实际安装候选为 `c71ab66c727b1dc71854a993ba232f6515975328f684e365d3fc9bef801f2707`；sidecar `94207d45`、DMG `7b6a7a5f`，构建/只读复制安装/严格签名通过，当前包15个不同程序门禁通过；[真实.app启动完整窗口](evidence/2026-10-03-d-c71-launch-services-window-fixed/README.md)双档案48/48通过，全部kernel exit0、清理与持久身份保持。[启动方式对照](evidence/2026-10-03-d-c71-activation-context-control/README.md)四次Settings/API和原生菜单最小化恢复通过；原裸宿主启动8阶段后焦点失败与首轮适配器空环境参数拒绝均保留，不推断所有历史失败根因。零活动显示器条件门禁不适用，pending复活与物理UI仍待验。[Wails 全局 .env 迁移](evidence/2026-10-03-d-wails-env-migration/README.md)源码已实现、兼容旧官方 provider/UTF-16，Go配置及25项真实sidecar凭据事务回归通过（内存凭据后端）；[首轮](evidence/2026-10-03-d-wails-env-package-run/README.md)、[续跑](evidence/2026-10-03-d-wails-env-package-continuation/README.md)与[双屏8项](evidence/2026-10-03-d-wails-env-displays/README.md)保留精确结果；[官方Wails历史/附件/检查点回退](evidence/2026-10-03-d-wails-env-legacy-rollback/README.md)、[配置备份实际恢复](evidence/2026-10-03-d-wails-env-backup-apply-legacy-control/README.md)当前包通过，测试后两包严格签名通过且无匹配运行实例。不继承a62通过，真实OS全局来源迁移/系统授权与物理UI仍待验，D未完成/E未开启。

前一 a62f8d6d 候选：
[隐藏 WebView 权限验收扩展](evidence/2026-10-03-d-native-permission-expansion/README.md)已构建/只读复制安装/严格签名通过，生产权限行为未改；真实隐藏 IPC 检查6个自定义命令与伪造 main 参数、对话框插件拒绝，[新包八组门禁](evidence/2026-10-03-d-native-permission-package-run/README.md)全部通过；6命令×普通/伪造main双档案共24次固定caller拒绝、隐藏dialog/text插件拒绝、原剪贴板完整恢复及AppKit取消通过。[新包服务/生命周期六组](evidence/2026-10-03-d-a62-services-lifecycle-run/README.md)、[官方Wails历史/附件/检查点回退](evidence/2026-10-03-d-a62-legacy-rollback/README.md)与[配置备份实际应用](evidence/2026-10-03-d-a62-backup-apply-legacy-control/README.md)均实际通过。完整窗口、显示器恢复后pending与其余GUI尚未在新包复验；不继承前包通过为当前验收。D未完成/E未开启。

上一实际安装候选为 `7ac271422cee864ac7a296a1b0e9504370148efc359c2904e857bd36b205acb5`；sidecar `cd726201`、DMG `5ac3d738`。
[零活动显示器延迟恢复修复](evidence/2026-10-03-d-deferred-window-restore-change/README.md)的源码、8项窗口状态回归、strict clippy、app/DMG 构建、只读复制安装/严格签名通过；新包双档案原生反例保护通过，窗口种子不再从副屏横坐标改写为主屏。[新包七项门禁](evidence/2026-10-03-d-deferred-window-package-run/README.md)全部通过，包含新增条件门禁与原六项基础包门禁。[新包单实例/启动拒绝/lifetime三组](evidence/2026-10-03-d-deferred-window-lifecycle-run/README.md)通过，sidecar异常退出8/8；[官方Wails历史/Global附件/检查点真实回退](evidence/2026-10-03-d-7ac-legacy-rollback/README.md)及[当前包配置备份实际应用/旧CLI回读](evidence/2026-10-03-d-7ac-backup-apply-legacy-control/README.md)通过；分别保留范围，不据此放行图片渲染或任意自定义目录互斥。[新包原生服务四组](evidence/2026-10-03-d-7ac-native-services-run/README.md)通过：双档案菜单/剪贴板与真实AppKit取消、托盘语言、默认浏览器双入口和OS通知投递/移除/重启。活动显示器恢复后 pending 完成、完整窗口与其余 GUI 尚未在新包复验；此修复不证明最小化根因或 D 完成，E 未开启。

上一实际安装候选为 `cbb590ba4a2dd45afa322508387ddc07383993550bd9a48e17fedb4fc617a384`；sidecar `cd726201`、DMG `82e90085`。
[Wails 旧钥匙串入口/回归/构建及原生失败证据](evidence/2026-10-03-d-wails-keyring-change/README.md)、[新包六项门禁](evidence/2026-10-03-d-wails-keyring-package-run/README.md)。六项全部通过；Wails provider→APIKeyEnv 由鉴权 core 映射，前端不能提交任意账号，旧 Preview/Wails 入口分开；拒绝覆盖、同步回滚、固定错误/草稿保护通过源码回归。五次私有原生测试均在读取假来源时失败，最近 status=-25293，不能算实际 Wails 导入通过，现代全局 `.env`（StageModelCredentialLocked生成REASONIX_CONNECTION_*_KEY、UserCredentialsPath指向Reasonix home/.env）迁移当时尚未实现，后续已补源码入口与回归，见[全局来源迁移](evidence/2026-10-03-d-wails-env-migration/README.md)，实际OS导入仍待验；配置导入明确不复制.env，系统授权/取消仍待验。[私有来源CLI读取对照](evidence/2026-10-03-d-wails-source-cli-control/README.md)精确dummy/exit0，而原生仍未通过；不据此推定根因。
[新包单实例/启动拒绝/lifetime](evidence/2026-10-03-d-wails-keyring-lifecycle-run/README.md)三组全部通过；后者双档案 × idle/streaming × SIGTERM/SIGKILL 共8项，kernel 确认 sidecar exit0、同档案重启/原件保持/清理通过。[新包完整窗口](evidence/2026-10-03-d-cbb-window-run/README.md)首个 managed exercise 最小化失败，completed stages=0；原生服务与 GUI 尚未复验。新发现零活动显示器时保存的副屏坐标被缓存主屏覆盖，[延迟恢复修复](evidence/2026-10-03-d-deferred-window-restore-change/README.md)源码/8项几何回归/clippy通过，新7ac27142包已构建，双档案反例保护通过；其余门禁以当前索引为准；最小化根因未确认。D 未完成/E 未开启。用户提供浏览器已到达本机验收页的截图，仅作可见结果确认，不能绑定新候选身份。
上一 `0e607a62` 的[宿主应用数据根保护修复/构建](evidence/2026-10-03-d-host-app-data-boundary-change/README.md)、[六项门禁](evidence/2026-10-03-d-host-app-data-boundary-package-run/README.md)通过。不是任意跨版本自定义目录互斥或实际应用备份证明。
上一d295[真实文件选择及取消](evidence/2026-10-03-d295-real-file-dialog/README.md)通过；目录确认时 CUA 报告 Mac 锁定，目录成功与重启持久化未证实。
上一实际安装候选为 `d29503c849e5303eca92054bc8a6d4010e2bee1eaea90deb07d6ccc54a9c0822`；sidecar `29d985c7`、DMG `29091590`。
[父路径解析修复/构建安装](evidence/2026-10-03-d-explicit-parent-resolution/README.md)、[上一d295五项门禁](evidence/2026-10-03-d-explicit-parent-package-run/result.json)：parent-resolution/explicit-boundary/package/boundary/profile全部通过。alias/../home的Rust凭据身份和Go工作区实际位置一致，正式版原件保持；默认正式版目录重叠拒绝及双档案普通运行/配置修改持久化与备份保护通过。不是任意跨版本共享目录互斥或实际应用备份证明。
655真实父路径夹具exit2且原树新增元数据已复现并修正；首版d904环境路径检查失败也保留。
上一d295[八阶段独立最小化切片](evidence/2026-10-03-d295-minimize-controls/README.md)通过：双档案原API独立启动各两次、原生菜单和快捷键；保持原超时及原生断言，不代表物理点击或完整流程稳定。其完整窗口失败记录另列。
上一d295[identity/lifetime/startup通过、完整窗口首项失败](evidence/2026-10-03-d295-window-lifecycle-run/README.md)：managed exercise调整尺寸、隐藏/显示后的最小化超时，原生未最小化且无will/didMiniaturize；completed stages=0，后续及explicit完整流程未运行。无剩余Preview/现场进程，失败夹具保留。655[完整窗口第9项失败](evidence/2026-10-03-d-explicit-boundary-window-lifecycle/README.md)也保留，根因未确认。GUI及其余门禁未完成，D未完成/E未开启。

上一d295[几何四样本对照](evidence/2026-10-03-d295-geometry-control/README.md)均通过，不能解释完整流程失败；[实际窗口操作](evidence/2026-10-03-d295-physical-window/README.md)菜单和黄色按钮最小化均未完成，原生未最小化且无will/did事件；实际全屏/快捷键退出/原尺寸恢复和正常Quit通过（显式档案）。不据变形截图判定根因。
上一d295[原生服务三组复验](evidence/2026-10-03-d295-native-services-fixed-run/README.md)通过：双档案独立菜单/剪贴板/实际AppKit取消、默认浏览器双入口请求、六次OS通知投递/移除/重启。首轮剪贴板恢复工具交换UTF8/UTF16格式顺序的[失败](evidence/2026-10-03-d295-native-services-run/README.md)保留，[顺序恢复与预检修复](evidence/2026-10-03-d-clipboard-type-order-fix/README.md)保持完整有序字节比较且回归通过。不是全部剪贴板GUI、真实选择保存、通知点击/权限恢复验收。

下表是上一 `d1c9fd5a` 候选的功能切片和尚未关闭的功能门禁，不能继承为新7ac27142候选已验收。新7ac27142候选窗口、lifetime/startup、GUI钥匙串/剪贴板及其他门禁须复验；d295的窗口失败保留。历史失败和待办均保留。

| 要求 | 上一d1候选证据及范围 | 未完成门禁 |
| --- | --- | --- |
| 构建、最小权限与 sidecar 退出 | 上一d1候选 app/DMG 与 strict ad-hoc 签名、双私有档案 package、就绪后 8 次闲置/流式 SIGTERM/SIGKILL 与重启清理通过；capability 仍限主窗口文本剪贴板、打开对话框、事件监听/取消和拖动 | 正式签名/公证；独立剪贴板实际图片/隐藏调用方拒绝及原件恢复通过；其他自定义命令权限不能仅据静态 capability 认定通过 |
| 数据目录、身份与备份回退 | 上一d1候选边界、身份、导入后修改持久化及备份保护、启动异常、对话框取消与托盘语言七项通过，见[分项门禁](evidence/2026-10-03-d-pending-profile-permission-run/result.json)，以其逐项状态为准 | profile的restore阶段仅检查修改后重启保持，不是应用备份；历史官方Wails未参与新目录锁协议，显式共享目录互斥仍有缺口；上一d1候选[单份配置备份实际应用/官方旧CLI回读](evidence/2026-10-03-d-backup-apply-legacy-control/README.md)已通过；完整历史数据回退与设置页导入点击待验 |
| 菜单、快捷键、剪贴板、窗口与多显示器 | 上一d1候选[首轮完整窗口48/48](evidence/2026-10-03-d-pending-window-run/result.json)和[重复48/48](evidence/2026-10-03-d-pending-window-repeat/README.md)通过；[实际Cmd+逗号/全屏菜单/退出全屏按键](evidence/2026-10-03-d-pending-window-physical/README.md)通过。600完整窗口第9项失败、独立Cocoa复现仍保留，未认定根因或修复 | 长期稳定性、物理最小化最终状态（本次截图/AX不足）、IME/可配置快捷键、遗漏复制入口、不同缩放/拔插/物理拖动待验 |
| 托盘、关闭与退出 | 上一d1候选生命周期退出及两轮完整窗口矩阵通过；上一d1候选实际Quit菜单点击kernel/open0、无sidecar/ready残留；既有旧包证据保留 | 实际托盘点击、Dock重开及其余物理关闭路径待验；程序动作不等于物理点击 |
| 文件/目录对话框与外部链接 | 现有入口/边界实现与组件回归保留；Chrome 用户截图只证明本机 canary 显示 | 上一d1候选对应原生包/GUI选取取消与真实来源点击、邮件/OAuth未全验 |
| 通知 | 原生权限/送达与点击路由已实现；旧包实际送达记录保留 | 上一d1候选六次实际送达、横幅/点击、授权拒绝/恢复、冷启动完整链路待验 |
| 钥匙串 | 上一d1候选源代码原生私有锁定/拒绝/解锁恢复通过；600 explicit迁移/重启/删除通过；上一d1候选等待提示组件回归通过；上一d1候选 d1 [预先解锁 explicit GUI 保存/替换成功反馈、重启就绪且不回显、删除与退出](evidence/2026-10-03-d-pending-keychain-unlocked-gui/README.md)通过 | 替换值未原生读回；上一d1候选 GUI 等待提示、managed跨档案GUI、系统授权取消待验；CUA禁止SecurityAgent，需人工完成该取消切片；旧Preview迁移不等于Wails服务迁移 |
| E 全部能力 | remote host/bot部分接口和页面已在，但未按E逐项验收；updater目前为说明/手动下载入口 | D验收且Preview稳定后再推进remote host、bot、updater与复杂管理页；插件注册不能算更新流程完成 |
| A/B/C 与发布 | 本文前部已列明发布缺口；本轮D切片未关闭A/B/C | A合同/事件错误覆盖，B历史/Markdown图片与检查点等完整兼容，C终端/Browser/worktree/MCP插件真实生命周期；正式发布/默认下载切换须新授权 |

以下日期条目保留历史实施和失败现场。当前索引与逐包回执优先用于评估本候选，不能把累计局部通过计为D/E完成。

**2026-10-03 父路径解析与655窗口失败：** Rust/Go alias/..元数据位置差异源码及655真实包复现失败，词法路径对齐且不改环境后19回归/Clippy/完整构建、新d295五组真实门禁通过。655窗口第9阶段再次失败，原trace/夹具保留，不用d1旧成功替代。详见当前索引与[修复证据](evidence/2026-10-03-d-explicit-parent-resolution/README.md)，D未完成/E未开启。

**2026-10-03 显式正式版目录边界：** 新655候选增加启动前默认正式版目录重叠拒绝，18项回归/Clippy/完整构建及当前四组真实包门禁通过；首版路径改写package失败保留并已修正。已知默认根保护不能替代任意历史自定义根互斥，新包其他功能仍待复验，详见当前索引及[实施证据](evidence/2026-10-03-d-explicit-boundary-change/README.md)。

**2026-10-03 当前托管路径复制 GUI：** d1 原始私有 clipboard path 夹具，实际配置目录按钮反馈已复制，精确系统文本/代次 claim 通过，空默认/当前工作区复制禁用；Cmd+V 完整路径匹配，清空草稿后 Cmd+Q 正常退出。原件/身份与 sidecar/readiness 清理、原剪贴板条目顺序/类型/字节完整恢复通过。[脚本与操作观察](evidence/2026-10-03-d-pending-path-copy-ui/README.md)保存，未复制用户原剪贴板快照到证据目录。仅 managed 配置目录实测，不替代 explicit、消息/hooks、真实拒绝或 IME 验收；本轮未改生产代码，D未完成/E未开启。

**2026-10-03 当前窗口重复与实际操作：** d1 原始完整窗口矩阵再次managed24/24、explicit24/24通过，当前两轮共96阶段，不放宽断言或增加重试。实际设置快捷键、全屏菜单进入/Ctrl+Cmd+F退出、Quit菜单退出与sidecar清理通过。实际Minimize截图是缩小变换画面，AX/WindowServer元数据不足以确认最终原生最小化状态，标为inconclusive，Show恢复完整主窗口。[重复矩阵](evidence/2026-10-03-d-pending-window-repeat/README.md)与[物理操作记录](evidence/2026-10-03-d-pending-window-physical/README.md)保存。未改窗口生产代码，未确认历史间歇根因已修复；D其余门禁仍待验，E未开启。

**2026-10-03 当前候选钥匙串 GUI：** 预先解锁新私有 fixture.keychain，d1 explicit 设置页实际保存、替换均显示成功/就绪并清空输入；同 app/root 正常重启身份保持、就绪且不回显。实际删除显示已删除/未配置，指定私有 service/account 元数据查询 exit44；旧 Preview 原件 hash/mode/mtime 保持，两次 CmdQ kernel/open0、无 sidecar/ready 残留。[操作观察与退出证据](evidence/2026-10-03-d-pending-keychain-unlocked-gui/README.md)保存。未原生读回替换值，未验 managed/跨档案 GUI、系统授权取消、pending 提示真实 GUI 或删除后再次 GUI 重启；本次成功不能抹除旧锁定等待失败。D 未完成，E 未开启。

**2026-10-03 当前独立门禁与窗口/备份应用：** d1c9fd5a boundary/identity/profile/startup/failure-exit/dialog-cancel/tray-language七项通过，
profile restore仅修改后重启与备份保护，已更正此前过宽“备份回退”表述。
[当前48/48窗口](evidence/2026-10-03-d-pending-window-run/README.md)本轮通过，历史间歇失败不抹除、未认定原生根因已修复。
另将本候选真实备份实际应用到另一私有旧版配置目录，官方1.38.3 CLI摘要先验、CNY回读0、恢复文件/Preview备份/原件保持，
[独立应用证据](evidence/2026-10-03-d-backup-apply-legacy-control/README.md)保存。
单份配置CLI回退不能算历史GUI/会话附件/全部检查点/目录锁兼容已通过。D物理交互、重复稳定性、钥匙串授权取消及其余门禁仍未完，E未开启。

**2026-10-03 钥匙串等待反馈与新候选：** 600真实设置页保存假值持续pending；精确PID仍活，
采样定位SecKeychainAddGenericPassword等待，指定私有钥匙串locked，解锁后未完成。
CUA明确禁止SecurityAgent，未查看/取消弹窗；CmdQ kernel0/open0/无sidecar或ready残留。
补三语请求处理中提示，60秒待决保留草稿/一次写入/完成替换回归通过，未加重试或放宽权限。
[现场、源码及构建](evidence/2026-10-03-d-keychain-pending-recovery/README.md)保存，普通DMG失败保留后重建成功。
新host d1c9fd5a，sidecar29d985c7，DMG2be16391，从只读DMG私有安装strict签名通过；
[同包package/lifetime](evidence/2026-10-03-d-keychain-pending-package-run/result.json)均通过。
新提示真实GUI、保存/替换/重启、授权取消未验；新候选完整窗口矩阵未验，600已知失败不抹除。
D未完成/E未开启，未正式发布或切换默认下载。

**2026-10-03 原生钥匙串锁定/恢复：** 新ignored测试受私有HOME/默认钥匙串/无符号链接门禁保护，
生产PlatformCredentialBackend实际锁定读/替换/删除均拒绝且脱敏；解锁原值保持，替换/删除恢复。
两个全新夹具各1/1通过，正常用户default/search/偏好hash前后不变；首次45秒超时仍保留。
[源码、隔离回执与失败](evidence/2026-10-03-d-native-keychain-lock-recovery/README.md)保存。
最终普通回归23通过/2ignored、离线构建与tests Clippy通过。仅测试进程禁可选交互，生产授权不变；
不算600包真实GUI锁定/用户取消、managed隔离或Wails服务迁移验收。窗口仍失败，D未完成/E未开启。

**2026-10-03 纯 Cocoa 呈现对照：** 独立 Swift/AppKit/WKWebView，无 Tauri/Tao，
相同尺寸/有效权限，固定 loaded/none/loaded/none 四次同二进制私有启动。
loaded 重排两次 min/demini 原生状态和通知通过；none 两次 active/key 请求后 failed-mini，kernel1/open0。
[源码、构建与内核回执](evidence/2026-10-03-d-pure-cocoa-reorder-control/README.md)保存。
因此不能把全部失败归于 Tauri/Tao 事件循环；顺序环境变量仍未控、重排也不是稳定修复，未改生产。
当前600候选窗口门禁仍失败，D未通过/E未开启。

**2026-10-03 失败现场方法与呈现生命周期对照：** 同一私有诊断二进制 ebcf416b，
loaded 重排组 Cocoa4/4、Tauri3/4通过；Tauri失败时 active/key/visible/可最小化/屏幕存在，
请求前后及最终 miniaturize/performMiniaturize/deminiaturize 实现均与 NSWindow 相同。
去掉诊断重排后两种窗口8/8失败、原生最小化通知均未出现，不能把问题限定为 TaoWindow 覆盖，
也不能将重排视为稳定修复。两组顺序运行存在环境未控变量。
[完整16次回执与源码](evidence/2026-10-03-d-window-phase-method-failure/README.md)保存，
修正不存在getter后离线 build/clippy通过，初始编译失败保留。生产未改，当前候选窗口门禁失败仍有效，D未通过/E未开启。

**2026-10-03 当前窗口失败与方法只读对照：** 60076bd5 完整 window 门禁 managed 前 8 项通过，
第 9 项 menu-settings-minimized 失败，后续及 explicit 未执行；请求前 loaded/active/key/visible 前提成立。
[当前候选失败现场](evidence/2026-10-03-d-current-window-keychain-copy/README.md)保留。
同一私有诊断二进制 Cocoa 两次通过、Tauri 一次失败一次通过；新增只读方法比较后四次均通过，
miniaturize/performMiniaturize/deminiaturize 在成功路径均继承 NSWindow，未取得失败时比较结果。
[对照源码与完整回执](evidence/2026-10-03-d-window-method-comparison/README.md)保存，离线 build/clippy 通过。
生产未改、未放宽门禁，不将诊断正样本视为修复；D 未通过，E 未开启。

**2026-10-03 当前包钥匙串真实重启/删除：** 60076bd5新explicit私有HOME，真实迁移假值成功；
已有与缺失旧凭据新恢复文案均实际验收。两次同包同档案重启身份保持：首次恢复已就绪，实际删除/重复删除后，
下次启动仍未配置且不自动重导入。指定私有keychain/profile/account只读查询确认项目不存在，旧文件hash/mode/mtime保持。
[三轮实际UI与退出证据](evidence/2026-10-03-d-current-keychain-restart-delete/README.md)保存，三次CmdQ0/open0/无残留；
strict签名/hosthash保持。normal默认/搜索列表/偏好hash与较早基线一致，未读取真实密码。
managed、UI保存/替换、跨档案GUI隔离、锁定/拒绝和Wails服务兼容尚未验；D未完成，E未开启。

**2026-10-03 钥匙串迁移拒绝恢复修复：** import返回固定脱敏错误码，设置页中/繁/英分别说明已有不可覆盖/缺失旧凭据，
未知平台错误保持通用恢复；Rust23通过/1忽略、设置页回归及frontend生产门禁通过。真实app/DMG构建与只读安装strict签名通过，
新host60076bd5（sidecar29d985c7）；[源码/构建/失败现场](evidence/2026-10-03-d-keychain-import-recovery/README.md)和
[新包门禁](evidence/2026-10-03-d-keychain-copy-package-run/result.json)保存。旧249同档案重启身份保持但CUA两次app/一次inventory超时，
独立确认为live后精确SIGTERM清理signal15/无残留，不算UI恢复/删除通过。启动器受限复用与旧成功回执失效已补齐。
新提示GUI、钥匙串重启/删除/锁定等待验，旧249通过不升级为600结果；D未完成，E未开启。

**2026-10-03 当前包私有钥匙串成功迁移：** 249dfd15新explicit私有HOME，隔离探针实际写/查/删通过，
normal HOME默认/搜索列表与偏好摘要前后不变；普通sandbox创建错误保留。真实设置页旧Preview假值迁移
显示已就绪/已保存，空输入框不回显，旧文件hash/mode/mtime保持；再次迁移拒绝但通用错误文案不准确。
[成功迁移与隔离证据](evidence/2026-10-03-d-keychain-success-ui/README.md)及CmdQ0/无残留已保存。
同档案重启、实际删除、锁定/拒绝、managed和Wails服务兼容仍未验；D未完成，E未开启。

**2026-10-03 浏览器用户确认：** 用户回复“浏览器打开了”，截图显示 Chrome 本机 `open_external_url`
验收页已渲染。[用户确认记录](evidence/2026-10-03-d-browser-user-confirmation/README.md)补入外链可见结果证据。
随后用户再次提供 `open_external_link` 验收页截图并确认已可用；结合双入口实际回执，此浏览器打开检查已通过，停止无关改动后的重复测试，仅在相关逻辑/权限/运行环境变化或新失败时复验。
成功夹具端口/nonce 未归档，无法对应准确档案或候选 hash；不把旧包门禁升级为当前包全部外链验收。
不涵盖来源真实点击、mailto/OAuth 或所有入口；D 其余验收未完成，E 未开启。

**2026-10-03 当前通知/钥匙串负向界面：** 同249dfd15两轮通知门禁通过，验收工具新增白名单receipt输出，
[逐条六次送达](evidence/2026-10-03-d-notification-receipts/receipts.json)含2次actual active和4次hidden/非active、精确清理/重启保护；未改变门禁或OS设置。
当前源码真实macOS随机命名空间Keychain测试1/1通过；当前包新explicit设置页迁移旧假值实际失败并持续恢复提示，
只读查询证明私有HOME无默认Keychain（normal HOME存在），旧文件hash/mode/mtime保持，CmdQ0/无残留。
[钥匙串负向UI证据](evidence/2026-10-03-d-keychain-unavailable-ui/README.md)不算锁定/授权拒绝或成功迁移；
旧Preview服务/夹具不等于Wails reasonix服务凭据兼容性。D其余权限/点击/稳定性未完成，E未开启。

**2026-10-03 当前候选实际窗口复验：** 249dfd15新explicit私有档案，菜单/按钮/CmdM尝试后
均无原生minimized或will/did-mini/demini；截图按钮区存在紫色共享形状控件但来源/因果未验证，
不宣称实际黄色按钮命中或selector内部计数。View菜单全屏尝试未生效；Ctrl+Cmd+F真实进入/退出，
原生fullscreen位与几何精确恢复。一次防过期拦截后刷新状态才继续；CmdQ kernel0/open0、无残留。
[当前包实际交互证据](evidence/2026-10-03-d-current-physical-window/README.md)保存；
本快捷键全屏切片不代表菜单/最小化/稳定性通过，生产未改，D/E未完成。

**2026-10-03 当前候选窗口全矩阵复验：** 249dfd15签名/摘要前后相同，既有未改完整window门禁，
managed24/24通过；explicit8项通过，第9项menu-settings-minimized未进入原生最小化，总runner1。
[当前包完整失败回执与现场](evidence/2026-10-03-d-current-window-after-phase/README.md)保存；
explicit后续阶段未执行，managed不能替代explicit，重复启动稳定性仍失败。生产代码未改，D未通过/E未开启。

**2026-10-03 状态阶段最小化对照：** 私有example按loaded/key/mini/demini推进，要求瞬时前提和原生通知。
Cocoa+WK两次通过；Tauri overlay初次失败，后同二进制重跑及保留代理的新采样版也通过。
[完整时序/失败/重跑证据](evidence/2026-10-03-d-state-phase-window/README.md)保存；
移除NSWindow代理不是已证明修复，生产未改。原生状态前后不稳定根因仍未确定，
当前249dfd15完整窗口门禁独立复验，不借诊断正样本关闭D。

**2026-10-03 AppKit run-loop timer 调度对照：** 私有 example 新增50ms单次原生定时器，
默认feature离线release构建与Clippy通过。同二进制650f2b60的Cocoa+WK/loaded reorder及Tauri overlay
均满足激活/可见/页面前提，mini/demini各执行一次仍无原生最小化（kernel2/open0）；
紧接复验Swift Cocoa正对照收到will-mini/did-mini/did-demini且kernel0/open0。
[调度对照证据](evidence/2026-10-03-d-run-loop-timer/README.md)保留失败与正对照；
不能归因为Tauri事件循环，下一步控制加载完成后的激活/调用时序。生产代码/权限未变，D/E未完成。

**2026-10-03 当前真实包目录/文件/保存对话框交互：** 249dfd15隔离explicit档案，
CUA实际目录Open与文件Open返回精确私有路径/待发送附件；取消保持原选择，附件清空。
诊断Save选择含中文/空格/&目录，0600有效JSON；覆盖警告Cancel保持原件hash/mode/mtime，
Replace写出不同有效0600JSON且首份备份保留。Cmd+Q kernel0/open0、sidecar/readiness无残留。
[真实选择与保存证据](evidence/2026-10-03-d-dialog-physical-selection/README.md)已保存；
仅此explicit单文件/目录/诊断入口切片，不代替所有入口、多选/失败/托管矩阵或Wails ZIP格式等价性。
D窗口/权限其余验收保持未完成，E未推进。

**2026-10-03 原生主题runtime边界收敛：** 对照Wails1.38.3，将四类WindowSet主题/背景调用
移入窄Wails adapter，theme.ts不再直连runtime。主题308项、Tauri appearance回归、frontend生产门禁通过；
当前249dfd15完整app/DMG、只读安装/strict ad-hoc签名通过，同包package通过，
新增appearance切片沿用既有Rust断言，两档案各7/7主题切换/重启/拒绝/失败回退阶段通过，
凭据身份稳定、正常退出与无sidecar/readiness残留。
[源码/构建证据](evidence/2026-10-03-d-theme-boundary/README.md)及
[当前包门禁](evidence/2026-10-03-d-theme-boundary-run/result.json)已保存；tsx sandbox IPC失败保留。
前包窗口全屏/退出切片不得直接沿用；最小化和其余D矩阵尚未通过，E未推进。

**2026-10-03 Wails sender 对照与官方包恢复：** 冻结 Wails v2.13.0 Minimise 使用nil sender，
私有 Tauri direct nil 对照满足激活/加载前提仍失败（kernel2/open0），
[证据](evidence/2026-10-03-d-wails-sender-control/README.md)已保存，生产未改。
官方Desktop1.38.3归档重新完整下载、固定摘要及strict签名通过，隔离GUI已启动；
菜单最小化后的AX树不足以证明其状态；可见状态确认待补，180秒观察超时无正常退出回执，
opener已结束但TERM后host仍存活；重新监视确切PID并实际Cmd+Q取得kernel0，
原runner超时仍保留，最小化未计通过，
[基线记录](evidence/2026-10-03-d-official-window-baseline/README.md)保留。D未结案。

**2026-10-03 当前真实包原生交互：** `bfb47fd8` 同一隔离explicit进程，CUA Window→Minimize、
黄色按钮及Cmd+M尝试后 nativeMiniaturized=false、无will/did-mini事件；最小化未通过。
View菜单全屏进入/退出实际通过，FullScreen样式位出现/消失、几何精确恢复；Cmd+Q
kernel0/open0，sidecar/readiness清理。仅此菜单全屏切片通过，未代替整个窗口稳定性或按钮/快捷键矩阵。
[真实交互证据](evidence/2026-10-03-d-installed-physical-window/README.md)已保存；生产源码未改，D保持未完成。

**2026-10-03 Tauri Cocoa/WKWebView 完成后显示对照：** 私有 example 加入真实
非持久 WKWebView；相同二进制 none/loaded 两份都满足页面完成/激活/key前提，
loaded 重新显示执行，mini/demini仍失败（kernel2/open0）。release/Clippy通过。
通过 Swift 样本改1280×820仍通过，延迟样本 failed-key无有效最小化调用；
[四份原始证据](evidence/2026-10-03-d-tauri-web-loaded/README.md)已保存。
不将实验当生产修复；后续转实际菜单/按钮与程序调用对照，D 未结案。

**2026-10-03 Cocoa 新构建与启动身份复核：** 原通过 Swift 源码的新身份样本 failed-key，
采用通过样本身份的新构建通过全部 mini/demini 原生事件；同身份空窗口样本前提未成立。
[三组原始证据](evidence/2026-10-03-d-cocoa-startup-identity/README.md)已保存，不能认定身份是根因，
也不能用前提失败样本比较最小化。下一步直接控制 Tauri 私有样本的 WKWebView 完成后显示条件。
生产行为未改，D 窗口与稳定性门禁保持未完成。

**2026-10-03 Cocoa 变量控制纠正调查范围：** 普通 NSApplication 空窗口在与 Tauri
样本对齐的内容/尺寸/节奏下同样失败（激活前提成立、kernel2）。单独增加 WKWebView、
改用 Timer、缩小尺寸仍失败；orderOut 样本激活前提未成立，移除 WKWebView 的阶段式
样本 failed-mini。原先通过的 Cocoa 二进制同轮复跑仍通过。
[七组证据](evidence/2026-10-03-d-cocoa-controlled-variables/README.md)已保留，不能直接归因于 Tauri
事件循环；下一步控制 WKWebView 完成后显示与调度组合。生产行为未改，D 未完成。

**2026-10-03 原生窗口事件采样：** 独立 example 增加目标 NSWindow 通知监听，
release/Clippy 通过。激活前提成立的样本仍失败（kernel 2/open 0），最小化调用后
收到 resign-key，恢复后收到 key/occlusion，未收到 will-mini/did-mini/did-demini。
[源码与事件证据](evidence/2026-10-03-d-window-native-events/README.md)已保存；生产代码未改。
两类对照仍有 WKWebView 内容与调度差异，具体根因尚未证实，继续控制这些变量。

**2026-10-03 当前环境窗口有效失败：** 独立 Cocoa 对照通过后重新构建 Tauri example，
保留 TaoApp/delegate 的独立 Cocoa NSWindow 满足激活/key/visible/pageLoaded 前提，
原生 mini/demini 各执行一次但 minimized 始终 false，kernel exit 2、open exit 0。
[失败证据](evidence/2026-10-03-d-current-tauri-window/README.md)已保存；runner 成功采集不等于窗口通过。
生产窗口未修改，具体根因未证实，D 窗口门禁仍未通过。

**2026-10-03 外链同包独立复验：** 当前 `bfb47fd8` 候选在默认浏览器 Chrome 已运行的环境下，
托管/显式档案各两个原生入口均收到真实浏览器回执并正常清理；用户截图确认本机验收页已渲染。
[复验结果](evidence/2026-10-03-d-links-handler-run/result.json)与
[handler 查询和用户确认](evidence/2026-10-03-d-browser-handler-recheck/README.md)已保存。
前次超时仍保留、根因未确认，未宣布 Preview 稳定或 D 完成。
同轮[独立 Cocoa 对照](evidence/2026-10-03-d-current-cocoa-activation/README.md)重新构建并通过
激活/key、原生最小化与恢复（kernel/open exit 均 0），Tauri 窗口问题继续单独定位。

**2026-10-03 外链 runtime 收敛：** BrowserOpenURL 直连移入窄 Wails adapter，Tauri
入口与拒绝不回退保持；外链回归及生产前端门禁通过，新 `bfb47fd8` app/DMG 构建与
只读安装/strict ad-hoc 签名通过。同包 failure-exit/package 通过；links 首个托管档案
没有收到实际浏览器 loopback 回执而失败，显式未开始，顺序 lifetime 未运行；另开独立
同包 lifetime 门禁，结果见其记录。外链失败原因未确认，不能沿用旧包通过。
独立同包 lifetime 八种组合全部通过，未覆盖或撤销外链失败。
同包 gate 终态见
[程序门禁](evidence/2026-10-03-d-external-boundary-run/result.json)，
[源码与构建证据](evidence/2026-10-03-d-external-boundary/README.md)保留工具链首次拒绝及后续通过。
未完成窗口/权限验收，D 未结案、E 未推进。

**2026-10-03 实际激活尝试未完成：** 独立诊断样本等待 CUA 点击后再启动采样；UI 绑定
异常长时间 -10005 超时，应用自身 60 秒期限结束，确切 PID 已退出，没有实际点击。
超时结果字段缺失导致 runner 解析失败且未保存 kernel 回执，源码/runner 已修复，
未将其记为窗口通过；[失败证据](evidence/2026-10-03-d-manual-activation/README.md)保留。
窗口 UI 路径暂未取得有效结果，继续可执行的 D adapter/源码与包级回归，未延期窗口要求。

**2026-10-03 delegate 对照前提失败：** 独立 example 的两份 keep 与一份 none 样本均
visible=true 但 active/key=false；实际 delegate 存在/缺失已逐快照核对，kernel=2、
runner=1，全部退出。没有形成有效最小化比较，不能排除或证明 delegate 根因。
下一步检查应用激活；[原始失败证据](evidence/2026-10-03-d-delegate-control/README.md)保留。
生产包未改，D 未结案、E 未推进。

**2026-10-03 应用类对照：** 独立 example 的同一二进制在 TaoApp 与普通 NSApplication
两种实际应用类下，普通 Cocoa 窗口都满足 active/key/visible 前提、请求执行 1/1，
但最小化失败且 kernel=2。TaoApp 子类不是复现的必要条件；delegate/事件循环等仍待区分。
生产 host 未改，参见 [应用类对照证据](evidence/2026-10-03-d-application-class-control/README.md)。
完整 D→E 目标已重新建立，D 未结案、E 未推进。

**2026-10-02 macOS 后台关闭基线修复：** Wails 使用 application hide，Tauri 原先仅 window
hide。现改为 macOS AppHandle.hide；原生 smoke 明确要求隐藏/取消隐藏应用。新 `15fcd43b`
app/DMG、Clippy、strict ad-hoc 签名通过；实际关闭 hidden=true、sidecar 保留，实际
Window→Show Reasonix 后 hidden=false、恢复完成 1/1、Reopen=0；实际 Cmd+Q 清理通过。
同新包错误退出两档案=2、正常 package 两档案=0、异常生命周期 8/8 均通过。
AX 读取/重新绑定未恢复；Dock/SystemUIServer 绑定超时，手动 Dock/托盘未验收。
首次 DMG 失败未定位原因，正确入口 verbose 与最终重建通过，全部保留。
见 [应用隐藏修复证据](evidence/2026-10-02-d-application-background-close/README.md)
及同包 [程序门禁](evidence/2026-10-02-d-app-hide-installed-run/result.json)。D 未结案、E 未推进。

**2026-10-02 实际关闭按钮：** 同 `e0fc4d12` 包的两个私有档案验证默认后台关闭
隐藏窗口并保留进程，以及真实设置页切换 `quit`、落盘后点击原生关闭按钮，kernel/open=0、
sidecar/ready 无残留。后台样本的 AX 读取触发了 Reopen，不能计为手动 Dock/托盘恢复。
首轮 300 秒等待超时保留；工具增加显式 1..900 秒交互等待，普通门禁默认不变。
Rust host preferences 10 项与 Wails 关闭/恢复基线回归通过；实际活动回合关闭、安装包
同档案重启、手动 Dock/托盘恢复和窗口失败仍未验收。
见 [实际关闭证据](evidence/2026-10-02-d-physical-close/README.md)。D 未结案、E 未推进。

**2026-10-02 实际键盘剪贴板：** 同 `e0fc4d12` 新私有样本通过实际 Cmd+C/X/V、
精确 canary/generation、草稿清空、Cmd+Q=0 与该轮开始时的所有剪贴板格式恢复。
首轮因 canary 文本建议变化/选择工具错误及超时失败；旧夹具在保留变化后的内容时
误删备份，原剪贴板未恢复且不能从夹具取回，已说明并保留证据。已修复失败时独立
恢复副本保留，假数据回归旧两项失败/新三项通过；后续通过不覆盖首轮影响。
见 [键盘与恢复工具证据](evidence/2026-10-02-d-keyboard-clipboard/README.md)。D 未结案、E 未推进。

**2026-10-02 生产退出码修复：** 旧 `9d58aae0` 明确 native 错误 marker/请求 exit(2)
却实际 kernel=0；新 host 用 run_return 完成原清理后报告明确请求码。新 `e0fc4d12`
app/DMG、Clippy、strict ad-hoc 签名通过；两档案错误 kernel=2、正常 package=0、
异常生命周期 8/8 和实际 Cmd+Q kernel=0/无 sidecar/ready 均通过。
`failure-exit` 加入默认安装包门禁。参见 [退出码修复证据](evidence/2026-10-02-d-host-exit-code/README.md)。
窗口失败与完整 D/E 验收未解决，不发布、不切换默认下载。

**2026-10-02 主队列窗口对照：** 同二进制 Cocoa direct/main-queue 均满足前提、
动作执行 1/1，但最小化失败；Tauri 普通栏前提不满足，Overlay 未运行。
没有证明只改主队列调度能修复窗口，参见 [主队列证据](evidence/2026-10-02-d-mainqueue-window/README.md)。

**2026-10-02 Cocoa/Tauri 应用上下文对照：** 普通 Cocoa NSWindow（无 Tauri 窗口子类/
delegate）在 Tauri 应用事件循环中仍复现最小化失败，active/key/visible 前提满足；
独立 Cocoa/WKWebView 当前复跑通过，minos/SDK/entitlement 对齐后仍通过。复现不需
Tauri 窗口子类，但尚未定位应用 delegate、事件循环操作时机或其他宿主上下文差异。
首轮前提失败保留；生产 host 未改。
见 [Cocoa/Tauri 上下文证据](evidence/2026-10-02-d-cocoa-tauri-loop/README.md)。D 未结案，E 未推进。

**2026-10-02 同依赖最小 Tauri 对照：** 独立、固定页面、无 sidecar/产品设置的
普通和 Overlay 标题栏均在页面 Finished/active/key/visible 前提满足后复现原生
最小化失败，两个最终诊断退出回执=2。业务页、sidecar、恢复路径和覆盖标题栏
不是复现的必要条件，仍未定位 Tauri/Tao 窗口或事件循环的具体原因。首轮夹具
退出码不传播已修正，仅改变 example，不改生产 host。
见 [最小 Tauri 证据](evidence/2026-10-02-d-tauri-window-baseline/README.md)。D 未结案，E 未推进。

**2026-10-02 实际窗口只读观察与退出回执：** 新 `9d58aae0` 包完成 app/DMG、
strict ad-hoc 签名和两种私有 LaunchServices package 复验。实际 Cmd+Q 的确切
PID kernel 退出码 0，sidecar/ready 无残留，退出后不再读取失效 UI binding。
实际 Cmd+M、按钮和 Window 菜单最小化均无 Will/Did 事件且 native minimized=false；
全屏进入有原生状态，但退出操作未清除 FullScreen 位。这两项不判通过，仍需定位。
见 [实际窗口证据](evidence/2026-10-02-d-interactive-window/README.md)。D 未结案，E 未推进。

**2026-10-02 正常 LaunchServices 与实际文件确认：** 同一 `317d96bb` 包两种私有
档案经 open/LaunchServices 正常启动、实际目录/401/身份及 kernel 正常退出核对
通过。新的显式 UI 档案通过实际文件 Open 后出现待发送 chip、再次 Cancel 保留
附件、移除草稿及实际 Cmd+Q（私有 host/sidecar/open 全部退出且原件不变）。
退出后 CUA 观察转到新默认档案，立即停止操作并清理精确测试副本；不声称默认
档案数据无写入，后续退出观察改用已确认 PID。首轮夹具错误、范围与证据见
[LaunchServices 与文件确认](evidence/2026-10-02-d-launch-services/README.md)。D 未结案，E 未推进。

**2026-10-02 最新验收探针包：托盘 IPC caller。** host `317d96bb` 仅扩展 opt-in probe，
真实主 WKWebView 新命令成功，真实隐藏第二窗口的正常/伪造来源请求均由主窗口
custom guard 拒绝；两档案菜单/剪贴板/语言 6/6、正常 package、clippy 与完整
app/DMG 通过。原生产策略未改；48+8+8 仍归属前一 `4d2a6865` 包。范围、摘要
及后续正常 LaunchServices 启动验证见 [caller 证据](evidence/2026-10-02-d-tray-caller/README.md)。
物理验收和偶发窗口失败根因仍保留，D 未结案，E 未推进。

**2026-10-02 托盘语言候选同包完整复验：** `4d2a6865` 的完整窗口 48/48、
异常退出 8/8、双屏 8/8 均通过，包摘要/签名和验收依赖一致。未执行 CUA 操作；
不撤回前包失败或推断根因已解决。实际弹出/点击和不同缩放/拔插仍待验，详见
[同包窗口与生命周期](evidence/2026-10-02-d-tray-current-window/README.md)。新增命令的真实第二 WebView caller 检查正在另批构建，未转记为此包已验。

**2026-10-02 最新候选：托盘语言基线补齐。** host `4d2a6865` 接入已解析 UI 语言的
runtime-only 托盘标题同步，Auto 不改写持久化语言，未知输入拒绝，更新失败显示
重试提示；主窗口 custom caller 限制和 capability 保留。完整前端、Auto/错误反馈
组件回归、测试类型检查、clippy 与生产 app/DMG 通过；真实只读安装包两档案
菜单/剪贴板/语言 6/6 及正常 package smoke 通过。标题读取为 Tauri 菜单项缓存，
不代替实际托盘弹出/点击。新包完整 48 阶段窗口等未复验，前包失败保留，详见
[托盘语言证据](evidence/2026-10-02-d-tray-language/README.md)。D 未验收，E 未推进。

**2026-10-02 当前候选独立异常退出：** 同一 `b95b38c0` 包的托管/显式 × 空闲/实际流式
任务 × SIGTERM/SIGKILL 共 8/8 通过，sidecar kernel 正常退出、上游断开、原件及
重启身份检查通过。独立运行不覆盖完整窗口批次中 lifetime 未执行的记录；runner
另补齐未跟踪生产代码快照及摘要，详见 [同包退出证据](evidence/2026-10-02-d-current-lifetime/README.md)。

**2026-10-02 当前候选完整窗口仍失败：** `b95b38c0` 托管档案 23/23 全通过，显式
前 8 项通过后 Settings 最小化前提失败，合计 31/46；后续串行 lifetime 未执行。
请求时真实 active/key/occlusion 可见且主页面完成，Will/Did 最小化仍为 0，随后失活；
未确认根因，也不将成功档案推广为稳定。新 CUA 会话绑定仍报 -3811，文件确认未操作；
失败现场、原始诊断、同包摘要及私有清理见 [完整窗口证据](evidence/2026-10-02-d-current-window/README.md)。D 仍未验收。

**2026-10-02 当前包原生取消路径：** `b95b38c0` 同包托管/显式档案各三个独立阶段
通过，共 6/6，包含菜单、文本剪贴板及文档保存/诊断导出/主题导入/目录选择四类
实际面板取消；后续两档案 package smoke 也通过，摘要与严格签名复核一致。
串行 runner 支持独立 `dialog-cancel`，且修复 Python 3.9 摘要兼容。范围和初次初始化
失败见 [同包取消证据](evidence/2026-10-02-d-current-dialog-cancel-run/README.md)。不代替选中/保存成功或完整窗口门禁。

**2026-10-02 最新候选物理交互补验：** 同一 `b95b38c0` 安装包的显式私有档案
通过实际 Cmd+, Settings、目录选择、附件文件取消与完整 host 重启后的工作区恢复。
文件确认后观察被 ScreenCaptureKit -3811 打断，重新绑定仍失败；实际最小化操作
仅有 AX/单窗口截屏，不能据此确认状态，仍待核实。私有进程经精确路径核对后
SIGTERM 清理无残留，不能代替 UI Quit。详见 [实际对话框证据](evidence/2026-10-02-d-interactive-dialogs/README.md)。
原生窗口失败与其余物理待验仍阻塞 D，E 未推进。

**2026-10-02 最新候选：遗留 runtime 收敛与窗口保存取消。** host `b95b38c0e35d66c964bc370b621a46c9d69116b7408a7c7567f4c715abc17734`
将 Wails 窗口几何和文本剪贴板访问收敛到轻量适配器；hook 卸载取消尚未提交的旧
观察/排队捕获，旧逻辑的三个确定性失败修复后通过，剪贴板及完整测试类型检查通过。
生产 app/DMG 构建和真实只读安装、独立菜单/剪贴板 4/4、正常双档案 package smoke
通过。本轮不撤回已发送的保存请求，也不改变 Tauri 窗口策略或解决最小化；完整窗口
未复验，失败与物理待验项仍阻塞 D。源码补丁、新适配器快照和范围见
[runtime 生命周期证据](evidence/2026-10-02-d-runtime-lifecycle/README.md)。E 未推进。

**2026-10-02 前一直接调用诊断包：窗口仍失败。** host `de4f88c88dc80f3757b577315dbe9d42b8f75134e914851065a70bae34d814f5`
在实际主线程绕过 Tauri 调度和菜单 responder 直接调用 NSWindow，托管首次最小化
仍失败，显式未开始；正常两档案 package smoke、clippy 与生产 app/DMG 通过。
独立最小 Cocoa/WKWebView 对照通过，不能将失败视为系统完全无法最小化；当前
Stage Manager 只读状态为开启，未修改，也未确认其为原因。详情见
[直接 NSWindow 对照](evidence/2026-10-02-d-direct-minimize/README.md)。D 仍未验收。

**2026-10-02 前一呈现诊断包：窗口仍失败。** host `c29beab60b5b30e7634b2ff2114b4dfe189ec673e44324fb13ae487d8d8b5bbf`
增加 opt-in 遮挡/应用激活事件及单调时间诊断，完整生产 app/DMG、clippy、正常两档案
package smoke 通过；独立原 API 和可见呈现探针均未通过。原 API 请求时已有 key/激活/
可见状态，仍没有最小化事件；可见呈现探针在前提阶段失败，未请求最小化。前一安装包
的真实 Minimize responder 也失败。未修改生产恢复策略、未替换原门禁，也未解释根因；
详见 [原生呈现证据](evidence/2026-10-02-d-minimize-presentation/README.md)。

**2026-10-02 前一候选：启动身份拒绝修复，窗口门禁仍失败。** 已安装 host
`49e8b9cda038bd5f002178079aa3895690dbff3ee1d8489334dd0c8b60fedd1a` 将返回的 setup
错误集中处理为正常非零退出；身份损坏/冲突/链接的两档案拒绝与备份恢复 6/6、
默认目录拒绝、配置导入、启动中断 12/12、独立异常退出 8/8 通过；完整 Rust
218 通过、2 默认忽略，clippy 与生产 app/DMG 构建通过。完整窗口在托管 8 项后
Settings 最小化前提失败，显式未开始；没有转记上一包成功或重跑覆盖失败。
源码、当前包摘要及失败/成功证据见 [启动恢复证据](evidence/2026-10-02-d-startup-refusal/README.md)。
物理交互及窗口稳定性仍未验收，D 未结案，E 未推进。

**2026-10-02 前一候选同包复验（持续目标已建立）：** 已安装 host `9676735e7a5a046de9592d7d633fb9cf632280159b3a8d5081f6f430f23cf220`
已通过完整窗口 46/46、就绪后异常退出 8/8、双屏 8/8、两档案外链与六条通知送达；
官方 Wails 1.38.3 私有历史/附件/最新检查点回退通过，显式原生钥匙串测试 1/1 通过。
这补齐最终包的证据归属，不解释历史偶发窗口失败，也不代替下方物理交互待验项；D 未结案，E 未推进。
下表和后文保留历史包记录，该包摘要、精确范围及可复跑串行 runner 见
[同包验收证据](evidence/2026-10-02-d-current-candidate/README.md)。A/B/C 与签名/公证及发布授权缺口保留。

**当前平台范围（2026-09-30 用户确认）：本轮只推进 macOS。** Windows 和 Linux 因缺少实际测试环境，改造及原生验收均延期，不作为本轮 D/E 完成或 macOS 发布候选的阻塞项。此前已提交的跨平台实现和验证记录保留，不能据此宣称 Windows/Linux 已受支持；本轮尚未提交的 Windows 通知改动已撤回。下方历史记录中的跨平台待办转入延期范围，后续有环境再恢复。

**外接屏范围调整（2026-10-02 用户确认已打开）：恢复连接显示器的原生定位与重启验收。** 当前应用识别到两块 2× 缩放屏幕；不同缩放、实际拔插与物理拖动仍未验收，需在相应条件可用时继续。2026-10-01 的延期及既有单屏证据保留为历史记录；Windows/Linux 继续延期，D→E 顺序和正式发布门禁保留。

macOS 继续按以下顺序收尾；每项分别记录源码回归和真实安装包验收，未执行的交互不标记通过：

1. 菜单/快捷键与系统剪贴板：真实 WebView 编辑操作、可配置快捷键冲突、复制/粘贴失败反馈。
2. 窗口/托盘与退出：隐藏和最小化恢复、关闭后后台任务、Cmd+Q/托盘退出、第二实例唤起；外接屏已恢复可用，先验副屏定位和重启；不同缩放、实际拔插与物理拖动继续待验。
3. 对话框/外部应用：选择与取消、另存为、Finder/编辑器/终端工作目录、浏览器/邮件和 OAuth。
4. 通知/钥匙串：授权拒绝和重新开启、实际横幅及前后台/冷启动点击、钥匙串拒绝与设置页凭据迁移。
5. 数据兼容/回退：旧 Wails 二进制、导入与回退、旧 Global 文件引用/附件/检查点；再次核对安装包生命周期和无残留。
6. macOS D 验收且 Preview 稳定后，依次审计并迁移 E 的 remote host、bot、updater、复杂管理页。A/B/C 剩余缺口及正式签名/公证继续保留为 macOS 发布门禁。

| 项目 | 当前实现与验证 | 待完成验收或改造 |
| --- | --- | --- |
| D：菜单与快捷键 | macOS 编辑项改为 Tauri 原生 responder-chain 角色；设置菜单先恢复主窗口并发出设置事件。Reload 移除 Cmd+R，保留给可配置的会话刷新；设置和文字大小也不安装固定原生组合。原生编辑/退出/隐藏/最小化/全屏及系统 Emoji 组合禁止保存为 Preview 动作；旧冲突组合回落默认值，单项重置校验默认组合占用。真实 AppKit 菜单组合与共享保留表、设置菜单动作、组件与独立 Chrome 页面回归通过。新增独立 `--edit` 原生编辑门禁。私有真实包的产品输入框通过 CUA 原生按键验证输入、全选删除、撤销/重做；实际 Edit→Redo 点击与最小化后的 Settings 点击也通过。 | 干净包 `e3e030586` 两种档案的严格 `--edit` 已通过：真实 key window、WKWebView responder、选择及 Copy/Paste/Cut/Select All/Undo/Redo 和完整剪贴板恢复。托管私有普通档案的产品输入框 Cmd+C/X/V 已由 CUA 原生按键及独立系统代次核对通过；两种普通私有档案的配置目录实际按钮/成功反馈/输入框 Cmd+V 已通过；两种普通私有档案的实际输入框 Cmd+A/C/X/V 与 WebKit 右键 Copy/Cut/Paste 已通过精确文字、操作前后系统代次和原件恢复验收；消息菜单、实际失败反馈、IME/组合输入、隐藏其他应用、全屏与可配置快捷键路由仍待验收。程序化原生菜单动作和 Chrome 页面回归不代替用户原生交互。 |
| D：剪贴板 | 接入官方 clipboard-manager，主窗口仅允许读写文本；Tauri 原生拒绝后不再使用浏览器/Wails/execCommand 回退；Hooks JSON Copy/Paste 已从浏览器直连收敛到共享文本入口，拒绝/空值/旧 workspace 结果保护草稿，组件回归通过；共享写入/读取路径覆盖 Tauri、浏览器与 Wails。消息、存储路径、hooks 路径及输入框复用；复制成功反馈等待实际写入成功。原生调用模拟、拒绝/忙碌回退、失败剪切不删文本、空剪贴板不覆盖选择与成功反馈测试通过。实际 macOS WKWebView IPC 与系统文本读写通过；主窗口图片读取及无权限的同源隐藏窗口读写均拒绝，独立系统值/代次核对和完整原件恢复通过。 | 托管私有普通档案的输入框 Cmd+C/X/V 已通过实际 CUA 原生按键：剪切清空、粘贴精确文本恢复、系统值/代次及剪贴板原件恢复；`ada308408` 包两种普通私有档案的配置目录按钮、空工作区禁用状态、已复制反馈及输入框 Cmd+V 已通过实际 CUA 操作和系统值/代次核对，原剪贴板完整恢复；两种普通档案的输入框实际 Cmd+A/C/X/V 与 WebKit 右键 Copy/Cut/Paste 及空值禁用状态也已通过，Copy/Cut 要求严格新代次；其他存储/hooks 路径、消息复制/菜单、实际失败反馈和 IME/组合输入仍待验收。 |
| D：自定义命令权限 | 自定义 native/bridge 总入口按 Tauri 原生调用者限制为主窗口，插件保持独立 capability；真实包双 WKWebView 在两种档案通过主窗口 catalog/可执行目标拒绝、第二窗口 8 个文档和 2 个 bridge 查询拒绝、伪造 window 参数无效及原件/退出保护；剪贴板插件 ACL 和完整原件恢复也通过。 | 不能推广为全部会话/资源权限、MCP iframe、物理交互或其他平台已验收；`2cbed72bb` 的 42 阶段曾在最小化前提停止；后续诊断包 `e3e030586` 的完整 46 阶段已通过，仍不推断旧失败原因。 |
| D：窗口与多显示器 | 保存普通窗口位置和显示器缩放，最大化/最小化不覆盖普通尺寸；按当前工作区限制恢复位置，移除外接屏后回到主屏。状态文件原子替换，兼容旧尺寸文件。窗口几何回归通过；真实 macOS `.app` 原生 API 已验证窗口隐藏/最小化恢复、最大化退出后重启与取消最大化、普通尺寸/位置重启恢复。私有包已通过实际最小化按钮、Cmd+M 及原生 Settings 点击恢复窗口/渲染设置页。 | 2026-10-02 外接屏已恢复可用，原生 API 识别两块 2× 屏幕；当前 `37674eea7` 实际副屏定位/普通重启及跨屏恢复/再次重启两种档案 8/8 通过，不同缩放/拔插/物理拖动未标记通过；历史诊断包 `e3e030586` 的 46 阶段曾全过，`30a93592d` 为 31/46；`5f153aae7` 为 8/46、`67e7fcd8b` 为 31/46；新包 `6d405f8f8` 的完整 46 项及两种档案原生菜单曾通过，但追加原生菜单/原 API 首个托管启动均再次失败。两次外观请求都已跳过写入、恢复请求/完成各 1、Reopen=0，仍无 Will/Did Miniaturize 或 Did Deminiaturize。最新 `67c36323c` 的 4 个 API 启动样本及两种原生菜单曾通过，但完整门禁首项失败；追加只读模态/调整诊断的 `ada308408` 为 23/46，显式首项失败，实际启动已完成且无模态/sheet/实时调整。2026-10-02 `851111594`、`7a83505ce` 及 `1b6a7706d` 生产包的独立完整门禁各 46/46 曾通过；`c0e3871c5` 完整门禁曾为 19/46；`b96367355` 及最近完整门禁的 `f43825cc9` 在两块屏幕已连接后仍失败：托管前 8 项通过，随后设置最小化前提超时，显式档案未开始，见下方记录。历史及当前偶发失败根因仍未确定，本轮上一候选和修复后 DMG 临时安装包的完整门禁分别 46/46 通过，新安装包双屏恢复 8/8 通过；历史偶发失败根因仍未解决，窗口稳定性未结案，未放宽断言。 |
| D：原生窗口外观 | macOS 保存/读取外观偏好及启动恢复同步到 Tauri 原生应用主题；串行处理避免旧读取覆盖新外观，保存拒绝时不修改原生主题。真实 AppKit dark/light/auto 与整包重启、非法主题/样式保持不变的验收通过。已修复首次 native 失败回滚将未配置外观变为显式 auto 的问题；鉴权 CAS 回滚保留未配置/旧样式并拒绝过期写入。真实 Go/AppKit 故障注入及不完整原生回滚后的整包启动恢复通过。设置页深色模式实际点击、选中状态及整窗深色渲染已通过。修复 Vite glob 编译条件后，真实 `de8afeeec` 包的两种档案均显示八套官方主题和图片，托管实际官方主题选择与背景预览已通过。`9972828f4` 托管普通档案的双图片主题导入、重复副本、无效包原件保护及两份主题重启预览通过；显式历史捕捉失败保留记录；当前 `5882a249d` 显式普通档案已补齐五阶段、双图片与两次正常退出，见下方记录。 | 原生标题栏细节、系统明暗切换、真实系统/磁盘错误导致的失败路径仍待执行；端口错误注入不等同于系统真实拒绝。 |
| D：托盘与退出 | 托盘有显示/退出菜单；主点击已对齐 Wails 始终打开/恢复窗口，移除可见时反向隐藏；托盘、Dock 重开及单实例唤起共用主线程恢复入口，补上 macOS 应用取消隐藏。退出沿用 supervisor 停止路径。真实包原生 CloseRequested 验证关闭后继续运行、关闭即退出、偏好重启恢复与 sidecar 清理；应用隐藏/取消隐藏也已验证。实际 Go 流式任务在关闭后继续产生事件并完成历史保存；原生 Show 菜单恢复几何，Quit 菜单在第二个任务运行中退出并清理上游及 sidecar。CUA 原生 Cmd+Q 和设置关闭即退出后实际窗口关闭按钮均已验证退出码 0、sidecar/readiness 清理。 | 真实托盘点击、人工物理按键/托盘退出及 Dock 重开仍待验收。程序化原生菜单动作不能代替按键/点击；新包两种档案的第二实例严格焦点门禁已通过；实际 Dock/托盘交互仍待验收。 |
| D：对话框与链接 | 现有 Tauri 选择器保留；共享外部链接及本地文档 adapter 已接入 Rust host。Markdown 默认打开、定位、另存为及指定已安装应用均走原生入口，错误不退回 browser mock。文档可执行目标拒绝、特殊路径、取消保存、源文件别名保护及权限拒绝已有回归。外部链接支持 HTTP(S)/受限 mailto，OAuth 入口仅接受 HTTP(S)。真实 macOS 包的主 WKWebView IPC 非法 URL 拒绝及两个原生入口打开默认浏览器、本机页面请求和退出清理已在默认/显式私有档案通过；Ghostty 专用入口已补上系统启动器退出码/超时反馈，实际不可启动私有应用的主 IPC 错误及原件保护门禁通过；系统 Terminal 的目录/文档入口已在真实包两种档案核对两个新会话的实际 kernel cwd 和清理。 | macOS 已增加系统应用注册查询、Spotlight 自定义安装位置和原生 64×64 图标；项目会话顶部选择器已接入配置偏好与卸载回退。Linux 已增加 XDG desktop entry 发现、GIO 原生启动、六类终端目录策略和有界 PNG 图标转换，共用逻辑回归通过；Linux 原生分支编译/图标/GUI 验收待执行。Windows 已增加 App Paths、安装目录/Toolbox 发现、终端目录策略和原生 PNG 图标代码；共用逻辑与 Win32 API 类型检查通过，Windows 原生 host 编译/注册表/图标/GUI 验收待执行；Global 会话已接入档案内稳定目录与会话身份查询。实际 AppKit 目录选择已验证中文/空格路径、默认工作区/项目显示及重启恢复，实际文件选择已验证文本附件显示为待发送。两种普通私有档案的诊断导出实际 Save/Cancel/Replace 面板已通过：中文/空格文件名、新保存、取消覆盖原件保护及确认覆盖；新包 `de8afeeec` 的两种普通档案已通过实际创建/编辑无图片主题、导出 Save/Cancel/Replace、旧包字节/权限/mtime/inode 保护及正常退出清理；`9972828f4` 托管普通档案的实际主题 Cancel/双图片导入/重复导入/无效包/重启及两次正常退出通过；显式历史捕捉失败保留记录，当前 `5882a249d` 已补齐五阶段、双图片重启及两次正常退出。当前 `851111594` 普通包两种私有档案的本地文档另存为实际 Cancel/Save/Cancel Replace/Replace 已完成，共两次正常生命周期/八项文件检查；原件和副本取消保护通过。当前 `5882a249d` 两种档案的同源/缺失源拒绝、mode-000 源实际读取拒绝及中文解决步骤已实际通过；0500 目的地真实写入拒绝与零输出/原件保护两档案通过，新简明提示原生复验待完成；实际应用主题、文档源目录拒绝及新写入拒绝提示原生复验、浏览器/邮件、指定应用的真实 UI 交互及 OAuth 仍需验收。 |
| D：通知与钥匙串 | macOS 通知改为原生 UserNotifications：读取实际授权、报告发送失败、点击恢复对应会话；冷启动队列、档案隔离、失效会话与重复点击已有回归。Linux 已接入 XDG 服务/能力查询、实际发送与运行中点击；独立真实 D-Bus 联调通过，授权无标准查询时报告 unknown。凭据按持久档案身份隔离；设置页显式迁移旧 Preview 凭据，保留原件并拒绝覆盖。迁移/保存/删除与重启串行，写入及桥接同步失败回滚；macOS 原生隔离读写、真实 bridge 迁移/重启/删除及不落盘回归通过。干净包 `67e7fcd8b` 两种私有档案的 6 次实际 OS 送达、固定标题/正文和精确通知清理通过，其中 2 次实际 active、4 次应用已隐藏；正常退出和同档案重启保持。 | 真实系统通知授权拒绝、横幅视觉与前后台/冷启动实际点击、前端事件/设置的完整 UI 链路、钥匙串锁定/授权拒绝、原生设置页迁移操作仍待验收。Linux native host、桌面环境/Wayland 焦点及冷启动点击待验收/补齐；Windows 原生授权读取/点击和 Windows/Linux 凭据后端仍待补齐/验收。 |
| D：界面存储档案隔离 | macOS 生产包使用持久档案身份绑定 UI origin，防止 WKWebView 默认数据仓库在不同 HOME/core 间混用默认工作区和界面偏好；同档案重启和目录迁移保留 origin。干净包 `673c30023` 的托管/显式档案共 18 个真实 WKWebView 阶段通过，含实际产品工作区渲染恢复、两个档案交替重启及测试值清理。 | 已提供设置→存储与路径中的显式旧偏好预览/确认导入/撤回；22 项固定 UI key，完整回退记录先于写入，后续修改通过值比较保留。真实旧 origin 只读 IPC、重复窗口销毁和实际预览 UI 已通过；新增只在实际非持久 WKWebView 中写入三项固定偏好的私有来源，`f72a9b7a2` 两种档案共 4 次启动/8 次精确读取通过。导入/重挂载/撤回与写入失败组件回归已过；`ada308408` 包托管/显式各三次真实 UI 生命周期已完成，工作区、深入模式和较大字号实际应用、撤回后重启恢复及记录消失均由 CUA 验证，六次来源/身份/原件/退出检查通过。其余偏好及修改/冲突/失败的实际 UI 路径待验收，不推及所有 22 项。开发 URL 不变，范围为 macOS 生产包。 |
| D：单实例与数据保护 | Tauri 单实例及独立默认 Preview 数据目录已存在。当前源码构建的 Wails 与 bridge 启动均持有配置/状态两处目录锁；共享任一目录都会拒绝第二个写入宿主，目录别名去重，失败释放已取锁。真实 bridge/Wails 拒绝启动测试及配置原件/备份回退回归通过。导入页在操作前展示来源、目标目录与回退说明；配置备份根拒绝符号链接/非目录，新根与批次为 0700，当前 `05cf57e1d` 三阶段真实包导入/回退通过；真实包宿主入口的配置/项目目录导入、Preview 修改/重启、原件及备份保护和显式目录拒绝导入通过；经官方发布 SHA-256 和干净基线提交核对的原生 arm64 CLI 1.38.3，已通过导入/重启后的原目录回读及原件保护验收；官方旧 Wails GUI 的真实历史恢复、会话 writer 租约和原生退出回退已通过；本轮进一步通过官方旧 CLI 的私有 Global 文本/图片引用解析、实际文件编辑与检查点保护、Preview 退出后续写和最新检查点实际代码恢复。 | 未参与目录锁协议的旧稳定版仍需兼容性验收；不能将当前两个宿主的测试推广为所有历史二进制互斥。当前回退夹具覆盖单份 Global 历史、文本/图片附件字节和两个实际编辑检查点；旧版最新检查点恢复后，连续向更早检查点回滚因文件冲突拒绝，未标记该操作通过。不能推广为 Preview 已迁移旧 Global 数据、图片渲染/像素送模型、全部历史配置/附件/检查点或宿主目录生命周期互斥已通过。真实 Wails 单实例通知/唤起、设置页导入点击与完整会话/数据回退操作仍待验收。 |
| D：强制宿主退出 | macOS bridge 从 token 握手前跟随实际父进程，宿主丢失时停止自己的任务/HTTP/SSE，处理断开日志管道的 SIGPIPE。宿主先发布 0600 启动记录，bridge 仅清理匹配记录/instance ID 和同 inode 的空私有目录。最近完成生命周期复验的包 `af7cf185d` 已通过启动 12 项、就绪后空闲/实际流式 8 项共 20 项通过（历史 `30a93592d` 同门禁也通过）；kernel 确认 sidecar 正常退出码 0，实际上游断开、原件保护与同档案重启通过。 | 覆盖 sidecar 已启动后的三个启动边界及就绪后状态；不能推广为宿主尚未 spawn 子进程时的所有临时目录回收。历史 `1b6a7706d` 完整窗口门禁 46/46 通过，`c0e3871c5` 为 19/46；最近完整门禁 `f43825cc9` 托管 8 项后在 Settings 最小化前提失败，显式未开始；`63984aca9` 的页面完成后独立最小化也失败，后续候选至 `ad3eb7070` 未复跑完整窗口。历史 Settings/最小化及焦点失败根因仍待定位；异常退出和程序化窗口门禁不代替全部物理 UI 验收。 |
| E：remote host / bot / updater / 管理页 | remote host 与 bot 已有部分设置/bridge 接口；updater 插件已注册。macOS 菜单改为“Updates…”说明入口，如实提示 Preview 尚无更新检查并给出手动下载地址，移除没有实现依据的“启动时自动检查”文案。 | D 验收后对照 Wails 逐项审计和补齐；当前更新入口仍是说明对话框，插件注册不能视为更新流程完成。 |

累计门禁：`pnpm test:clipboard`、输入框剪贴板回归、terminal selection、`pnpm test:tauri`、`pnpm build`，以及使用真实 Go bridge 的 Rust 测试（最新 XDG 通知切片 189 项通过、2 项默认忽略；其中 1 项独立真实 D-Bus 联调已显式通过，另有前轮 1 项显式 macOS 原生钥匙串测试通过）。最新 XDG 通知切片已从干净提交 `f8aba01793814b7119c4de7829bf747e42a94d53` 通过 `pnpm tauri:build -- --bundles app` 构建与本地 ad-hoc 签名；`tools/tauri/smoke-packaged-app.py` 在临时 HOME 分别验证默认和显式数据目录、私有凭据身份、真实 macOS 通知授权查询、实际 Global 工作区解析与私有目录权限、sidecar 就绪、未认证请求拒绝以及退出无残留。此 smoke 没有执行菜单/托盘等 UI 点击；两次桌面自动化分别超时和报 ScreenCaptureKit `SCStreamErrorDomain -3811`，所以真实 UI 验收保留待办。本地 ad-hoc 签名不是正式发布签名/公证。

剪贴板插件接入与文本权限参考 [Tauri 官方文档](https://v2.tauri.app/plugin/clipboard/)。

#### D：默认档案链接拒绝与正常启动失败退出（2026-10-02）

- 源码与私有真实安装包均确认：旧 `create_dir_all(reasonix-core)` 会跟随默认根的符号链接，链接到旧 `.reasonix` 后可修改其配置并生成 Preview 凭据身份。确定性回归旧逻辑失败，未操作用户旧档案。现改为私有叶目录创建及不跟随链接的普通目录检查，拒绝链接/悬空链接/普通文件；新根为 0700，已有正常档案内容保留，显式 `REASONIX_HOME` 语义未变。
- 首个拒绝包保护了原件，但将 Err 返回给 Tauri setup 后仍 panic；仅改 build 错误处理的中间包门禁明确报 exit=-6、expectedMessage=true，未记为通过。已核对当前 Tauri 运行阶段的 setup Err panic 路径，最终在已知档案拒绝时、WebView/sidecar 创建前输出恢复说明并 exit=1。其他 setup 失败没有据此全部结案。
- 档案 16 项专项、使用当前包真实 bridge 的完整 Rust 回归（218 通过、2 按原规则忽略）、严格 all-targets clippy 和完整 `.app`/DMG 构建均通过。最终 DMG 校验、只读挂载及临时安装后，三种非法根均正常拒绝，原件字节/mode/mtime/inode/链接目标保持，无 sidecar/readiness 残留；两档案正常包级启动/退出、3 阶段配置导入/恢复/显式拒绝以及 12 项启动中断清理/重启通过。签名为 ad-hoc，未公证。
- 中间包另有完整窗口 46/46 和宿主异常退出 8/8 通过，其摘要与日志独立归属，不转记到最终包。本轮原生 UI 连接再次超时，实际交互未新增验收；默认叶目录检查不是任意外部目录替换的生命周期锁，也不能证明显式共享根、旧无锁宿主或全量数据回退安全。D 未结案，E、A/B/C 和正式发布门禁保持。
- 源补丁、runner 快照、包摘要、失败过程和最终安装门禁见 [目录边界验证证据](evidence/2026-10-02-d-managed-root/README.md) 与 [机器可读结果](evidence/2026-10-02-d-managed-root/result.json)。未新增延期确认、发布或默认下载切换。

#### D：宿主事件收敛、状态竞态修复与 DMG 安装复验（2026-10-02）

- 对照 Wails 1.38.3 的共享关闭提示，确认较早 `GetDesktopShellStatus` 查询可覆盖更晚的托盘事件；专项在旧代码确定失败，现改为先订阅、事件到达后拒绝旧查询结果。设置菜单和关闭提示的组件 `window.runtime.EventsOn` 直连已移入 `bridge.ts` 两个固定宿主事件接口，解除订阅幂等且排队回调失效。Tauri 独立入口仍使用已有 `host:open-settings`；没有修改 Rust 最小化行为、注入 Wails shim 或扩大权限。
- 源码专项、桌面 `test:app-lifecycle`、完整测试类型检查和生产前端门禁通过。测试类型检查发现既有原生快捷键 JSON 可选 modifier 的 unknown 类型，改为严格布尔比较后通过，未改变生产快捷键或门禁断言。使用项目要求的本地缓存 pnpm 10.34.5 构建完整 `.app` 与 DMG；签名为 ad-hoc，严格校验通过，未公证。源码尚未提交，源 HEAD 和补丁 SHA-256 已归档。
- DMG 经校验、只读挂载、复制至私有临时安装目录和严格签名校验后卸载；安装后 host/sidecar SHA-256 与本轮构建一致。安装包七组门禁均 exit 0：两种档案的包级隔离/退出、完整窗口各 23/23（含原生编辑、剪贴板最小权限、取消面板、第二实例焦点及流式任务退出）、宿主 SIGTERM/SIGKILL 空闲/运行中共 8/8、两块 2× 屏幕的副屏/跨屏与重启 8/8、配置导入/恢复/显式拒绝 3 阶段、WKWebView 外链拒绝与浏览器回执、6 次系统通知送达及精确清理。通知两档案 initial active 均 false，不算前台或横幅/点击验收；未传旧 CLI，不算旧 GUI/全量数据回退。
- 未改产品的上一候选在本轮亦完整通过 46 项窗口门禁及独立 8 项菜单/剪贴板/编辑/取消面板。更早同一候选曾托管 23/23 后显式首项最小化失败，日志已保留；本轮成功没有解释历史偶发失败，不认定 Preview 稳定性已结案。Wails 1.38.3 基线没有当前 `profilegate` 协议，不能将新宿主锁测试推广为任意旧版显式共享目录互斥。
- 原生 UI 工具三次连接均 30 秒超时并重置。实际托盘/Dock、IME/自定义组合、通知拒绝/点击、钥匙串拒绝/设置迁移、不同缩放/实际拔插及其他物理门禁继续待验收；已请求用户决定是否延期，未收到确认前不扩大延期范围。D 未结案，E 继续等待 D 验收和 Preview 稳定；A/B/C、全量数据回退、Developer ID/公证与发布授权缺口保留。
- 可审阅的基线说明、门禁范围、A/B/C/E 与其他发布阻塞、源补丁、原生/构建日志及包摘要已保存在 [本轮验证证据](evidence/2026-10-02-d-installed-host-events/README.md) 和 [机器可读结果](evidence/2026-10-02-d-installed-host-events/result.json)。本轮没有发布或切换默认下载项。

#### D：当前候选完整源码回归汇总（2026-10-02）

- 本轮开始 HEAD `5fe64f202` 为证据提交，实际候选生产提交 `ad3eb7070`；没有产品源码改动或重打包。使用该 `.app` 内的 `reasonix-desktop-bridge` 显式设置 `REASONIX_TAURI_BRIDGE_TEST_BIN`，运行完整 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml -- --test-threads=1`，**215 通过、0 失败、2 默认忽略**，日志 `/private/tmp/reasonix-current-candidate-rust-full.log`。
- 已核对输出，真实 bridge 启动/停止、会话目录/游标/顺序同步、Global/project/重启工作区解析、钥匙串迁移/保存/重启/删除和单 Provider 恢复失败隔离、通知真实会话路由与删除目标拒绝确实执行。受控 bundled child 回执、窗口状态/跨屏算法、源文件替换保护、通知队首溢出及权限/备份/资源作用域回归也纳入同次完整 suite。macOS Finder/Terminal 原生 catalog 和 64×64 PNG 图标读取检查通过，但没有打开这些应用或执行真实用户点击。
- 两项忽略分别是显式 macOS 私有服务钥匙串 smoke 和独立 D-Bus broker smoke。前者最近在 `253abf30a` 来源的独立测试已实际通过，证据保留该轮归属，不把本次忽略写成通过；后者按用户 Windows/Linux 延期范围保留，不能视为 Linux 平台验收。显式传入包内 bridge 后的真实 integration 通过不推广为普通 Preview UI 已通过。
- 同一最终工作树完整 `pnpm test:tauri` **exit 0**，日志 `/private/tmp/reasonix-current-candidate-tauri-full.log`；包括设置/Provider/钥匙串反馈、Hooks 和存储剪贴板竞态、消息/导航、通知生命周期、工作区 opener 与 external-links 全部现有回归。该 suite 使用 adapter/stub/组件环境，不代替物理按键、OS 弹窗和当前包实际菜单/托盘/通知点击。
- 测后只读进程核对仍仅普通 Preview host/sidecar 原 PID 16939/16944，没有额外测试 sidecar；没有操作/关闭普通窗口。当前候选完整 macOS 构建与签名证据仍为 `/private/tmp/reasonix-notification-head-build.log`（ad-hoc、未公证）。完整窗口最近仍在 Settings 最小化前提失败；当前包尚未复跑该门禁和相关原生 UI。D 未结案，E 保持 D 验收后推进；旧版共享目录互斥决策、B 图片/历史、C 工具生命周期及正式 Developer ID/公证/发布授权门禁保留。

#### D：通知容量溢出时保护正在处理的队首（2026-10-02）

- 本轮先追踪生产窗口事件/restore/show 调用：Moved/Resized 仅 capture，窗口状态 restore 仅 setup；未找到抵消最小化请求的具体生产恢复事件证据，也未放宽最小化门禁。普通 Preview 原进程仍运行，未操作/关闭它。
- 进一步核对 native 通知 registry 发现两个消费者竞态：32 条 pending 满时新点击 pop_front，可移除前端已开始导航但尚未 ack 的队首；256 条 target 满时发送新通知 remove(0)，可移除同一队首的会话映射。两项确定性专项在旧代码分别失败：队首被替换及 pending 变空，日志 `/private/tmp/reasonix-notification-head-before.log`、`/private/tmp/reasonix-notification-target-head-before.log`。均使用私有模拟 backend，不发送大量 OS 通知。
- 产品 `ad3eb7070` 在 pending 容量满时保留有效队首并淘汰最旧等待项；先清除过期/映射已淘汰的 pending，避免不可见项占有队首。target 容量满时保留首个有效 pending 的映射，淘汰其他最旧 target。32/256 上限、TTL、档案身份、token 校验、持久 ack 及失败保留规则不变，不放宽任意 JS token 的 acknowledge 权限。依赖已有单 consumer 每次 fresh-head 处理约定；超量等待项仍可按有界策略淘汰，不承诺所有超量点击永不丢失。
- 最终 9 项 Rust 通知回归全部通过，日志 `/private/tmp/reasonix-notification-head-final-tests.log`。新场景确认满队列队首与最新项保留、32 上限、原队首能 ack 并完成剩余 drain；target 溢出下保持映射/可 ack，持久目标仍恰为 MAX_TARGETS。现有授权拒绝不反复提示、冷启动/档案隔离/durable ack、损坏/过期/权限失败保护及真实当前包 bridge 会话路由/不重建已删除会话回归保留。前端通知点击回归 exit 0，日志 `/private/tmp/reasonix-notification-head-frontend.log`，包括前轮 32 条批次中补入第 33 条的交接场景。
- 严格 all-targets clippy 和完整 macOS 候选构建 exit 0，日志 `/private/tmp/reasonix-notification-head-clippy.log`、`/private/tmp/reasonix-notification-head-build.log`；前端类型/587-method 契约/体积、Go sidecar/arm64 host/ad-hoc 签名通过。本轮未运行新包 native delivered/UI 点击门禁；真实横幅/前后台和冷启动点击/OS 拒绝恢复、最小化/其他物理 D、旧版目录互斥、A/B/C/E 与正式签名/公证/发布门禁保持。

#### D：另存为期间源路径替换的原件保护（2026-10-02）

- `save_local_path_as` 原先在打开系统面板前保留源 File；保存时仅比较目标与旧句柄 inode。面板期间源路径被 rename/替换后，新源 inode 与旧句柄不同，选择原源路径可错误覆盖新原件。临时目录的确定性回归旧逻辑失败（本应拒绝却返回成功），日志 `/private/tmp/reasonix-save-source-before.log`，没有操作用户文件或系统面板。
- 产品 `033803e32` 将初始解析的源路径保留至复制事务，在复制前及临时文件 sync 后/最终 persist 前核对源路径仍指向已打开句柄；替换、删除或无法确认身份均拒绝，提示重新打开文档再保存。取消仍直接返回空结果；原有 hardlink/symlink 源别名拒绝、目录拒绝、权限复制和原子替换保持。此为两个边界上的身份校验，不是外部程序写入锁，不承诺同 inode 内容并发修改的一致快照或消除最终校验与 rename 之间所有外部竞态。
- 12 项 Rust local_paths 回归通过，日志 `/private/tmp/reasonix-save-source-after.log`；新增场景核对取消无写入、替换后原源与另一已有目标均拒绝并保留字节、无临时文件残留、重新打开新源后正常保存。严格 all-targets clippy exit 0，日志 `/private/tmp/reasonix-save-source-clippy.log`。
- shared Markdown Save As 把固定 source-changed 错误与源不可读错误映射为三语言“重新打开并检查读取权限”的恢复提示，不展示临时路径；不增加权限或回退 transport。首轮 external-links 回归因旧文案断言失败，更新到当前恢复语义后完整 `pnpm test:external-links` exit 0，日志 `/private/tmp/reasonix-save-source-final-links.log`，含新增错误映射及已有原生拒绝/零 Wails fallback、偏好并发保护。
- 完整 macOS 候选构建 exit 0，类型/587-method 契约/体积门禁（未改预算）、Go sidecar/arm64 host/ad-hoc 签名通过，日志 `/private/tmp/reasonix-save-source-build.log`。本轮未操作仍运行的普通 Preview，未执行新包的实际 Save/Replace 面板或物理源替换复验；历史面板结果保留各自提交归属。D 窗口/托盘/通知/钥匙串物理门禁、旧版目录互斥、A/B/C/E 与正式签名/公证/发布门禁继续待完成。

#### D：普通 Preview 运行期间的独立钥匙串复验（2026-10-02）

- 当前 HEAD `8c4a43296`（证据提交），候选产品 `253abf30a`。普通 host/sidecar PID 仍为 16939/16944，故本轮不启动同 bundle 的包级 UI 夹具，也未操作或关闭普通窗口。当前 `.app` 内实际 `reasonix-desktop-bridge` 作为 `REASONIX_TAURI_BRIDGE_TEST_BIN`，运行 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml keychain::tests:: -- --test-threads=1`，**22 通过、1 原生测试按默认忽略**；日志 `/private/tmp/reasonix-current-keychain-transactions.log`。
- 其中两项真实 bridge 测试确实执行：迁移/保存/重启/删除的私有配置原件与密钥不落盘，以及单个 credential read 失败后继续恢复其余条目。后端模拟专项覆盖不确定写入/删除先回滚再同步、桥接响应失败恢复两边状态、迁移失败移除目标并保留来源、symlink/超量/异常条目拒绝和命名 Provider 限制。模拟拒绝不记为真实 OS 拒绝。
- 随后单独显式运行 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml keychain::tests::native_keychain_profile_isolation_and_cleanup -- --ignored --exact --test-threads=1`，**1/1 通过**；日志 `/private/tmp/reasonix-current-keychain-native.log`。实际 macOS backend 以临时目录生成两份随机 service 身份和固定假值，验证缺失、写入、读回、跨档案不可见、替换、精确删除和重复删除返回缺失；原生断言及 Drop 仅清理这两份自有服务的固定测试 key，不访问真实用户现有条目。
- 测后只读进程核对仍仅普通 host/sidecar 原 PID，未发现额外测试 bridge。独立测试进程/临时 Provider 不等于打包应用设置页实际迁移、原生权限弹窗、系统钥匙串锁定或拒绝/重新授权通过；这些物理门禁、完整窗口稳定性、其余 D/A/B/C/E/正式发布门禁保持。目标继续进行，不因普通窗口运行把整个目标标记 blocked。

#### D：钥匙串失败反馈与恢复步骤（2026-10-02）

- 实际 ProviderSettings 的 showStatus 原先给所有结果安装 2 秒清除计时器，保存/删除/迁移拒绝和“写入成功但 summary 刷新失败”的恢复提示均会消失。受控时钟专项回归旧代码确定失败（tick 2001 后保存失败不再可见），日志 `/private/tmp/reasonix-keychain-feedback-before.log`。
- 产品 `247212b09` 仅普通成功反馈按原 2 秒到期；错误/部分成功恢复提示保留至下一次显式操作或页面卸载。失败状态 role=alert，成功 role=status；新操作仍清除旧反馈/计时器。三语言补充钥匙串解锁/应用访问权限/重试步骤；保存失败使用专用 Preview key，不修改 Wails 共享保存失败文案。summary 刷新失败保留已保存/已删除/未找到的真实结果，并提示关闭、重开设置刷新；已核对生产 settingsOpen 条件确实卸载/重建设置页。原始错误与凭据仍不进入反馈，现有请求锁、后端钥匙串事务和权限不变。
- 专项 adapter 回归通过：普通成功仍过期且 role=status；保存失败及部分成功刷新失败超过 2 秒仍可见，失败 role=alert；下一次迁移操作替换旧保存失败；原有拒绝/草稿保留/不泄露诊断、重复保存/迁移互斥及删除 .env 后仍 configured 等断言保留。日志 `/private/tmp/reasonix-keychain-feedback-after.log`；完整 `pnpm test:tauri` exit 0，日志 `/private/tmp/reasonix-keychain-feedback-tauri-tests.log`。
- 首次包构建在繁体 locale 预算失败：81206 gzip bytes 超现有 81203.2 上限，日志 `/private/tmp/reasonix-keychain-feedback-build.log`。`253abf30a` 精简重复繁体措辞但保留解决步骤，新 chunk 为 81197 bytes，未调整预算。最终完整 macOS 构建 exit 0，类型/587-method 契约/体积、Go sidecar/arm64 host/ad-hoc 签名通过，日志 `/private/tmp/reasonix-keychain-feedback-final-build.log`。
- 普通 Preview 仍存活，本轮没有操作、关闭或锁定用户钥匙串，也未启动私有 native smoke。该组件回归不代替真实钥匙串锁定/OS 授权拒绝/迁移点击验收；新包原生/物理 D、最小化稳定性、A/B/C/E 与正式签名/公证/发布门禁继续保留。

#### D：托盘主点击对齐稳定宿主（2026-10-02）

- 对照当前 Wails `desktop/tray.go` 的 `SetOnTapped` 与 `desktop/tray_common.go::showFromTray`：主点击始终走打开/恢复入口，secondary click 留给系统菜单。Tauri 原主点击却按 visible/minimized 切换，窗口已可见时 hide，存在迁移行为差异；仅凭窗口 visible 也不能表达完整应用隐藏/恢复语义。不宣称已物理复现 macOS app-hidden 下的具体错误。
- 产品 `55fd3a7d7` 将原有 Left/Up 点击分支直接接到 `show_main_window`，与 Show 菜单、Dock 和第二实例共用主线程应用 unhide、unminimize、show、focus。已显示窗口点击也恢复/聚焦，不再隐藏；右键/其他事件过滤、托盘 Quit、关闭偏好与 sidecar 停止路径保留，没有新增权限或 renderer 命令。
- 严格 `cargo clippy --locked --manifest-path desktop/tauri/Cargo.toml --all-targets -- -D warnings` exit 0，日志 `/private/tmp/reasonix-tray-open-clippy.log`；完整 macOS 候选构建 exit 0，前端类型/587-method 契约/体积、Go sidecar/arm64 host 和 ad-hoc 签名通过，日志 `/private/tmp/reasonix-tray-open-build.log`。
- 普通非测试 Preview 实例仍在运行，已请求用户保存工作后退出；没有操作或关闭该普通实例，也没有启动依赖同标识空闲的私有包门禁。源码基线对照/编译不代替实际托盘 Left/Up、右键 Show/Quit、应用隐藏/最小化恢复或 Dock 点击通过。新包原生 smoke 与物理验收继续待执行，旧包证据保留各自提交归属。
- D 尚未结案，E 仍在 D 验收后推进；最小化稳定性、其他原生交互、旧版共享目录互斥、A/B/C、正式签名/公证及发布授权门禁保留。

#### D：Hooks 工作区切换的异步请求归属修复（2026-10-02）

- 核对实际 Hooks 设置消费者：原先只阻止旧剪贴板结果写文本/反馈，工作区变化后 busy 仍由旧请求占有；save/apply/manual reload 没有一致的上下文归属。专项回归在旧代码确定失败：新工作区编辑器仍 disabled，日志 `/private/tmp/reasonix-hooks-request-before.log`。
- 产品 `3fc6eb16e` 在 scope/workspaceRoot 提交时使旧请求失效、释放旧上下文 busy、清除旧视图/反馈与 pendingApply；加载使用统一 latest-request fence，复制/粘贴/保存/应用使用操作代次，各自仅释放所属 busy。手动刷新也同步持有 busy，避免刷新时进入并发编辑/剪贴板/保存；卸载使旧结果失效。旧保存结果不能替换新工作区 view/text/revision/path，也不能显示成功或解除新请求的锁。已进入原生或后端的操作仍可在原上下文完成，不宣称取消系统剪贴板写入或后端保存/应用。
- 专项 adapter 回归通过：旧 paste 完成不改新 draft、不解除新 paste busy；旧 save 完成不改新 view/path/draft、不显示旧保存成功、不解除新 clipboard busy；旧 manual reload 不覆盖最新工作区加载。原生 JSON Copy/Paste、拒绝/空值草稿保护和零 browser fallback 原断言保留。日志 `/private/tmp/reasonix-hooks-request-after.log`。最终源码完整 `pnpm test:tauri` exit 0，日志 `/private/tmp/reasonix-hooks-request-tauri-final-tests.log`。
- 完整 macOS 候选构建 exit 0，类型/587-method 契约/体积门禁、Go sidecar/arm64 host 与 ad-hoc 签名通过，日志 `/private/tmp/reasonix-hooks-request-build.log`。普通非测试 Preview host/sidecar 仍存活，本轮没有终止或操作它，也未启动需要同标识空闲的私有原生验收。因此本次组件竞态回归不等于实际 Hooks 物理 UI 或新包 native smoke 通过；前轮包级结果保留对应产品提交，不转记为此包通过。
- D 继续进行：完整窗口最小化稳定性、托盘/Dock、实际通知点击/拒绝与钥匙串交互、旧版共享目录互斥决策及其他物理项未结案；B 图片/旧历史、C 工具生命周期、E 和正式 Developer ID/公证/发布授权门禁保留。

#### D：当前候选的宿主生命周期与存储复制 UI 复验（2026-10-02）

- 同一产品 `af7cf185d` 生产包（本轮开始 HEAD `8d6c06b85` 为证据提交）重新执行独立宿主丢失验收 **20/20 通过**。`smoke-host-lifetime.py` 两种私有档案 × TERM/KILL × 空闲/真实流式共 8 项，日志 `/private/tmp/reasonix-current-candidate-host-lifetime.log`；`smoke-native-startup.py` 两种档案 × TERM/KILL × 父进程检查前/token 前/token 后 ready 前共 12 项，日志 `/private/tmp/reasonix-current-candidate-startup-lifetime.log`。kernel sidecar 回执均为正常退出码 0；实际上游断开、目录锁/启动记录清理、原件保护及同档案重启通过。只终止已核实的自有宿主，不扩大为 spawn 前目录回收或完整窗口通过。
- 当前候选托管普通私有档案完成真实 CUA 设置→存储操作：空工作区复制禁用，配置目录按钮实际点击后显示“已复制”；只读 claim 核对系统精确私有路径及代次，返回输入框以 Cmd+V 粘贴相同路径，Cmd+A/BackSpace 清空且发送禁用，最后实际 Cmd+Q。runner exit 0，正常 host/sidecar/ready 清理、档案身份和原件保护通过；原剪贴板逐项/类型/顺序/字节独立恢复确认。日志 `/private/tmp/reasonix-current-candidate-ui-second-observation.log`；成功私有夹具及控制文件已删除。未发送消息，不将托管结果扩大为当前候选显式档案或真实拒绝反馈通过。
- 首个私有 UI 夹具在绑定/观察期间到达 60 秒期限，runner exit 1，未复制或输入，不记通过；日志 `/private/tmp/reasonix-current-candidate-ui-observation.log`。后续先核对新自有 PID，再用完整应用路径绑定；Raise 后获得完整私有页面。bundle ID 在不同 checkout 的应用间存在歧义，不能用它独立证明实例归属。
- 第二夹具 Cmd+Q 后误执行已退出绑定的只读 AX 查询，返回不同 origin 的普通档案页面；进程核对发现新的普通 Preview host/sidecar，已经停止输入，也未关闭该普通实例。不将它作为测试残留终止，亦不声称全局无 Preview 进程；当前仅确认自有夹具退出/删除。后续严格遵守退出后仅查 runner/进程回执、不查询已退出应用绑定；每次输入前核对自有存活 PID 与私有 origin，无法确认即停止。
- 连接双屏恢复 8/8 的独立证据保留；最小化稳定性、混合缩放/实际拔插/物理拖动、其余 D、A/B/C/E、正式签名/公证和正式发布门禁继续待完成。

#### D：当前包与官方旧版的私有回退复验（2026-10-02）

- 前轮产品 `af7cf185d` 已完成完整 macOS 构建，HEAD `15c1d6249` 仅追加证据。当前同一 `.app` 本轮执行 `smoke-legacy-rollback.py --workspace-data`，仍用固定摘要核对官方 arm64 Desktop/CLI 1.38.3 三个二进制、bundle 身份和旧/新严格签名；全部数据在生成的私有 HOME/core/缓存，provider 仅固定 loopback、本轮随机凭据不放 argv/URL，不访问真实用户 profile。
- **全部规定阶段通过**，日志 `/private/tmp/reasonix-current-candidate-legacy-rollback.log`：旧版 GUI 前后两次启动保持原历史、拒绝同会话并发 CLI writer且无 provider 请求、真实原生 quit 正常；Preview 实际配置 import/restore/explicit 三阶段，私有文件/配置备份/身份与原树保护通过；旧版实际 Global 文件/文本+图片引用及附件字节、首个编辑检查点保持，回退后真实续聊生成第二个回复/检查点。最新检查点实际恢复 preimage；更早检查点因冲突拒绝，不记为早期代码回退成功；transcript/附件保持且回退不调用 provider。
- 独立确认 Preview 与官方旧版宿主均退出，成功私有夹具 `/private/tmp/reasonix-official-wails-history-smfomu9o` 已删除。此结果验证当前包的代表性旧版回退/原件保护，不是 Preview 已迁移旧完整历史、图片像素已传模型、所有旧字段或设置页物理点击通过，也不证明不参与新目录锁的旧客户端可并发共享目录。
- 当前构建契约仍实际核对 587 个 Go 方法；源码复核 Tauri 消息直接使用共享 Markdown，而 `hasMarkdownImageResolver` 仍仅识别 Wails binding，故本地/旧 Global 图片 resolver 缺口保留。Tauri 消息工作区的 terminal 引用为主题偏好，不是嵌入 shell/terminal、Browser/worktree 生命周期入口；A/B/C 与 D/E 完成、真实资料迁移、正式 Developer ID 签名/公证及正式发布授权均未据此放行。

#### D：通知点击队列补入后的批次交接修复（2026-10-02）

- 核对原生 registry：32 是同时排队上限，处理点击期间可补入新 token；前端却在累计确认 32 条后直接停止。实际模拟中首条确认腾出位置后补入第 33 个点击并调用 consumer.wake()，随后 fresh-head 查询消费了 coalesced wake 标志；首批结束仍留有已收到点击，没有后续唤醒。专项回归确定失败（32 次 ack、期望 33），日志 `/private/tmp/reasonix-notification-batch-before.log`；整个队列始终不超过 32，不是虚构超界输入。
- consumer 现在统计成功确认数；完整确认一批后继续一次 fresh query，处理补入的点击，直到空队列/不可导航/停止/故障。原生队列上限、权限、目标身份、单 consumer、失败保留和停止规则未改；故障后的 coalesced wake 仍不会造成自动重试环。专项回归验证 33 次打开和唯一确认、最终空队列及总计 34 次 pending 查询（最后一次为空），也保留已存在的空快照竞争、停止、导航拒绝、目标不匹配、失败 ack/查询无重试环回归，日志 `/private/tmp/reasonix-notification-batch-after.log`。
- 完整 `pnpm test:tauri` exit 0，日志 `/private/tmp/reasonix-notification-batch-tauri-tests.log`；随后专项增加打开次数断言再次通过。产品提交 `af7cf185d` 完整 macOS 构建通过（类型/contract/budget、Go sidecar/arm64 host 与本地 ad-hoc 签名），日志 `/private/tmp/reasonix-notification-batch-build.log`。
- 当前包两种私有档案的实际 macOS 通知提交/系统 delivered 查询全部通过：每种 3 条固定文案，其中 2 条应用隐藏时发送，精确删除本次通知、原件保持、同档案重启、身份及正常退出/sidecar/启动记录清理；日志 `/private/tmp/reasonix-notification-batch-native-delivery.log`，独立确认无 Preview 残留。该 native 门禁不运行用户点击 pump，不将实际送达等同于批次 UI 集成通过。
- 批次回归为依赖层模拟；通知横幅、实际前后台/冷启动点击、授权拒绝恢复仍待物理验收。最小化稳定性、不同缩放/拔插/物理拖动、旧版目录互斥决策及其余 D/A/B/C/E/正式发布门禁保留。

#### D：存储路径复制的重复请求与来源反馈修复（2026-10-02）

- 当前桌面 inventory 再次 10 秒超时，没有执行物理 UI。转而核对实际 Tauri 存储页：复制没有同步请求锁；来源变化/刷新后，旧异步完成仍可设置同一行的已复制图标或错误。新增回归在旧代码确定失败，连续两次同一事件批次点击产生两次 native 写入（期望一次），日志 `/private/tmp/reasonix-storage-copy-before.log`。
- 存储复制现在用同步请求锁和请求代次控制忙碌状态；当前工作区、默认工作区、存储快照/语言变更、刷新或卸载均使旧反馈失效。旧完成不能标记新路径成功、显示旧错误或解除新请求忙碌；已经进入 native 的写入无法撤销，不宣称取消了系统写入。复制错误与加载/选择错误分别保存，旧复制完成不能清除刷新失败。保留原生文本 helper、当前标题/ARIA 和行布局，不新增权限或 browser/Wails 回退。
- 专项 adapter 回归通过，覆盖原生正确路径、拒绝后清除成功/标准解决提示/不泄露原始拒绝、同步重复点击一次写入、旧来源完成不标记新来源/不解除新忙碌、刷新失败保留与零浏览器回退；日志 `/private/tmp/reasonix-storage-copy-after.log`。该回归已加入 `pnpm test:tauri`，完整 Tauri 回归和共享 native/browser/Wails 剪贴板回归 exit 0，日志 `/private/tmp/reasonix-storage-copy-tauri-tests.log`、`/private/tmp/reasonix-storage-copy-clipboard-tests.log`。
- 产品提交 `a8b06f7ea` 完整 macOS 构建通过，日志 `/private/tmp/reasonix-storage-copy-build.log`（类型/contract/budget、Go sidecar/arm64 host 与本地 ad-hoc 签名）。当前包两种私有档案的独立原生菜单/剪贴板 **4/4 通过**，实际 WKWebView 文本读写、主窗口图片拒绝/未授权同源窗口读写拒绝、私有身份/正常退出与完整原剪贴板恢复通过，日志 `/private/tmp/reasonix-storage-copy-native-clipboard.log`；独立确认无 Preview 残留。
- 专项组件测试为 adapter 模拟，包门禁验证实际 native ACL/IPC，均不代替存储页物理点击、真实拒绝 UI 或完整窗口验收。不同缩放/拔插/物理拖动、最小化稳定性、其他物理 D、旧版目录互斥决策、A/B/C/E 与正式发布门禁保持；前轮跨屏修复 8/8 独立证据保留，Windows/Linux 继续延期。

#### D：跨屏窗口恢复选错主屏的产品修复（2026-10-02）

- 发现生产 `restored_bounds` 按显示器顺序选择首个可见标题栏区域。主屏优先排序导致大部分标题栏在副屏、少部分伸入主屏的窗口被选到主屏，之后边界约束把它移回主屏。负坐标跨屏回归先在旧逻辑失败（实际 x=0、期望副屏约束后的 x=-1280），日志 `/private/tmp/reasonix-display-overlap-before.log`。
- 修复只改变目标屏幕选择：在可达标题栏候选中选择可见标题栏最多的区域，同分保持主屏优先；保留现有完整窗口工作区约束、逻辑尺寸缩放、缺屏主屏回退及旧文件兼容，不新增权限/字段或原生恢复重试。8 项窗口状态回归全部通过，含负坐标跨屏、不同缩放目标选择、同分、断屏极端坐标、旧状态/无效状态，日志 `/private/tmp/reasonix-display-overlap-after.log`；严格 clippy 通过 `/private/tmp/reasonix-display-overlap-clippy.log`。
- `smoke-native-displays.py --straddle` 在生成的私有档案完成普通副屏定位/重启后，仅把本夹具保存的几何改为跨屏状态；要求真实生产恢复回到副屏的边界约束位置，核对保存文件，再整包重启复验。此为保存状态故障场景，不是物理拖动。修复前 `63984aca9` 真实包复现：savedX=-1400、期望副屏 x=-2000，实际主屏 x=0；日志 `/private/tmp/reasonix-display-overlap-package-before.log`，保留私有现场 `/private/tmp/reasonix-native-displays-4f52zd4u`，无 Preview/sidecar/ready/launch-owner 残留。
- 产品提交 `37674eea7` 完整 macOS 构建通过（前端类型/contract/budget、sidecar/arm64 host、本地 ad-hoc 签名），日志 `/private/tmp/reasonix-display-overlap-build.log`。新包实际双屏两种档案 **8/8 全部通过**：普通副屏定位/重启 + 跨屏恢复/再次重启，各 4 项；实际副屏 x=-2000、保存文件/尺寸/2× 缩放/身份/宿主退出码与 sidecar/启动清理一致，日志 `/private/tmp/reasonix-display-overlap-package-after.log`。成功夹具删除，独立确认无 Preview 残留。
- 当前两块屏幕均 2×；不同缩放只有源码回归，实际拔插、混合缩放和物理拖动仍待验。此修复不涉及最小化路径，也没有复跑完整窗口门禁；最小化/API 与原生菜单失败记录、物理 UI、其余 D/A/B/C/E 和正式发布门禁继续保持。

#### D：页面初始化完成后的最小化对照（2026-10-02）

- 前轮请求最小化时 mainPageFinished=false，虽然 key/active 为 true，仍无法排除 WKWebView 初始化时序。Settings 验收入口现在先等待可信主页面真实 PageLoad::Finished，使用已有 5 秒等待、原有 key/active 前提及实际最小化断言；没有产品窗口行为改动、无额外 sleep/重试，不将等待本身记作产品修复。
- `63984aca9` 严格 clippy 与完整 macOS 构建通过，日志 `/private/tmp/reasonix-settings-page-ready-clippy.log`、`/private/tmp/reasonix-settings-page-ready-build.log`。在新生成私有档案仅复制前轮私有普通几何、不复制凭据，单独复验此前失败的 API 最小化 + Settings 恢复阶段；实际 host/sidecar/身份/鉴权检查通过后，**最小化仍失败**，未进行恢复动作。日志 `/private/tmp/reasonix-settings-page-ready-probe.log`，保留现场 `/private/tmp/reasonix-settings-page-ready-iv986zw2`。
- 请求前 mainPageFinished/applicationActive/nativeKeyWindow/focused 均 true；最终仍 minimized=false、will/did-miniaturize=0，窗口失去 key。因而页面未加载完成也不能单独解释失败，不认定系统/产品根因，不重复运行已被该阶段阻断的完整门禁。当前包只进行了上述独立失败切片，最近完整运行仍为 `f43825cc9` 托管 8 项后失败、显式未开始。
- 独立确认无 Preview/sidecar/ready/launch-owner 残留。追加源码核对：Tauri 专属前端无直接 navigator.clipboard/window.runtime/window.go 调用；搜索命中的旧 SettingsPanel/StorageSettingsPage、未引用 InlineDiff 不是当前 Tauri 入口证据，本轮未改这些非当前路径。其他应用最小化对照仍待用户答复；物理 UI、窗口稳定性、其余 D/A/B/C/E 与发布门禁继续保留。

#### D：最小化前的活动 key window 前提与原生菜单对照（2026-10-02）

- 核对锁定的 Tauri runtime/Tao 源码：`minimize()` 在 UI 线程调用 AppKit `miniaturize:`；原生 Minimize 菜单发送 `performMiniaturize:`。现有测试在可见、非最小化、非隐藏后就请求最小化，未等待实际活动 key window。新增严格的活动应用 + 主 key window 等待，使用已有 5 秒上限，不增加重试/延时、不改变产品窗口恢复实现；固定 trace 增加 `settings-minimize-ready`，最多 5 行且总量仍为 16 KiB。五阶段字段/数量边界检查通过。
- `f43825cc9` 严格 clippy 与完整 macOS 构建通过，日志 `/private/tmp/reasonix-settings-key-precondition-clippy.log`、`/private/tmp/reasonix-settings-key-precondition-build.log`。新包完整 `--dialogs --edit --focus` **仍失败**：托管前 8 项通过，第 9 项设置最小化超时，显式档案未开始。日志 `/private/tmp/reasonix-settings-key-precondition-full-window.log`，保留私有现场 `/private/tmp/reasonix-native-window-smoke-n7beuomk`。
- 新 `settings-minimize-ready` 确认请求前 applicationActive/nativeKeyWindow/focused 均 true、策略 Regular=0；最终窗口失去 key，但未收到 will/did-miniaturize，minimized=false。因此等待 key 前提本身未解决问题，不能将此前失败归因于“请求时未激活”。新前提保留为更严格的验收要求，不记作产品修复。
- 同一包另在新生成私有档案中复制上述私有普通几何（不复制凭据），运行已安装的真实 Minimize 原生菜单 + Settings 恢复阶段；要求活动 key window、实际菜单 enabled/原生 role 和原生事件。对照 **仍在最小化前提失败**，日志 `/private/tmp/reasonix-settings-native-role-probe.log`，现场 `/private/tmp/reasonix-settings-native-role-eix20zxg`。此结果表明失败不限于 Tauri `minimize()` API；不改用菜单成功假设绕过完整门禁。
- 只读进程检查 Dock、WindowServer、loginwindow 各存在一个，不能据此认定桌面正常或证明系统/产品根因。已请求用户提供其他应用黄色按钮/Cmd+M 最小化对照，未答复不推断。独立确认两份失败现场无 Preview/sidecar/ready/launch-owner 残留，保留失败证据。副屏 4/4 为前轮独立结果；完整窗口稳定性、物理 UI、其余 D/A/B/C/E 和正式发布门禁保持。

#### D：外接屏恢复与真实副屏重启验收（2026-10-02）

- 用户确认外接屏已打开，本轮恢复连接屏幕的原生验收。桌面观察工具两次 inventory 超时，仍无法执行物理 UI；早期 `system_profiler` 仅返回 GPU，未用其认定无屏幕。新包原生 API 实际识别到两块 2× 屏幕，工作区分别为 `(0,60,3840,1966)` 和 `(-3840,60,3840,2100)`。只读诊断增加固定 activation-policy 数值及最多 8 个屏幕工作区，不含名称/标识符或任意环境内容；解析器拒绝无效缩放、过多屏幕和未知字段。
- `b96367355` 严格 clippy、字段边界检查及完整 macOS 构建通过，日志 `/private/tmp/reasonix-monitor-policy-clippy.log`、`/private/tmp/reasonix-monitor-policy-build.log`。新的 LaunchServices 私有编辑探测通过，日志 `/private/tmp/reasonix-monitor-policy-launch-probe.log`：实际运行策略均 Regular=0，应用激活/key window/可见遮挡状态为 true，编辑回执通过、原剪贴板恢复。此为当前独立结果，不证明历史失败根因或整体稳定性已经解决。
- 同一包完整 `--dialogs --edit --focus` 复测仍 **失败**：托管前 8 项通过，随后 `menu-settings-minimized` 的最小化前提超时；显式档案尚未开始，编辑和第二实例焦点未在该完整运行中执行。日志 `/private/tmp/reasonix-external-display-full-window.log`，保留私有现场 `/private/tmp/reasonix-native-window-smoke-0xl3zri4`；独立确认无 Preview/sidecar/ready/launch-owner 残留。无放宽断言或把独立编辑结果折算为完整通过。
- 新增 opt-in 副屏定位/重启阶段及 `smoke-native-displays.py`，选真实非主屏工作区居中定位，核对实际物理位置/尺寸/缩放、生产保存文件和整包重启恢复，复用私有身份、鉴权拒绝、真实宿主退出码及 sidecar 清理检查。没有 renderer command 或产品恢复逻辑改动。`f51979947` clippy 及完整构建通过，日志 `/private/tmp/reasonix-secondary-display-clippy.log`、`/private/tmp/reasonix-secondary-display-build.log`；真实包两种私有档案 **4/4 全部通过**，日志 `/private/tmp/reasonix-secondary-display-package-smoke.log`，成功夹具已删除。
- 本轮只验原生 API 副屏几何和重启，不等于物理拖动、WebView 渲染、不同缩放或实际拔插验收。上述项、完整窗口稳定性、其余物理 UI、历史旧版互斥、A/B/C、D→E 与正式发布门禁保持；Windows/Linux 继续延期，外接屏可用范围已恢复。

#### D：LaunchServices 桌面启动的编辑激活对照（2026-10-02）

- 新增 `probe-edit-launch-services.py` 独立诊断：用 `/usr/bin/open -n -W` 启动同一 `ad0d62200` macOS 包，显式固定生成的 HOME/config/state/cache 四处私有目录；从私有 launch-owner + kernel 进程关系确认宿主和 sidecar，核对私有凭据身份、未鉴权拒绝和空 token，不打印环境。复用严格原生编辑探测及多格式剪贴板保护，失败仅清理确认自有宿主，在恢复剪贴板前等待自有进程停止；保留失败现场。`open -W` 不是实际宿主 exit code，该诊断不会替代原完整门禁。
- 第一次探测在复用正常 smoke 的“不可有 state override”前提处停止，日志 `/private/tmp/reasonix-edit-launch-services-probe.log`；这是探测前提不匹配，不计产品/激活验收。诊断随后单独核对有意设置的私有 state 路径，未修改正常 smoke 的断言或任何产品代码。
- 最终对照 **失败**，日志 `/private/tmp/reasonix-edit-launch-services-final-probe.log`，私有现场 `/private/tmp/reasonix-edit-launch-services-tct95jkg`。实际 LaunchServices host/sidecar 归属、profile/token 和 readiness 检查后，原生编辑仍出现与直接启动相同的状态：启动/page 完成、可 key/活动 Space、无模态/sheet/实时调整，恢复请求/完成 1/1，无 became-key，applicationActive/keyWindow/occlusion-visible 为 false。因此不能将缺少直接启动的 LaunchServices 上下文作为唯一解释；系统/产品根因仍未证明。
- 两次桌面探测原剪贴板格式核对恢复，独立确认没有 Preview/sidecar/ready/launch-owner 残留。诊断脚本语法及 diff 检查通过；当前包完整窗口 19/46 和原生编辑焦点失败保持，未放宽激活前提、改为后台编辑或用程序化回执冒充物理 UI。D→E、实际 UI、旧版目录互斥与正式发布门禁继续保留。
- 追加静态核对当前包 Info.plist：LSUIElement、LSBackgroundOnly、NSPrincipalClass 均未设置，CFBundleExecutable 为 reasonix-tauri；仓库没有显式 activation-policy 设置。这不证明运行期激活策略或桌面状态正常。已请求用户确认当前会话是否登录解锁且其他应用可正常前台，未收到答复前不据此认定环境原因。

#### D：当前包完整窗口门禁的原生编辑焦点失败（2026-10-02）

- 当前产品 `c0e3871c5` 完整 `--dialogs --edit --focus` 在托管档案第 20 阶段 menu-editing 停止，**19/46** 完成，显式档案未开始；日志 `/private/tmp/reasonix-sidecar-terminal-full-window.log`。设置隐藏/最小化/应用隐藏恢复、后台任务菜单退出、外观重启回滚、剪贴板和对话框取消等前置通过不等于完整窗口通过，也不能用此前 `1b6a7706d` 的 46/46 替代当前包。
- 原生编辑前提的实际错误为 firstResponderAccepted=true、keyWindow=false、applicationActive=false、windowVisible=true、applicationHidden=false；不归因为应用崩溃、屏幕锁定或权限。原剪贴板格式核对恢复，失败私有现场 `/private/tmp/reasonix-native-window-smoke-ox67kbni` 保留。
- `ad0d62200` 只增加 edit focus 的恢复前/恢复后/超时只读 AppKit 状态快照及 runner 固定字段解析，提供启动完成、模态/sheet、活动 Space、occlusion 和原生转换记录；不改变激活调用、5 秒前提、编辑动作或断言。严格 clippy 与三条固定状态 parser 检查通过（额外诊断字段不会输出），日志 `/private/tmp/reasonix-edit-focus-trace-clippy.log`。下一次针对性探测用于读取新增证据，不把重跑通过当成旧失败已修复。
- 该诊断提交完整 macOS 构建通过，日志 `/private/tmp/reasonix-edit-focus-trace-build.log`。新私有托管档案 `--independent --edit --profile managed` **2/3，编辑焦点失败**，日志 `/private/tmp/reasonix-edit-focus-trace-probe.log`，现场 `/private/tmp/reasonix-native-window-smoke-s5c7ojyd` 保留。三份快照均表明实际启动/page 已完成、窗口可 key/位于活动 Space、应用仍运行且未隐藏，没有 modal/sheet/live resize；恢复请求/完成由 0/0 变为 1/1，became-key/resigned-key 为 0，应用和窗口 occlusion-visible 为 false，激活与 key window 始终 false。该证据不能确定系统/产品根因，不据此认定锁屏、权限问题或原生激活 API 无效。
- 两次探测原剪贴板格式都已核对恢复；独立确认两份失败现场没有 Preview/sidecar/ready/launch-owner 残留。当前完整 19/46 与诊断切片 2/3 均为失败记录；新增诊断不是产品修复，下一步需核对实际桌面 LaunchServices 启动的激活上下文，完整窗口/编辑/第二实例焦点、全部物理 UI 和 D→E 门禁继续保留。

#### D：sidecar 退出回执与日志错误分离（2026-10-02）

- 核对当前锁定版本 `tauri-plugin-shell 2.3.6` 的事件定义与 pipe reader，`CommandEvent::Error` 同时用于等待或 stdout/stderr 读取错误，并不证明子进程已退出。原 bundled monitor 收到 Error 或通道关闭均把 running 标志清零，可能过早放弃现有子进程状态。事件回归修复前 **1 失败、1 通过**，日志 `/private/tmp/reasonix-sidecar-terminal-before.log`；不计为原生 OS 故障验收。
- 产品 `c0e3871c5` 仅在明确 `Terminated` 回执后清零；读取错误或通道关闭无回执时保留当前子进程所有权，不输出底层诊断，不因不确定证据放行另一次 spawn。实际句柄的 shutdown/kill 路径、父进程监督与目录锁保持。三项事件序列回归通过（错误后无回执、错误后真实终止事件、无回执关闭通道），日志 `/private/tmp/reasonix-sidecar-terminal-after.log`；严格 `cargo clippy --all-targets -- -D warnings` 通过，日志 `/private/tmp/reasonix-sidecar-terminal-clippy.log`。
- 同提交完整 macOS 包构建、托管/显式双档案正常启动与退出 smoke 通过，日志 `/private/tmp/reasonix-sidecar-terminal-build.log`、`/private/tmp/reasonix-sidecar-terminal-package-smoke.log`。同包宿主 TERM/KILL × idle/streaming × 两档案 **8/8 通过**，kernel 实际确认八次 sidecar 正常退出码 0，就绪/启动记录和目录锁清理、原件保护及同档案重启通过，日志 `/private/tmp/reasonix-sidecar-terminal-host-lifetime.log`；独立确认无 Preview 残留。受控 shell Error 序列不是实际 OS pipe 故障注入，基础/异常退出 smoke 不代替全部物理菜单/托盘或当前包完整窗口验收；其余 D/E、旧版互斥和正式发布门禁保留。

#### D：消息复制失败与异步内容边界（2026-10-02）

- Tauri 消息直接复用 `CopyButton`，原生剪贴板入口已接入，但写入拒绝被静默忽略，且前次成功状态仍可能显示。新增回归修复前失败：第二次失败后 copied class 仍为 true，日志 `/private/tmp/reasonix-copy-button-before.log`。
- 产品 `79e3adac6` 在每次复制开始时清除前次成功、等待期间禁用并合并重复点击；失败显示三语“重试/手动选择复制”toast 和可访问提示，不显示底层错误。来源 text/getText 替换或卸载使旧请求失效：延迟生成的旧文本不写剪贴板，已提交的旧写入返回不标记新内容成功，旧请求不释放新请求状态。成功定时器随来源变化/卸载清理；不增加权限或原生拒绝后的浏览器/Wails 回退。
- `pnpm test:clipboard` 通过，覆盖前次成功后失败清除、实际 clipboard-manager invoke 拒绝提示、待处理双击只写一次、内容替换 fence、过期异步生成零写入和 fallback 零调用，日志 `/private/tmp/reasonix-copy-button-after.log`；完整 `pnpm test:tauri`（含共享代码/图表/外链与工作区回归）通过，日志 `/private/tmp/reasonix-copy-button-tauri-tests.log`。这不等于 macOS 消息按钮物理点击、OS 实际拒绝或整项 D 已验收；这些待办继续保留。
- 核对独立 Wails 1.38.3 基线的 CopyButton，保留其浏览器开发/Wails 回退需求；当前共享 clipboard 回归仍验证这些宿主分支，Tauri 拒绝继续终止。当前产品完整 macOS 构建通过（类型/contract/budget、sidecar、arm64 宿主和本地 ad-hoc 签名），日志 `/private/tmp/reasonix-copy-button-build.log`；不是正式签名/公证。
- 同包双档案独立菜单/剪贴板门禁 **4/4 通过**，包含实际原生文本读写、最小权限拒绝、原剪贴板恢复和退出清理，日志 `/private/tmp/reasonix-copy-button-native-clipboard.log`；独立确认无 Preview 残留。这是宿主切片，不是当前包完整窗口或实际消息 UI 操作通过；D/E、旧版目录互斥及其他发布门禁继续保留。

#### D：组合输入与配置快捷键边界（2026-10-02）

- 核对发现消息输入框已有 IME 保护，但 Tauri 全局快捷键匹配、面板 Escape 和设置页录制未检查组合输入。新增匹配回归修复前明确失败：`isComposing` 的 new_session 组合仍返回 true，日志 `/private/tmp/reasonix-shortcut-ime-before.log`。额外独立工作区样本的修复前挂载停滞并终止，不计通过，也不将停滞归因为产品；之后将面板检查放回原有工作区回归夹具。
- 产品 `c07a78ac3` 共用 `isComposing || keyCode === 229` 检查，覆盖匹配、全局面板 Escape、快捷键录制和原输入框入口。IME 事件保持未 preventDefault，不触发应用动作、不结束录制或覆盖保存绑定；不修改系统输入法、保留组合表或稳定版偏好。完整 `pnpm test:tauri` 通过，覆盖 43 个动作两种 IME 标记、真实设置组件录制与普通组合键保存、工作区面板保持和普通设置路由，日志 `/private/tmp/reasonix-shortcut-ime-tauri-tests.log`。
- 原生观察工具本轮 `cua.getState()` 只读查询 10 秒超时并重置 kernel，未绑定/启动 Preview、未发送输入。它不证明用户输入法故障，不能替代 macOS 实际候选输入/取消或自定义物理组合键验收；这些 D 门禁及 D→E 顺序继续保留。
- 同提交完整 macOS 包构建和托管/显式两档案基础 smoke 通过：类型/contract/budget、私有凭据身份、实际通知授权查询、Global 工作区、sidecar readiness/未认证拒绝及退出清理；独立确认无 Preview 残留。日志 `/private/tmp/reasonix-shortcut-ime-build.log`、`/private/tmp/reasonix-shortcut-ime-package-smoke.log`。本地 ad-hoc 签名不等于正式签名/公证，此 smoke 不发送 IME 或快捷键，不把当前包基础检查推广为完整窗口/物理路由已通过。

#### D：通知点击查询与新事件交错（2026-10-02）

- 前端 `NotificationClickPump` 查询期间收到真实点击唤醒，但在途查询返回旧空快照时，原逻辑因没有 acknowledge 进展而丢弃唤醒；点击仍留在原生队列，却可能直到下一次 readiness 改变才处理。确定性回归在修复前失败（实际打开 0 次、预期 1 次），日志 `/private/tmp/reasonix-notification-wake-before.log`。
- 产品 `2e4837c26` 保留查询期间收到的新唤醒，空快照或 readiness 检查提前返回后仍发起一次新查询；桥接/存储异常继续等待下一次真实唤醒，不形成失败重试循环。原有单消费者、卸载 fence、拒绝导航不 ack、冷启动队列和持久 ack 规则保持。新增竞态及故障不循环回归通过，日志 `/private/tmp/reasonix-notification-wake-after.log`；完整 `pnpm test:tauri` 通过，日志 `/private/tmp/reasonix-notification-wake-tauri-tests.log`。
- 同提交完整 macOS 生产包构建通过（类型/contract/budget、sidecar、arm64 宿主和本地 ad-hoc 签名），日志 `/private/tmp/reasonix-notification-wake-build.log`。同包托管/显式两档案基础 smoke 通过：私有凭据身份、真实通知授权查询、Global 工作区、sidecar readiness/未认证请求拒绝及退出清理，日志 `/private/tmp/reasonix-notification-wake-package-smoke.log`；独立确认无 Preview 残留。此确定性前端竞态证据不等于真实通知横幅/点击/冷启动或 OS 拒绝恢复已验收；D/E、旧版目录互斥及正式发布门禁继续保留。

#### D：Provider 凭据部分恢复失败隔离（2026-10-02）

- 对照 Wails 基线按 Provider 解析凭据的流程，Tauri 原生凭据恢复遇到任一 load/sync 错误即停止；有效的后续条目也不会恢复。新增真实 Go sidecar 回归修复前失败：第一个故障条目后的有效第二 Provider 仍未配置，日志 `/private/tmp/reasonix-keychain-read-failure-before.log`。较早同一用例先暴露了故障样本原始错误返回，日志 `/private/tmp/reasonix-keychain-partial-before.log`；调整断言顺序明确捕获主恢复缺口，不计失败为通过。
- 产品 `e8bf42227` 在凭据锁内继续恢复其他 Provider，记录部分读取/同步失败，结束时返回固定恢复指引；不返回名称、密钥或 backend 诊断，不将部分完成报告为完整成功，不改变原生存储或迁移/保存/删除回滚规则。启动保持有效 Provider 可用，重启仍向 UI 返回部分恢复失败；恢复不新增 renderer 命令、capability、持久密钥缓存或日志。
- 真实 sidecar + 受控故障 credential backend 验证单条目失败后其他 Provider 配置成功、整体仍返回失败、故障解除后恢复全部、再次重启部分失败仍保持 sidecar 和有效 Provider 可用、原凭据/配置不变及档案全文件树没有假密钥。钥匙串相关回归 **21 通过、1 个原生 OS 测试按原计划默认忽略**，日志 `/private/tmp/reasonix-keychain-partial-restart-tests.log`；严格 clippy 通过，日志 `/private/tmp/reasonix-keychain-partial-clippy.log`。这不是 macOS 实际锁定/拒绝 UI 的验收，该物理门禁继续保留。
- 追加核对全局身份损坏：`ebae594f4` 给不可用 backend 增加明确 availability 分流，保持原有“恢复元数据备份/检查档案权限”提示，恢复函数在该情况下不访问 sidecar。新增未启动 supervisor 的回归要求原指引精确返回，不把全局损坏误报为普通部分失败；最终相关回归 **22 通过、1 默认忽略**、严格 clippy 通过，日志 `/private/tmp/reasonix-keychain-partial-final-tests.log`、`/private/tmp/reasonix-keychain-partial-final-clippy.log`。第一版 `e8bf42227` 的完整构建与双档案包级启动 smoke 已通过，日志 `/private/tmp/reasonix-keychain-partial-build.log`、`/private/tmp/reasonix-keychain-partial-package-smoke.log`，不以它替代最终版本的包级结果。
- 最终 `ebae594f4` 完整生产构建通过：类型/contract/budget、Go sidecar/arm64 宿主及本地 ad-hoc 签名，日志 `/private/tmp/reasonix-keychain-partial-final-build.log`。同包托管/显式两档案基础 smoke 均通过：私有凭据身份、通知授权查询、Global 工作区、sidecar readiness/未认证拒绝及退出清理，日志 `/private/tmp/reasonix-keychain-partial-final-package-smoke.log`；独立确认无 Preview 残留。故障恢复由真实 sidecar 与受控 backend 回归证明，包级 smoke 不制造真实钥匙串锁定/拒绝；实际 OS 异常/迁移 UI、窗口稳定性、历史旧版互斥及其余 D/A/B/C/E、正式签名/公证继续待验。

#### D：当前生产包 UI 观察复验（2026-10-02）

- 同一包完整 `smoke-native-window.py --dialogs --edit --focus` **46/46 通过**，两种档案各 23 项，日志 `/private/tmp/reasonix-mermaid-full-window.log`。严格最小化恢复、几何/外观重启和回滚、后台实际流式任务、原生编辑/剪贴板原件恢复、面板取消、第二实例焦点与关闭/退出清理均通过，独立确认无 Preview 实例残留。断言保持，未重试失败阶段；历史偶发最小化根因仍未确定，不把本次通过当作稳定性修复或物理 UI 已验。
- 当前生产包仍来自 `1b6a7706d`，没有修改或重建产品。本轮以新普通托管档案补验 `document-write-denied`：确认自有宿主 87115/直属 sidecar 87123 存活后首次绑定和同宿主只读重新绑定均报 ScreenCaptureKit `-3811`。没有输入/保存操作或 UI 回执，不能把新中文提示记为验收通过。
- 只向确认身份的自有宿主发送 SIGTERM；runner 终止 exit 1 来自测试主动停止。独立确认两个 PID 均退出，ready/launch-owner 文件清理、目标目录元数据保持且零文件、两个源文件字节/权限/mtime/inode 保持。失败现场 `/private/tmp/reasonix-native-ui-export-76c5b7370b9041a0945cb9b9cc5be28a`、控制 `/private/tmp/reasonix-write-denied-mermaid-control.json`、日志 `/private/tmp/reasonix-write-denied-mermaid-ui.log` 保留；该停止不是正常 Cmd+Q 验收或应用自行崩溃。
- 迁移清单仍保留消息/Hooks/诊断/来源/图表的真实 UI、IME/自定义快捷键、托盘/Dock、Finder/editor/mail/OAuth、通知点击/权限恢复、钥匙串拒绝/迁移、完整历史数据回退与未参与锁协议的旧版共享目录并发。E 必须等 D 验收且 Preview 稳定，A/B/C 图片及会话/数据缺口、正式签名/公证继续阻碍发布；只有 Windows/Linux 与外接屏按用户既有指示延期。

#### D：Mermaid 链接鼠标动作与原生拒绝恢复（2026-10-02）

- 核对 Wails 1.38.3 与当前 Mermaid 消费者：SVG `onAuxClick` 不区分中键/右键，右键释放会调用外部打开；原生封装返回 false 后没有用户反馈。新增真实组件回归在修复前失败，右键实际调用数为 1 而非 0，日志 `/private/tmp/reasonix-mermaid-native-before.log`。
- 产品 `1b6a7706d` 将 auxclick 限定为中键，保留左键/中键默认导航阻止和原生打开；内部片段/过滤后的危险协议不送入 opener。打开拒绝使用现有三语错误与“复制链接”恢复，复制复用共享文本入口并报告实际成败，原生拒绝不使用浏览器回退，也不暴露内部错误。Toast 更新复用同一个 innerHTML 对象，保留 SVG DOM 与缩放状态；不变更协议允许范围或 capability。
- 新真实组件/native adapter 回归覆盖右键零打开、左/中键 IPC 参数、片段与危险来源拒绝、打开失败反馈、原生复制恢复成功/拒绝、浏览器 transport 零调用和失败反馈时原 SVG 节点保持连接。新定向回归、原 Mermaid 渲染 **105/105** 与完整外链回归通过，日志 `/private/tmp/reasonix-mermaid-native-after.log`、`/private/tmp/reasonix-mermaid-rendering-tests.log`、`/private/tmp/reasonix-mermaid-external-tests.log`；新测试纳入 `test:external-links`，因此也由 `test:tauri` 调用。该组件证据不是实际消息中图表的物理点击验收。
- `1b6a7706d` 完整生产构建通过，类型/contract/budget、sidecar/arm64 宿主与本地 ad-hoc 签名通过，日志 `/private/tmp/reasonix-mermaid-native-build.log`。新包双档案实际 WKWebView 原生打开、非法协议/userinfo/mail 参数拒绝、默认浏览器本机页面回执及退出清理通过，日志 `/private/tmp/reasonix-mermaid-native-links.log`。该原生链路与上述组件分别验证；不扩大为图表物理点击、邮件客户端或 OAuth 完整交互通过。
- 同包双档案独立菜单/原生剪贴板 **4/4 通过**，主窗口文本读写/图片拒绝、同源受限 WebView 文本拒绝、系统剪贴板完整原件恢复、身份保持及退出清理通过，日志 `/private/tmp/reasonix-mermaid-native-clipboard.log`；独立确认无 Preview 实例残留。完整窗口稳定性和物理 UI 仍待验，历史共享目录并发的延期选择尚无答复，未自行减少 D/E 目标范围。

#### D：官方旧版目录锁负向探测（2026-10-02）

- 新增 `tools/tauri/probe-legacy-profile-gate.py`，只允许固定已核验 SHA-256 的官方 arm64 1.38.3 CLI（独立发行或 Desktop 内附），不启动用户档案、Wails GUI 或网络 provider。生成私有 HOME/config/state/cache，持有 config/state 两处 `.reasonix-session-profile.lock` 的 macOS 排他 flock，并以独立文件描述符确认两处均拒绝第二个锁参与者。
- 官方 Desktop 内附 CLI 在两锁持续持有期间，`config currency USD` **仍退出 0，并把生成的 CNY 配置实际修改为 USD**；探测按设计 exit 1，日志 `/private/tmp/reasonix-legacy-profile-gate-negative.log`。这证明旧 CLI 配置写入忽略新增宿主目录锁，属于互斥失败证据，不是通过。生成夹具由 finally 清理，未修改旧二进制、用户配置或原验收断言。
- 当前 Wails/bridge 的两目录生命周期锁与旧会话租约不能据此推广为历史宿主已互斥；该探测未执行 GUI 生命周期或会话/附件写入，也不保证整个旧目录的一致快照。不能只新增锁文件就补上未参与协议的历史程序；历史共享目录并发、旧版停机后的完整回退仍需按各自范围验收。保持默认 Preview 独立目录，不提前放行 D/E 或正式发布。
- 本轮通过 CUA 重新枚举桌面，`getState` 30 秒观察超时并重置会话，没有启动或操作 Preview，也没有推断用户锁屏/权限根因；生产包仍为 `81b229d4a`，画面观察与真实 UI 验收继续保留。

#### D：诊断报告原生复制与异步反馈（2026-10-02）

- 对照 Wails 1.38.3，诊断页复制仍直接调用 `navigator.clipboard.writeText`，失败写入诊断加载错误。该组件由 Tauri 设置页实际消费。新增真实组件回归在修复前失败：WebView 拒绝时原生剪贴板没有收到报告，日志 `/private/tmp/reasonix-diagnostics-copy-before.log`；未使用的 `InlineDiff` 不作为当前 Tauri 入口完成证据。
- 产品 `81b229d4a` 将诊断 JSON 复制接入共享 `writeClipboardText`，成功反馈等待实际原生结果；失败清除旧成功提示并显示三语“无法复制诊断报告/稍后重试”，复制错误与报告加载错误独立。pending 禁止重复复制；刷新或切换 runtime 选项使旧请求失效，旧完成不得标记新报告已复制，卸载使请求失效并清理反馈计时器。保留原报告规范化/脱敏流程，没有新增自动读取或 capability。
- 新真实组件/native adapter 回归覆盖浏览器拒绝时原生写入、原生 busy 后成功反馈清除与内部错误隐藏、pending 禁用、刷新前旧复制完成拒绝以及独立加载错误，浏览器写入调用数为 0。已纳入完整 `pnpm test:tauri`；新定向、原诊断页测试和完整 Tauri 回归均通过，日志 `/private/tmp/reasonix-diagnostics-copy-after.log`、`/private/tmp/reasonix-diagnostics-existing-tests.log`、`/private/tmp/reasonix-diagnostics-tauri-tests.log`。组件样本不等同真实设置页点击或系统剪贴板验收。
- `81b229d4a` 的完整生产构建通过，类型/contract/budget、sidecar/arm64 宿主及本地 ad-hoc 签名通过，三语文案未增加预算，日志 `/private/tmp/reasonix-diagnostics-native-build.log`。新包双档案独立菜单/原生剪贴板 **4/4 通过**，主窗口文本读写、图片拒绝及无权限同源 WebView 文本拒绝、系统剪贴板原件恢复、身份/退出清理通过，日志 `/private/tmp/reasonix-diagnostics-native-clipboard.log`；独立确认无 Preview 实例残留。原生链路和组件消费者分别验证，未扩大为诊断设置页物理点击或完整窗口稳定性已验；D/E 和正式签名/公证继续待验，用户明确延期项保持。

#### D：搜索来源原生打开与复制（2026-10-02）

- 对照 Wails 1.38.3 的 `SearchSourcesPanel`，当前共享组件仍使用普通 `target=_blank` 默认跳转和 `navigator.clipboard?.writeText`；它由消息/搜索脚注实际消费，未覆盖 Tauri 原生打开与文本剪贴板边界。新增真实组件回归修复前失败：点击没有阻止 WebView 默认导航，日志 `/private/tmp/reasonix-search-sources-before.log`。
- 搜索来源接入既有 `openExternal` 与 `writeClipboardText`，普通点击、Command 点击和中键点击均阻止默认导航并打开系统浏览器；原生拒绝不会改用浏览器 transport。复制等待实际结果提供成功/失败提示，打开失败提供复制链接操作，复用已有三语文案且不暴露内部错误。来源 HTTP(S) 过滤/去重与原消息数据保持，不增加 capability 或导航权限；Wails/browser 使用共享封装的现有宿主分流。
- 新回归覆盖真实组件的三种激活、规范化后的原生参数、危险来源过滤、实际复制成功、打开拒绝后的复制恢复及复制拒绝，浏览器替代 transport 调用数为 0。已纳入 `pnpm test:external-links`（也由 `test:tauri` 调用），新增定向与完整外链回归通过，日志 `/private/tmp/reasonix-search-sources-after.log`、`/private/tmp/reasonix-search-sources-external-tests.log`。完整生产构建（类型/contract/budget、sidecar/arm64 host、本地 ad-hoc 签名）通过，日志 `/private/tmp/reasonix-search-sources-native-build.log`。
- 新包两种私有档案实际 WKWebView 原生打开命令、非法协议/userinfo/mail 参数拒绝及默认系统浏览器本机回执通过，日志 `/private/tmp/reasonix-search-sources-native-links.log`，正常退出和成功现场清理通过。这是组件与真实原生链路的分层证据，未执行生产消息中的搜索来源物理点击；不扩展为该物理 UI、mail/OAuth、历史画面采集故障、完整 D/E 或正式发布已验收。
- 同一包双档案基础 smoke 通过：私有凭据身份、通知授权查询、Global 工作区、sidecar readiness、未认证请求拒绝和退出清理，日志 `/private/tmp/reasonix-search-sources-package-smoke.log`；独立确认无 Preview 实例残留。签名仍为本地 ad-hoc，正式签名/公证未完成。

#### D：当前剪贴板包完整窗口门禁与 UI 观察阻塞（2026-10-02）

- 当前产品 `7a83505ce` 的普通生产包执行 `smoke-native-window.py --dialogs --edit --focus`，**46/46 通过**（托管/显式各 23 项），日志 `/private/tmp/reasonix-window-boundary-current.log`。包含严格最小化/恢复、设置恢复、后台流式任务、外观恢复/回滚、原生编辑/剪贴板、面板取消、第二实例焦点和退出重启；没有放宽断言或重试失败阶段。此前偶发最小化失败根因仍未确定，本次通过不能作为稳定性修复或正式发布放行。
- 随后新建普通托管档案补验 `document-write-denied`，首次 CUA 绑定仍报 ScreenCaptureKit `-3811`，没有输入、保存或 UI 回执，新中文提示仍未完成真实 UI 验收。日志 `/private/tmp/reasonix-write-denied-after-window-ui.log`、控制 `/private/tmp/reasonix-write-denied-after-window-control.json`、失败现场 `/private/tmp/reasonix-native-ui-export-300f27df691645b89810312066077f0c` 保留。只向已确认自有宿主 74606 发送 SIGTERM；独立确认宿主/sidecar 74618 均退出、ready/launch-owner 文件清理、零输出且两个原件的字节和元数据不变。runner exit 1 来自测试主动停止，不能认定为应用自行崩溃或正常 Cmd+Q 通过。
- 观察失败限制物理 UI 验收，独立窗口 API 门禁仍可推进。Hooks 物理操作、保存失败提示、托盘/通知等真实 UI、历史 Wails 目录互斥及其余迁移门禁继续待验；外接屏和 Windows/Linux 延期保持，E 与正式发布不提前放行。

#### D：Tauri 剪贴板原生拒绝终止边界（2026-10-02）

- 上轮 Hooks 入口收敛属于实际进展。对照主窗口 capability（仅文本 read/write）及现有受限 WebView 门禁，发现共享 clipboard helper 在 Tauri 原生拒绝后仍尝试 `navigator.clipboard`、Wails runtime 或 `execCommand`。新增回归提供可成功的替代 transport，修复前实际返回 true 而非 false，日志 `/private/tmp/reasonix-clipboard-boundary-before.log`，不能把它当作通过。
- Tauri 写入拒绝立即 false，严格读取拒绝传播，兼容读取仍空串；ACL 拒绝和 native busy 均不切换宿主/浏览器权限。用户可重试，Hooks 严格读取保持草稿并提供失败解决步骤。Wails/browser 自身回退保留，未改 capability/native 插件权限或系统设置。回归要求两种拒绝下三个替代 transport 调用数全为 0、没有 fallback textarea，Wails 退出 Tauri 模式后仍成功，浏览器 focus/selection 恢复及 CopyButton 成败契约保持；`pnpm test:clipboard` 通过，日志 `/private/tmp/reasonix-clipboard-boundary-tests.log`。
- 首次完整 `pnpm test:tauri` 在 storage path copy 停止，旧设置夹具未响应原生 clipboard 命令、依赖浏览器回退才通过。夹具增加实际插件命令样本，Hooks 测试令浏览器写入拒绝，原路径/反馈断言不降；完整 Tauri 回归复跑通过，失败日志 `/private/tmp/reasonix-clipboard-boundary-tauri-tests.log` 与通过日志 `/private/tmp/reasonix-clipboard-boundary-tauri-tests-fixed.log` 保留。
- 产品提交 `7a83505ce` 完整生产构建通过，类型/contract/budget、sidecar/arm64 host 与本地 ad-hoc 签名通过，日志 `/private/tmp/reasonix-clipboard-boundary-build.log`；设置夹具调整在前端构建完成后，仅影响测试，不改变已构建产品代码。当前真实包两种档案独立菜单/文本 IPC 门禁 **4/4 通过**，主窗口文本读写/图片拒绝、未授权同源 WebView 文本读写拒绝、系统剪贴板完整原件恢复、档案身份与退出清理通过，日志 `/private/tmp/reasonix-clipboard-boundary-native.log`，独立确认无 Preview 实例残留。该门禁直接验证 native ACL，helper 的拒绝后零回退由上述 adapter 回归证明；不扩大为 Hooks 物理点击或失败 UI 已验；Hooks 物理点击、真实失败反馈、窗口稳定性、其余 D/A/B/C/E 和正式签名/公证保持待验，外接屏与 Windows/Linux 延期不变。

#### D：Hooks JSON 原生剪贴板与草稿保护（2026-10-02）

- 上轮配置备份目录修复和真实包三阶段属于实际进展。进一步审计当前 Wails runtime/浏览器直连，发现 Tauri Hooks 路径复制已走共享入口，但 JSON Copy/Paste 仍直接用 `navigator.clipboard?`，接口缺失时无反馈，读取失败与异步上下文变更缺少草稿保护。冻结 Wails Hooks 也使用浏览器剪贴板；本切片只改 Tauri Hooks 消费者与共享文本读取的新增严格入口，保留原 Wails/browser 回退语义。
- JSON/路径复制共用 `writeClipboardText`，成功反馈等待实际完成，失败清除旧成功提示。新增 `readClipboardTextOrThrow` 供编辑器区分读取拒绝与真正空值，旧 `readClipboardText` 空串契约不变；Hooks 空剪贴板/读取拒绝保留草稿，pending 禁止冲突编辑/保存，workspace/scope/unmount 代次隔离旧结果。剪贴板动作不持久化配置、不执行 hooks、未增 renderer capability；继续沿用既有主窗口原生文本 ACL 和共享回退策略。
- 新真实组件/native adapter 回归覆盖浏览器权限拒绝时原生 JSON Copy/Paste、拒绝保持草稿/失败清除旧成功、空剪贴板、pending 禁用、切换工作区后旧粘贴结果拒绝、没有配置写入；共享回归确认严格读取拒绝与旧空串契约兼容。`pnpm test:tauri`（已加入新测试）、共享 clipboard 回归及三语反馈后的 Hooks 定向回归通过。日志 `/private/tmp/reasonix-hooks-tauri-tests.log`、`/private/tmp/reasonix-hooks-shared-clipboard-tests.log`、`/private/tmp/reasonix-hooks-clipboard-tests.log`；这些 IPC 为受控组件样本，不等同 macOS 系统剪贴板实际 UI。
- 三语路径/JSON 复制与粘贴失败补充重试/手动复制步骤，粘贴明确草稿保留。首次完整构建在繁体 gzip cap 停止，日志 `/private/tmp/reasonix-hooks-clipboard-build.log`；用同一 Node level-9 压缩器仅把三条提示恢复旧值测量 81128→81084 字节（增加 44，旧 cap=81100.8，超出约 27）。繁体单包 cap 79.2→79.3 KiB，其他预算不变；Python gzip 实现初次测量与 Node 不同，未用其数值决定 cap。生产代码 `912c87a4e` 的修订后完整构建通过（类型/contract/budget、sidecar/arm64 host、本地 ad-hoc 签名），日志 `/private/tmp/reasonix-hooks-clipboard-build-complete.log`；新包两种私有档案独立菜单/剪贴板 **4/4** 通过，包含实际 WKWebView 文本 IPC、权限拒绝、系统剪贴板原件恢复和身份/退出清理，日志 `/private/tmp/reasonix-hooks-clipboard-native.log`，独立确认无 Preview 进程残留。这不是完整窗口门禁或 Hooks 物理点击证据；Hooks 两种档案的物理点击/系统值和剪贴板原件恢复仍待验，新写入拒绝提示、历史目录互斥、窗口稳定性及其余 D/A/B/C/E 门禁保留，外接屏及 Windows/Linux 延期不变。

#### D：配置备份目录隔离修复与采集失败后的继续推进（2026-10-02）

- 上轮写入拒绝、修复和生命周期属于实际进展。当前 `30c311130` 包用新私有托管档案补验提示，在首次绑定即 `-3811`；应用清单确认 Preview 在运行，窗口枚举超时并重置观察会话。未发送固定提示或执行保存，不计 UI 通过；日志 `/private/tmp/reasonix-write-denied-recheck-ui.log`，控制 `/private/tmp/reasonix-write-denied-recheck-control.json`，现场 `/private/tmp/reasonix-native-ui-export-01118029de6642ef948ae400d23d64f3` 保留。本次只向确认自有宿主发 SIGTERM，两个 PID 独立确认退出、回环服务关闭，零输出/无 UI 回执，sidecar/readiness 清理通过；不把测试停止当作应用自行崩溃。
- 检查 D 数据互斥/回退时确认旧官方 Wails 会话租约与新目录锁协议仍不同，该历史宿主互斥缺口没有关闭。另发现当前 `PreviewProfile::create_backup_dir` 会跟随 `backups` 符号链接。新增真实文件系统回归在修复前失败（导入成功，可能写入外部备份目录），日志 `/private/tmp/reasonix-backup-redirect-before.log`，不计通过。
- 修复改为创建普通备份目录并用 `symlink_metadata` 拒绝符号链接/非目录，Unix 新建根和批次目录用 0700；原文件和新配置/备份仍为 0600、备份先于导入、不覆盖目标、显式档案拒绝导入等原契约保留。回归要求符号链接外部目录零新增、稳定原件不变、没有 Preview 配置；非目录原件保持；正常导入两级目录均 0700。**配置档案 13 项通过**、严格 clippy（含测试）通过；日志 `/private/tmp/reasonix-backup-root-tests.log`、`/private/tmp/reasonix-backup-root-clippy.log`。检查不声称能抵御同 UID 在检查后恶意替换目录的全部竞态。
- 包级导入 runner 增加真实备份根/批次普通目录和 0700 检查。从干净 `05cf57e1d` 完整构建生产前端/sidecar/arm64 macOS 包，类型/contract/budget 与本地 ad-hoc 签名通过，日志 `/private/tmp/reasonix-backup-root-build.log`。新包实际 import/restore/explicit **三阶段全部通过**：真实 host 入口/bridge 设置、两级备份目录 0700、文件 0600、项目元数据仅复制 root/title、修改后 sidecar/整包重启、原件/备份保持及显式档案拒绝导入、正常退出清理；日志 `/private/tmp/reasonix-backup-root-package-import.log`。独立确认无 Preview 实例残留；这是原生入口与包级证据，不代替设置页物理点击或所有历史 Wails 目录互斥。新写入拒绝提示原生复验、历史目录互斥、其余 D/A/B/C、E 与正式签名/公证仍待完成；外接屏及 Windows/Linux 延期保持。

#### D：macOS 文档目的地真实写入拒绝（2026-10-02）

- 对照冻结 Wails `SaveLocalPathAs` 的系统保存面板和目标复制；Tauri 仍通过真实 Save 面板选择目标、只在选定目录内使用临时文件原子替换。新增普通 runner `--scenario document-write-denied`，源 A/B 均为 0600，可浏览/搜索的目标目录为 0500。启动前要求实际创建得到 PermissionError，能绕过权限的 runner 拒绝启动；没有故障 IPC、renderer 替身或修改用户目录权限。
- 当前 `5882a249d` 包的托管/显式私有档案分别通过真实本机固定回复→Original A 右键另存为→中文/空格目标目录→Save，native 实际创建临时文件得到 Permission denied，应用显示保存失败，**2 项真实写入拒绝/2 次正常 UI 生命周期通过**。源完整指纹、目标目录权限/mtime/inode、零输出、core canary/身份及 host/sidecar/readiness 清理通过，成功现场/控制删除，四个自有 PID 独立确认退出。日志 `/private/tmp/reasonix-write-denied-ui.log`；基线截图 `/private/tmp/reasonix-write-denied-managed.png`、`/private/tmp/reasonix-write-denied-explicit.png`。本轮基线 UI 无捕捉失败，不推断历史稳定性已解决。
- 基线错误包含 OS 英文与内部临时文件名。保存分支对已知创建/替换目的地的 Permission denied/Operation not permitted 单独提示“无法写入保存位置。请选择有写入权限的文件夹后重试。”，同步三语，不改 native 权限/复制逻辑；其他错误仍保留原因与可写位置步骤。真实菜单组件/adapter 回归新增实际临时路径样本及替换拒绝，并确认已知拒绝不显示临时路径，完整 `pnpm test:external-links` 通过，日志 `/private/tmp/reasonix-write-denied-tests.log`。夹具另通过真实写入 PermissionError、目录元数据保持、可写目录/符号链接拒绝及已有场景顺序保持检查。
- 从干净 `30c311130` 完整生产构建成功，前端检查/类型/contract/budget、sidecar/arm64 host 和本地 ad-hoc 签名通过，预算未改。日志 `/private/tmp/reasonix-write-denied-build.log`。新包两种私有档案的包级 smoke 通过，覆盖身份隔离、Global 工作区、通知只读授权、sidecar readiness 与正常退出；日志 `/private/tmp/reasonix-write-denied-package-smoke.log`，不代替真实提示验收。
- 新包实际 UI 在第一次托管绑定即连续 `-3811`，已确认两个自有进程存活，同宿主重新绑定及重置采集会话仍失败；尚未发送固定提示或执行保存。只终止本次自有进程组，runner exit 1 并保留现场 `/private/tmp/reasonix-native-ui-export-5e22aacb55dd40c29877bee48d39782a`、控制 `/private/tmp/reasonix-write-denied-fixed-control.json` 和日志 `/private/tmp/reasonix-write-denied-fixed-ui.log`。无 UI 回执/输出；两个 PID 独立确认已退出，本机服务关闭，但失败现场保留 launch-owner/ready 文件。该停止向整个自有进程组发 SIGTERM，包含 sidecar，不等同只终止宿主或正常 Cmd+Q，不能将日志的非正常退出当作应用自行崩溃。
- 为核查上述残留追加当前包 `smoke-host-lifetime.py`：托管/显式 × 单独宿主 SIGTERM/SIGKILL × 空闲/真实流式任务，**8/8 通过**，sidecar 均 kernel exitCode=0、readiness 清理、上游取消及原件保护/同档案重启通过。日志 `/private/tmp/reasonix-write-denied-host-lifetime.log`。该证据支持单独宿主死亡处理正常，不把向整组同时发信号的失败现场推广为产品生命周期回归。
- 新提示真实原生验收仍待完成；目录源拒绝、Finder/editor、其余 D/A/B/C、E、正式签名/公证和历史窗口偶发最小化缺口保持。外接屏及 Windows/Linux 延期不变。

#### D：显式档案主题导入缺口关闭（2026-10-02）

- 复用干净 `5882a249d` 的当前生产包，产品代码/权限和 runner 未变；新私有显式档案通过实际原生 Import Cancel、中文/空格路径选择、双图片导入、重复导入、schema-99 无效包拒绝，以及 Cmd+Q 后同档案普通重启。**五阶段磁盘检查、两次正常 UI 生命周期通过**，日志 `/private/tmp/reasonix-theme-explicit-oct02.log`。
- 首次导入实际首页显示红色城市图片，切换工作区显示绿色人物图片；重复导入后我的主题为 2，独立回执核对 `user-ui-import` 与 `user-ui-import-2` 且第一份未覆盖。无效包实际显示“主题导入失败。请使用 Reasonix 导出的主题包，并确认文件完整、可读取后重试。”，两份主题和偏好完整指纹不变。
- 重启后实际点击两张卡片，逐份切换首页/工作区，两种图片均恢复。截图 `/private/tmp/reasonix-theme-explicit-restart-first-home.png`、`/private/tmp/reasonix-theme-explicit-restart-first-task.png`、`/private/tmp/reasonix-theme-explicit-restart-second-home.png`、`/private/tmp/reasonix-theme-explicit-restart-second-task.png`。源包/图片的字节与权限/mtime/inode、core canary/身份、host/sidecar/readiness 清理通过；四个自有 PID 独立确认已退出，成功私有目录和控制已删除。
- 重复文件选择的 AX 文本点击未选中，观察后改用已验证的原生 Go To Folder；Open 后两次观察 `-3812`，核对同宿主/sidecar 存活，通过只读截图与完整 AX 恢复观察并确认主题数为 2，没有重复导入或重启宿主。后续全流程完成；这关闭显式主题导入待验项，也补齐当前中文导入失败提示的原生验收，不能推断历史捕捉故障根因或窗口稳定性已解决。托管对应五阶段仍引用此前 `9972828f4` 的成功记录，不声称本轮当前包两档案均重测。
- 主题应用及实际系统明暗/磁盘错误、文档目的地写入拒绝、Finder/editor、通知/钥匙串完整 UI、其余 D/A/B/C、E 和正式签名/公证继续待验；外接屏与 Windows/Linux 延期保持。

#### D：macOS 文档真实读取权限拒绝（2026-10-02）

- 当前普通生产包仍来自干净 `5882a249dc9d7ae12441f01b5d0362d9f34f6633`；本轮没有修改产品前端/native/权限或重建包。新增普通 runner 场景 `document-permissions`，每档案创建两份固定私有原件，预先持有 Original A 的只读描述符后将其权限设为 000。启动前要求当前 UID 按路径实际打开得到 PermissionError，能绕过权限的 runner 明确拒绝启动宿主，不能假装完成 OS 拒绝。
- 描述符只保留在 runner，不传给宿主或 sidecar；实际文件链接仍走生产 `save_source` 的路径打开。控制/回执持续要求 A 为当前 UID 普通 mode-000 文件且 inode/mtime/size 保持，B 的完整指纹/内容不变，源目录只有 A/B、输出为空；每次实际 Cmd+Q 后 runner 再用既有描述符核对 A 的精确原字节。没有临时恢复源权限、绕过 app 读取、新增 renderer API 或固定 IPC 响应代替真实文件操作。
- 两种普通私有档案各通过真实固定本机回复→右键 Original A→另存为，均即时显示“无法读取源文件。请确认文件仍然存在且有读取权限后重试。”，没有 Save 面板或副本。**2 项真实读取拒绝/2 次正常 UI 生命周期通过**：两份原件、core canary/身份及 host/sidecar/readiness 清理通过，持有描述符确认不可读原件字节保持。日志 `/private/tmp/reasonix-document-permissions-ui.log`，截图 `/private/tmp/reasonix-document-read-denied-managed.png`、`/private/tmp/reasonix-document-read-denied-explicit.png`。成功私有目录/控制已删除，四个自有 PID 独立确认退出，回环服务关闭。
- 托管首次发送动作因窗口状态变化未执行，重新读取 AX 确认输入仍在且会话未创建，再发送；provider 每档案恰好一次请求。显式输入后的观察一次 `-3812`，重新只读 AX 确认已生效，没有重复输入或重启宿主。记录这些观察限制，不能推广为捕捉根因或窗口稳定性已修复。夹具另通过真实 PermissionError、持有描述符读回、可读 mode 拒绝、符号链接/错误大小拒绝和旧场景阶段路由检查。
- 源目录拒绝、目的地实际系统写入拒绝、Finder/editor 打开、主题显式导入、通知/钥匙串完整 UI、窗口偶发最小化、其余 D/A/B/C、E 与正式签名/公证仍待验收；外接屏及 Windows/Linux 延期保持。

#### D：macOS 文档另存为错误路径与解决步骤（2026-10-02）

- `smoke-native-ui-export.py --scenario document-errors` 在普通私有档案用唯一回环服务生成实际文件链接，固定两阶段 same-source/missing-source。源文件和中文/空格目录持续保护；同文件必须实际 Save→Replace→应用错误，缺失源必须实际菜单调用后即时错误且没有保存面板；每阶段回执要求原件指纹不变、源目录只含两份原件、输出为空，错误切片与四阶段正常保存分别计数。
- 干净 `851111594` 生产包两种普通档案的两种拒绝 **4 项真实操作/2 次正常退出全部通过**。确认覆盖源文件后 native 拒绝，缺失文件在打开面板前拒绝；回执保护、core canary/身份、host/sidecar/readiness 清理和成功夹具删除通过。日志 `/private/tmp/reasonix-document-errors-ui.log`。托管绑定加载后的捕捉一次 `-3811`、显式输入后的观察一次 `-3812`，后续只读 AX 成功确认当前状态，没有重复输入/发送或重启宿主。
- 实际错误提示复现“无法使用 另存为… 打开”并夹杂英文。保存分支改用保存专属文案：同源目标提示改用其他文件名/位置；源无法访问/读取提示确认存在及读取权限；其他失败保留错误内容并提示可写位置。简体中文、繁体中文及英文字典同步，已知源错误不展示 OS 细节或私有路径；打开/显示动作及 native 原子复制/校验/权限不变。现有真实菜单组件/adapter 回归新增 native 同源、Wails 同源措辞、缺失/读取拒绝、其他失败恢复提示，`pnpm test:external-links` 全部通过，日志 `/private/tmp/reasonix-document-errors-tests.log`。新包文案另由下述当前包真实 UI 验证，不以旧包原件保护通过替代。
- 完整生产构建首次在繁体中文新增键缺失处停止，补齐三条翻译；下一次在繁体语言包 gzip 预算停止。对当前产物仅移除三条新增键作归因测量：81047→80938 字节，增加 109 字节，旧 cap=80998.4，超出约 49 字节。保留完整解决步骤，繁体单包 cap 从 79.1 到 79.2 KiB，简体及其他预算不变；原预算命令与新完整生产构建均通过。失败日志 `/private/tmp/reasonix-document-errors-package-build.log`、`/private/tmp/reasonix-document-errors-package-build-fixed.log` 保留，不能算构建成功。
- 从干净 `5882a249d` 完整生产构建前端/sidecar/arm64 macOS 包成功，生产检查、类型检查、contract、budget 及本地 ad-hoc 签名通过；日志 `/private/tmp/reasonix-document-errors-package-build-complete.log`。该新包两种普通档案各实际生成一次固定回复，源文件 Save→Replace 后显示“目标文件是原文件。请选择其他文件名或保存位置后重试。”；Missing source 菜单操作即时显示“无法读取源文件。请确认文件仍然存在且有读取权限后重试。”，没有保存面板、打开措辞或 OS 英文。**4 项新包实际错误操作/2 次正常退出通过**，原件/身份/core canary、零输出及 host/sidecar/readiness 保护通过，日志 `/private/tmp/reasonix-document-errors-fixed-ui.log`。成功现场/控制已删除，四个自有 PID 独立确认退出，本机服务关闭；新包这两次 UI 验收没有捕捉失败，但不能推广为历史偶发故障根因已修复。
- 真实截图：`/private/tmp/reasonix-document-errors-fixed-managed.png`（同源提示）、`/private/tmp/reasonix-document-errors-fixed-managed-missing.png`、`/private/tmp/reasonix-document-errors-fixed-explicit.png`（缺失源提示）。实际原生文案覆盖简体中文；英文本身经真实菜单组件/adapter 五种错误样本回归，繁体通过键集类型与完整构建检查，不把这些证据扩大为三语都做过原生视觉验收。错误切片回执另验证跨场景/乱序拒绝、缺失源被创建和意外输出拒绝。
- 实际系统写入拒绝、源目录/读取权限拒绝的原生 UI、Finder/editor 打开、窗口偶发最小化、主题显式导入及其他 D/A/B/C、E、正式签名/公证仍待验收；外接屏与 Windows/Linux 延期保持。

#### D：macOS 本地文档另存为实际 UI 验收（2026-10-02）

- 对照冻结 Wails `desktop/external_opener.go::SaveLocalPathAs`：操作从本地文件链接菜单进入，保存面板默认源目录/文件名，取消返回空路径且没有成功提示；正常保存按源权限复制，源文件不变，覆盖经过系统确认。Tauri 现有 `local_paths.rs` 已使用打开的源文件、同 inode/硬链接/符号链接别名拒绝及原子临时文件替换。本轮产品代码/权限未修改，也未重新构建；复用干净 `8511115940d4362ee413b6aa1c1e2ee49f3b8ddf` 的真实生产包。
- 普通 runner `smoke-native-ui-export.py --scenario document` 新增两份 0600 原件、中文/空格源目录与输出名，私有 core 配置只指向 `127.0.0.1` 固定 SSE 服务。实际用户输入固定提示、点击发送，经真实 core/主界面生成 Original A/B 文件链接，再由 CUA 实际右键→另存为。每档案只允许一次固定模型请求，服务不记录请求/头/凭据，不启用原生探针、不改写 DOM、不向外部模型发送内容。
- 托管和显式各完成：Original A 的 Cancel→新建副本，再从 Original B 对同名副本 Cancel Replace→Replace。实际 Save 面板默认源目录/源文件名，Go To 进入私有输出目录；取消覆盖后回到 Save 面板，再 Cancel 才完成命令取消。新建和确认覆盖均实际显示“已保存到 …”，取消没有该提示。独立回执核对新文件为 A、取消覆盖时字节/权限/mtime_ns/inode 保持、确认覆盖变为 B，输出无额外临时文件；两份原件的 SHA-256/权限/mtime_ns/inode 始终不变。
- **两次普通 UI 生命周期、八项文件检查均通过**。两次实际 Cmd+Q 正常退出，凭据身份与 core canary 保持，host/sidecar/readiness 清理通过。日志 `/private/tmp/reasonix-document-save-ui.log`；截图 `/private/tmp/reasonix-document-save-managed.png`、`/private/tmp/reasonix-document-save-explicit.png`。成功私有 nonce 目录及控制已删除，四个自有 PID 独立确认退出，两个本机服务随 runner 关闭。回执防护另通过乱序、源符号链接/内容篡改、错误首份内容、覆盖取消变化和未替换拒绝检查；这些模拟文件检查与真实 UI 证据分开。
- 首次托管绑定捕捉 `-3811`，核实原宿主/sidecar 存活后重新读取成功；显式输入后的观察一次 `-3812`，重新只读 AX 状态确认输入已生效，未重复输入或发送，后续全流程完成。不能将本轮可恢复观察推广为历史持续捕捉故障已修复。源同文件/目录/实际系统写入拒绝的 UI 错误、Finder/editor 打开、主题显式导入、窗口偶发最小化、其余 D 与 A/B/C、E、正式签名/公证仍待验收；外接屏及 Windows/Linux 延期保持。

#### D：解除整体停滞的窗口复验与独立验收（2026-10-02）

- 当前生产包仍来自干净 `8511115940d4362ee413b6aa1c1e2ee49f3b8ddf`，本轮没有修改或重建产品。完整 `smoke-native-window.py --dialogs --edit --focus` **46/46 通过**：托管/显式各 23 阶段，包含实际窗口 API 最小化/恢复、Settings 恢复、原生编辑/剪贴板原件恢复、四类面板取消、真实流式任务、第二实例严格焦点和两种退出路径；各档案重启身份保持及 host/sidecar/readiness 清理通过。日志 `/private/tmp/reasonix-blocked-window-current.log`。这是一次独立完整复验，不是失败后重试；此前偶发最小化失败的根因尚未确定，不能将本次通过作为稳定性修复或正式发布放行。
- 窗口 runner 默认继续检查两种档案，新增 `--profile managed|explicit` 供隔离定位；单档案结果不代替默认完整门禁。每阶段立即输出，成功清理私有档案，失败保持非零并在自有进程回收后保留 0700 现场及原生结果/trace，输出已完成阶段数。原窗口动作、阶段顺序、等待期限及严格断言均不变。脚本隔离检查验证默认两档案、显式选择、成功清理、失败保留/传播、非法参数与独立焦点组合拒绝；这些是 runner 行为证据，不是原生 UI 验收。
- 改进后的 runner 实际执行 `--independent --edit --dialogs` **8/8 通过**（两种新私有档案各 4 阶段），包含原生菜单合同、剪贴板 IPC、四类面板取消及严格原生编辑；重启身份、剪贴板原件恢复和 host/sidecar/readiness 清理通过，成功私有夹具已删除。日志 `/private/tmp/reasonix-blocked-window-independent.log`。该切片不代替完整窗口门禁或实际鼠标/键盘交互。
- 截图工具 `-3812` 与原生窗口门禁分开记录。独立菜单/剪贴板/编辑/面板取消可使用已有 `--independent --edit --dialogs` 路径继续推进，不要求截图或最小化。保留主题导入等实际对话框 UI 的未验收状态；不把固定 IPC 浏览器样本或原生探针当作物理操作证据。整体工作继续按 macOS D 顺序推进，E 仍须 D 验收且 Preview 稳定；外接屏、Windows/Linux 延期不变。

#### D：macOS 主题导入失败提示补齐（2026-10-01）

- 托管真实无效包已显示错误，但只有“主题导入失败。”。中英提示增加可执行解决步骤：使用 Reasonix 导出的主题包，并确认文件完整、可读取后重试。沿用现有安全提示，不展示 native 错误中的私有路径/内容；没有改变包校验、取消、持久化或权限。
- 完整 `pnpm test:tauri`（含真实 Vite 官方目录构建、设置组件及 adapter）和 diff 检查通过；日志 `/private/tmp/reasonix-theme-import-feedback-tests.log`。此为文案修改，未新增镜像测试；新包实际显示另行核对，不将组件或构建结果当作实际 macOS 对话框验收。
- 从干净 `8511115940d4362ee413b6aa1c1e2ee49f3b8ddf` 完整重建前端/sidecar/arm64 macOS 包，生产检查、contract、budget 及本地 ad-hoc 签名通过；日志 `/private/tmp/reasonix-theme-import-feedback-package-build.log`。新托管普通档案实际 Cancel 返回及独立原件检查通过；第二次 Open 后 Cmd+Shift+G 捕捉连续报 `-3812`，未观察到路径面板、未选择/导入无效包，新文案的系统对话框 UI 未验收。仅结束自有宿主，runner exit 1；日志 `/private/tmp/reasonix-theme-import-feedback-ui.log`。失败控制/夹具保留，回执只有 cancel；独立确认主题为零、源原件不变、自有两个 PID 和 readiness 已清理，不计作正常退出。
- 原生捕捉失败后，使用 CUA 的 IAB 浏览器和本机临时 Vite 页面渲染当前真实 `TauriSettings`/`TauriThemeGallery`、adapter、字典及产品样式；IPC 是明确的固定失败/取消/成功样本，不调用 native 或读取用户档案。中英各实际浏览主题→失败提示→取消清除错误/零主题→再次失败→成功重试清除错误/新增一个主题，导入按钮保持可用，native 样本中的私有错误未展示。标题和 URL 正确，页面非空、无框架覆盖及相关 console error/warn，1280×820 桌面截图通过；900×620 最小 macOS 窗口的英文提示完整滚动可读，client/scroll 宽高相同，没有文本溢出。首次临时页面漏载宿主样式，补载完整样式后重验；不计为产品布局问题。
- 浏览器证据保存在 `/private/tmp/reasonix-theme-feedback-browser-9fg8wcnz/qa-evidence.json`，截图为同目录 `zh-error.png`、`en-error.png`、`en-minimum-window.png`；临时标签已关闭、viewport override 恢复、本轮本机服务已停止。此结果仅验证组件渲染及失败后恢复，不能替代 macOS Open/Save、实际主题持久化或原生窗口稳定性；相关 D/发布待办保持，外接屏及 Windows/Linux 继续延期。

#### D：macOS 导入主题图片修复后的真实包验收（2026-10-01）

- 从干净提交 `9972828f480ad56e09a09fb1b5cb6c97b97bf05c` 完整构建前端、当前 sidecar 和 arm64 macOS `.app`，生产检查/contract/budget 及本地 ad-hoc 签名通过；日志 `/private/tmp/reasonix-theme-import-package-build.log`，未进行正式签名/公证。
- 托管普通私有档案实际 Cancel 返回画廊、Go To Folder 选择当前中文/空格源包、Open 导入后，首页显示赤曜新城图片，工作区显示鼠尾草清风图片；两张预览截图和切换通过。重复实际导入同一包后“我的主题”变为 2，独立核对 `user-ui-import`/`user-ui-import-2` 的完整 metadata、颜色和两张图片逐字节，首次主题未被修改。实际导入 schemaVersion=99 包返回“主题导入失败。”，原画廊保留；完整偏好字节/权限/mtime_ns/inode、主题和源原件不变。错误提示当前没有解决步骤，文案仍需补齐。
- 实际 Cmd+Q 后正常退出、身份与 core 原件及 sidecar/readiness 清理通过；普通重启后实际重新进入画廊，两份主题卡片均有图片，原使用中的石墨保持。实际分别点击两张卡片，每份首页/工作区图片均显示正确，未应用主题。托管 **5 项文件检查及两次正常 UI 生命周期通过**。
- 显式普通私有档案实际画廊初始零自定义主题，实际取消 Import 返回后独立 cancel 检查通过。再次打开 Import，在 Go To Folder 改为显式源包并 Return 后，捕捉连续报 ScreenCaptureKit `-3812`；宿主/sidecar 仍在，重读及 reset 后绑定同一宿主也失败。没有继续点击 Open，未计入导入/重复/无效/重启。仅 SIGTERM 自有宿主，runner exit 1；其源原件不变、自定义主题仍为零，sidecar/readiness 清理通过，**不能计作正常退出或显式完整 UI 通过**。
- 日志 `/private/tmp/reasonix-theme-import-fixed-ui.log`、控制与私有失败夹具保留；独立确认托管两次及显式一次的六个自有 PID 均退出，两种 tmp 的 readiness 目录为 0。仅托管已补齐主题导入图片/重启验收，不把整体 runner 失败改记为通过，不推断资产修复解决捕捉或窗口最小化问题。显式验收、实际主题应用、文档另存为及其余 D、A/B/C、E、正式发布门禁继续保留，外接屏和 Windows/Linux 延期。

#### D：macOS 导入主题图片缺口及资产目录修复（2026-10-01）

- `de8afeeec` 普通托管私有档案实际取消 Import，画廊保持零自定义主题；再次通过真实 Open/Go To Folder 选择含中文和空格的私有包，显示 `Imported UI Theme`，但首页/工作区预览均没有图片。两张 WebP 已落盘，独立检查确认 metadata、规范化颜色、canonical 文件名和源图片字节正确，原件未变；**图片实际显示未通过**，未执行重复导入、无效包或重启。
- 根因是 asset protocol 的 `$APPDATA` 已包含当前 bundle identifier，配置又添加一次标识，实际主题路径被拒绝。修正为 `$APPDATA/theme-assets/**/*`，仍只开放应用主题资产。使用当前 Tauri 实际路径解析及 FsScope 的回归先复现旧配置拒绝图片，修复后首页/工作区资产可读；偏好、core 凭据、目录穿越、隐藏目录、其他应用和临时目录均拒绝。mock runtime 测试不读写用户档案，不代替 WebView 实际加载。
- 新增普通 `smoke-native-ui-theme-import.py`，使用既有两套官方 WebP 构造私有有效/无效包。独立核对实际取消/导入/同 ID 再导入/无效包/重启的顺序、固定 metadata 和颜色、图片逐字节、无 staging 残留、首次主题保护、无效导入及重启不改变完整偏好、源原件与 core canary 的身份/内容及正常退出。命令不操作 UI。首轮 record 的偏好 `0600` 假设错误：产品现有结果为 `0644`，父级 HOME 为私有 `0700`；工具调整为仅接受自有普通有界 `0600/0644` 偏好，仍保存精确权限/mtime/inode，控制/源包/回执的 `0600` 门禁保持。manifest 大写颜色按产品规则小写后比较，原件字节不变，不将工具误差归为产品失败。
- 首轮仅 cancel 文件检查计入回执；实际 Cmd+Q 正常退出、sidecar/readiness 清理后，runner 因步骤不完整正确失败，夹具和日志 `/private/tmp/reasonix-theme-import-ui.log` 保留，两个 PID 已独立确认退出。没有应用/删除主题或改动用户数据。新配置完整包及图片 UI 仍待执行，窗口稳定性、其余 D、A/B/C、E 和正式签名/公证保持；外接屏及 Windows/Linux 继续延期。
- 本轮当前 Go bridge 重新构建，设置正确 `REASONIX_TAURI_BRIDGE_TEST_BIN` 的 Rust 回归 **203 项通过、2 项既有忽略**；日志 `/private/tmp/reasonix-theme-scope-rust-tests.log`。锁定离线严格 clippy、fmt/diff、Python AST、真实源图片夹具及四项偏好权限/符号链接/源原件拒绝检查通过。测试 feature 仅在 dev-dependency 启用，不进入生产包。

#### D：macOS 官方主题恢复及两种档案的主题导出验收（2026-10-01）

- 从干净提交 `de8afeeeccbffa0160835511a56a2538fb7f8f56` 完整执行 `pnpm tauri:build -- --bundles app`，前端生产检查、TypeScript、Wails contract、CSS/主题 token、bundle budget、sidecar 和 arm64 `.app` 构建通过，源记录无 dirty，本地 ad-hoc 签名严格核对通过；日志 `/private/tmp/reasonix-official-theme-package-build.log`。未修改系统设置或新增文件/网络权限，未进行正式签名/公证。
- CUA 在新包托管/显式普通私有档案实际进入设置→外观→浏览主题：两种均显示“旗舰主题 8”、“基础配色 6”、“我的主题 0”，截图显示八套图片，页面非空且无框架错误覆盖。托管实际点击“赤曜新城”卡片后详情名称/描述、选中边框和实际背景预览更新，未点击应用主题；返回外观页并重新进入画廊后仍选中使用中的石墨。这项证明目录和实际预览修复，不推广为八套主题全部应用/重启或控制台无错误；本轮未单独收集产品 console。
- 两种档案各通过实际创建并保存固定无图片主题，导出时点击真实 Save→Cancel 返回画廊，按钮可用且未出现错误；再次通过真实 Go To Folder 改为当前档案私有 `报告 测试` 目录，保存 `主题 副本.reasonix-theme`，独立文件检查确认 ZIP 仅有合法 schemaVersion=2 的 theme.json、固定 ID/名称/明暗三项颜色/密度/圆角及 `0600` 权限。名称与中文/空格目标通过可访问性字段赋值准备，不证明输入法或自动键入 Unicode。
- 实际通过编辑入口更名并保存第二版，导出同名目标实际出现系统 Cancel/Replace。先点击 Cancel 返回 Save 面板，再点击 Save 面板 Cancel，画廊保留第二版且导出/编辑可用；首份输出全部字节、权限、mtime_ns 和 inode 完全不变。再次实际导出第二版、同名 Save、确认 Replace，面板关闭回到第二版详情；独立核对新输出名称和字节更新、固定 ID/全部颜色/私有权限保持及无额外临时输出。显式首次保存时面板记住托管目录，已通过真实路径字段改为显式目录后才保存，未跨档案写入。
- **托管/显式各四项文件检查，共八项；各一次普通 UI 生命周期，共两次正常退出全部通过**。最后分别实际 Cmd+Q，退出码 0、同档案凭据身份、core canary 字节/权限/mtime/inode 保护及 sidecar/readiness 清理通过，成功 nonce 目录/控制自动删除。固定日志 `/private/tmp/reasonix-theme-fixed-ui.log`；独立确认四个自有宿主/sidecar PID 均不再存活、没有运行的 Preview，成功目录/控制不存在。没有模型请求、消息发送、主题应用/删除、用户 Documents 写入或系统剪贴板改动。
- 前述 `ada308408` 捕捉失败和部分完成记录保持，不将其改记为通过，也不推断主题目录修复解决了 ScreenCaptureKit `-3812`；本次是代码修复后的新包独立结果。本地文档另存为、实际主题导入/图片/应用重启及写入错误 UI 继续保留；窗口最小化稳定性仍未解决，本轮未重复完整窗口门禁。D→E 顺序及其余 D、A/B/C、E、正式签名/公证门禁保持，外接屏与 Windows/Linux 继续延期。

#### D：macOS 主题导出现场缺口与官方主题目录修复（2026-10-01）

- 继续复用 `ada308408` 包，普通导出 runner 增加 `--scenario theme`。只通过实际设置→外观→浏览主题创建无图片的石墨自定义主题 `Reasonix UI Theme`，第二版沿实际编辑入口更名为 `Reasonix UI Theme Revised`；固定 ID、明暗 bg/fg/accent、密度/圆角及无图片 ZIP 清单由独立文件检查核对。保留原取消、新保存、取消覆盖字节/权限/mtime/inode、接受覆盖内容更新及退出保护，不增加前端/native 权限。
- 托管普通档案在点击设置后的捕捉连续报 ScreenCaptureKit `-3812`，同一宿主仍存活、重读及重新绑定失败，未执行主题验收。仅终止自有宿主，输出/验收回执均不存在；失败日志 `/private/tmp/reasonix-theme-export-ui.log`。不重跑已失败的托管 UI，仅推进尚未执行的显式档案。
- 显式普通档案已实际创建并保存主题，首次实际 Save 面板取消后主题和导出/编辑按钮保留；再次通过实际 Go To Folder 选择当前私有目录、保存 `主题 副本.reasonix-theme`，独立 ZIP/完整固定字段/0600 权限检查通过，原包私有备份完成。然后实际编辑并保存第二版名称，导出同名文件实际显示系统 Cancel/Replace 提示。点击 Cancel 后捕捉连续报 `-3812`，同一宿主存活，第一份导出字节不变；未观察返回 Save 面板/完成取消回调，也未执行 Replace，**仅 cancel/new-save 两项文件检查通过，完整 UI 生命周期及覆盖流程未通过**。日志 `/private/tmp/reasonix-theme-export-explicit-ui.log`；两份失败夹具/控制及原包保留。自有宿主和 sidecar 均已退出，readiness 目录均为 0，失败 finally 的 parent-loss 等待清理在本轮非正常退出路径实际核对；不视为正常退出通过。
- 实际画廊同时显示“旗舰主题 0”。对照磁盘八套官方 manifest/背景/预览资源，新增 `test:tauri-theme-catalog` 使用当前 Vite 的真正 client 构建与执行，复现官方目录为空。根因是 `typeof import.meta.glob === "function"` 运行时判断：Vite 已将 glob 调用编译为导入对象，运行时没有该函数，故资源虽打包但被跳过。改用编译时 MODE 区分 Vite 模块与不展开 glob 的直接 Node 组件测试，不改变资产路径、主题 ID 或 URL 权限。参考 [Vite glob import 文档](https://vite.dev/guide/features.html#glob-import)。
- 修复后的独立 Vite client 门禁已通过八套完整官方主题 ID/manifest、16 张源图片与实际输出逐字节比较、当前主题选择及默认基础主题状态；背景必须在同 origin 内精确登记，外部 origin/未登记背景仍拒绝。测试将实际编译产物放入具有正确浏览器 module URL 的 VM，未改写产物，不将此当作 rendered UI。首轮资产验证误用 Node file: module URL，后续修正 VM module origin 及跨 realm 比较；该测试工具误差不计为产品失败。门禁加入 `pnpm test:tauri`，完整 Tauri 组件/adapter 回归及 TypeScript typecheck 均通过；Python AST、固定主题包有效样本/四项拒绝检查和 diff 检查通过。真实 macOS 重建与修复后画廊 UI 另行记录。
- 主题导出覆盖、实际主题导入/图片/重启、文档另存为和窗口稳定性仍待验收；不因两项文件检查、目录构建门禁或捕捉错误而标记完整 D/E 或正式发布通过。外接屏及 Windows/Linux 延期范围保持。

#### D：macOS 诊断导出的实际另存为、取消与覆盖（2026-10-01）

- 复用干净 native 提交 `ada308408038e9e9771abb60880d5c022352d4d9` 的完整生产包，不修改产品前端/native、权限或重建。新增普通包 runner `smoke-native-ui-export.py`，只管理托管/显式私有档案、实际导出文件检查和退出；不启用原生窗口探针，不代替用户操作 Save 面板。通过 CUA 在设置→诊断实际启用前端记录、添加标记及停止导出，没有模型请求或消息发送。
- 两种档案均实际取消首次 Save 面板：页面保留“待导出”、9 条事件和可用导出按钮，私有输出目录为空。再次导出仍是同一报告 ID；通过实际 Cmd+Shift+G 的系统路径输入框选择当前档案 `报告 测试` 目录，Save As 字段填写 `报告 副本.json`，实际点击 Save 后页面恢复未记录状态。文件是 schemaVersion=2、带实际 marker 的报告，权限 `0600`；只在当前私有输出目录生成一个目标。中文/空格路径及文件名使用可访问性 setValue 准备，不以此证明 Unicode 自动键入或输入法验收。
- 实际开启第二次记录、添加标记并导出同名文件，观察系统覆盖提示的 Cancel/Replace。点击 Cancel 返回 Save 面板，再取消该面板，页面保留第二份待导出记录；独立核对第一份报告全部字节、权限、mtime_ns 和 inode 完全不变。再次导出保留同一第二报告 ID，实际选择同名目标并点击 Replace，页面恢复未记录状态，导出报告 ID 与内容确实更新且保持合法格式/私有权限，无额外临时文件。显式档案面板初始记住托管路径，已通过实际 Go To Folder 改为显式私有目录后才保存。
- **两种档案各一次普通 UI 生命周期、各四项文件检查，共两次生命周期/八项文件检查通过**。每次实际 Cmd+Q，正常退出码 0、sidecar/readiness 清理、凭据身份稳定和 core canary 字节/权限/mtime/inode 原件保护通过。固定日志 `/private/tmp/reasonix-diagnostic-export-ui.log`；成功私有档案和控制文件已删除，独立核对四个自有宿主/sidecar PID 不再存活、没有运行的 Preview。没有写入用户 Documents 或修改系统剪贴板。
- `--record cancel|new-save|overwrite-cancel|overwrite` 只检查已有实际文件并保存私有回执；固定顺序、当前阶段、存活进程和规范 nonce 路径必须匹配。控制/回执/报告要求当前用户普通有界 `0600` 文件，目录为普通 `0700`，拒绝链接、外部目标、公开文件和跳步。启动只接受 `/private/tmp` 下的新控制文件，备份/首次回执不覆盖已有文件。Python AST、10 项有界拒绝检查和 diff 检查通过；这些工具检查不计为实际 UI 行为，故障清理分支未另行注入验收。
- 本切片补齐诊断导出的正常 Save/Cancel/Replace UI；本地文档另存为、主题导出、磁盘写满/权限拒绝及别名目标的实际 UI 仍待验收，已有 native 文件回归不能代替这些现场操作。窗口稳定性仍未通过；其余 D、A/B/C、E 及正式签名/公证门禁保持，外接屏和 Windows/Linux 按用户要求延期。

#### D：macOS 两种普通档案的键盘及右键编辑菜单（2026-10-01）

- 复用 `ada308408038e9e9771abb60880d5c022352d4d9` 的完整生产包，不修改产品前端/native、依赖、权限或重建。普通 UI runner 增加 `--scenario edit` 和 `--profile managed|explicit|both`（默认 path/both），支持分别验收尚未完成档案并报告实际完成数量；编辑文字是对应档案内唯一私有目录路径，含中文、空格和 emoji，不请求模型或发送消息。源码核对 Tauri 的实际 textarea 使用 WebKit 原生上下文菜单，此次不借用 Wails `Composer` 的自定义菜单实现或组件回归证明产品 UI。
- 为拒绝“已有相同文字”的假通过，Swift helper 新增只读 `checkpoint`：每次真实 Copy/Cut 前记录系统代次，保留上一所有权回执；`claim-after` 要求实际唯一值、稳定代次且严格大于该检查点，成功后消费检查点，不允许重用。`--record checkpoint|claim|verify` 只接受有界普通 0600 控制文件、nonce 对应的当前用户 0700 私有目录、固定 helper/快照/回执命令及仍存活的宿主/sidecar，不能从控制文件执行任意命令。保存/登记仅写私有回执，系统剪贴板不写入。私有命名 pasteboard 的未写入拒绝、同文字新代次、检查点不可重用、原有多格式恢复及来源保护自测均通过。
- 两种档案各完成实际 Cmd+A/C/X/V 和右键 Copy/Cut/Paste。每档案四次、共八次 Copy/Cut 的每一次均经操作前检查点及操作后精确值/新代次验证；两次 Paste 后要求真实输入框完整恢复且独立系统值/代次未变。两种剪切方式均实际清空输入框并禁用发送；空输入框原生菜单 Copy/Cut 禁用，Paste 可用且恢复完整 Unicode 字符串。少数菜单点击后的即时 AX 树仍保留选中菜单，通过 Escape 关闭已执行菜单后核对实际空值/恢复值；没有重做复制/剪切/粘贴动作。最后实际全选删除清空草稿，Cmd+Q 正常退出。
- 显式档案的 CUA `typeText` 实际只输入 ASCII 部分，未完整输入中文/emoji；使用已暴露的输入框 `setValue` 准备完整文字，AX 确认精确值及实际 Cmd+A 全选后才进行编辑门禁。托管新档案同样用可访问性赋值准备文字，没有通过 JS 修改输入框、临时替换系统剪贴板或用 CUA paste 代替系统粘贴。因此证据证明 Unicode 内容的复制/剪切/粘贴，不证明中文输入法、组合输入或 CUA 自动键入 Unicode 已通过。
- 托管、显式 **各一次实际 UI 生命周期通过，共两次**；固定日志 `/private/tmp/reasonix-composer-clipboard-managed-ui.log`、`/private/tmp/reasonix-composer-clipboard-explicit-ui.log`。均核对原件字节/权限/mtime、稳定档案身份、正常退出码 0、无 sidecar/readiness 残留，并完整恢复/独立比较所有原剪贴板 item/type 的顺序与字节。两份成功私有档案及控制文件删除，独立确认没有运行的 Preview。Swift 编译/私有自测、Python AST 及 diff 检查通过；记录入口另核对已退出夹具、伪造命令、公开控制文件及符号链接均在执行 helper 前拒绝。
- 最初托管尝试在输入操作后观察连续报 ScreenCaptureKit `-3812`，同一宿主存活但重新绑定仍失败，未执行编辑验收。仅终止该自有宿主，原剪贴板未改写且完整原件保留；退出码非 0，不计 UI 或正常退出通过。进程均已退出，但私有 bridge 目录保留，失败日志 `/private/tmp/reasonix-composer-clipboard-ui.log` 及夹具保留。runner 的失败 finally 原先立即强制清理 sidecar，现先给既有 parent-loss watcher 五秒释放 readiness，再强制回收自有残留子进程；没有把此改动当原生宿主问题修复或尚未执行的故障路径通过。之后只推进未执行显式及托管新阶段，不将失败命令总结果算通过。
- 输入框原生键盘和上下文编辑切片已补齐；消息复制、其他存储/hooks 路径、实际失败反馈、IME/组合输入及其余菜单/快捷键仍需按范围验收。窗口最小化稳定性依然未通过，D→E 顺序及其他 D、A/B/C、E、正式签名/公证门禁保持；外接屏及 Windows/Linux 继续按用户要求延期。

#### D：macOS 存储路径按钮与实际系统粘贴（2026-10-01）

- 继续复用干净 native 提交 `ada308408038e9e9771abb60880d5c022352d4d9` 的完整生产包，没有修改产品前端、native、依赖或权限。新增普通包 runner `smoke-native-ui-clipboard.py`，只管理两种隔离档案的宿主、系统剪贴板保护和退出检查；不启用原生窗口探针，不通过 JS 改写产品输入框。实际按钮、反馈和粘贴另由 CUA 验证。
- 原剪贴板在任何实际复制前完整保存为 0600 有序多项/多类型二进制快照，继续保留原有数量、总量、不可读取和文件承诺边界。新增路径来源只接受当前用户拥有的 `/private/tmp/reasonix-native-ui-clipboard-{32位nonce}` 下 managed/explicit 普通 0700 目录及普通 0600 控制文件，固定字段为 nonce/path；路径必须在对应档案内，每一级都是当前用户拥有的普通目录，拒绝符号链接、`.`/`..`、额外字段、外部路径和超限数据。兼容 Foundation 将 `/private/tmp` 表示为 `/tmp`，不允许任意来源。
- 显示过的路径不能沿用“未展示唯一 canary”的中断恢复规则：实际 UI 复制后用独立 AppKit helper 只读核对精确文本、单项内容及稳定系统 changeCount，再登记该代次；退出恢复必须同时匹配路径、nonce 和代次。即使外部重新复制相同文字，也保留当前内容并使验收失败。原有 canary 流程不变。私有命名 pasteboard 自测覆盖有序富文本/二进制恢复、无回执路径拒绝、重复相同文字的新代次拒绝、外部路径/公开文件/来源与路径符号链接/`..` 拒绝；编译及自测通过。测试夹具曾因复用 AppKit 已写入项和私有临时目录别名判断失败，均在实际系统复制前修正，不计为 UI 失败或产品修复。
- 两种普通私有档案均通过实际设置→存储与路径操作：默认和当前工作区未设置，其复制按钮禁用；配置目录与核心报告的精确私有路径一致。实际点击“复制Preview 配置目录”后 Help 变为“已复制”，独立 helper 确认实际系统文本和代次。返回主窗口空输入框，实际 Cmd+V 粘贴完整路径；托管路径包含 `Library/Application Support` 的空格，显式路径来自自定义 core。没有使用 CUA paste 临时替换剪贴板、发送消息或请求模型。
- 托管、显式各一次实际 Cmd+Q 正常退出，**两次 UI 生命周期和系统/退出检查通过**。每次核对凭据身份、保护样本字节/权限/mtime 不变、无 sidecar/readiness 残留，随后恢复并独立比较原剪贴板的每项/每类型顺序和全部字节。固定日志 `/private/tmp/reasonix-path-clipboard-ui.log`；成功私有目录和控制文件已删除，独立确认没有 Preview 或关联 sidecar。Python AST 和 diff 检查通过。
- 此证据限于两种档案的配置目录按钮、空工作区禁用状态、成功反馈及实际输入框粘贴；其他存储/hooks 路径、消息复制、上下文菜单、实际失败反馈和显式输入框完整 Copy/Cut 链路仍待验收。窗口稳定性仍为最新包 23/46、显式最小化失败，其余 D、A/B/C、E 和正式签名/公证保持；外接屏及 Windows/Linux 按用户要求延期。

#### D：macOS 三项旧界面偏好的真实导入、重启及撤回（2026-10-01）

- 复用干净 native 提交 `ada308408038e9e9771abb60880d5c022352d4d9` 的完整生产包，不修改前端/native 或重建。通过 CUA 操作普通私有 Preview 的实际设置页，使用上述已验证非持久旧 origin 来源：固定私有工作区、`deep` 模式和正确持久字号枚举 `large`。托管/显式档案各经历导入前、导入后和撤回后的三个新进程，**共六次实际 UI 生命周期完成**；每次均重新预览以获得该进程的新原生回执，再通过实际 Cmd+Q 正常退出。
- 两种档案初始主窗口均无默认工作区，通用设置为“标准”。实际来源预览显示三项固定值，确认复选框初始未勾选且导入禁用；勾选后才启用导入，实际点击显示“已导入 3 项”、确认/导入禁用及撤回记录。同档案重启的主窗口/信息栏与存储页显示确切私有工作区，通用设置“深入”选中、“标准”未选，外观设置“大”选中、“默认”未选；再次预览仍为原三项，重复导入保持禁用。来源列表、选中状态和实际截图均由 CUA 核对，不只读取 localStorage 或原生回执。
- 实际撤回后再通过 Cmd+Q/新进程核对：主窗口恢复“选择工作区”，存储页默认/当前工作区未设置；通用恢复“标准”，外观恢复“默认”，撤回按钮及记录提示消失。再次预览原三项仍在，确认框重新未勾选、导入禁用，没有再次导入。显式档案撤回时实际显示“已恢复 3 项”。托管档案点击撤回后的即时观察曾连续报 ScreenCaptureKit `-3812`，未重复点击；核实同一宿主存活后正常退出，新进程的三项恢复及记录消失均已由实际 UI 确认，不把缺失的即时反馈当作已观察。
- 托管三阶段日志 `/private/tmp/reasonix-private-legacy-ui-current-cua.log` 均记录新来源回执、稳定身份、来源/core canary 字节/权限/mtime 保护及正常退出清理。随后显式阶段启动前检查因 runner 多设 `REASONIX_STATE_HOME=core` 停止，**显式 UI 尚未执行**；源码核对显式档案尊重调用者环境，而既有包检查要求该测试不设置额外 state override。仅移除夹具变量，保持产品行为和环境断言；新增 `--profile managed|explicit|both`（默认 both），输出实际完成阶段数，只对未完成显式档案重新验收。显式三阶段日志 `/private/tmp/reasonix-private-legacy-ui-explicit-cua.log` 最终通过，不重做已完成托管 UI，不将先前设置失败记为通过。
- 六次正常退出均独立核对同档案 origin/凭据身份稳定，来源和保护样本不变，无 sidecar/readiness 残留。显式成功档案由 runner 删除；托管成功档案因下一显式设置失败曾保留，现已在核对三个完成回执、原生进程与目录所有权后仅删除该成功档案。显式设置失败夹具和历史捕捉/字号失败日志保留；没有清空共享系统 WebKit 仓库、访问用户模型或改动用户旧数据。
- 此证据完成**三项代表偏好的实际预览→确认导入→重启→撤回→重启**，不推广为全部 22 项、迁移后的修改/冲突/存储失败的全部实际 UI 路径或真实旧用户档案迁移通过；这些已有组件保护回归，仍需按具体 UI 范围验收。窗口最小化稳定性及其余 D、A/B/C、E、正式签名/公证继续保留；外接屏及 Windows/Linux 按用户要求延期。

#### D：macOS 最小化失败的启动、遮挡及模态状态核对（2026-10-01）

- 本轮继续定位窗口稳定性，native 提交 `67c36323c` 仅在 opt-in 主线程快照加入实际 `NSRunningApplication.isFinishedLaunching/isActive` 以及 NSApplication/NSWindow occlusion 的 Visible 标志；Python trace 白名单只允许对应布尔字段。没有新的 renderer API、延迟/重试、额外恢复或激活、写入窗口属性或修改最小化断言。真实 bridge 的 Rust **202 项通过、2 项既有忽略**，严格 clippy/fmt/Python 语法和 diff 检查通过；从干净提交完整生产构建前端/sidecar/arm64 `.app`，contract/budget 与 ad-hoc 签名通过。
- `67c36323c` 包执行 `smoke-native-menu-window.py --api-startups 2`，**4 个独立原 API 启动样本、2 个原生 Minimize/Settings 恢复、2 个菜单合同重启阶段共 8 项全部通过**。每个样本是新进程，托管/显式档案各自身份稳定，退出清理通过，无失败重试。所有已记录的请求前快照 FinishedLaunching=true，但应用和窗口 occlusion Visible=false，最小化仍发生；本观察不允许给已有门禁添加“必须未被遮挡”的前提。日志 `/private/tmp/reasonix-window-presentation-startups.log`。
- 同一包的完整 `--dialogs --edit --focus` 严格门禁在托管首项 exercise 的最小化失败，**0/46 完整阶段通过，1 项实际失败，其余未执行**。请求时和超时后的 FinishedLaunching=true、应用实际 active、应用/窗口 occlusion Visible=true，Will/Did/Deminiaturize 均为 0；请求前主窗口为 key/main，随后失去 key/main、主页面完成，窗口仍可见且未最小化。restoreRequests=restoreCompletions=2、Reopen=0，两次外观请求均跳过重复写入。本次失败时不存在“应用尚未完成启动”或“系统报告窗口不可见”的状态；不能推广为根因已修复或所有窗口管理竞争均已排除。日志 `/private/tmp/reasonix-window-presentation-full.log`。
- 为继续核对具体拒绝条件，native 提交 `ada308408038e9e9771abb60880d5c022352d4d9` 只读追加 NSApplication running/是否存在 modalWindow、主窗口 attachedSheet/isSheet/inLiveResize，仍限定固定布尔字段。严格 clippy/fmt 和完整干净生产构建通过。该包完整门禁 **23/46 通过**：托管全部 23 项（含完整原生编辑/剪贴板恢复、四类面板取消、流式任务、第二实例严格焦点和关闭退出）通过，显式首项 exercise 最小化失败，其余未执行。请求前及超时均 running/FinishedLaunching=true，modalWindow/attachedSheet/isSheet/inLiveResize=false，occlusion Visible=false；Will/Did/Deminiaturize 仍为 0，恢复/外观计数与前述失败一致。没有 AppKit 模态面板或实时调整状态；尚未核定 AppKit/窗口管理拒绝的具体根因。日志 `/private/tmp/reasonix-window-modal-resize-full.log`。
- 独立限定当前 UID 和系统 Dock 可执行路径的进程统计为 1，仅证明采样时 Dock 存在，不推断其健康、用户是否锁屏或环境根因。核对锁定 Tao 0.35.3 本地实现，最小化直接发送 `miniaturize:`；本轮读取的 [Tao 上游实现](https://github.com/tauri-apps/tao/blob/dev/src/platform_impl/macos/window.rs) 仍采用该调用。未找到可支持当前失败根因或依赖升级修复的直接证据，不修改依赖版本、底层库或系统设置。
- `ada308408` 普通包生命周期两种私有档案通过：实际通知授权均为 granted、Global 私有工作区、401 拒绝、档案凭据身份和正常宿主退出/sidecar/readiness 清理；日志 `/private/tmp/reasonix-window-modal-resize-package.log`。完整门禁失败处同样经过 runner 清理，成功/失败阶段计数按实际日志核对。**窗口稳定性继续未验收**；此次字段补充取得新的条件证据，不作为问题修复或完整 46 项通过。旧 UI 其余偏好/异常交互及其余 D、A/B/C、E、正式签名/公证门禁保持，外接屏及 Windows/Linux 继续延期。

#### D：macOS 旧界面迁移的私有来源与读取重启验收（2026-10-01）

- native 提交 `21afb89dc` 增加 opt-in `REASONIX_TAURI_LEGACY_UI_SMOKE=private-source`。仅接受规范绝对路径的 `reasonix-ui-migration-*/tmp` 私有目录、当前用户拥有的普通 0700 目录及普通 0600 来源文件；拒绝符号链接、外部工作区、额外字段和超限数据。来源仅指定 nonce 及本轮私有工作区，偏好由 native 固定为默认工作区、深入模式和较大字号；不接受任意 key/value，不新增 renderer 命令或权限。
- 隐藏 reader 使用 incognito，主线程在实际 `WKWebsiteDataStore.isPersistent()==false` 后才允许写入测试来源，要求来源 localStorage 为空。然后沿用固定旧 origin、空页面/标题核对、22 key 白名单和正常主 IPC 返回；快照必须与三项固定值完全相同才发布 0600 原生回执。真实共享旧 WebKit 仓库没有被种入、清空或导入测试值，reader 销毁和关闭即退出策略保持。
- 使用正确 `REASONIX_TAURI_BRIDGE_TEST_BIN` 指向本轮真实 Go bridge 的 Rust **202 项通过、2 项既有忽略**，严格 clippy/fmt、迁移组件回归与 diff 检查通过；首次错误命名的 bridge 环境变量运行不计为真实 bridge 验收。从干净 `21afb89dc` 完整生产构建通过，其正常旧 origin 只读 IPC及普通包生命周期两种档案均通过。随后仅修正夹具字号枚举，从干净 `f72a9b7a2285bdd478e04066d73e6e3698d33530` 再次全量构建前端/sidecar/arm64 `.app`，contract/budget 与本地 ad-hoc 签名通过，尚未正式签名/公证。
- 新增普通包 runner `smoke-native-ui-migration.py`，在托管/显式私有档案分别管理导入前、导入后、撤回后三次启动；要求实际 UI 预览、确认导入/撤回及 Cmd+Q，并独立核对身份、来源与 core canary 的字节/权限/mtime、退出和 sidecar/readiness 清理。每次启动删除上一原生回执，必须重新预览才能通过该次检查；不能沿用旧回执。runner 本身不判断 UI 功能通过，实际界面证据需另行记录。
- 首次托管 CUA 流程真实预览三项、未确认时禁用导入、确认后显示导入 3 项；Cmd+Q 后工作区与“深入”模式正确恢复。字号来源使用错误数字字符串 `18`，当前及冻结 Wails 基线 `textSize.ts` 均只接受 `small/default/large/xlarge/xxlarge`，实际 UI 将该值归一化为默认字号；此项不计为三项完整迁移通过。实际撤回显示恢复 2 项（字号已是原值），未记录撤回后的第三次重启。已将夹具修正为 `large`，没有修改产品字号读取/迁移行为。
- 有效枚举的新档案普通包已启动，但 CUA 按唯一包路径读取窗口连续两次报 ScreenCaptureKit `-3811` 捕捉错误，尚未开始此档案的 UI 导入。核实自有 runner/host/sidecar PID 后仅中止该 runner，由 finally 清理自有进程；独立 ps 确认宿主/sidecar 均已退出。失败目录/日志保留，强制清理不计正常退出验收，未推断系统锁屏或失败根因。日志 `/private/tmp/reasonix-private-legacy-ui-valid-cua.log`。
- 继续执行同一 `f72a9b7a2` 包的 `smoke-native-ui-legacy-read.py --private-source`：两种档案各两次真实包启动，每次实际主 IPC 连续读取两次，要求新回执 `readCount=2`、三项值精确匹配、非持久仓库、reader 销毁、关闭即退出偏好下主窗口保持及正常退出清理。**4 次启动/8 次读取全部通过**，重启身份和来源/canary 字节/权限/mtime 保持；成功私有夹具已删除。普通不带标志的两种档案旧 origin 只读门禁再次通过，并要求不存在私有种值回执。日志 `/private/tmp/reasonix-private-legacy-ui-read-restarts.log`、`/private/tmp/reasonix-private-legacy-ui-production-read-current.log`。回执门禁另验证缺失/已移除旧回执、错误次数、布尔/浮点类型、公开权限和符号链接拒绝。
- **完整普通 UI 导入→重启→撤回→重启未验收通过**，尤其显式档案及正确字号完整链路仍待执行；读取重启证据不能推广为这六次 UI 生命周期完成、所有 22 项偏好或真实旧用户数据已迁移。窗口稳定性、其余 D、A/B/C、E 和正式发布门禁保持；外接屏及 Windows/Linux 按用户要求延期。

#### D：macOS 官方旧版 Global 引用、附件与实际检查点回退（2026-10-01）

- 扩展 `smoke-legacy-rollback.py --workspace-data`，仍固定核对官方 Desktop/CLI 1.38.3 三个二进制摘要、bundle 身份及旧/新严格签名。复用干净 native 提交 `6d405f8f8a3734b806829e88b9a6bf72f9c700e3` 的完整生产 `.app`；本轮只修改 Python 验收及记录，没有重建或修改旧版包、host/前端/Go core。临时 HOME/core/缓存和伪 provider 归属本轮，服务只监听 127.0.0.1、随机 token 经 0600 文件及鉴权 cookie 使用，不将 token 放入 argv/URL；不继承用户凭据。
- 官方旧 CLI 的 `serve` 入口真实提交 Global 文件、YAML 附件和固定有效 1×1 PNG 的 `@` 引用。provider 要求实际展开的文件内容/文本附件正文及图片路径提示，固定返回 read_file、edit_file、最终回答，共三次请求；要求实际文件内容改变、schema v3 检查点、before/after SHA-256、原始 preimage 和兼容 marker 全部对应。同一旧版原生 GUI 在 Preview 前后恢复原历史，取得实际 PID 的 writer 租约、拒绝第二 CLI writer且不访问 provider，并正常原生退出。
- 实际 Preview 的 import/restore/explicit 三阶段通过，包含真实 bridge 修改/重启及私有备份；与旧 GUI 首次退出后的完整旧版 core 文件集合、摘要、权限和 mtime 比较全部相同。官方旧 CLI 再次恢复同一历史，重新解析当前引用并实际编辑，生成严格递增的第二个检查点；两次回复使用不同 canary，实际接口和历史文件均要求第二次回复落盘，不能将旧回复当作续写完成；原附件和首个检查点的字节/权限/mtime 保持。旧版 `/checkpoints` 确认恢复前后可实际读取代码恢复能力，不仅离线解析 JSON。
- 再次启动同一官方旧 CLI，实际 `/rewind` 最新检查点返回 204，文件从第二次编辑结果恢复到确切 preimage，主历史字节、附件及会话身份保持，provider 请求仍为六次。随后连续请求较早检查点返回 500，具体正文为 `file conflicts detected`；独立诊断确认文件和主历史未改变。最终门禁将这两项分别验证：实际恢复必须发生且拒绝 no-op，后续冲突必须为该固定错误并保持整个工作区文件摘要/权限/mtime、历史和 provider 请求数不变。**连续跨检查点回滚未通过**，不能把受保护拒绝写成第二次恢复成功。
- 完整扩展门禁及第二次独立回复落盘的最终版本本机通过，日志 `/private/tmp/reasonix-official-global-rollback-accepted.log` 与 `/private/tmp/reasonix-official-global-rollback-distinct-reply.log`；所有旧 serve、Wails、Preview 正常退出，sidecar/readiness 清理，成功私有夹具删除。最终旧 CLI currency 只读回读要求旧版 core 全文件树不再变化。原有纯文本门禁另行复验，结果记录在 `/private/tmp/reasonix-official-global-rollback-text-regression.log`。
- 首轮 runner 使用 headless `run`，源码核对确认它不展开 `@` 引用；改用官方鉴权 serve 的真实提交。又核对首次提交才分配 transcript 路径、`status.cwd` 实为历史存储目录（独立 lsof 检查实际 kernel cwd），及 turns/conflicts/guardian JSONL 是 sidecar；按旧 `store.IsSessionTranscriptName` 区分会话。早期夹具失败不是 Preview 数据损坏，日志保留；旧版连续回滚 500 的真实边界也保留，没有改旧版、放宽数据保护或重试到恢复成功。
- 配置导入仍仅复制配置及项目 root/title，不复制旧 Global 会话/附件/工作区；本证据是原件保护与退出 Preview 后旧版可用性，不能当作 Preview 已展示/迁移旧附件、图片像素传输/渲染、GUI 回滚点击、全部历史格式或旧宿主目录锁验收。窗口最小化稳定性、其余 D、A/B/C、E 和正式签名/公证继续保留；外接屏、Windows/Linux 按用户要求延期。

#### D：macOS 外观重复写入收敛与原生菜单/启动样本（2026-10-01）

- 本轮先复验 `67e7fcd8b` 完整门禁，结果 **31/46**：托管 23 项、显式前 8 项通过，显式 Settings 最小化失败。原生前后状态仍是允许最小化但 minimized=false、Will/Did/Deminiaturize 均为 0；没有额外 restore/Reopen。日志 `/private/tmp/reasonix-notification-package-full-window.log` 保留。
- 确认启动同步与页面偏好读取会重复请求同一个 NSApplication 外观。`apply` 现先在主线程读取实际显式/有效外观及 Tauri 缓存，三者满足请求时不再调用 setter；没有仅靠请求缓存跳过真实漂移。auto 必须是显式 nil 且缓存匹配系统有效外观，显式浅色不能代替 auto，显式选择和过期缓存仍更新。排队的检查只读，超时后的回调不会留下迟到的主题写入；保存/配置 CAS 回滚和原有 native 更新路径保留。opt-in 增加固定 `appearance-skipped` 计数，不增加窗口恢复请求、激活重试或 renderer 权限。
- 新增 `smoke-native-menu-window.py`：实际 active/main key window、安装菜单的 `performMiniaturize:` 与 nil responder target、已验证的 enabled 状态均必须成立；要求真实最小化、Will/Did Miniaturize、Settings 恢复及 Did Deminiaturize，设置事件仍恰好一次。它与原 Tauri `miniaturize:` API 门禁并存，不修改默认 46 项顺序和断言；成功 trace 新增最小化后的第四个状态，使通知观察器的正向工作也有实际证据。
- 使用真实 bridge 的 Rust **201 项通过、2 项既有忽略**，新增 auto/显式选择和缓存漂移回归，严格 clippy/fmt、Python 语法及 diff 检查通过。从干净提交 `6d405f8f8a3734b806829e88b9a6bf72f9c700e3` 完整构建生产前端、Go sidecar 与 arm64 `.app`，contract/budget、本地 ad-hoc 签名通过；原生菜单 runner 的严格 codesign 校验通过。
- 同一新包两种私有档案的原生 Minimize 菜单及 Settings 恢复 **2 项均通过**，各自正常退出并再启动菜单合同门禁、持久身份保持。实际最小化 snapshot 中 Will/Did=1、minimized/nativeMiniaturized=true，页面仍未完成；Did Deminiaturize 由独立严格等待和成功结果确认。随后原有 `--dialogs --edit --focus` **46/46 全部通过**（每档案 23 项），包含正常几何、外观回滚/恢复、编辑/剪贴板原件恢复、面板取消、后台实际流式任务、第二实例严格焦点和两种退出路径。日志 `/private/tmp/reasonix-appearance-idempotent-native-menu.log` 与 `/private/tmp/reasonix-appearance-idempotent-full-window.log` 保留，不能据此认定偶发问题已解决。
- 为核对新启动稳定性，增加有界 `--api-startups 0..5`：每项是新进程、同一私有档案的独立样本，失败立即停止且不重试。首次安排每档案 4 个 API 样本时，先执行的原生菜单控制阶段在首个托管启动失败，**API 尚未执行**；日志 `/private/tmp/reasonix-appearance-idempotent-startup-samples.log`。将原 API 样本放在菜单控制之前后，首个托管 API 启动同样失败，**0 项通过、1 项实际失败，其余未执行**；日志 `/private/tmp/reasonix-appearance-idempotent-api-startups.log`。没有把计划的 8 项记为已执行，也没有重试到成功。
- 两次失败都显示 appearanceRequests=appearanceSkipped=2、restoreRequests=restoreCompletions=1、Reopen=0，仍未收到 Will/Did/Deminiaturize，实际窗口可见而非最小化。请求前 active/key/main=true，超时后 active=true 但 key/main=false、主页面完成。因此问题不仅限于 Tauri API，且重复外观写入不是失败的必要条件；未证实显示/首次绘制、AppKit 或系统窗口管理中的根因。本次外观收敛保留为真实幂等修正，窗口稳定性继续保持未验收。
- 同一包独立通知 **6 次送达/精确清理复验全部通过**，2 次 active、4 次应用已隐藏，原件/身份/同档案重启保持；日志 `/private/tmp/reasonix-appearance-idempotent-notifications.log`。所有原生样本和通知进程均已终止，独立核对没有 Preview/sidecar；测试 runner 已清理自有私有目录，失败固定日志保留。
- 外接屏、Windows/Linux 继续延期；物理窗口/托盘/通知点击、其余 D、A/B/C、E 与正式发布门禁继续保留。

#### D：macOS 真实通知送达与精确清理（2026-10-01）

- 对照 Wails 的 `desktopControllerSink → notify.NewSink → PlatformSender.Send`：三类事件根据配置发送提示，macOS `osascript` 旧入口启动后异步等待，不核对实际送达。Tauri 已有 UserNotifications 后端和固定提示；此前真实包只读取授权，不能证明系统接受并送达。本轮新增 `smoke-native-notifications.py`，专门覆盖生产 `send_system_notification` host 命令、实际 bridge 会话校验、Notification Center 回执和本次通知清理，不执行 renderer IPC 或合成用户点击。
- 原生探测只在 opt-in 私有包阶段运行，先读取已有 granted/provisional 授权并拒绝已有目标的非空档案，创建自己的会话。三类固定标题/正文经正常命令及 backend 提交；只核对本次随机 identifier 的实际送达内容，不记录其他通知、token 或私有会话内容。后两次先通过 AppKit 确认应用隐藏，第一次如实记录 active 状态。`DeliveryReceipt` 正常/错误返回均只移除对应 identifier 的 pending/delivered 请求，没有使用移除全部通知的 API；成功还要求实际查询确认其已消失。超时发送的不确定目标也纳入本次清理。
- 真实 bridge 的 Rust **199 项通过、2 项既有忽略**，严格 clippy/fmt 及 Python 语法通过。从干净提交 `67e7fcd8b99bdbaea8b0ce39235c1722e1b17b84` 全量构建生产前端、Go sidecar 与 arm64 `.app`，contract/budget 及本地 ad-hoc 签名通过；runner 在运行前独立 `codesign --verify --deep --strict` 通过。不是正式 Developer ID 签名或公证。
- 同一包托管/显式两种新私有档案共 **6 次真实通知送达全部通过**：每档案 turn_done、approval_request、ask_request 各 1 次，真实标题/正文符合固定提示。两次首次发送时 AppKit 实测 active=true；另外四次实际 hidden=true、active=false。每次只移除自己 token 的通知并确认不存在；两种档案实际宿主正常退出、sidecar/readiness 目录清理、canary 字节/权限/mtime 不变与同档案再启动的持久凭据身份保持。成功私有目录已删除，随后独立确认没有 Preview/sidecar；固定日志 `/private/tmp/reasonix-native-notification-packaged.log`。
- 本轮证明实际 OS 送达及前台/隐藏状态下的后端链路，不证明屏幕可见横幅、用户点击、冷启动点击、真实授权拒绝、前端事件到通知设置/错误反馈的完整 UI 流程或钥匙串拒绝。没有请求新系统权限、改动通知设置或扩大 WebView 权限，相关待办保留。
- 最小化调用链继续核对：锁定的 `tauri-runtime-wry 2.11.4` 在 `send_user_message` 中对主线程直接调用 `handle_user_message`，Minimize 进入 `tao 0.35.3::set_minimized → NSWindow::miniaturize`。现有 smoke 的 `on_main` 已在主线程调用此 API，不能再把失败解释为仍在 Tauri 队列里等待；AppKit 动画/状态根因仍未证实。最新完整窗口证据仍为 `5f153aae7` 的 8/46，不能推广到本次包已稳定；此前独立切片也不代替窗口门禁。
- 外接屏及 Windows/Linux 延期范围不变，D→E 目标继续进行中。

#### D：macOS 独立验收切片与窗口事件诊断（2026-10-01）

- 当前原生包从干净提交 `5f153aae745719f531ed8dad31ca3b61bbdb8ff1` 完整构建生产前端、Go sidecar 和 arm64 `.app`；contract/budget、本地 ad-hoc 签名、真实 bridge 的 Rust **199 项通过、2 项既有忽略**及严格 clippy 通过。此次 native 修改只有 opt-in 诊断：在 AppKit 主线程给实际主窗口注册最小化/取消最小化及 key-window 通知观察器，退出时移除；外观计数记录请求而非完成，不新增 renderer 权限，不改变窗口动作。
- 同包完整 `--dialogs --edit --focus` 门禁 **8/46**：托管档案前 8 项通过，`menu-settings-minimized` 失败，尚未进入显式档案。失败前 key/main/active=true、主页面未完成；超时后 active/key/main=false、页面完成，窗口始终可见且未最小化，styleMask=32783 且允许最小化。Will Miniaturize、Did Miniaturize、Did Deminiaturize 均为 0，key-window 计数有变化；恢复请求/完成各 1、Reopen=0，外观请求从 1 到 2。仅排除已有额外恢复请求和“已观察到最小化再恢复”的说法；尚不能认定 Tauri 队列、AppKit、页面完成或外观同步是根因。独立最小化短探测两种档案均通过，也不能消除完整门禁的失败。
- 新增 runner 的 `--independent`，仅选择不需要已保存窗口几何的菜单组合、剪贴板 IPC，以及可选的严格原生编辑和真实面板取消。默认完整顺序、几何/最小化断言保持；`--focus` 与此选项明确拒绝组合。该切片不验证窗口/Settings 恢复、后台任务、菜单退出或第二实例，不把部分通过变为完整通过。
- 使用同一 `5f153aae7` 原生包执行 `--independent --edit --dialogs`，托管/显式两种新私有档案 **8 项全部通过**：真实菜单组合与共享保留表、主 WKWebView 文本剪贴板 IPC 和图片/隐藏窗口拒绝、4 类实际 AppKit 面板取消、真实 active/key-window 的 Copy/Paste/Cut/Select All/Undo/Redo。系统剪贴板完整原件恢复、同档案连续重启的凭据身份、实际 sidecar 鉴权及正常退出清理保留。日志 `/private/tmp/reasonix-independent-native-gates.log`；完成后独立确认没有 Preview/sidecar，私有档案由 runner 删除。此切片只修改 Python 和文档，未重建或更改原生包。
- 真实托盘点击另用普通私有托管档案尝试。准确核对宿主/直属 sidecar 后，绑定产品窗口成功；读取系统菜单栏的 CUA 接口约 952 秒后超时，未点击 Show/Quit，900 秒 fixture 超时强制回收自己的宿主不计正常退出验收。随后核对记录 PID 均不存在、无关联 sidecar/readiness 且 canary 原件保持，再清理自有临时目录；失败日志 `/private/tmp/reasonix-tray-cua.log` 保留。没有完成托盘点击、没有把工具超时当作产品托盘失败。
- 外接屏/不同缩放/拔插继续按用户要求延期；其余 macOS D、A/B/C、E、正式签名/公证和发布授权门禁继续保留。

#### D：macOS 产品输入框实际复制/剪切/粘贴（2026-10-01）

- 同一生产包 `30a93592d` 在私有托管普通档案中、没有自动 smoke 退出标记，用 CUA 定位实际产品输入框并通过原生 Cmd+A/C/X/V 操作唯一测试文本。输入框剪切后为空且发送按钮禁用，粘贴后精确文本恢复；独立 AppKit helper 核对系统文本和实际 pasteboard changeCount。清空草稿后实际 Cmd+Q 退出码 0，持有进程句柄的 runner 确认 sidecar/readiness 目录清理、档案身份及 canary 原件不变。
- 扩展测试 helper 的 `claim` 仅认可捕获快照 nonce 对应的唯一文本、单个 pasteboard item 和稳定代次，既不写系统剪贴板，也不打印原件；在私有命名 pasteboard 的多格式恢复、代次/外部修改保护自测通过。退出后完整恢复捕获的所有原剪贴板 item/type，独立内容比较通过；成功私有 fixture 已删除，固定结果保留在 `/private/tmp/reasonix-startup-cua-result.json`。
- 首次 fixture 因 helper 自测调用漏传参数在编译时停止，尚未备份/写系统剪贴板或启动 Preview；修复后编译和命名 pasteboard 自测通过。第一次实际 UI 尝试已观察复制/剪切/粘贴，但超过 180 秒未完成正常 Quit，runner 的自有宿主强制回收不计通过；原剪贴板完整恢复。保留失败日志后改用有界较长 fixture 时限，完整短流程重新通过。全应用 inventory 也曾超时；之后只绑定已核对活着的确切私有 `.app`，退出后不查询应用，避免启动普通档案。
- 第一次实际 UI 尝试发出了 Cmd+M，但 AX 树没有暴露可确认的最小化状态，随后绑定不活跃；不标记本轮 Cmd+M 已通过，不将它当作最小化根因证据。31/46 原生窗口门禁的明确失败继续保留。
- 两份失败私有 fixture 在核对 owner-only 目录、记录进程均不存在、无关联 sidecar 且原剪贴板恢复快照已移除后清理；独立确认没有运行中的 Preview，失败固定日志继续保留。
- 本轮范围为普通托管档案的真实产品输入框；没有发送消息/调用模型，不等同于消息/设置路径复制按钮、上下文菜单、实际权限拒绝反馈、可配置快捷键、显式档案完整 UI 或全部剪贴板功能已验收。其他 D、A/B/C、E 和正式发布门禁保留。

#### D：macOS 启动握手阶段宿主退出保护（2026-10-01）

- 对照 Wails 的 `OnShutdown → app.shutdown → shutdownBody`：正常退出时停止同进程 controller；Tauri 的 Go runtime 在独立子进程中，不能只依赖原生宿主退出回调。此前 parent watcher 在读取完整 stdin token 后才安装，握手中断时尚无早期清理权。现将观察提前到 token 读取前；取消会关闭本次输入 reader，打断仍等待换行的有界读取，不能将 token 遗留给 core 子工具。
- Rust 在 0700 私有目录中、spawn 前以 `create_new` 和 0600 写入 `launch-owner.json`，只含宿主 PID 和随机 launch ID，不含认证 token 或用户数据。Go 有界读取并核对 ID、PID、文件/目录 UID、权限、非别名和 inode，既不靠 PID 存在性接管进程，也不向其他进程发信号。匹配记录的 kernel parent 已改变时，拒绝读取 token/创建 core 并清理自己的启动记录与空目录；缺失、错误、替换和其他文件不会获得扩大删除权限。就绪后的 instance ID 清理仍保留。
- 新增真正阻塞在半截管道 token 上再取消的回归；启动记录的错误 ID/PID、权限、符号链接、目录替换及其他原件保护回归通过。完整 Go bridge、相关 race、严格 clippy/fmt 通过，使用新真实 bridge 的 Rust **199 项通过、2 项既有忽略**。
- 从干净提交 `30a93592d79c4b5291dca530c8fe6ddeb55d12a2` 全量构建生产前端、Go sidecar 和 arm64 `.app`，contract/budget 与严格本地 ad-hoc 签名通过。`smoke-native-startup.py` 的 **12 项全部通过**：托管/显式档案 × SIGTERM/SIGKILL × 父校验前/token 读取前/token 已读取但 core 尚未启动。opt-in 私有探测仅暂停真实启动边界，外部 runner 核对实际直属进程后终止自有宿主，kernel 对每个 sidecar 确認正常退出码 0；启动记录/ready 目录清理、原件内容/权限/mtime 保持、同档案正常重启及持久身份保持通过。失败强制回收不计为通过，成功 fixture 已删除。
- 同一包就绪后的空闲/实际流式 SIGTERM/SIGKILL **8 项复验全部通过**，provider 在 fixture 关闭前实际断开，sidecar/ready 目录清理、目录锁释放与同档案重启保持。启动门禁覆盖已经 spawn 的 sidecar；不宣称宿主在创建目录至 spawn 前被终止的全部临时目录回收已验证。
- 同一新包普通生命周期两种档案通过，实际通知授权均为 granted；只读权限查询不证明横幅或点击已验收。
- 为最小化继续加入只读、opt-in 的恢复请求/回调执行及 Reopen 计数，没有更改窗口动作、强制激活或放宽断言。同包完整 `--dialogs --edit --focus` 门禁 **31/46 阶段通过**：托管档案 23 项全过（包括编辑、剪贴板原件恢复、任务/Menu Quit、面板、第二实例严格焦点），显式档案前 8 项通过，在 `menu-settings-minimized` 的最小化前提停止。请求时真实 key/main=true、mainPageFinished=false；超时后 key/main=false、mainPageFinished=true，始终 minimized=false。期间 restoreRequests=restoreCompletions=1、reopenEvents=0，排除了额外恢复/Reopen 请求的假设；尚不能认定页面加载、外观同步或系统状态是原因，也不能宣称本次启动修复解决了最小化问题。
- 原生最小化稳定性、完整旧 UI 偏好导入/撤回、实际通知横幅/点击及其余 D 项继续推进；A/B/C、E 与正式 Developer ID 签名/公证门禁保留。外接屏、Windows/Linux 继续按用户要求延期。

#### D：macOS 强制宿主退出后的 sidecar 生命周期（2026-10-01）

- macOS 原生 launcher 现在传递实际宿主 PID。Go bridge 在创建档案/就绪文件前检查 kernel parent 身份，并持续观察父子关系；宿主退出或被 SIGKILL 后取消自己的 HTTP/SSE 上下文，先停止实际任务，再关闭 listener、释放目录锁及执行清理。不向其他 PID 发信号，也不通过 PID 存在性判断所有权；没有 `--host-pid` 的独立 bridge 客户端行为保留。没有新增 renderer 命令或 capability。
- Rust 创建的 readiness 目录显式设为 0700；bridge 只在成功移除匹配 instance ID 的 ready 文件后，移除同用户、同 inode、非别名且仍为空的原目录。最初包使用 tempfile 默认 0755，安全清理拒绝该目录；修复创建权限，没有放宽所有权条件或递归删除。
- 流式强制退出的第二个真实缺陷由 kernel 退出回执确认：宿主关闭了日志管道 reader，Go 在清理期间写 stderr 被 SIGPIPE(13) 终止。仅实际 macOS 受管 bridge 使用 `signal.Notify(SIGPIPE)`，让写入返回 EPIPE 并完成清理。真实子进程回归证明普通进程仍按默认 SIGPIPE 退出、受管写入返回 EPIPE、exec 的 shell 没有继承忽略 SIGPIPE 的行为；未使用会影响子工具的 `signal.Ignore`。
- 新增 `smoke-host-lifetime.py`。干净提交 `715f8743c83392ccbe4c02c1807d4958ae01b89b` 全量构建前端、Go sidecar 与 arm64 `.app`，contract/budget 和严格本地 ad-hoc 签名通过。同一包 **8 项全部通过**：托管/显式档案 × SIGTERM/SIGKILL × 空闲/实际流式任务。先核实自有宿主/直属 sidecar 的 UID、出生身份和完整路径，只终止持有进程句柄的宿主；kernel kqueue 对每个 sidecar 确认正常退出码 0。实际 provider 连接在 fixture 停止前断开，sidecar、ready 文件和目录均清理；原文件内容/权限/mtime 保持，同档案重启保留持久身份并成功获取目录锁。runner 的失败强制清理不计为验收。
- 完整 Go bridge 回归、涉及宿主观察/目录保护/SIGPIPE 的 race 回归、严格 Rust clippy/fmt 通过；使用此包实际 Go sidecar 的 Rust **199 项通过、2 项既有忽略**。普通安装包生命周期两种档案通过，实际通知授权均为 granted。
- 此包完整 `--dialogs --edit --focus` 窗口门禁在首个 `exercise` 的最小化前提停止：实际 applicationActive=true、nativeMiniaturizable=true，但 minimized/nativeMiniaturized=false，主窗口不是 key/main。保留失败记录，不放宽断言、不将旧包 46 项通过推广到新包，也不能推断与本次 Go 生命周期修复的因果关系。窗口最小化稳定性继续定位。
- 同包另外独立执行 **10 个正常阶段全部通过**（每档案菜单组合、后台真实流式任务/Menu Quit、clipboard-native、四类 AppKit 面板取消、严格 menu-editing）。先启动并正常退出，读取原生保存的实际普通窗口 frame，供任务原有严格前后几何检查使用；没有伪造几何回执或取消任务断言。真实关闭后流继续、历史完成、菜单恢复及第二个运行任务中的 Quit/provider 断开通过；编辑/剪贴板沿用原件恢复保护。该独立验收不覆盖最小化/最大化恢复或第二实例焦点，不算完整 46 项通过。成功 fixture 已清理。
- 三次早期强制退出失败的私有 fixture 已逐份核对 owner-only 目录、记录的宿主/sidecar PID 均不存在且无关联 sidecar 后清理；固定失败与 kernel SIGPIPE 诊断日志保留。没有操作用户档案或未知进程。
- 8 项覆盖 **readiness 完成后的**空闲和流式宿主丢失；token/启动握手完成前的强制终止与临时目录回收仍需单独验收。完整旧 UI 偏好导入/撤回、实际通知横幅/点击、其他 D、A/B/C、E 及正式签名/公证门禁继续保留。外接屏与 Windows/Linux 按用户要求延期。

#### D：macOS 旧界面偏好显式迁移与撤回（2026-10-01）

- 新增设置→存储与路径→旧 Preview 界面偏好。仅在用户点击预览后，从旧 `tauri://localhost` 的无产品脚本隐藏文档读取 22 个固定界面 key；检查来源、总量/单值边界和返回类型，主窗口导航仍限定当前档案。无 capability 的读取窗口不运行产品页面，不允许 renderer 选择来源或任意 key；自定义命令总入口仍限制真实 `main` 调用者。读取窗口直接销毁，主窗口退出偏好与几何事件也限定 `main`，避免“关闭即退出”误退出整个应用。
- 用户核对显示的旧工作区/偏好并勾选后才可导入。导入前先保存完整 before/after 回退记录；源仓库不修改，已有迁移记录阻止覆盖。写入失败会尝试恢复，失败的回退记录保留供重启后重试。撤回仅恢复仍匹配导入值的项目，保留后续用户修改并报告冲突；外观缓存服从当前档案已保存的原生外观配置，核心配置/会话/凭据仍使用各自入口。
- 新增组件及存储故障回归：未确认/未知 key/错误来源/非法工作区/无效 JSON/越界拒绝，已有值和无关项保护，组件重挂载后撤回，后续修改保护，写入失败且恢复也失败后的重启恢复，回退记录无法保存时零偏好写入均通过。`pnpm test:tauri`、类型检查、严格 clippy/fmt 与使用真实 Go bridge 的 Rust **199 项通过、2 项既有忽略**。
- 首次构建因中文共享语言包新增文案越过 78.9 KiB 门禁而停止；将三语言文案放入按需 Tauri 设置资源，没有增加预算。从干净提交 `3225f43c3cbddb7b26fbcbda421e5194b7b120f9` 全量构建前端/Go sidecar/arm64 `.app` 成功，contract/budget 和本地严格 ad-hoc 签名通过，正式签名/公证保留。
- 同一包两种私有档案的 `smoke-native-ui-legacy-read.py` 通过：真实主 WKWebView IPC 连续两次读取旧 origin，检查快照相同、当前 origin 不变、读取窗口已销毁，主窗口设为关闭即退出仍存活并正常退出。扩展的双 WKWebView 权限门禁也在两种档案通过：主窗口 catalog/可执行目标检查、第二窗口 8 个文档及 3 个查询（含旧 UI 读取）拒绝、伪造窗口参数无效、原件保护和退出清理。普通生命周期两种档案通过，实际通知授权均为 granted。
- 在私有普通档案用 CUA 点击实际设置/存储/旧偏好预览：旧工作区及长 JSON 偏好显示、勾选框为未确认、导入按钮禁用；滚动后内容换行和按钮可见。返回工作区仍没有默认工作区，证明只读预览未隐式导入；Cmd+Q 退出码 0、sidecar/readiness 无残留。本次未确认或导入无法可靠归属的旧共享值，真实 UI 的完整导入/重启/撤回仍需受控私有来源验收，不能用组件回归直接标记通过。
- 第一次临时 UI runner 的 readiness glob 写错后曾用 SIGTERM 终止自有宿主，留下其 sidecar；这不算正常退出通过。按私有 HOME、PID/UID、可执行路径和出生身份独立核对后，仅停止该残留进程；确认无 Preview/相关 sidecar 后清理两次私有 UI fixture，仅保存固定状态。**新增待办：宿主遭强制终止后的 sidecar 生命周期保护**，不能将正常 Cmd+Q/Close/Menu 验收推广为异常终止清理已通过。
- 旧偏好原件未清空，未发送消息或调用模型；通知实际横幅/点击、其他 D、A/B/C、E 和正式发布门禁继续保留，外接屏与 Windows/Linux 仍按用户要求延期。

#### D：macOS 持久界面存储档案边界修复（2026-10-01）

- 实际全新私有 HOME/core 继承旧工作区的问题已修复。macOS 生产包在创建主窗口前取得现有持久档案身份，以 `reasonix-preview://<profile-id>.localhost/` 作为 UI origin；不同档案使用不同 origin，同档案重启及移动保留身份。主窗口导航仅接受当前档案 origin；自定义协议只通过 Tauri asset resolver 提供编译资产，保留 MIME/CSP，不提供用户文件或网络代理，不扩大 capability。开发服务器 URL 不变，Windows/Linux 不在本轮验收范围。
- 严格 clippy/fmt、使用真实 Go bridge 的 Rust **198 项通过、2 项既有忽略**。从干净提交 `673c3002314b85534e774b6176b2b967c2a367a5` 全量构建生产前端、Go sidecar、arm64 `.app`，contract/budget 和严格本地 ad-hoc 签名通过；正式 Developer ID 签名及公证未执行。
- 新增 `smoke-native-ui-storage.py`：两种目录模式各用两个私有档案，实际 WKWebView 检查初始空值、写入、重启后精确值及产品工作区按钮渲染、跨档案返回和仅清理自有测试值。**18 阶段全部通过**，身份在重启中保持、档案之间不同；未清空系统 WebKit 仓库或读取/迁移旧共享偏好。
- 同一生产包完整 `smoke-native-window.py --dialogs --edit --focus` 的 **46 阶段全部通过**；同一新 origin 的双 WKWebView 文档/自定义 bridge 权限门禁两种档案通过，主窗口正向 catalog 调用、可执行目标拒绝、第二窗口 8 个文档和 2 个查询拒绝、伪造调用窗口无效及原件保护保持。普通安装包生命周期两种档案通过，实际通知授权均为 `granted`，正常退出后无 sidecar/readiness 残留。
- **兼容性尚待完成：**旧 `tauri://localhost` 共享界面偏好保留，新的档案 origin 首次运行使用默认界面偏好。不能静默归属并导入未知档案的旧工作区；需补上可确认来源的显式迁移及回退。此项与 core 配置/会话导入是不同验收范围，现有数据导入通过不代表界面偏好迁移已完成。
- 系统通知横幅及点击仍待实际验收；此前准备因存储混用在发消息前停止，没有通知通过证据。旧最小化失败的原因未确认，不能将当前回归通过称为该失败的根因修复。外接屏/Windows/Linux 按用户要求延期，其余 D、A/B/C、E 与正式发布门禁继续保留。

#### D：macOS 原生窗口诊断与完整回归（2026-10-01）

- 加入仅 opt-in native 验收启用的页面加载观察及主线程只读 NSWindow 状态：实际 key/main window、可成为 key、当前 Space、miniaturizable/miniaturized/visible、页面 Finished。设置阶段记录 Ready/show/minimize-requested 固定状态；runner 失败后只输出有界的布尔白名单，临时目录清理前保留证据。没有新增 renderer 命令、权限、强制激活、状态重试或放宽断言。
- 严格 clippy/fmt、真实 Go bridge 的 Rust 196 项/2 项既有忽略通过；从干净提交 `e3e03058637bdc0b4847d16e8af26547e43fe92e` 完整构建生产前端、Go sidecar、arm64 `.app`，预算/contract 与本地严格 ad-hoc 签名通过。独立最小化阶段通过：Ready 和 show 时实际主窗口已是 key/main，主页面尚未 Finished；不能把等待页面加载视为已证实修复，也不能称诊断解决了旧失败根因。
- 同一包 `smoke-native-window.py --dialogs --edit --focus` 完整 **46 阶段（每档案 23 项）全部通过**。包括原先最小化/设置恢复，真实 WKWebView 原生编辑与剪贴板原件保护、第二实例实际 active/key-window 焦点、四类真实面板取消、外观回滚/恢复、后台流式任务和原生菜单/close 退出清理。两种档案的普通包生命周期也通过，实际通知授权均为 `granted`，只读查询没有弹出授权提示。
- 随后的产品通知准备使用全新 HOME/core，却在真实 UI 观察到上一私有档案的默认工作区。这证明 host/core/凭据隔离不能推广为 WKWebView UI 存储隔离；系统 WebKit 默认数据仓库没有随 HOME 切换。已在发送消息前 Cmd+Q，真实退出码 0、sidecar/readiness 清理，provider 请求数为 0，没有通知发送或点击验收。下一步优先修复持久 UI 存储的档案边界，保留旧 UI 偏好迁移和回退要求，再恢复通知测试。

#### D：macOS 产品界面与原生输入验收（2026-10-01；外接屏仍延期）

- 复用干净 native 提交 `2cbed72bbaed1f427d035227c16e218571a95f3d` 的正式 Preview 标识、本地 ad-hoc 包；本轮未修改 native/前端/Go 源码或重建包。当前 CUA 能读取实际 `tauri://localhost` 页面并操作原生界面，不能继续将此前捕获/激活失败视为所有 UI 验收均不可推进。
- 在 owner-only 私有 HOME/TMPDIR 中启动普通 Preview，不设置 native smoke 标志，先核对真实 sidecar 和 401 拒绝。产品 textarea 中键入唯一测试文字，Cmd+A/Backspace 后为空；Cmd+Z 恢复原文及选择，Cmd+Shift+Z 再变为空。随后 Cmd+Z 恢复，点击实际 Edit→Redo，输入框再次为空。没有发送消息或调用模型；这证明 CUA 输入和实际 responder 行为，不等于人工硬件键盘验收，也不等于完整 `--edit` 门禁通过。
- 实际最小化按钮曾显示缩小后的窗口缩略图，Raise 恢复整窗。本轮 Cmd+M 同样收起窗口，随后从原生应用菜单点击 Settings…，整窗恢复且实际设置页面出现。该观察不代替自动阶段的 `is_minimized()` 断言；没有降低断言或增加强制激活重试。
- 点击工作区按钮，使用真实 AppKit 目录面板定位私有中文/空格目录并确认 Open。页面显示正确默认工作区、项目侧栏及信息栏；实际文件面板选择目录内的 `说明.txt`，页面显示唯一待发送附件，随后移除。Cmd+Q 后原宿主退出码为 0，无 sidecar/readiness 残留；用同一私有 HOME 重启，默认工作区和项目侧栏正确恢复，附件未遗留。此范围只覆盖默认托管档案、目录/文本文件正向选择，其他面板取消/另存为/覆盖及完整 Global 会话应用操作继续保留。
- 重启后的实际设置页点击深色模式，选中状态与整窗截图均呈深色；回到通用设置选择“退出 Reasonix”，选中状态改变。点击真实窗口关闭按钮后宿主退出码为 0，无 sidecar/readiness 残留；私有 `host-preferences.json` 也保存 `closeBehavior=quit`。未修改系统主题或通知/钥匙串权限，不把整窗颜色观察推广为全部原生标题栏、系统主题切换或整包外观重启验收。
- 退出后的界面状态查询报捕获错误，并在同一秒出现了没有私有 HOME/TMPDIR 的新 Preview 进程。核对完整可执行路径、启动时间及其原直属 sidecar 后，仅清理这次新启动进程；其终止不计入正常退出验收，也不声明普通档案启动完全没有副作用。后续在 Cmd+Q/关闭即退出后只检查已持有的进程句柄与私有文件，不再查询已退出应用，以免工具再次启动普通档案。
- 单独用 LaunchServices 的 `open -n -W` 和显式私有环境启动同一包，`menu-settings-minimized` 仍在真实最小化前提失败：`applicationActive=true, focused=false, minimized=false, visible=true, scale=1`，几何为默认 1280×820；宿主及 sidecar/readiness 已清理。本轮私有 UI/启动探测目录在独立确认无 Preview 或相关 sidecar 后已清理，仅保留固定验收状态。不能将原失败归因于 runner 直接执行二进制，也不能将上述实际界面通过记为 42 阶段自动门禁全过。外接屏、Windows/Linux 延期保持，其他 D 与 A/B/C、E、正式签名/公证门禁继续保留。

#### D：自定义 native/bridge 主调用窗口边界（2026-10-01）

- 现有 8 个文档/工作区应用命令已在各自入口校验 `window.label()==main`；增加真实主/第二 WKWebView IPC 探测后，实际同源第二窗口通过这 8 项拒绝，却能调用 `desktop_preferences`。探测只丢弃返回值并发布固定 `unexpected-native-access` 状态，不保存偏好内容；该结果不能视为远端页面获得权限，当前 Tauri 对非本地来源仍执行 ACL 检查。
- 当前锁定 Tauri 2.11.6 的 `webview/mod.rs` 明确区分插件、有 app ACL manifest 和远端来源的 ACL；当前项目只有 `main-window` 插件 capability，没有 app ACL manifest。现已在唯一自定义 `invoke_handler` 总入口按 Tauri 注入的 `invoke.message.webview_ref().label()` 校验主窗口，其他窗口固定拒绝后停止分发，覆盖已注册 native/bridge 命令；前端传入 `window:'main'` 不能伪造调用者。插件继续先走 Tauri 的插件/ACL 分支，现有主窗口权限未扩大。
- 新增 `tools/tauri/smoke-document-scope.py`：主窗口真实 IPC 要求直接可执行文档及其符号链接别名在默认/指定应用入口均拒绝，并读取实际 Finder/Terminal PNG catalog；同源隐藏窗口测试 8 个文档命令及桌面偏好/bridge 状态拒绝。独立控制 nonce 与原生窗口标签共同绑定回执，隐藏窗口不能提供主窗口通过回执。修复后的真实包默认/显式两种档案均通过：4 次主窗口可执行目标拒绝、catalog 正向调用和 10 次第二窗口拒绝；原文档、可执行 canary 和别名的内容/权限/mtime 保持，executable 未执行，宿主正常退出且 sidecar/readiness 无残留，成功档案已删除。未增加 renderer 命令或权限。
- 首个 native-only 私有探测未启用依赖的 `tauri/custom-protocol`，停在主页面加载前提，不是权限验收结果；按生产特性重建并复用原前端/sidecar 后，实际复现第二窗口的偏好调用未被拒绝。此私有探测包不能替代修复后的干净全量 `.app`；普通 Preview 包没有被该副本替换。该门禁不证明 Finder/编辑器打开、对话框实际选中、物理点击或全部会话/资源边界，其他 D、A/B/C 与 E 门禁保留。
- 修复后 Rust 使用真实 Go bridge 的 196 项通过/2 项按原计划忽略，严格 clippy/fmt、`pnpm test:tauri`、Python 语法和 diff 检查通过；从干净提交 `2cbed72bbaed1f427d035227c16e218571a95f3d` 完整构建生产前端/Go sidecar/Rust arm64 `.app`，前端 contract/budget 和严格本地 ad-hoc 签名通过。正式签名/公证尚未执行。
- 此包 42 阶段原生门禁的本轮复验尚未全过：托管档案前 8 阶段通过，在 `menu-settings-minimized` 的真实最小化前提停止，未执行后续设置菜单恢复动作，不能推广旧包 42 项通过为此轮全部通过。原始状态为 `applicationActive=true, focused=false, minimized=false, visible=true, scale=1`。独立只读 AppKit 查询当前仅一台 1× 显示器、Chrome 前台，与此前单屏 2×/Codex 的观察不同；不推断锁屏、设备原因或权限修复导致失败，也不降低最小化断言。该窗口前提继续单独调查，外接屏延期范围不变。
- 因总入口改动直接涉及插件/custom 分发隔离，另对同一包独立执行实际 `clipboard-native` 阶段的默认/显式档案：主 WKWebView 文本写入/读回、图片拒绝和同源隐藏窗口文本读写拒绝均通过；沿用多格式原件备份、独立系统值/代次验证及恢复保护，完整原件恢复通过。这不推广为受最小化前提阻断的其他原生阶段或物理按键通过。

- 同一干净包的最终权限脚本及普通包生命周期两种档案均通过：实际通知授权读取、Global 私有工作区、凭据身份、401 拒绝和退出清理同时核对。独立确认无运行中的 Preview 或私有探测进程后，已清理两次探测 fixture 和自有 .app 副本，日志仅保留固定接受/拒绝状态。

#### D：macOS 实际 Terminal 工作目录验收（2026-10-01）

- 对照 Wails 和 Tauri 的 macOS 系统 Terminal 路径：通过系统 `/usr/bin/open -a` 打开目录，文档目标先转换为所属目录。新增 `tools/tauri/smoke-native-terminal.py`，由真实 Preview 主 WKWebView IPC 分别打开私有目录及其中的文档，要求两个新系统 Terminal shell 的实际 kernel cwd 均为该目录；并非只核对生成的命令参数。
- 目录包含中文、换行、引号和 `$`。本机 `lsof -F0` 仍把路径内换行显示成转义文本，不能用于原始字节判定；新增独立私有 C helper，用当前 SDK 的 `proc_pidinfo(PROC_PIDVNODEPATHINFO)` 读取原始 cwd，最多接收 64 个显式 PID，先检查同用户 UID，输出有界 PID/NUL/path/NUL 字段。测试进程独立特殊路径探测已通过，未弱化路径相等断言。
- runner 只筛选启动前不存在、实际系统 Terminal 祖先链下的会话根 shell，排除 shell 启动/配置时派生的子 shell；核对出生时间、可执行名称及私有 cwd，成功后只回收本次 shell。身份/目录改变时停止回收并保留私有记录，baseline 和已观察到的所有权保存在私有记录中。可能留下可手动关闭的已结束测试窗口，不操作既有窗口或请求 Apple Events/录屏权限。实际包默认/显式两种档案已全部通过：每种均观察到两个新系统 Terminal 会话的精确 kernel cwd，同时核对 sidecar 鉴权、凭据身份、原文档内容/权限/mtime、宿主退出和测试 shell 回收；成功档案已删除。
- 首次探测在额外 shell 计数时停止，原筛选会把会话根 shell 的启动子 shell 也计入；调整为会话根 shell 后，仍保留两个新会话、原始 cwd 相等和清理断言。首轮唯一剩余测试会话已根据 PID/出生时间、系统 Terminal 祖先及私有 cwd 独立核对并回收。未将计数失败当成产品工作目录缺陷，也没有用命令参数或 `$PWD` 环境值代替 kernel cwd。
- Rust 196 项/2 项按原计划忽略、严格 clippy/fmt、Python 语法和独立 C helper 编译/特殊路径读回通过；从干净提交 `a9217f8288b45243552e66b3b3d21bcf464e7a54` 完整构建生产前端/Go sidecar/Rust arm64 `.app`，前端 contract/budget 与严格本地 ad-hoc 签名通过。首个构建被独立 helper 测试留下的 pyc 标为 dirty；已删除该缓存，完成同一提交的无 dirty 完整重建，再执行实际 Terminal 验收。之后仅修正 Python 会话根 shell 筛选及记录，native 源码和验收包未改动。
- 同一新包的应用启动拒绝、默认浏览器实际请求及普通包生命周期均在两种档案复验通过；实际通知授权读取、Global 私有工作区、凭据身份、401 拒绝和退出清理同时核对。测试后已独立确认无运行中的 Preview，首轮私有目录亦无剩余 shell/sidecar，已清理首轮失败 fixture。
- 本门禁不证明 Finder/其他终端/编辑器、物理快捷键、工作区选择器点击或项目/Global 会话的整条 UI 打开流程；原生编辑/焦点和其余 D 待办保留，外接屏及 Windows/Linux 继续延期，E 仍等待 D 验收和 Preview 稳定。

#### D：macOS 指定应用启动失败反馈（2026-10-01）

- 对照 Wails `darwinExternalOpenerCommand` 与 Tauri `launch_with_opener`：两者的 Ghostty 路径均使用 `/usr/bin/open -na ... --args --working-directory=...`；Tauri 原专用分支在 spawn 后立即返回成功并后台忽略退出码。实际 LaunchServices 拒绝损坏应用时，调用方仍会收到成功。
- macOS Ghostty 分支现等待系统启动器完成交接，检查非零退出码并返回“问题 + 重新选择已安装应用”的错误。最多等待 10 秒；超时/等待错误只结束并回收本次启动器，不能关闭已运行的目标应用或用户终端。参数仍独立传递，标准输入/输出/错误不进入 UI，不增加任意命令入口。
- 新增真实 LaunchServices 拒绝不可启动私有 `.app`、启动器零退出码及超时回收的三项 Rust 回归；使用真实 Go bridge 的完整 Rust 196 项通过、2 项按原计划忽略。`tools/tauri/smoke-native-app-failure.py` 在私有 HOME 内创建声明了缺失 executable 的 Ghostty 测试 bundle，经生产 catalog、本地文件校验和主 WKWebView IPC 测试未知 ID/实际启动拒绝；宿主先确认 catalog 目标恰好为私有测试 bundle，否则停止，不能操作真实已安装 Ghostty。实际包的默认/显式两种档案已执行通过，要求错误包含固定解决提示、原文档不变、正常退出及 sidecar/readiness 清理，成功后的私有档案已删除。
- 私有文档路径包含中文、换行、引号和 `$`，失败后检查内容、mtime/权限不变；两种档案分别核对 sidecar 鉴权、凭据身份、正常退出和清理。该门禁不证明真实 Finder/编辑器/终端正常打开、终端 cwd、物理点击或界面错误 toast 已渲染；相关 macOS D 待办保留，Windows/Linux 和外接屏仍按用户要求延期。
- 严格 clippy/fmt、Python 语法与 diff 检查通过；从干净提交 `c83f947e5abd2f1736d5ca79c722395e03d6a112` 完整构建生产前端/Go sidecar/Rust arm64 `.app`，前端 contract/budget 与本地严格 ad-hoc 签名通过。共用 WebView 回执通道改动后，同一新包的实际浏览器门禁两种档案复验通过，普通包生命周期的两种档案也通过（实际通知授权读取、Global 工作区、私有凭据身份、401 拒绝与退出清理）；正式签名/公证和其余 D/E 项仍待完成。

#### D：macOS 默认浏览器实际打开验收（2026-10-01）

- 新增 `tools/tauri/smoke-native-links.py` 与仅在 opt-in `external-browser` 阶段运行的宿主探测。主 WKWebView 经实际 IPC 调用现有 `open_external_link` / `open_external_url`，拒绝文件/应用 scheme、userinfo、NUL、邮件附件参数及 OAuth 邮件入口，再由系统默认浏览器打开两个私有 loopback canary 页面。
- runner 验证两个浏览器请求、真实 sidecar 身份/未鉴权拒绝、私有凭据档案和退出清理；默认托管与显式 core 目录分别执行，不继承用户凭据环境，不增加 renderer 命令或权限。页面不加载外部资源，可能留下可手动关闭的本机测试标签页；不关闭用户既有页面。真实包两种档案均已执行通过，两个浏览器请求与主 IPC 成功回执同时满足，宿主正常退出且无 sidecar/readiness 残留。
- Rust 使用真实 Go bridge 的 193 项回归通过、2 项按原计划忽略，严格 clippy/fmt 通过。从干净提交 `185b7223de9a6568f9ae2a97487606e223407dd4` 完整构建生产前端/Go sidecar/Rust `.app`，前端 contract/budget 门禁及严格本地 ad-hoc 签名通过；此签名不等于 Developer ID/公证。runner 后续修复只涉及 Python，不改 native 源码或重签验收包。
- 首次探测的两个浏览器请求和宿主成功回执已到达，但 runner 把流式任务存活检查用于浏览器阶段，在正常退出边界产生竞态失败；改为由独立 HTTP 线程收取请求，沿用包级成功回执、退出码和完整 sidecar/readiness 清理断言后，两种档案复验通过。没有降低 native 链接成功或实际页面请求条件。
- 此门禁不证明物理点击、邮件客户端、OAuth 登录/回调、指定编辑器/终端或页面视觉，相关待办保留。
- 同一新包的普通 `smoke-packaged-app.py` 默认/显式档案均通过：实际通知授权读取、Global 私有工作区、凭据身份、sidecar readiness/未鉴权拒绝和正常退出清理。测试结束已独立确认无运行中的 Preview，首轮 runner 竞态留下的自有私有目录经成功回执及无 sidecar 核对后清理。

#### D：macOS 剩余验收条件核对（2026-10-01；外接屏已延期）

- 当前工作树的 D 表仍保留原生编辑/第二实例焦点、真实菜单/托盘/对话框/外部应用、通知授权及点击、钥匙串拒绝、外观视觉和不同缩放显示器拔插等未完成验收；官方旧 GUI 的文本历史回退通过没有关闭这些门禁。E 仍须等 D 验收及 Preview 稳定。
- 用只读 AppKit 查询核对当前环境：`NSWorkspace.shared.frontmostApplication` 返回 Codex，`NSScreen.screens` 只有一台、`backingScaleFactor=2`。没有读窗口内容或截图，也没有启动正常用户档案；此查询不证明 Preview 已激活或完整桌面交互权限可用。单显示器环境无法证明不同缩放外接屏拔插通过。
- 当前 SDK `NSApplication.h` 明确说明 `activate()` 不保证立即或最终激活，并建议前台应用先让出激活权；`NSRunningApplication.h` 说明 `activateIgnoringOtherApps` 在 macOS 14 后不再生效。本轮没有用新的强制激活重试或降低 inactive/key-window 断言制造通过，原生编辑/第二实例测试保持未完成。此前请求人工协助激活 Preview 及提供外接屏；用户随后明确要求延期外接屏、推进其他工作。只有显示器测试转入延期，原生焦点及编辑门禁继续保留。
- 当前 `internal/profilegate` 和实际 bridge 启动仍按配置/状态目录取得生命周期锁；原计划官方基线 `fa018e4` 源码没有该 gate/锁名。已通过的旧版会话 writer 租约与新宿主目录锁是两个验证范围，不能将文本回退通过推广为所有历史二进制已遵守新协议。历史宿主目录互斥及完整附件/检查点回退继续保留。

#### D：macOS 官方 Wails 实际历史与 Preview 回退（2026-10-01）

- 新增可复跑 `tools/tauri/smoke-legacy-rollback.py`。要求原生 arm64 macOS、Swift 编译器、已验证的官方 Desktop/CLI 1.38.3；启动前固定核对三个旧版可执行文件 SHA-256、bundle 身份/版本及旧/新 `.app` 严格签名，拒绝已运行的旧 GUI/Preview。官方 plist 以 DOCTYPE 开头，按实际 XML 格式解析；没有重新签名或修改旧版包。测试不继承用户凭据环境，使用 owner-only 临时 HOME/core/state/cache；失败保留私有测试记录，成功清理。
- 官方 CLI 经仅接受唯一假 key/model 的有界 loopback provider 完成一个真实请求，保存实际 user/assistant JSONL 历史。旧 GUI 恢复这份会话，必须保持 tab/model/原路径，取得与实际宿主 PID 对应的会话租约；同时用官方 CLI 尝试写入同一会话，必须因活跃租约拒绝且不发起 provider 请求。随后通过核对 PID、bundle 路径和标识的 `NSRunningApplication.terminate()` 发出实际原生退出，要求宿主正常退出且无子进程残留。
- 复用现有实际 Preview 包的 import/restore/explicit 三个阶段，包含配置/项目目录导入、真实 bridge 修改与 sidecar/整包重启、私有备份和显式目录拒绝；结束后完整旧版测试树的文件集合、摘要、权限和 mtime 与旧 GUI 退出后的基线完全一致。再次启动同一官方旧 GUI，原历史及会话租约、拒绝另一个 writer、原生退出均通过；历史文件字节保持不变，受保护配置/凭据文件和哨兵的摘要、权限、mtime 不变。最后旧 CLI 回读原 CNY，总 provider 请求仍只有构造历史的一次。
- 本机真实包执行全部通过；已按 JSON role/content 核对历史，而非只搜索 canary 字节。上一探测等待空会话 JSONL 的条件不适合旧版延迟保存：实际观察已有 session path、lease 和 metadata，JSONL 尚未生成；新场景先由官方 CLI 生成真实历史，不把空会话等待超时当成产品缺陷或降低历史完整性要求。`.events.jsonl` 与会话 JSONL 分别识别。
- Preview 验收包仍为干净代码提交 `b920717ffeeaf6d0e8fb65af7c2d8c3564b4e6e3`，本轮仅增加 runner 与记录，未改 host/前端/Go core，也未重建无源码变化的包。此证据覆盖官方旧 GUI 的单份实际文本历史回退与会话 writer 租约，不覆盖物理键盘/GUI 点击、全部历史配置/附件/检查点，也不证明旧版本参与本次新增的宿主目录生命周期锁。macOS D 的其余原生交互、E 和正式签名/公证仍待完成。

#### D：macOS 官方 CLI 1.38.3 配置兼容与回退（2026-10-01）

- 官方 `v1.38.3` arm64 CLI 归档经断点续传完整取得；17,110,738 字节及 SHA-256 `2077cc26cb4d3b2ebc1c980cdeb08d26072c291ee70b529c28085087fec537bd` 与官方 release API 资产元数据一致。校验通过后只提取唯一的常规文件 `reasonix`，拒绝链接/异常路径并使用私有目录与 `0700` 权限，没有安装、替换 `/Applications` 或执行不完整下载。提取后二进制 SHA-256 为 `49be12faf150879a1da58fb539871c26b6077fe2d45ffe3ad105bfb541d5a091`；Go metadata 显示 go1.26.6、GOARCH=arm64、原计划基线 `fa018e4109268c912063c8cc619302fccdb57d74` 和 `vcs.modified=false`，不再使用有修改的工作区二进制作官方证据。
- 在临时 HOME 的正式版/Preview 档案中运行既有 `smoke-profile-import.py --legacy-cli`。先由该官方 CLI 构造其有效配置，再保存原件基线；实际 Preview 包依次执行 import、restore、explicit 三个阶段，包含项目目录导入、真实 bridge 读取/修改、sidecar 替换重启、整包重启、备份保护、拒绝重复导入和显式档案导入。三个阶段全部通过，跨重启凭据身份与鉴权拒绝、正常退出及无 sidecar/readiness 残留同时检查。
- Preview 退出后，官方旧 CLI 的 `config currency` 查询仍返回原 CNY，正式版测试树的字节摘要、权限和 mtime 全部不变；原配置、项目 metadata、旧 sessions/cache/plugins 哨兵和 `.env` 保留。配置/项目备份保持私有且未被 Preview 修改覆盖，导入仅复制声明的 root/title。验收包为干净提交 `b920717ffeeaf6d0e8fb65af7c2d8c3564b4e6e3` 的完整 arm64 `.app`，未为了旧版测试重建或放宽产品导入规则。
- 此证据确认官方 CLI 的配置兼容和原目录回读，不证明所有旧配置组合、历史会话附件/检查点、Wails GUI 操作或历史原生进程的目录互斥通过。对基线源码的只读核对还确认其有短期配置编辑锁，但不能将其当成本次新增的完整宿主生命周期目录锁。官方 Wails 桌面归档已取得并核对，见下一节；D 的真实 UI、编辑焦点、显示器/通知/钥匙串交互及 E 门禁仍保留，正式发布签名/公证尚未执行。

#### D：macOS 官方 Wails 1.38.3 归档核对（2026-10-01）

- 官方 `desktop-v1.38.3` arm64 zip 经续传完整取得，88,965,850 字节及 SHA-256 `532b84dfd7691fa5ec006f88cf6937614b84cdf553d5498a722134d3d4e3a241` 与官方 release API 元数据一致。只在 owner-only 临时目录解压；拒绝链接、重复/异常路径及超限数据。Python 和系统 `ditto` 两种解压结果逐文件字节均与已校验归档一致，没有替换已安装的应用或重新签名。
- 归档内 `.app` 为 arm64/x86_64 universal，标识 `com.wails.reasonix-desktop`，版本 1.38.3；主程序链接版本和 revision 为 `v1.38.3`、`fa018e4109268c912063c8cc619302fccdb57d74`。主程序 SHA-256 为 `869b02d8f8a92f5847c1728923fde7153d931e105f26fd2926bfa8feeb863650`，内嵌 CLI 为 `5b1ab31424d45c8bf3cfe6a60b11da527df2252aee963d1c0c56352f57945962`。不据此增加 Intel Mac 验收范围，本机仍只验证 arm64 macOS。
- 两种解压结果在沙箱外的 `codesign --verify --deep --strict` 均返回 0；显示 Developer ID Application 证书链、TeamIdentifier `2S2W4BZFKM` 与 stapled notarization ticket。首次沙箱内校验返回 invalid signature 且 Authority unavailable，在相同文件的沙箱外校验中恢复正常；不能将该结果报告为官方归档签名损坏。此处只确认签名校验与票据存在，未进行独立 Gatekeeper/公证服务联网验收，也不证明新 Preview 已正式签名或公证。
- 私有 HOME/core/state/cache、无用户凭据及 loopback provider 的旧版启动探测写出了原模型的 Global tab 状态，但等待实际会话文件的严格阶段未通过；Preview 导入及旧 GUI 的后续回读/原生退出阶段未执行，未计入 Wails GUI 兼容通过。探测曾修正 macOS 临时目录路径别名及空会话路径的等待逻辑；没有修改旧版二进制或放宽产品导入规则。该失败探测未计入通过；后续先生成真实历史的旧 GUI 回退场景已通过，见上方最新记录。目录生命周期互斥、完整历史会话/附件/检查点与 GUI 交互仍待验收。

#### D：macOS 选择自动化约束与旧版安装包基线（2026-10-01）

- 试验性文件选择/保存探测在保存确认之前读到非预期目标 URL，阶段失败，未执行 Save/OK，也未计入通过。当前 Apple SDK 的 `AppKit.framework/Headers/NSSavePanel.h` 明确限制 `directoryURL` 与 `nameFieldStringValue` 只能在 configuration phase 设置，且 NSOpenPanel 不使用文件名属性；目录解析还可能异步完成。面板显示后修改这些属性的驱动不能作为实际用户选择的可靠证明，已全部撤回。本轮没有加入 `dialog-select` 阶段或放宽原生选择断言；`--dialogs` 及 CI 仍保持原先已通过的四种取消阶段，共 42 个阶段。选择/保存、覆盖确认和物理交互仍需要真实 UI 验收。
- 只读核对 `/Applications/Reasonix.app`：Info.plist 标记 1.38.3，严格签名校验通过但为本地 ad-hoc；内嵌 CLI 的 Go 构建信息为 `vcs.revision=2185b8e8ff8abb2166bf8e55df0694d05a0f9d76`、`vcs.modified=true`，Wails 主程序链接的 revision 也是该提交。CLI SHA-256 为 `8289154ee604d7bed0a1df2415650c6eed1fb5226f45883b27c5785c936118cd`，主程序为 `31b9e70e8efabd314c4bc327c8c2c4e1644652beab3760a1092eb297b3a2d0a9`；未启动、替换或修改该安装包。这不是已确认的官方历史二进制验收基线。
- 实时读取 [官方 Desktop 1.38.3 发布记录](https://github.com/esengine/DeepSeek-Reasonix/releases/tag/desktop-v1.38.3) 及 GitHub release API，正式 tag 对应原计划的 `fa018e4`；官方 arm64 zip 资产大小 88,965,850，SHA-256 为 `532b84dfd7691fa5ec006f88cf6937614b84cdf553d5498a722134d3d4e3a241`。首次下载到私有临时目录因网络低速在 180 秒后返回 curl 28，只取得 1,965,769 字节；不完整归档未解压或执行。另取得官方 CLI `v1.38.3` 的 arm64 tar.gz 元数据：17,110,738 字节，SHA-256 `2077cc26cb4d3b2ebc1c980cdeb08d26072c291ee70b529c28085087fec537bd`，用于后续校验与隔离配置兼容测试；资产元数据或开始下载不能视为旧版测试通过。
- 撤回试验后，从干净提交 `b920717ffeeaf6d0e8fb65af7c2d8c3564b4e6e3` 重新完整构建 arm64 Preview `.app`，前端生产门禁/体积预算、当前 Go sidecar、Rust host、严格本地 ad-hoc 签名通过；同一包的托管/显式档案启动/退出 smoke 通过，实际通知授权只读查询、Global 工作区、私有凭据身份、401 鉴权拒绝与退出无残留同时核对。工作树在构建和验收后干净；本轮没有扩大以前 42 个原生阶段的证明范围，未执行正式签名/公证。D 和 E 保持未完成。

#### D：macOS 实际系统对话框取消（2026-10-01）

- 增加独立 `--dialogs` / `dialog-cancel` 安装包门禁，使用既有临时 HOME/core/TMPDIR 和档案检查。宿主调用实际 `save_local_path_as`、`export_frontend_diagnostics`、`import_user_theme` 及相同官方插件的目录选择 API；明确观察当前进程唯一可见的 NSSavePanel/NSOpenPanel，核对类型及保存文件名后调用实际 AppKit `cancel`，不伪造插件 callback 的 None 或选中路径。
- 每次取消均要求生产回调返回预期空路径/false/None，实际面板关闭；主题列表不变、私有源文件内容/mtime/权限不变且没有额外文件。测试仅操作本次私有宿主的面板，存在其他可见面板、多面板、错误类型/文件名、超时或取消不一致时失败；无新增 renderer 命令/权限。仅为已锁定 objc2-app-kit 0.3.2 显式启用 Open/SavePanel 类型 feature，未新增依赖版本。
- 原生探测包的托管/显式档案均通过：每档案三个窗口前置阶段及一个包含四种取消操作的阶段，共八个阶段；鉴权拒绝、实际档案继承、跨重启凭据身份、窗口持久几何和退出无残留同步检查。探测构建复用上一干净包的前端和当前 sidecar，仅重建原生 host，不能代替干净源码完整生产构建。当前源码/真实 Go bridge 的 Rust 193 项通过、2 项默认忽略，严格 clippy、格式、Python 语法和 diff 检查通过；完整包另行记录。
- 随后从干净提交 `bad820b01c5c2e3882133777fb3f9e729f047f88` 完整构建 arm64 macOS `.app`，源码标识无 dirty 标记；前端生产门禁/现有体积预算、Go sidecar、Rust host 和严格本地 ad-hoc 签名校验通过。同一包的 `--dialogs` 门禁共 42 个阶段全部通过（两种档案各 20 个原有阶段及一个四类面板取消阶段），两种档案的标准包启动/退出 smoke 也通过；实际通知授权只读查询、Global 工作区、私有凭据身份、401 鉴权拒绝和无残留同步检查。构建和包级验收后工作树干净。macOS CI 的原生门禁已加入 `--dialogs`，YAML 与步骤参数检查通过，远端执行结果仍待确认；未运行仍受激活状态阻断的 `--edit`/`--focus`，也未执行正式 Developer ID 签名/公证。
- 此验收证明原生面板/取消动作与生产返回路径，不证明用户鼠标/键盘、文件/目录实际选中、确认覆盖保存、系统错误或 WebView 按钮已通过。编辑焦点、其他 macOS D 交互及 A/B/C 缺口保留，E 仍等待 D 验收和 Preview 稳定；Windows/Linux 继续延期。

#### D：macOS 对话框异常与诊断导出原件保护（2026-10-01）

- 核对 Wails `SaveLocalPathAs` 和现有 Tauri 文档/主题包另存为后，发现诊断导出独用 `fs::write`，会截断现有目标并沿符号链接写入其他文件。新增实际文件回归先复现该问题，再改为同目录私有临时文件、完整写入、sync 和原子替换；与其他另存为路径保持相同的发布方式。所选名字为符号链接/硬链接时，只替换该名字，保留其他名字对应的原件；macOS 导出权限为 `0600`。准备/写入/同步/发布失败分别返回固定解决提示，不输出系统路径或原内容。
- 主题导入、主题导出和诊断导出的回调通道此前用 `unwrap_or(None)` 把断开当成取消。共用接收逻辑现在只将实际 `None` 视为用户取消；回调断开或任务失败返回可重试错误。实际选中路径、取消返回值和原有主题格式保持，未增加 renderer 命令或权限。
- 六项诊断导出回归通过，覆盖格式/体积限制、有效 JSON、符号链接/硬链接原件保护、私有权限、非法报告不改旧文件、发布到目录失败与无临时残留；另一个回归区分选中路径、明确取消和通道断开。使用当前源码新建的私有 Go bridge，Rust 全量 193 项通过、2 项默认忽略；严格 clippy（all targets）、格式和 diff 检查通过。首轮误用旧 `bin/reasonix-desktop-bridge` 导致三个协议字段缺失失败，未计入通过证据；换用当前源码 bridge 后原三项及完整套件通过。干净提交的完整 macOS 包另行记录。
- 随后从干净提交 `e183dddb5ea2bc5a51917bb6202e67158e16a771` 完整构建 arm64 macOS `.app`，源码标识无 dirty 标记；前端生产门禁/现有体积预算、Go sidecar、Rust host 及严格本地 ad-hoc 签名校验通过。同一包的默认托管和显式档案启动/退出 smoke 均通过，包含实际通知授权只读查询、Global 工作区、私有凭据身份、401 鉴权拒绝和退出无残留。构建及包级验收后工作树干净。本切片未重复无相关改动的 40 个窗口/外观/菜单场景，也未运行仍受激活状态阻断的编辑/第二实例门禁；以前的原生证据保持其原有提交范围。
- 此切片修复保存和回调失败语义，不证明实际系统对话框选择/取消、主题设置页、磁盘写满或系统 UI 错误已验收。原生编辑焦点及其他 macOS D 交互待办保留；E 仍等待 D 验收及 Preview 稳定。

#### D：macOS 外观回滚、未配置状态与故障恢复（2026-10-01）

- 实际探测包注入首次原生更新失败，严格比较全部 desktop preferences，正确发现旧逻辑通过普通 setter 回滚为显式 auto，导致 `appearanceConfigured=false` 变为 true。新增后端恢复后，Go 测试还发现 `SaveUserSettingsDeltaTo` 使用渲染后的 auto 默认值代替空的存储状态；现在合并配置时保留原始空主题的语义，不将未配置状态当成用户显式选择。
- 增加仅 host/sidecar 使用的 `POST /v1/settings/desktop/appearance/restore`，沿用 bearer 鉴权、64 KiB 限制和 request ID 幂等。读取、比较当前归一化主题/样式/配置标记、窄写入都在配置编辑锁中进行；状态与保存成功时的快照不符则返回 409，拒绝覆盖另一次不同外观修改。恢复未配置状态时移除主题和样式；已配置状态保留显式 auto 和受支持旧样式。保留其他偏好及未知配置字段，失败不输出文件路径/原内容；不增加 renderer 命令、权限或普通 UI 可选样式。
- NativeAppearance 的生产保存与故障注入复用同一锁定事务；Go 保存拒绝时零 native 调用，native 错误后先恢复配置再恢复原生外观。完整恢复返回“已恢复、重试”的固定提示，配置/原生恢复失败或比较冲突返回“重启读取已保存外观”的固定提示；不将未完成回滚当成成功。
- 默认托管和显式 core 档案各新增三个原生阶段：首次失败保留未配置、六种确定性故障/冲突、整包重启恢复。六种情形包含 native 写入前/实际写入后错误、初次存储拒绝、存储回滚拒绝、回滚前另一次真实 Go 外观写入及 native 回滚拒绝。错误注入围绕同一事务端口，实际 Go 持久读写与 AppKit 属性参与验证；阶段中卸载前端页面，防止自动同步掩盖 native 失败。最后故意留下“保存 dark/native light”，下一宿主启动在测试发送偏好命令前必须恢复实际 DarkAqua。
- 同一探测包的默认 40 个原生场景（每档案 20 个）通过；配置层和 Go bridge 全量回归、真实 Go bridge 的 Rust 189 项（2 项默认忽略）、严格 clippy、格式与 Python 语法通过。Go 另覆盖未配置/显式 auto/旧版 glacier、鉴权拒绝、幂等重放、过期/非法快照与损坏配置的原件保护和错误过滤。干净提交完整包另行记录。
- 随后从干净提交 `1cb40d135b5e3be7fed9d7134d364d518223d64c` 完整构建 arm64 macOS `.app`，构建源码标识无 dirty 标记；前端生产门禁/现有体积预算、Go sidecar、Rust host 与严格本地 ad-hoc 签名通过。同一包的默认 40 个原生场景、三个配置导入/重启场景及两种档案生命周期全部通过，实际通知授权只读查询、Global 工作区、私有凭据身份、鉴权拒绝和退出无残留同时检查；构建与验收后工作树干净。配置层/Go bridge 的完整回归与定向 go vet 通过；未重复运行被同一激活状态阻断的编辑/第二实例焦点门禁，也未执行正式签名/公证。
- 此证据覆盖事务错误注入及恢复，不证明 AppKit/磁盘实际拒绝、设置页点击、标题栏视觉或系统明暗切换已验收。原生编辑/第二实例焦点、托盘/对话框/通知交互和其他 macOS D 待办保持；E 尚未开始迁移，A/B/C 图片缺口与正式签名/公证仍保留。

#### D：macOS 原生 responder 编辑严格门禁（2026-10-01，未通过）

- 新增独立 `--edit` / `menu-editing` 阶段，继续使用临时档案和完整系统剪贴板保护。通过 Tauri `with_webview` 在实际 WKWebView 中建立一次性 textarea，只有固定字符串状态通过 WKJavaScript completion 回传；不读出原剪贴板、不新增 renderer 命令或权限，也不修改产品输入框或 transcript。macOS 直接使用已锁定的 objc2-web-kit 0.3.2 类型，不新增依赖版本。
- 严格要求实际应用 active、主 NSWindow 为 key window 且 WKWebView 接受 first responder。然后核对已安装六类编辑菜单的 selector/nil target，更新实际菜单验证，再发出菜单动作；AppKit 的 nil target 沿 responder chain 分派，参考 [Apple sendAction](https://developer.apple.com/documentation/appkit/nsapplication/sendaction%28_%3Ato%3Afrom%3A%29?language=objc)。复制和剪切前写入不同的唯一 canary，动作后必须读回所选文本；粘贴要求文本与 input 事件，原生撤销/重做及全选要求值/选择范围匹配。该门禁不以剪贴板旧值相同、普通 JS 设置文本或固定 target 代替原生编辑成功。
- 本机 macOS 27.0.1 实际探测停在焦点前提：`firstResponderAccepted=true, keyWindow=false, applicationActive=false, windowVisible=true, applicationHidden=false`。先使用产品共享托盘/Dock/单实例恢复路径并等待状态，再在 opt-in 探测中加入当前 [NSApplication activate](https://developer.apple.com/documentation/appkit/nsapplication/activate%28%29?changes=la%2Cla) 请求，结果仍相同；没有跳过激活条件或继续执行编辑动作。此结果不能判定产品编辑缺陷，也不能标记 responder 编辑或物理按键通过；应待实际桌面能够激活 Preview 后复验，避免在相同外部状态下重复重试。
- 最新源码的真实 Go bridge Rust 189 项通过、2 项默认忽略，严格 clippy、格式和 Python 语法通过；默认 34 个原生场景与干净提交完整包另行复验。独立 `--edit` 保持显式失败门禁，未作为已通过场景加入默认统计或 CI；macOS D 与 E 整体尚未完成。
- 随后从干净提交 `f7c30746ff42aeafc6f09a8140def8c0a5ab8277` 完整构建 arm64 macOS `.app`，构建源码标识无 dirty 标记；前端生产门禁/现有体积预算、Go sidecar、Rust host 与严格本地 ad-hoc 签名通过。同一包的默认 34 个原生场景、三个配置导入/重启场景及两种档案生命周期全部通过，实际通知授权只读查询、Global 工作区、私有凭据身份、鉴权拒绝和退出无残留同时核对；构建与验收后工作树干净。此轮完整包复核没有重复运行已被同一激活条件阻断的独立编辑门禁，也未执行正式 Developer ID 签名/公证。
- 对首实例启动方式作独立比较：复用干净代码提交 `1cb40d135b5e3be7fed9d7134d364d518223d64c` 的同一 `.app`，以 `/usr/bin/open -n -W` 经 LaunchServices 启动首个宿主，传入私有 HOME/TMPDIR/core/cache 和 opt-in 阶段。通过私有 ready 根目录对应的 sidecar 父 PID 确认实际宿主，继续运行现有鉴权、档案环境和窗口断言；运行前拒绝已运行 Preview 及 launchd 的非空 state override。窗口 exercise、最大化重启和普通窗口重启三个前置阶段通过，`menu-editing` 仍报告相同的 inactive/key-window 状态，未执行后续编辑动作。失败路径确认原剪贴板全部格式恢复，测试宿主与 sidecar 已清理。这排除了“仅将首实例改为 LaunchServices 启动即可通过”的假设，不能据此排除应用缺陷或断定系统权限是原因。
- 本地包 Info.plist 未设置 `LSUIElement`/`LSBackgroundOnly`，锁定的 Tao 0.35.3 默认激活策略为 Regular，产品源码没有覆盖；这只是静态检查，未证明实际应用已激活。此次比较没有修改产品、放宽焦点条件、增加默认通过场景或推进 E；下一次交互验收应先确认实际桌面能将 Preview 激活并成为 key window。

#### D：macOS 系统剪贴板与 WKWebView 权限（2026-10-01）

- 新增 `clipboard-native` 包级场景，在已加载的实际主 WKWebView 内调用既有 Tauri IPC 文本插件，写入每次唯一的测试字符串并要求精确读回；图片读取必须返回 clipboard-manager 权限拒绝。另建实际同源隐藏 WKWebView，未授予主窗口 capability，其文本读取和写入也必须返回权限拒绝；拒绝探测后实际系统值保持原测试字符串。未新增 renderer 命令、权限或 public Tauri 全局配置；生产启动不启用探测。页面回执只传 nonce 和固定状态，不传剪贴板内容。
- Swift helper 使用 AppKit 独立核对系统剪贴板值和 changeCount。写入前在 owner-only 临时目录完整保存所有 item/type/data 的有序二进制快照（最多 32 项、每项 64 类型、总 32 MiB）；文件承诺、不可读取或超限内容在测试写入前拒绝。测试后要求格式和字节完整恢复；若剪贴板已被用户换成其他内容，保留当前内容并使验收失败，恢复失败/超时保留私有恢复副本，日志不输出原内容。缺少代次回执的中断只能恢复该次未展示的唯一测试字符串。
- 每个档案在实际系统写入前，先在独立命名 pasteboard 验证多项/富文本/自定义二进制恢复、中途复制保护、中断恢复及文件承诺拒绝；只读系统快照预检查也已通过。默认托管和显式 core 档案的实际剪贴板场景均通过，同一探测包共 34 个原生场景（每档案 17 个）；真实 Go bridge 的 Rust 189 项通过、2 项默认忽略，严格 clippy、格式和 Python 语法通过。干净提交完整包另行记录。
- 随后从干净提交 `b8e420d6c0f22f998c943f5a40ef5a8f0645efa6` 完整构建 arm64 macOS `.app`，包内源码标识无 dirty 标记；前端生产门禁/现有体积预算、Go sidecar 与 Rust host 通过。同一包的严格本地 ad-hoc 签名、34 个原生场景、三个配置导入/重启场景及两种档案包级生命周期验收全部通过；实际通知授权只读查询、Global 工作区、私有凭据身份、鉴权拒绝和退出无残留同时检查。构建与验收后工作树干净，正式 Developer ID 签名/公证仍未执行。
- 此项验证实际 WKWebView IPC、最小 capability 与系统 pasteboard，不证明共享 UI 按钮、物理 Cmd+C/V、responder 撤销/剪切或拒绝反馈已经验收。原生菜单/托盘/通知/对话框、第二实例焦点、其他 D 与 A/B/C 图片缺口保留；E 仍等待 macOS D 验收及 Preview 稳定。

#### D：macOS 原生外观同步与重启恢复（2026-10-01）

- 对照 Wails `theme.ts` 的原生 WindowSet*Theme 调用，Preview 的 `set_desktop_appearance` 原仅更新 Go 配置；页面使用 CSS 明暗模式，原生窗口未同步。真实探测包保存 dark 后，AppKit DarkAqua 严格门禁失败，不能将配置保存成功视为原生外观更新成功。
- 新增仅 macOS 的 `NativeAppearance`，启动、读取偏好和成功保存后使用 Tauri `set_theme` 同步原生外观。读取和保存串行；Go 校验/写入拒绝时不修改原生主题。原生更新返回错误时尝试恢复原偏好和外观，恢复失败则提示重启读取已保存外观；启动恢复失败保留可用应用并提示重试。未增加 renderer 窗口权限、直接 AppKit 写入或修改前端主题布局。
- Tauri 的 [setTheme](https://v2.tauri.app/reference/javascript/api/namespacewindow/#settheme) 在 macOS 作用于整个应用；auto 传 None，依 [Apple NSApplication.appearance](https://developer.apple.com/documentation/appkit/nsapplication/appearance?changes=_2_5&language=objc) 清除显式外观并继承系统。原生门禁读取实际 `NSApplication.appearance`，dark/light 要求 DarkAqua/Aqua，auto 要求 nil；不以当前系统颜色相同放行。重启检查在测试主动发送设置命令前读取原生属性，随后再保存下一模式，未改写系统全局外观。
- 默认托管和显式 core 档案各 16 个原生场景通过，共 32 个：原有 12 个，加 dark 保存、dark/light/auto 三种重启恢复。每阶段非法 theme/style 均拒绝，原配置与 AppKit 外观不变；普通窗口几何、凭据身份、真实鉴权和退出清理同时检查。探测包复用既有前端/sidecar；使用真实 bridge 的 Rust 189 项通过、2 项默认忽略，严格 clippy、格式和 Python 语法通过。干净提交完整包另行记录。
- 随后从干净提交 `f6f38bd32aaeac24dc159e6ea24c5c7b6b8d51c4` 完整构建 arm64 macOS `.app`；前端生产门禁/现有体积预算、Go sidecar 和 Rust host 通过。同一包的严格本地 ad-hoc 签名、32 个原生场景、三个配置导入/重启场景及两种档案启动/退出 smoke 全部通过；实际通知授权只读查询、Global 工作区、私有凭据身份、鉴权拒绝和退出无残留同时通过。构建与验收后工作树干净，正式 Developer ID 签名/公证仍未执行。
- 本轮为实际 AppKit 属性和配置持久化验收，未验证设置页物理点击、标题栏截图、系统明暗切换或原生失败回滚的故障注入。第二实例焦点、托盘/通知/对话框交互及其他 macOS D 待办保留；E 等待 D 验收且 Preview 稳定。
- 同次 runtime 直连审计还确认 A/B/C 图片缺口：`hasMarkdownImageResolver` 只检测 Wails binding，`app` Proxy 没有 Tauri 图片入口，Preview 消息也未设置 `MarkdownImageTabContext`。本地/旧工作区图片会回落到普通 URL，远程图片也未接入 Wails 的受限代理。后续须按实际会话/工作区授权迁移 resolver、数据/类型/尺寸限制和远程代理边界，再验证旧 Global 附件与文件引用；不能放宽 CSP 或通用文件权限代替迁移，也不能宣称 A/B/C 已完成。

#### D：macOS 安装包配置导入、重启与原件保护（2026-10-01）

- 新增 `smoke-profile-import.py` 和仅显式测试环境启用的原生入口；临时 HOME 内分别构造正式版和 Preview 档案，调用与设置页相同的确认入口。验证未确认时拒绝写入，配置先备份后导入，项目只复制 root/title，sessions/cache/plugins/`.env` 及旧会话/排序元数据未被复制；导入文件与备份权限为 0600，重复导入不覆盖、不增加备份，显式 `REASONIX_HOME` 不提供自动导入。
- 实际 Go bridge 立即读取导入的 Provider/默认模型；通过实际设置接口修改 Preview 默认模型，再重启 sidecar 和完整 `.app`，修改保留且凭据档案身份稳定。外部 runner 对比 host 的初始/最终 sidecar 身份，要求原进程退出、新进程唯一、未认证请求被拒绝以及最终 sidecar/readiness 清理。正式版测试树的文件集合及文件字节、权限、修改时间与原始基线一致，配置备份也保持原字节。
- 额外使用本地 1.38.3 CLI 构造其自身有效配置，Preview 退出后由相同二进制再次查询 currency，要求能读取原目录且整个正式版测试树不变。二进制 SHA-256 为 `400f6370943e861c04e5b81b78a932e08f79923a02d4c3852a2c9973e75b4d13`，版本输出 `reasonix v1.38.3`，Go build metadata 的 revision 为 `2185b8e8ff8abb2166bf8e55df0694d05a0f9d76` 且 `vcs.modified=true`；这是可定位的本地旧版兼容证据，不能作为正式发布 artifact 或旧 Wails GUI 已认证的证明。
- 第一轮旧版检查正确发现其查询前的主题初始化 `config.Load()` 会升级手写稀疏配置并改写文件。现在先由旧版构造有效配置，再保存原件基线；Preview 及最后旧版查询后的原件不变断言保留。未修改旧版行为或产品导入规则，未将旧版自己的初始化写入归因于 Preview。
- 标准探测包的 import/restore/explicit 三个场景通过；带旧版原生配置的相同三个场景及最后查询也通过。使用真实 bridge 的 Rust 全量 189 项通过、2 项默认忽略；严格 clippy、Rust 格式、Python 语法和 CI YAML 解析通过。macOS CI 加入标准包级导入门禁，远端结果待确认；探测包复用已有前端/sidecar，干净提交完整生产包另行记录。
- 随后从干净提交 `ccfa44177a1b5a68d9945b4d74464f85fd3ab81d` 完整构建 arm64 macOS `.app`，前端生产门禁/现有体积预算、Go sidecar 和 Rust host 通过。同一包的严格本地 ad-hoc 签名、标准三个导入场景、旧版配置三个场景及最后查询、原有 24 个原生场景和两种档案启动/退出 smoke 全部通过。窗口及启动脚本还注入不相关的验收环境变量，验证清除后独立运行；原生通知授权查询、Global 工作区、凭据身份、鉴权拒绝和退出无残留同时通过。构建与验收后工作树干净，正式签名/公证仍未执行。
- 程序化宿主入口不证明 WebView 设置页实际点击、旧 Wails GUI/目录锁、旧会话附件/检查点或全量数据恢复已验收；正式 Developer ID 签名/公证和其他 macOS D 待办保留，E 继续等待 D 验收且 Preview 稳定。

#### D：macOS 运行任务时关闭、后台完成与原生退出（2026-10-01）

- 新增真实包验收场景 `task-background-menu-quit`：临时私有档案配置有界 loopback 流式 provider，由实际 Go core 发起请求并通过宿主 SSE 转发读取真实 text 事件；没有调用外部收费服务、修改用户配置或新增 renderer 命令/权限。
- 首个任务收到实际内容且状态为 running 后触发原生窗口 close，严格要求窗口隐藏而任务仍运行。外部 runner 核对原宿主/sidecar PID、同一 ready 身份和未认证 health 被拒绝，再放行后续流；必须在隐藏期间收到新的真实内容、保持 running，最后核对 idle 和完整 assistant 历史。随后调用已安装的 AppKit Show Reasonix 菜单动作，要求恢复保存的普通窗口几何。
- 显式切换到第二个会话，读取实际流内容并确认 running，再调用已安装的 Quit Reasonix 菜单动作。成功路径不由 runner 调用 `app.exit`；必须观察实际宿主退出码 0、上游未完成流主动断开，以及 sidecar 和 ready 文件无残留。菜单项沿用实际 Cmd+Q 处理器；这不证明物理 Cmd+Q 或托盘点击已验收。
- 测试初始化曾遗漏 snapshot 后的 SSE 订阅，另曾用 Open 替换另一个已完成会话；依据现有协议补上正常订阅顺序及显式 Switch（仅空闲任务可切换）。这些是验收脚本修正，没有放宽任务状态、事件、历史或退出断言，也没有改变产品会话所有权规则。
- 探测包默认托管和显式 core 档案各 12 个场景通过，共 24 个。探测包复用既有前端/sidecar；干净提交的完整生产包另行记录。真实物理交互、第二实例焦点、外接屏及其他 macOS D 待办继续保留，E 仍等待 D 验收且 Preview 稳定。
- 使用真实 Go bridge 的 Rust 全量回归 189 项通过、2 项默认忽略；严格 clippy（含测试）、Rust 格式、Python 语法和 diff 检查通过。未修改前端渲染或会话运行时实现。
- 随后从干净提交 `c2da30b25cd3219e59d1562a340abb0ceb0381ca` 完整运行 `pnpm tauri:build -- --bundles app`：前端生产门禁/现有体积预算、Go sidecar 与 Rust host 均通过。对同一 arm64 macOS `.app` 的严格本地 ad-hoc 签名校验、24 个原生场景与原有两种档案包级 smoke 全部通过；实际通知授权只读查询、私有凭据身份、Global 工作区、鉴权拒绝和退出无残留同时通过。构建及验收后工作树干净；未执行正式 Developer ID 签名/公证，不能据此标记正式发布就绪。

#### D：macOS 原生菜单与配置快捷键冲突（2026-10-01）

- 对照当前 Wails 原生 Edit/Window 角色及 Preview 处理器，发现会话刷新默认 Cmd+R 与原生 Reload 的固定 Cmd+R 重复。移除 Reload 的原生 accelerator，菜单动作仍可手动使用；Cmd+R 继续由可配置的会话刷新处理器使用。
- 新增共享 `macosMenuShortcuts.json`，保留原生编辑、退出、隐藏、隐藏其他应用、最小化与全屏组合；保存配置及按键派发均拒绝占用它们。旧 Preview 已保存的原生冲突组合在解析有效绑定时回落默认值，保留存储原件供重置，不修改稳定版 Wails 快捷键偏好。单项重置若默认组合已被其他 Preview 动作使用，会提示冲突并保留配置；重置全部恢复默认组合。
- 在真实 `.app` 中读取当前 `NSApplication.mainMenu` 的 [AppKit `keyEquivalent`](https://developer.apple.com/documentation/appkit/nsmenuitem/keyequivalent) 与修饰键，要求 11 个应用原生组合全部存在，且每个实际 Cmd/Ctrl 菜单组合均在前端保留表中。首轮严格检查发现 macOS 自动补充 Emoji、听写与 Fn 项；两个实际 Emoji Cmd/Ctrl 组合加入可选保留表，空格统一为 `Space`。系统项目是否存在随 macOS/输入设置变化，其标题可本地化；可选项按组合匹配，不能因此放行未知 Cmd/Ctrl 菜单组合。Fn/无主修饰键组合不能配置为 Preview 全局动作，仍由系统处理。
- 探测包默认托管和显式 core 档案各 11 个场景通过，共 22 个（新增实际菜单组合契约场景）。原生检查没有伪造菜单键值，也没有新增 renderer 权限；实际物理按键、responder 编辑与第二实例焦点仍不在该证明范围。探测包复用已有前端/sidecar，干净提交的完整生产包另行记录。
- `test:tauri` 的快捷键单元和真实设置组件回归通过，覆盖保留组合拒绝、大小写/空格、旧原生冲突回落、替换组合保存、冲突重置不改配置与重置全部；Rust 使用真实 bridge 的 189 项回归通过、2 项默认忽略，严格 clippy 通过。
- 页面验证使用 `frontend-testing-debugging`：Browser 插件不可用，本机 Playwright WebKit 引擎缺失，改用已安装 Chrome 的独立浏览器上下文；未安装新依赖。临时页面 `http://127.0.0.1:5179/__shortcuts-qa.html` 直接挂载真实 Tauri 设置组件，独立 localStorage、Mac 平台标识和英文语言；不连接原生 host。1280×900 与 1024×768 下验证页面身份、有内容、无错误覆盖层、控制台零警告/错误、截图、录制原生组合后拒绝/保持录制、换键保存、重置冲突、重置全部与重载后再录制。截图检查反馈与按钮可见，没有横向溢出；这不是 WKWebView 或 macOS 原生按键路由验收。
- 随后从干净提交 `b9fccd040334ea1d68da5d02d358962a818c648c` 完整构建 macOS `.app`：前端生产门禁与现有体积预算、Go sidecar、Rust host 均通过；同一包的严格本地 ad-hoc 签名校验、两种临时档案共 22 个原生菜单/窗口/关闭场景及原有启动/退出 smoke 全部通过。凭据身份稳定、实际 macOS 通知授权只读查询、Global 工作区、鉴权拒绝及退出无残留同时通过，构建与验收后工作树干净。独立页面 QA 服务器已关闭；没有操作用户原生配置或安装新浏览器依赖。正式 Developer ID 签名/公证仍待完成。
- D 继续保留真实原生键盘编辑、系统剪贴板、托盘/通知/对话框交互、外接屏及数据回退待办；E 仍等 macOS D 验收且 Preview 稳定后推进。

#### D：macOS 设置菜单恢复与原生动作验收（2026-10-01）

- 真实 AppKit 设置菜单动作复现了旧问题：`host:open-settings` 已送达，但隐藏的窗口仍不可见。设置菜单处理器现先调用共用主线程窗口恢复入口，再发送设置事件；应用隐藏和窗口最小化也使用该恢复路径。
- 新增 `native_menu_smoke`，仅由现有显式环境变量验收入口调用。通过当前进程实际 `NSApplication.mainMenu` 查找已安装的 Settings 项，检查启用状态后调用 [Apple `NSMenu.performAction(forItemAt:)`](https://developer.apple.com/documentation/appkit/nsmenu/performactionforitem%28at%3A%29)，经真实 Tauri 菜单回调验证主窗口恰好收到一次设置事件，以及原生窗口可见、非最小化且应用已取消隐藏；监听随测试范围结束清理。没有直接伪造 Rust 菜单事件，也没有新增 renderer 命令或权限。
- 测试在启动就绪后直接最小化时，曾无法建立最小化前提：应用未激活、窗口保持可见且非最小化。初始化改为确认一次真实隐藏/显示转换，沿用已通过窗口门禁的准备流程；后续仍严格要求先读到真实最小化，再通过菜单动作恢复。未放宽最小化状态断言，未将启动时序问题宣称已由产品修复。
- 探测包中默认托管和显式 core 档案各 10 个场景通过，共 20 个：原有 7 个窗口/关闭场景，加隐藏窗口、最小化窗口、隐藏应用三种 Settings 菜单动作。档案身份稳定、普通几何保存、真实 sidecar 就绪/鉴权拒绝及退出无残留同时检查。使用真实 Go bridge 的 Rust 回归 189 项通过、2 项默认忽略；前端 `test:tauri`（含设置监听清理、设置覆盖层和快捷键组件回归）以及严格 clippy 通过。探测包复用已有前端和 sidecar，干净提交的完整生产包另行记录。
- 随后从干净提交 `6284dd44cbd663cb68c69613b6c668bf05fdb415` 完整运行 `pnpm tauri:build -- --bundles app`，前端生产门禁、Go sidecar 与原生宿主构建通过。对同一 `.app` 的严格本地 ad-hoc 签名校验、20 个原生窗口/Settings/关闭场景及原有两种档案的包级 smoke 全部通过；后者同时验证私有凭据身份、真实 macOS 通知授权只读查询、Global 工作区、鉴权拒绝及退出无残留。构建与验收后工作树保持干净；这不是正式 Developer ID 签名或 Apple 公证。
- 更新入口改为 Updates 信息对话框，移除没有实现依据的启动自动检查说明，提示 Preview 尚未提供更新检查并给出发行页面。这是能力说明纠正，E 的 updater 迁移仍未完成。
- 上述是程序化原生菜单动作与宿主事件验证，不包含真实鼠标/键盘操作、WebView 实际设置覆盖层渲染、编辑 responder、快捷键冲突或用户焦点。第二实例严格焦点门禁、托盘/通知交互、外接屏拔插及其他 macOS D 待办继续保留；D/E 尚未完成。

#### D：macOS 原生窗口、应用隐藏与关闭验收（2026-10-01）

- 新增 `tools/tauri/smoke-native-window.py` 和仅由环境变量显式开启的宿主验收入口，不新增 renderer 命令或权限。脚本拒绝启动已运行的同标识 Preview，使用临时 HOME/TMPDIR；默认托管和显式 core 档案各自连续重启，同一档案的凭据身份必须保持不变。
- 原生 API 分别操作真实 macOS 窗口并读取尺寸、位置、缩放、可见、最小化、最大化及应用隐藏状态。覆盖隐藏/显示、最小化/恢复、普通几何不被最大化覆盖、最大化退出后重启与取消最大化、普通窗口重启、应用隐藏/取消隐藏、CloseRequested 的继续运行/退出，以及退出偏好的下一次启动恢复。外部 runner 另检查真实 sidecar 就绪/未认证请求拒绝、实际继承的档案环境、正常退出与无残留。
- 共用 `show_main_window` 把恢复步骤放到主线程，并在 macOS 应用确实隐藏时先调用 Tauri `app.show()`；窗口显示和应用取消隐藏均有独立验收，避免只看窗口可见就认定应用已恢复。
- 本机 Stage Manager 已开启。首个靠近左边缘的测试位置在应用激活时从物理 x=80 移到 x=468；改用当前工作区中央的有效位置，仍严格要求恢复原始尺寸/位置。恢复断言没有放宽为“窗口可见即可”。在同一次应用隐藏/取消隐藏后立即最小化的组合试验中，曾观察到窗口没有进入最小化；独立场景通过不能证明该组合时序已解决，继续保留在交互验收中。
- 第二实例焦点为额外的严格 `--focus` 门禁：通过 LaunchServices 打开真实第二个进程，检查旧 sidecar 仍唯一、原窗口可见、普通几何正确且 `is_focused()` 为真。本机诊断中窗口可见但焦点为 false、应用未激活，门禁失败；该项仍是 macOS D 的未完成验收。按 [Apple 对 cooperative app activation 的说明](https://developer.apple.com/videos/play/wwdc2023/10054/)，激活请求可被系统拒绝；当前执行上下文可能有关，但这只是原因推断，不视为已排除应用缺陷或已通过真实用户唤起。
- 本机最终探测包的默认/显式档案各 7 个场景通过，共 14 个，包括关闭退出偏好的下一次启动恢复；严格 `--focus` 在第二实例阶段失败，失败状态已如实保留。使用本次真实 Go bridge 的 Rust 全量回归 189 项通过、2 项默认忽略；严格 clippy（含测试）、Rust 格式、Python 语法、CI YAML 与 macOS 窗口门禁配置检查通过。探测构建复用了已通过生产门禁的前端和 sidecar，仅重建原生宿主；干净提交的完整生产构建与包级回归仍需单独记录。
- 随后从干净提交 `91cbff49cbe10a0faab0a7570d880bad87ffc126` 完整运行 `pnpm tauri:build -- --bundles app`：前端生产门禁、Go sidecar 和原生宿主构建通过，包内 host/sidecar 严格本地 ad-hoc 签名校验通过。对同一 `.app` 的默认/显式临时档案原生窗口门禁 14 个场景再次全部通过；原有 `smoke-packaged-app.py` 两种档案均通过，真实 macOS 通知授权只读查询、私有凭据身份与 Global 工作区、鉴权拒绝及退出无残留同时通过。`pnpm test:tauri` 通过，构建与两套包级验收后工作树保持干净。这不是正式 Developer ID 签名或 Apple 公证。
- 本轮桌面自动化的 `getState()` 重新检查仍超时并重置工具会话，未取得可用于点击验收的桌面状态；没有把原生 API 门禁推广为菜单/托盘/编辑器的真实交互通过。
- 已接入 macOS CI 的默认窗口门禁，远端执行结果尚未取得。以上原生 API 验证不包含菜单/托盘点击、WebView 编辑、运行任务期间关闭、通知交互、钥匙串拒绝或真实显示器拔插；D/E 继续保持未完成。

#### D：Global 会话稳定工作区与原生打开（2026-09-30）

- Wails Global 对话使用 `ReasonixHomeDir/global-workspace`。Preview 此前未指定项目时把进程 cwd 交给 core，安装目录/启动位置可能漂移且顶部打开入口被隐藏；现在 rootless 会话使用当前 Preview 档案的 `global-workspace`，按需创建为 `0700`，文件与符号链接拒绝且不修改原目标。显式项目目录保留原选择，默认托管 Preview 不访问稳定 Wails 档案。
- 项目归属与实际工作区分离：Global 的会话/身份目录 `workspaceRoot` 仍为空，避免将内部实现路径写成项目。新增鉴权只读 `GET /v1/sessions/{id}/workspace-target` 与生成契约；runtime manager 持有 ownership 锁查询当前 controller 的规范路径，拒绝非当前、关闭或无 provider 的 runtime，不因查询创建/切换对话。
- 顶栏对所有已有会话查询能力，未发送草稿不显示。Rust 仍只接受会话 ID 与安装应用 ID，重新读取实际目录并验证响应版本/身份/绝对路径/边界与目录存在性；前端不推断 cwd 或传入任意 launch root。缺失目录或卸载应用失败时不给此前会话保留打开入口。
- 定向回归已通过：Go 的鉴权/所有权、不同档案、项目/Global 切换、内容保留与重启、默认目录文件/符号链接拒绝、关闭/不支持 provider；Rust 的不可信响应拒绝与真实 sidecar Global/项目/重启/旧会话拒绝；React 工作区的 Global 入口、仅身份跨 IPC、切换不可用目标后旧按钮消失。`pnpm test:tauri`、生产构建、Go 运行时/协议/sidecar 全量回归、Rust 全量（163 项通过、1 项原生钥匙串测试默认忽略）与严格 clippy 通过；Wails Global 目录能力基线、协议生成校验与 Go vet 通过；从干净提交 `065b1ae5f676e38a1f84a2a1bfab8f08a85bd7cc` 构建 `.app` 并通过本地严格 ad-hoc 签名校验，默认/显式档案包级 smoke 均通过，实际 sidecar 创建 Global 会话并解析当前档案内的 `0700` 目录，原生授权查询、鉴权拒绝及退出无残留同时通过。未启动外部系统应用。
- 此变更为没有显式 root 的会话设定稳定默认目录；旧 Preview 曾在启动 cwd 创建的文件不会自动搬迁，原文件保持原位。未记录实际旧目录的 rootless 会话需要单独做文件引用/附件/检查点兼容验收；不能把本轮新建/重启测试推广为旧会话完整迁移已通过。
- 真实系统应用启动及其目录/参数交互仍待现场验收；Linux 原生编译与桌面环境验收、Windows 原生 host 编译/注册表/图标/桌面环境验收继续保留。D/E 目标保持进行中。

#### D：Windows 应用发现、原生终端与图标（2026-09-30）

- 对照 Wails `external_opener_windows.go` 的 18 个编辑器/Explorer/终端身份，Rust 从绝对 PATH、[App Paths](https://learn.microsoft.com/en-us/windows/win32/shell/app-registration) 的 HKCU/HKLM 查询、默认安装目录及有界 JetBrains/Toolbox 版本目录发现应用。注册表只读，按用户优先查询 64/32 位视图；仅 `REG_SZ`/`REG_EXPAND_SZ`，限制读取体积，后者使用 Windows 原生环境变量展开。带引号的绝对 executable 路径可用，命令行尾缀、损坏/缺失值、脚本、非文件和相对路径拒绝。实际打开重新发现以处理卸载。
- Windows Terminal 通过直接进程参数 `-d` 打开实际目录，文件使用父目录；PowerShell/Command Prompt 使用 [ShellExecuteExW](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shellexecuteexw) 直接打开程序，`lpDirectory` 保留原目录且不传参数串。没有 `cmd /c start` 或提升权限 verb。独立 STA 线程平衡 COM 初始化，提交失败返回固定解决提示，启用 NO_UI 避免重复系统错误对话框；进程由后台回收，Shell 调用不索取进程句柄。提交成功不代表实际目录/窗口已验收。
- 沿用 Wails 的 Explorer 行为：目录以 `explore` 和尾部目录分隔符打开，避免同名 `.lnk` 抢先解析；文档保持系统关联打开。工作目录、固定程序路径和参数只留在宿主，前端仍仅传会话身份/已安装应用 ID，未增加通用 WebView launch/registry 权限。
- [SHGetFileInfoW](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shgetfileinfow) 获取原生图标，32×32 DIB 分别对黑/白背景绘制并 flush 后重建 RGBA，编码为最多 64 KiB 的 PNG。owned icon/DC/bitmap/selection 由 scope guard 释放并先恢复 GDI 选中对象。Windows Terminal 优先真实包图标，其次可渲染的 PowerShell 图标，再回退零字节 execution alias；保护目录不可读视为正常缺失。
- 新增 9 项共用逻辑回归，覆盖固定目录、PATH/注册表/安装目录顺序、卸载、32 位安装目录/Toolbox 版本树、字面根目录和扫描边界、坏注册值、终端别名图标回退、含空格/中文/`& % ^ ()` 的目录参数、目录与同名快捷方式、透明度重建及实际 PNG 解码。真实 bridge 的 Rust 全量回归 **181 项通过、1 项原生钥匙串测试默认忽略**；严格 clippy（含测试）、`pnpm test:tauri` 与 CI YAML 解析通过。独立临时 crate 在本机直接导入 Win32 实现及测试，以锁定的 windows-sys 0.61.2/png 0.18.1 做严格类型/lint 检查通过；该检查不链接或调用 Windows DLL，不证明 Windows host 或原生测试已通过。
- 从干净提交 `53c05ce5d9b588fc6178223a08ebb622355a8e57` 完成 macOS Preview `.app` 生产构建与严格本地 ad-hoc 签名校验；默认托管和显式临时档案包级 smoke 均通过，包括凭据身份、原生 macOS 通知授权查询、实际 Global 工作区、sidecar 就绪/鉴权拒绝及退出无残留。前端类型、分层、权限、滚动门禁和 bundle budget 同时通过。此包验证既有 macOS 行为，不包含 Windows DLL/原生 host/GUI 验收；正式签名/公证仍未完成。
- 新增 `desktop-tauri-windows` 门禁，配置真实 Go bridge、完整 native host 编译/严格 clippy/回归。Windows 专属回归包含实际 Explorer 图标、Shell 提交失败和独立临时 HKCU App Paths 的环境展开/删除恢复，不启动真实交互式控制台。**门禁尚未远端执行**；Windows 原生 host、SDK 运行、完整安装包/Explorer/终端/Unicode目录交互，以及 Linux 门禁和 GUI 验收仍待执行。D 尚未完整验收，E 按目标继续等待 D 稳定。

#### D：Linux 应用发现、终端目录与原生图标（2026-09-30）

- 对照 Wails `external_opener_linux.go`，Rust 增加固定编辑器/终端目录。按 [XDG 数据目录](https://specifications.freedesktop.org/basedir/latest/) 用户优先顺序查找 desktop entry，支持有界子目录扫描；只解析主段，动作段不能覆盖，`Hidden=true`、损坏条目和缺失 `TryExec` 拒绝。`NoDisplay` 不排除仍可打开文件的应用。PATH 只接受绝对目录中的可执行文件，实际打开重新发现以处理卸载。
- 编辑器优先直接 CLI；仅有 desktop entry 时交给 `gio launch` 处理 [Exec 字段扩展](https://specifications.freedesktop.org/desktop-entry/latest/exec-variables.html)，不自行拼接 shell。目录使用 `xdg-open` 或 `gio open`，GIO 默认目录关联提供实际名称与图标；无法解析时显示通用名称。Ghostty/GNOME Terminal、Konsole、Kitty、Alacritty 与系统 terminal 使用独立参数策略，打开文件时使用父目录，特殊字符作为单个参数。只接受已检测的固定应用 ID，原生程序、desktop 路径和参数不跨 IPC。
- GIO/目录启动器最长等待 3 秒，及时退出失败返回固定解决提示；持续运行的处理器和终端/编辑器由后台回收进程。成功表示启动已提交，不代表实际窗口、文件或目录已验收。
- 图标支持安装条目的绝对资源与有界主题/`pixmaps` 查找；含点的主题图标名称不误当扩展名，拒绝相对目录穿越、超大及无效内容。Linux 使用已锁定的 GdkPixbuf 0.18.5/GIO 0.18.4 原生加载器，把最多 1 MiB 输入转为最长边 64 像素、最多 64 KiB 的 PNG；不把 raw SVG、file URL 或 native 路径交给 WebView。本实现为固定尺寸/目录的有界查找，并非完整桌面主题继承引擎。
- 新增 9 项可在本机执行的共用逻辑回归，覆盖 XDG 优先级/默认值、用户覆盖、动作段隔离、TryExec、GIO 参数、卸载/执行权限、定制终端路径、特殊工作目录、图标点名/越界/体积及启动失败。真实 Go bridge 的 Rust 全量回归 **172 项通过、1 项原生钥匙串测试默认忽略**；严格 clippy（含测试）、`pnpm test:tauri` 与 CI YAML 解析通过。本机为 macOS，这些结果不包含 Linux 条件编译的 GIO/Pixbuf/native host 分支。
- 从干净提交 `b81abf555f63d2ef8fbd2280dfec74107af90c02` 完成 macOS Preview `.app` 构建及本地严格 ad-hoc 签名校验；默认托管/显式临时档案包级 smoke 均通过，包含凭据身份隔离、原生通知授权查询、实际 Global 工作区、sidecar readiness、未鉴权请求拒绝及正常退出无残留。生产前端构建的类型/分层/权限/滚动门禁与 bundle budget 同时通过。此包用于已有 macOS host 的回归验证，不覆盖 Linux native 分支或真实桌面应用启动；本地签名也不等同正式签名/公证。
- 新增 `desktop-tauri-linux` Ubuntu 24.04 门禁，配置 GTK/WebKitGTK/GIO/SVG loader 依赖、真实 bridge、严格 clippy 和 `dbus-run-session` 下的全量 Rust 测试；另有 Linux 专属 SVG 缩放/PNG 编码/坏输入回归。**门禁尚未远端执行**，不能声明 Linux 编译或原生图标测试已经通过。Linux GUI/包级验收、复杂 terminal wrapper/Flatpak 安装路径、完整主题继承仍待验证或完善；Windows 已在下一切片增加实现，原生 host 编译/注册表/图标/桌面环境验收继续保留。D/E 目标保持进行中。

#### D：Linux XDG 通知能力、运行中点击与退出清理（2026-09-30）

- 对照 Wails `internal/notify/sender_linux.go` 的 `notify-send` 基线，Linux 改为 Rust 直接调用 [XDG 通知协议](https://specifications.freedesktop.org/notification/latest/protocol.html) 的 `GetCapabilities` / `Notify`。未连接总线或无可用通知服务时报告 `unavailable`；服务可用时授权为 `unknown`，不把服务存在或 `actions` 能力当成用户已授权。Linux 没有协议内的标准授权/DND 查询或授权弹窗，此限制保持可见。
- `clickSupported` 取决于实际 `actions` 能力和已安装的回调。发送前先订阅通知信号，通知服务的唯一 owner、对象路径及接口均严格匹配；仅接受当前 ID 的 `default` 点击，忽略其他按钮、伪造发送者和坏消息。关闭、重复点击、意外 ID 复用、TTL 和容量上限均有保护；点击通过现有档案 token 映射、权威会话查询和主线程窗口恢复路径，不执行任务。后续能力查询或发送失败保留已成功通知的点击映射；服务 owner 替换或连接断开则清理旧 ID。
- 每个 host 懒创建一个通知 worker，不为每条通知创建阻塞等待线程。命令队列限 16 项，两类信号队列分别限 64 / 16 项，映射最多 256 项 / 7 天；单次 D-Bus 调用限 2 秒，整条命令含排队期限 5 秒、调用方等待最多 6 秒。过期命令不派发；应用退出先关闭队列、取消在途请求、释放回调并 join worker，连接关闭限 2 秒，再进入已有 sidecar 停止路径。
- 状态接口改为读取后端实际点击能力，macOS 未打包/不可用进程不再仅因编译目标而显示支持点击；macOS delegate 复用共享的点击恢复实现。前端五个专用通知命令、固定三语提示、权限范围与隐私字段限制继续沿用；Linux 不发送会话 ID、路径、凭据或任意正文，随机 token 仅保存在当前档案。
- 新增 7 项 XDG 单元回归和 1 项后端能力回归。另有 1 项显式联调，在 macOS 临时目录从[官方 D-Bus 1.14.10 源码](https://dbus.freedesktop.org/releases/dbus/dbus-1.14.10.tar.xz)构建测试 daemon 并启动独立私有总线，覆盖无服务、缺少 actions、真实 Notify 参数与静音 hint、回复前点击、伪造/重复/关闭后的点击、服务重启及 ID 复用、能力查询/提交失败不丢失已有点击、真实请求卡住后的超时恢复和退出取消。测试不连接用户的 session bus，也不显示系统通知。
- 真实 Go bridge 的全量 Rust 回归 **189 项通过、2 项默认忽略**（独立 D-Bus 联调已另行显式通过；原生钥匙串 smoke 前轮已显式执行）；严格 clippy（含测试）、`pnpm test:tauri`、CI YAML 解析通过。Linux CI 已增加显式独立总线联调，并将 clippy 扩展至测试代码；**远端门禁仍未执行**。
- 从干净提交 `f8aba01793814b7119c4de7829bf747e42a94d53` 完成 macOS Preview `.app` 生产构建及 `codesign --verify --deep --strict` 本地 ad-hoc 签名校验。默认托管和显式临时档案包级 smoke 均通过，覆盖独立凭据身份、真实 macOS 通知授权查询/点击能力、实际 Global 工作区、sidecar readiness、未鉴权请求拒绝和正常退出无残留；生产前端类型/分层/权限/滚动门禁与 bundle budget 同时通过。此包验证共享状态接口和既有 macOS host 生命周期，不覆盖 Linux native host 或真实系统通知点击；本地签名不等同正式发布签名/公证。
- 本轮仅完成协议与运行中点击实现/联调；Linux 完整 native host 编译、GNOME/KDE 横幅及通知中心操作、Wayland activation token/窗口焦点、安装包与冷启动点击仍待验收或补齐。Windows 原生授权与点击仍待改造，macOS 真实授权/横幅/点击仍待现场验收；D 尚未完成，E 继续等待 D 稳定。

#### D：原生通知授权、发送与点击定位（2026-09-30）

- 对照 Wails `internal/notify` 的固定事件提示，macOS 改用 [Apple UserNotifications](https://developer.apple.com/documentation/usernotifications/unusernotificationcenter) 读取真实授权状态并等待提交结果；仅 `not_determined` 请求授权，拒绝后不重复弹窗。设置页分别显示用户开关与系统授权；错误使用固定解决提示，不输出系统诊断。未打包开发进程报告不可用，不借用 Terminal 身份。Windows 使用 `notify-rust` 同步提交并返回错误，授权暂为 `unknown`、点击能力为 false；Linux 后续已接入 XDG 实际服务/能力查询和运行中点击，详见上一节，均不能据此声称已获系统授权。
- 删除旧通知插件的前后端依赖、注册及三个通用权限；五个专用命令均限定主窗口。发送只接受当前会话身份、事件类型、语言与失败布尔值，拒绝任意标题/正文。系统仅收到固定三语提示、应用标识及平台通知身份，不含问题、工具参数、原始错误、工作区路径或凭据；点击只导航，不审批、不回答、不提交任务。前端保留通知总开关、事件开关与声音偏好。
- token 到会话的映射在当前 core profile 内原子写入 `tauri-notification-targets.json`，Unix 权限 `0600`，与持久凭据档案身份匹配；最多 256 项 / 128 KiB / 7 天。重复 token、损坏、符号链接与外来档案拒绝读取且不覆盖原文件。提交结果不确定时保留映射，确认点击后才删除；持久写入失败保留队列供重试。
- 原生 delegate 在 WebView 就绪前安装并持有，点击队列限 32 项；前端先注册监听再读队列，忙碌/只读页面保留目标，恢复可用时再处理。host 依据当前权威目录及待删除清单解析工作区；缺失/待删除会话跳过且不创建，分页验证未完成则保留。复用现有会话切换流程，同会话只关闭面板并显示对话，卸载后不消费异步回复。
- 回归包括授权拒绝不重问、固定提示与隐私字段拒绝、发送不确定失败、档案重启与隔离、TTL/大小/损坏/权限/符号链接、确认失败回滚，以及真实 Go bridge 的权威目录解析与未知会话不创建。前端覆盖队列去重、忙碌推迟、目标不匹配、切换失败、卸载与确认失败；实际 React 工作区另测冷启动权威路径、已删除会话、同会话关闭设置、监听卸载且零审批/回答/提交。设置页另测只读查询、授权处理中防重复、拒绝状态与开关分别保存。
- 本轮五条新增三语提示使繁体中文 chunk 预算由 78.9 调整至 79.1 KiB，仅此文案上限增加 0.2 KiB；其他 JS/CSS 与简体预算保持原值。`pnpm test:tauri`、工作区通知集成测试、`pnpm test:tauri-build-contract`（36 项）、`pnpm build`、Rust 全量（161 项通过、1 项原生钥匙串测试默认忽略）与严格 clippy 通过；从干净提交 `cedc0c7d6d0a3925a5cd14fc1e7fbf4995dea310` 构建 `.app` 并通过严格本地 ad-hoc 签名校验，默认/显式档案的真实包级 smoke 均通过；只读通知查询返回有效原生授权状态，未申请权限、发送真实通知或执行 OS 点击。
- 真正的系统授权弹窗拒绝/重新启用、通知横幅/通知中心显示、前后台与冷启动的 OS 点击尚未执行；模拟回调和只读系统授权查询不替代这些验收。D 保持进行中，E 仍待 D 验收。

#### D：凭据档案隔离与显式迁移（2026-09-30）

- 新建 core profile 生成 128 位随机身份，保存在 `tauri-credential-profile.json`；原生服务名为 `com.reasonix.desktop.profile.<id>`。元数据仅含版本与身份，Unix 权限 `0600`，同目录无覆盖写入；并发首次创建采用同一胜出身份。可写档案创建 `.backup.json` 备份，备份创建失败不阻断已有有效主文件。主文件损坏时可读取有效备份，两个文件都损坏或两个有效身份冲突则禁用该档案凭据存储并提示恢复，不静默换身份。
- 重启、目录别名和移动保留身份；完整备份/复制包含两个身份文件时仍属于同一凭据档案。独立迁移实验须使用新目录并导入配置，不能把复制身份文件当作创建隔离档案。导入稳定版配置不会复制身份或自动读取稳定版 `reasonix` 钥匙串服务。
- 正常启动只读当前档案服务；旧 `com.reasonix.desktop` 服务和 app data 的 `keychain.dat` 不再自动迁移。设置页“迁移旧凭据”只传当前服务名给主窗口命令，由 host 验证该 Provider 已存在且需要密钥；优先读取旧 JSON 的同名 Provider，缺失时查询旧 Preview 原生服务。仅复制选中条目，保留旧原生服务和原文件，已有目标密钥则拒绝覆盖。回退旧 Preview 二进制时原凭据仍可用；新保存的密钥不会同步回旧服务。
- 旧文件限制 1 MiB / 256 项，拒绝符号链接、重复键、非法类型、控制字符及超长值；全文件验证后才写入选中项。原生写入可能已应用但响应失败时先恢复旧值，桥接响应失败时同时恢复原生存储并尽力对齐 sidecar 内存。恢复无法确认会报告重试/重启；不输出平台错误中的 credential debug 信息或密钥。命令限定 `main` 窗口并在 blocking worker 执行，迁移、保存、删除、bridge 重启共用凭据锁。
- 回归覆盖：不同档案/重启/移动/并发身份、元数据损坏和符号链接；迁移保留与拒绝覆盖、非法/缺失/不可用旧来源、原生不确定写入及删除、桥接失败回滚、迁移与手工写入并发、错误敏感信息过滤、旧版中文及含空格 Provider 名称兼容；前端防重复操作、仅 Provider 身份跨 IPC、未保存输入保留和状态刷新。
- 使用真实 Go bridge 的 Rust 全量 **154 项通过、1 项原生测试默认忽略**；额外显式运行 `native_keychain_profile_isolation_and_cleanup -- --ignored` **1 项通过**，仅使用随机临时命名空间和假密钥并清理条目。真实 bridge 验证导入/保存、重启恢复、删除不复活和文件无密钥。Go config/bridge 凭据定向测试、严格 clippy、`pnpm test:tauri`、`pnpm build` 通过，未放宽 bundle 门限。
- 原生钥匙串单元 smoke 不等同于授权弹窗拒绝、系统锁定或真实 WebView 迁移验收；这些状态及 Windows/Linux 后端继续保留待办。安装包 lifecycle smoke 另核对默认/显式档案的独立身份、私有权限和一致备份，不访问用户旧凭据。

#### D：共享外部链接入口（2026-09-30）

- 原 `bridge.openExternal` 只认识 Wails，Preview 中的 Markdown/Mermaid 会落到 `window.open`。现在保留原导出，转接共享平台 adapter：Tauri 优先调用 `open_external_link`，Wails 保留 `BrowserOpenURL`，普通浏览器保留独立 tab。原生拒绝不会走其他 transport 回退。
- Rust 对网页链接拒绝本地文件、任意应用 scheme、userinfo、控制字符和超长 URL；邮件链接仅允许收件人及 subject/body/cc/bcc，不允许附件参数、地址/标题头注入和 NUL。原有 OAuth `open_external_url` 仍仅接受 HTTP(S)，包括 loopback 回调 URL。
- Markdown 普通点击、中键和菜单使用同一路径；失败提示提供复制链接按钮，并复用现有本地化文案。Preview 入口补上共享 ToastProvider，使这些反馈实际可见。
- `pnpm test:external-links` 的 61 项断言覆盖 Wails、Tauri、浏览器及真实组件的点击/菜单行为，加入 `pnpm test:tauri`；完整前端构建通过且未调整体积门限。Rust 使用真实 bridge 的 129 项测试通过，其中 URL 校验覆盖合法邮件、中文网页、OAuth loopback 与禁止的 scheme/邮件字段。以上是组件与 host 校验测试，系统浏览器/邮件应用的真实打开操作仍需原生 UI 验收。
- 本切片的 `.app` 当时已构建并本地 ad-hoc 签名；同标识 release-candidate 实例曾阻塞独立 host smoke。后续应用发现及凭据隔离切片已在该实例退出后通过启动/退出 smoke，真实浏览器/邮件打开仍待验收。D/E 目标保持进行中。

#### D：macOS 应用发现、工作区打开与默认偏好（2026-09-30）

- `opener_catalog` 通过 [NSWorkspace](https://developer.apple.com/documentation/appkit/nsworkspace) 按固定 bundle ID 查询系统已注册的应用，支持改名与自定义安装位置；标准目录与 Spotlight 索引继续兜底。Spotlight 子进程限时 2 秒、结果限制 8 MiB 并回收进程，采用 NUL 分隔以正确保留含换行的路径。
- 使用 NSWorkspace 原生应用图标与 CoreGraphics 离屏渲染生成 64×64 PNG，避免全尺寸图标导致过大或丢失；图标输出不包含程序路径。展示目录缓存 15 秒；实际打开及保存偏好仍重新查询安装状态。
- Preview 项目会话顶部复用 Wails 的 `ExternalOpener` 选择器与并发偏好协调。工作区打开只传会话 ID 和安装应用 ID，由 Rust 获取 bridge 的 runtime 工作区查询（前轮使用 snapshot 的项目归属 root），拒绝会话身份不符、缺失目录和不可执行文档目标。普通 Markdown 默认打开仍遵循系统默认关联，指定应用打开可使用保存的偏好。
- `GET /v1/settings/desktop` 增加 `externalOpener`，`POST /v1/settings/desktop/external-opener` 使用已有鉴权、请求 ID 去重、共享配置锁及窄 TOML delta 写入。安装检测与原生进程启动留在 Rust；Go 仅保存稳定 ID。偏好写入失败不更新选择；卸载/复制自其他 OS 的 ID 在显示与默认打开时回退文件管理器，再回退首个应用，不重写原配置。
- 本机已验证 Finder/Terminal 真实目录与 64×64 PNG 图标、包含换行/中文的自定义索引路径、会话身份与目录保护。Go 定向测试验证未知配置项保留、坏输入/未鉴权不写入、去重冲突、写入失败保留原件及重启后读取；Rust 真实 sidecar 验证偏好往返保存及重启恢复。
- 前端原生 adapter/菜单/工作区选择器回归、共享链接 61 项、Wails 本地文档 20 项、共享应用选择器 32 项及跨会话偏好 4 项通过；`pnpm test:tauri`、完整 `pnpm build`、Wails 原生应用目录/图标/偏好定向测试、协议生成校验、Go vet、Rust 144 项与严格 clippy 通过。顶栏沿用固定高度且未修改 transcript viewport writer。
- 本切片 `.app` 构建及本地 ad-hoc 签名完成；此前另一份 Preview 已退出，安装包 smoke 已在默认托管与显式 profile 两种临时目录中独立通过：sidecar 就绪、继承环境清理、未鉴权 health 拒绝、正常退出与无残留。未操作用户原 profile，未将该启动测试当作应用菜单或系统程序的 UI 点击验收。
- 剩余：Linux 原生编译与桌面环境验收、Windows 原生 host 编译/注册表/图标/桌面环境验收；真实 WebView/系统应用交互，以及完整 D 现场验收。不能据此标记 D/E 完成。

#### D：本地文档打开、定位与另存为（2026-09-30）

- `app` Proxy 在回落 browser mock 前解析 Tauri 的五个文档 binding；Wails 的同名方法与调用形状保留。Preview 的 Markdown 点击/中键/菜单接入 `local_paths`，默认打开错误现在显示反馈。
- Rust 原生文档操作限定 `main` 窗口，路径必须绝对且存在；独立校验设备路径、Windows ADS/保留名称和 file URL 原始 authority。打开动作拒绝可执行后缀、Unix executable mode 及符号链接指向的可执行目标，定位/复制不执行目标。文件选择器的取消结果为空字符串，不显示成功。
- 另存为先持有源文件句柄，再由系统对话框选择目标；比较文件身份拒绝源文件、硬链接及符号链接别名，采用同目录临时文件、权限保留、sync 与原子替换。复制失败不截断原文件或原目标；未添加通用前端文件系统/任意命令权限。
- 引入官方 [opener 插件](https://v2.tauri.app/plugin/opener/)，替换外部链接的 deprecated shell open；关闭插件自动拦截 JS 链接，所有打开仍通过 host 的校验命令。shell 插件保留供 sidecar supervisor 使用，未开放其前端执行权限。
- 应用选择仅传 native catalog 的 ID，宿主重新检测应用是否存在；不接收 renderer 提供的程序路径或命令参数。macOS 标准目录中的常见编辑器/终端已检测，Ghostty 使用专门 working-directory 参数；完整 Wails 应用发现、图标、跨平台终端及偏好持久化仍待迁移，不能将这部分视为全部完成。
- 验证：Wails 本地文件打开/路径/复制定向 Go 测试、共享链接回归（61 项）、Wails Markdown 点击/菜单回归（20 项）、新增 Preview 原生文档菜单/adapter 回归、`pnpm test:tauri` 和完整前端构建通过；Rust 使用真实 bridge 的 139 项测试与严格 clippy 通过。本机无权限文件测试实际走拒绝分支。组件与文件操作测试未执行系统对话框、Finder 或编辑器的真实 UI 点击，仍保留安装包现场验收。
- 本切片 `.app` 已重新构建并完成本地 ad-hoc 签名。另一份同标识 release-candidate Preview 仍在运行，独立启动/退出 smoke 暂未执行；未退出或操作该运行中的实例。该构建结果不等于系统对话框与指定应用的现场验收，D/E 目标保持进行中。

#### D：配置/状态目录互斥与回退验证（2026-09-30）

- `profilegate.TryAcquireDesktop` 同时保护配置 home 和独立 state home；canonical/文件身份去重避免符号链接及不区分大小写卷上的重复取锁。竞争失败会释放先前取到的目录锁，正常退出与进程崩溃沿用 OS 锁释放机制。
- bridge 在身份库迁移、listener 和后台运行时启动前取锁；Wails 在诊断与主运行时初始化前取锁，持有至 `wails.Run` 返回。冲突启动移除绑定及 startup/shutdown 写入回调，保留原生 single-instance handoff 并在 DOM ready 退出。
- 使用本次构建的真实 bridge，验证 Wails 目录所有者能拒绝“共享配置、不同状态”的第二个进程，且未发布 readiness、未修改配置。使用本次构建的原生 Wails，验证目录被占用时自动退出，原配置不变，未初始化 sessions。
- Rust 导入回归覆盖 Preview 配置被修改后，正式版原件与时间戳备份仍保持原内容。回退方法是在退出 Preview 后重新打开 Wails 原目录；Preview 中的新数据不会自动回写，配置备份不是完整会话备份。
- 门禁：Go profilegate/bridge 启动测试、Wails profile/single-instance 测试（提供 `REASONIX_TAURI_BRIDGE_TEST_BIN` 与 `REASONIX_WAILS_PROFILE_TEST_BIN`），前端构建及 `pnpm test:tauri`，使用真实 bridge 的 Rust 测试（128 项通过）。旧版兼容与真实 UI 待验项继续保留，D/E 目标未完成。
- 当前改动的 `.app` 已重新构建并本地签名，打包 bridge 也已通过上述真实互斥测试；本切片当时因另一份 release-candidate Preview 正在运行而触发单实例转交、在 readiness 前正常退出，未通过该次 host smoke。脚本已在启动前检查同 bundle identifier 的其他 `.app` 副本；后续切片在该实例退出后重跑通过，结果见上方累计门禁。

### 旧格式对话分叉切片验证（2026-09-30）

旧格式会话现可从检查点预览独立对话分叉。提交在原会话目录中生成新的 `tauri-*` 文件名，身份库登记新 ID，界面刷新目录后打开新会话；原 transcript 和工作区文件保持不变。无效方案被拒绝，失败且未生成文件时尝试清理预留身份。隔离测试验证新 ID 可经正常打开流程恢复，路由测试验证会话隔离和请求去重。

### 文件与对话组合回滚切片验证（2026-09-30）

- Preview 使用同一 `both` 方案预览文件与对话，旧格式会话禁用。提交需要覆盖缺口确认；结果区分全部成功与文件事务未完成的部分成功，部分成功时保留新对话版本并提示检查工作区文件。
- 完全成功的组合回滚提供专门撤销入口：文件事务撤销后，若新对话尚未继续，核心会同时返回原 head；若已经继续，只撤销文件，界面显示差异并保留版本导航。
- Go 隔离目录测试覆盖覆盖缺口确认、错误 scope、双侧成功、空与已继续 head 的撤销、提交前手工改文件导致的部分成功及旧格式禁用；bridge 路由覆盖会话隔离与 request ID 去重，前端测试覆盖预览、确认、历史刷新、撤销与部分成功提示。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、`pnpm test:tauri` 与 `pnpm build` 通过。`make lint` 因未安装 `golangci-lint` 未运行；手动 `repolint` 仍报仓库级 ratchet 超限，未更新基线；`internal/tool/builtin` 与 `internal/boot` 的完整测试因沙箱禁止监听回环端口失败。未改动真实 profile，未执行旧版兼容门禁。

### 同日志对话版本导航切片验证（2026-09-30）

- Preview 的检查点面板列出当前 transcript 的活跃版本，可切回原始版本或继续过的回滚版本；切换仅接受 head ID，旧格式会话返回空列表，工作区文件与会话路径不变。列表限制为原始版本、当前版本与最近版本，名称和预览长度受限。
- Go 测试覆盖继续回滚后往返切换、重新打开后保留选择、未知 head 与旧格式拒绝；bridge 路由覆盖会话隔离与 request ID 去重；前端测试覆盖版本列表与历史刷新。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、`pnpm test:tauri` 与 `pnpm build` 通过。`make lint` 因未安装 `golangci-lint` 未运行；手动 `repolint` 仍报仓库级 ratchet 超限，未更新基线；`internal/tool/builtin` 与 `internal/boot` 完整测试因沙箱禁止监听回环端口失败。

### 检查点对话回滚切片验证（2026-09-30）

- 仅对同 transcript 可切换 head 的会话开放；旧格式或文件分支策略在预览阶段禁用，提交再次校验 scope 与会话头。回滚后重新读取历史，文件不变；空 rewind head 可返回原对话。
- Go 隔离目录测试覆盖回滚、同路径、文件不变、错误 scope 和返回原对话；bridge 路由测试覆盖会话隔离与 request ID 去重，前端测试覆盖预览、提交、历史刷新与返回。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、前端完整构建与 `test:tauri` 通过。`make lint` 因未安装 `golangci-lint` 未运行；手动 `repolint` 仍报仓库级 ratchet 超限（含本切片触及的既有超限文件），未更新基线；`internal/tool/builtin` 与 `internal/boot` 完整测试因沙箱禁止监听回环端口失败。未改动真实 profile，未执行旧版兼容门禁。

### 检查点代码回滚切片验证（2026-09-30）

- 隔离目录测试覆盖多文件恢复、覆盖缺口确认、预览后手工改动拒绝、错误 scope 拒绝、对话不变和撤销；bridge 路由测试覆盖会话隔离与 request ID 去重。
- `go vet ./...`、协议生成 `-check`、`cargo clippy --locked --all-targets -- -D warnings`、前端 `typecheck`、`lint:hooks`、`test:tauri` 与 `pnpm build` 通过；构建体积仍低于现有门限。
- 本机 `make lint` 因未安装 `golangci-lint` 未运行；`internal/tool/builtin` 与 `internal/boot` 的完整测试因沙箱禁止监听回环端口失败。未修改真实 profile，也未执行旧版兼容门禁。

### 对话版本与检查点归属（2026-09-30）

- schema-2 检查点保存其边界用户消息 ID；列表、预览、提交及按检查点分叉均核对当前对话版本中的对应消息，防止另一版本在相同位置的新消息误用旧回滚边界。
- 旧的未绑定检查点只在单版本会话中补写消息 ID；已有多个版本且无法证明归属的检查点不会显示为可回滚。回归测试覆盖切换版本、预览后切换及重开会话。
- `internal/checkpoint` 完整测试、`internal/control` 的检查点/回滚/分叉/版本定向测试、bridge 回滚定向测试及 `go vet ./...` 通过。完整 `internal/control`、`internal/tool/builtin` 和 `internal/boot` 测试在需监听回环端口的用例处受沙箱限制；`make lint` 因缺少 `golangci-lint` 未运行。

### 检查点面板回合刷新（2026-09-30）

- Tauri Preview 在回合运行时撤下旧检查点及回滚预览；bridge 快照确认会话空闲后，若面板仍打开则自动重读检查点与对话版本。关闭面板或切换会话后，过期读取结果不会写回当前列表。
- 组件回归覆盖旧读取延迟返回、新检查点自动出现和切换到分叉会话后的数据隔离。
- `pnpm test:tauri`、`pnpm build`（含类型检查、hooks lint 与体积门限）通过。该界面只在 Tauri WebView 中启用，本机未运行真实 host 的交互截图验证。

## 当前事件面

主 Agent 流为 `agent:event`。此外，前端消费了下列 Wails runtime 事件：

```text
InboxChanged                     agent:ready
app:open-settings                config:load-warnings
desktop:shell-status             history-index:changed-v1
project-tree:changed             project-tree:changed-v2
project-tree:runtime-changed     remote-tab:<tabId>:event
remote-tab:<tabId>:state         remote-tab:opened
remote-tab:updated               remote:forwards
remote:server                    remote:status
runtime-state:changed            runtime:rebuilt
session:active-version-changed   session:recovered
session:recovery-failed          tab:meta
terminal:exit                    terminal:output
topic:activation                 updater:progress
agent:event
```

Phase 2 bridge 只承诺 `agent:event` 的语义等价版本。其余事件按上述分组进入
版本化协议；没有对应协议前，Tauri UI 不显示或不启用相关功能，不能伪造本地状态。

## 尚存的直接 runtime 依赖

当前源码中，文本剪贴板、外链、主题与窗口状态的 Wails 原生调用已集中到
`wailsDesktopRuntime.ts`；共享调用方先选择 Tauri/Wails transport，Tauri 拒绝不回落
Wails 或浏览器绕过权限。菜单设置事件也已通过宿主适配器；不能继续将这些列为散落直连。
`bridge.ts` 仍集中消费 Wails 事件；`remoteTabEvents`、`runtimeStateSync`、history/config
/catalog/inbox/project-tree 等订阅及 crash telemetry、若干宿主识别判断尚有 runtime 依赖。
其中 E 的 remote/bot/updater 不在 D 验收前机械迁移，A/B/C 事件与真实进程覆盖仍待补齐。
此源码收敛不代替原生调用方最小权限、物理剪贴板/菜单/拖放等完整验收。

## 迁移守卫

1. 每一组必须先有 Wails adapter 测试，再增加 Tauri adapter 测试。
2. `app` 的调用点不直接感知 transport，不出现散落的 Tauri import。
3. 前端 reducer 按 `(session/tab, sequence)` 去重；不能假定 WebView 事件绝不重复。
4. 任何订阅断线都先获取权威 snapshot，不能凭内存事件重建会话。
