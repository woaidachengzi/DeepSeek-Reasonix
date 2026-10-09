# E 迁移清单

## 2026-10-10 bot 基础批次超过 30 文件 review 收敛

达到 31 文件后暂停扩展并整批 review，修复 scoped 入站消息可指定其它 connection/domain 权限的问题，补真实绑定校验与零回调拒绝回归；最终十次专项、五包完整 race/vet/生成/格式门禁通过，缓存范围与证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-bot-基础批次超过-30-文件-review-收敛)。按规则 review 后提交，不 push；完整 host/watch 消费/过滤/通知、全 local/remote 目录与真实 IM/Plan/Recovery E2E 仍未完成，Desktop 继续未启用。App 仍为 `0366aa636`，无新包或真实账号/迁移/发布，完整目标继续。

## 2026-10-10 精确私有提示快照与 bot 决策 ticket（后续源码）

新增 core/manager 精确归属下的私有提示读取，不回放/补路由/修改提示；bot 随机 ticket 绑定聊天、actor 与完整 prompt/owner 身份，显示刷新不重置单次决策，严格 Ask 解析与 Plan/Recovery/MCP 专用 grammar/codec 已接。实际核心 Ask/Approval/MCP 读取及 Preview Ask→MCP 决策、票据隔离、最终十次专项与五包完整 race/vet/生成/格式证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-精确私有提示快照与-bot-决策-ticket后续源码)。Plan/Recovery 实际 bot E2E、完整 host 注册、watch 消费/过滤/收回通知、全 local/remote 目录和 IM 回流未完成，不能替代原生或真实服务验收。App 仍为 `0366aa636`；累计 30 文件尚未超过提交门槛，无提交/push/新包，下一批继续前准备整批 review 收敛，完整目标继续。

## 2026-10-10 Preview 本地驾驶、收回标记与 watch 存储（后续源码）

补齐绑定实际本地 Controller/聊天/actor 的驾驶组件及 manager 原子 LocalInputVersion 收回 fence，同 ID 模型替换和本地发送后旧绑定不可复用。watch 保存 actor 并以现有 strict 无凭据事务逐路由落盘，保留其它配置、失败后的本进程状态和并发顺序，旧无 actor/重复归属不启用。实际 Controller 与临时配置专项十次 race、四包完整 race/vet/生成/格式检查通过，范围及证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-preview-本地驾驶收回标记与-watch-存储后续源码)。完整 Preview DesktopBridge 尚未注册：事件消费/过滤/通知、五类提示、完整 local/remote 目录与实际 IM 联动未完成，不能替代原生验收。App 仍为 `0366aa636`，累计 26 文件未达超过 30 文件先 review 后提交门槛，无提交/push/新包，完整目标继续。

## 2026-10-10 bot 入站精确 Desktop 命令入口（后续源码）

实际 gateway 入站命令和继续接管可选择精确宿主入口，保留聊天/连接/认证 actor 与 context，回答不分离查找/提交，失败不退回 ID-only 接口或自动重试；修复 watch status 别名及解除未确认时的错误成功文案。实际入站权限/跨连接隔离等专项十次 race、三包完整 race/vet/生成/格式检查通过，缓存范围和证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-bot-入站精确-desktop-命令入口后续源码)。Preview scoped host 尚未注册，Desktop 仍未启用，watch 消费/持久化/过滤、实际接管收回和 IM 回流未完成；App `0366aa636` 不含本批。累计 20 文件未达超过 30 文件先 review 后提交门槛，无提交/push/新包，完整目标继续。

## 2026-10-10 bot Desktop 私有事件来源与发布生命周期（后续源码）

补齐绑定实际 manager/Controller 双代际的私有事件来源，以及打开、模型、effort、配置重建四条发布路径；旧来源关闭退役旧观察者，队列异常明确要求新快照。实际 Controller 的 Ask/decision、模型替换及配置候选失败保留/成功退役回归，最终专项十次 race、两包完整 race、vet/生成/格式检查通过，初稿失败与修复证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-bot-desktop-私有事件来源与发布生命周期后续源码)。尚未启用 bot Desktop 或接入 IM/watch/权限过滤/接管回流，不能标记完整 bot 或原生验收完成；App 仍为 `0366aa636`、不含本批源码。累计 16 文件未达超过 30 文件先 review 后提交门槛，无提交/push/新包，完整目标继续。

## 2026-10-10 bot Desktop 的本地精确命令归属基础（后续源码）

新增当前 Preview 本地 runtime 的精确观察/提交/决策基础：manager 与 Controller 双代际、路径/轮次/idle revision/prompt routing fence，复用原子提交和严格五类 resolver；不按旧 ID fallback，不恢复/切换或自动重建模型。实际 Controller 的文本、真实 Ask/durable decision、重复/撤销/同会话模型重建隔离，以及十次专项和两包完整 race/vet/生成器通过；回归另修复三个旧测试继承状态目录造成标题/删除/图片历史污染，失败记录和复验证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-bot-desktop-的本地精确命令归属基础后续源码)。这不是完整 bot 会话/watch/审批/接管/回流/本地收回或 native/外部账号验收，Desktop Bridge 仍未启用。累计 8 文件未达超过 30 文件提交门槛，未提交/push/重建 App，完整目标继续。

## 2026-10-10 历史文件失败复验与配置隔离

截图中的两个历史文件测试在当前 bridge 包复验通过；补齐它们及相邻 Git preview 测试的独立配置/状态/缓存档案，并增加附件原件不变、副本非同一文件断言。三个专项十次 race、完整 bridge race、vet 和格式/diff 检查通过，范围及原始误筛选记录见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-历史文件失败复验与配置隔离)。当前桌面接口超时，CGSession 返回 unavailable，不能沿用旧锁屏结论或标记原生可见 UI 通过。bot Desktop 仍未接入；当前 3 个文件未达超过 30 文件提交门槛，无提交/push/产品包变化，完整目标继续。

## 2026-10-10 提示卡片 macOS App 重建与隔离启动

12 文件完成 review 并提交 `0366aa636`，无 push；从干净提交构建 arm64 可运行 App，含提示协议/native 基础及五类实际卡片。原完整 frontend build、Rust release、本地 ad-hoc 严格签名和实际 App managed/explicit 独立临时档案 smoke 均 exit 0；旧 App 已备份。包路径、哈希与实际证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-提示卡片-macos-app-重建与隔离启动)。没有正式签名/公证/发布，也未使用真实配置/API key。隔离 smoke 仅验证原生启动/档案/sidecar/鉴权/退出链路，不替代可见窗口交互、真实 SSH/bot/外部 MCP 或其他完整目标门禁。

## 2026-10-10 提示卡片提交前复核

本批 12 文件 review 完成，未发现阻塞提交的问题，diff 检查通过；为形成可复现 App 版本先提交再打包，不 push。遵循后续超过 30 文件先 review 后提交规则。构建及原生隔离 smoke 尚待执行，不能提前标为包级通过。

## 2026-10-10 远程五类提示卡片与刷新草稿归属（App 后续源码）

实际远程历史页已复用 Ask/Approval/Plan/Recovery/MCP 卡片，显式事件接单次精确执行层；unknown 不退场、不自动重试，receipt 不清卡片，同身份刷新保留草稿、换 generation 只退役请求，reconcile 释放旧刷新锁。禁用本地文件引用，URL 只经显式点击与现有 native URL 校验。完整原 Transcript/Tauri/类型/build、既有审批动画和实际 Chrome 桌面/窄屏五类交互与截图证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程五类提示卡片与刷新草稿归属app-后续源码)。当前 12 个变更文件，未触发超过 30 文件先 review 后提交阈值，未提交/push/重打包。现有 App `49f7a76bd` 不含本批及已提交 `18d8654ba` 基础，原生包/实际 SSH/bot 和完整目标其余门禁仍待验证，不能以 Chrome fixture 代替。

## 2026-10-10 远程 prompt 决策归属与超过 30 文件 review 收敛

累计达到 31 文件后暂停扩展，完成本批 scoped Controller/Serve/shared client/SSH bridge/schema/native/展示投影与单次前端决策执行层 review。修复 Go Ask 对 null 选项及嵌套重复决策键的错误接纳；最终 Go 四包 race/vet/生成、Rust 299 项/clippy、完整原 Transcript/Tauri/类型/build 预算通过，证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程-prompt-决策归属与超过-30-文件-review-收敛)。按先 review 后提交规则收敛，不 push。执行层尚未挂到实际卡片，App `49f7a76bd` 不含本批；原生可见交互、真实 SSH/bot 和完整目标其余门禁未完成，不以本批源码回归替代。下一步复用卡片时须保留草稿/挂载身份、等待事件确认清除，禁用本地文件引用入口，避免现有审批点击退场掩盖 unknown。

## 2026-10-10 远程 pending prompt 的展示恢复（App 后续源码）

验证后的 replay cut 和 live 事件现可从共享 reducer 导出五类 display-only 提示快照；保留精确 turn/id/独立 routing stamp，深复制隔离 transport/consumer，旧回执或无身份回执不能清掉当前提示，身份异常/歧义不暴露可操作卡片。完整原 remote-controller 回归和两份 TS 类型检查通过，证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程-pending-prompt-的展示恢复app-后续源码)。这里只完成展示投影，实际卡片与用户决策入口、原生可见交互尚待接验，App `49f7a76bd` 不含本批。累计 29 文件，未达超过 30 文件先 review 后提交阈值；无提交/push/新包/真实账号或发布，完整目标继续。

## 2026-10-10 精确远程 prompt 的原生 IPC 与连接层（App 后续源码）

主窗口限定 typed native session-prompt、严格五类输入/收据以及可选前端 adapter/pool/lease 已接，保留实例与 prompt routing 双身份、单次 dispatch/unknown/no retry，await 前复制嵌套 answer，释放后的旧收据不影响新 owner。Rust 299 项/clippy、实际 loopback/前端 binding/pool 回归、原完整 remote-controller/类型和 Go 四包 race/vet/生成证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-精确远程-prompt-的原生-ipc-与连接层app-后续源码)。实际 prompt 恢复/卡片和原生交互尚未接验，App `49f7a76bd` 不含本批，完整目标继续。累计 27 文件未达超过 30 文件先 review 后提交阈值，无提交/push/重打包或真实账号发布。

## 2026-10-10 精确远程 prompt 的 Go bridge 与协议镜像（App 后续源码）

接入 bridge token/已保存 SSH owner 下的固定 session-prompt/capability，补 schema 五类 answer 与 kind 关联、实际 TS/Rust 生成和 Go DTO conformance；严格单次 scoped decision、私密收据剥离、unknown/no retry，不借本地 RuntimeManager。自有 SSH 转发专项、实际 schema 正反例、最终 Go 四包 race/vet/生成 check、Rust 296 项/clippy 与类型证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-精确远程-prompt-的-go-bridge-与协议镜像app-后续源码)。Rust native 命令/前端恢复决策尚未接，App `49f7a76bd` 不含本批，不能当作原生或实际 SSH/bot 完成验收。累计 19 文件未达超过 30 文件先 review 后提交阈值，无提交/push/新包，完整目标继续。

## 2026-10-10 精确远程 prompt 的 Serve/shared client 链路（App 后续源码）

接入认证固定 session-prompt 与 typed shared Client：既有 owner/精确双 epoch/turn/prompt、后台退役 gate、严格五类 answer union、单次发送/未知不重试，不 resume/切前台/扩大执行权限。实际 Agent/Serve 前后台 Ask、身份/认证/foreign writer/退役/重复/owner 撤销及最终三包专项和完整 race/vet 证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-精确远程-prompt-的-serveshared-client-链路app-后续源码)。其它类型未获远程端到端验收，bridge/schema/native/UI 尚未接，不能算远程审批完成；App `49f7a76bd` 不含本轮源码，完整目标继续。累计 11 文件未达超过 30 文件先 review 后提交阈值，无提交/push/重打包/真实账号或发布。

## 2026-10-10 精确远程 prompt 接纳基础（App 后续源码）

新增 path/Controller 实例/活动轮次与独立 prompt routing identity 的精确接纳入口，锁等待后检查撤销，复用现有五类专用 resolver/durable transition，不 resume/切前台/扩大权限。实际 Ask/Approval/MCP 专项十次 race、完整 Controller race/vet 与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-精确远程-prompt-接纳基础app-后续源码)。尚未接认证 Serve/shared client/bridge/native/UI，不能算远程审批完成或原生验收；当前 App 来自 `49f7a76bd`、不含本轮源码，完整目标继续。累计 5 文件未达超过 30 文件 review 后提交阈值，无提交/push/新打包/真实账号。

## 2026-10-10 已提交远程发送 macOS App 与包级 smoke

已从干净提交 `49f7a76bd` 重建 arm64 ad-hoc 可运行 App，包含 scoped Send 后端/native/UI，原完整 build gates/预算及实际严格签名通过；managed/explicit 两个独立临时档案的启动、私有身份、401 鉴权、Global workspace、只读通知查询、普通退出和 sidecar 清理通过。旧包完整备份、新包 SHA 和日志见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-已提交远程发送-macos-app-与包级-smoke)。未以包级 smoke 替代 WKWebView 可见交互或真实 SSH/bot，完整目标其余门禁继续；没有正式签名/公证/发布、默认下载切换或真实账号/数据迁移。本轮仅两文档追加收据，未提交/push，以下保留历史阶段状态。

## 2026-10-10 远程发送界面与超过 30 文件 review 收敛

达到 32 文件后暂停扩展，先 review 再提交，不 push。补齐 typed native adapter/pool 与 owner-fenced 发送框；修复 settled cut 不含活动 ID 的边界，精确事件序列/idle scope 校验、防重复、未知保留草稿/刷新、旧响应隔离以及同订阅问题/回答展示通过。最终 Go race/vet/生成、Rust 296 项/clippy、完整 Transcript/Tauri/类型/原 build 预算与实际 Chrome 桌面/窄屏交互截图证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程发送界面与超过-30-文件-review-收敛)。accepted 不是回答/保存完成；当前 App a678bf4f7 尚未包含本批源码，提交后需重建及隔离 smoke。浏览器 fixture 不替代 WKWebView/真实 SSH/bot，完整目标与其它未完成门禁保留，不发布/默认下载切换或使用真实账号。以下保留历史阶段状态。

## 2026-10-10 远程发送的 Go bridge 与原生 IPC（App 后续源码）

固定认证 bridge session-submit/capability、schema 与真正 TS/Rust 生成/Go conformance、main-only typed native 命令已接，保持 SSH owner/精确 scope、单次发送与 Unknown/no retry，不借本地 RuntimeManager。自有 SSH 转发和 Rust loopback 专项、最终 Go 四包完整 race/vet/生成检查、Rust 296 项/clippy 及类型/既有 bindings 证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程发送的-go-bridge-与原生-ipcapp-后续源码)。accepted 不是回答/保存完成。未接前端发送与同订阅/迟到响应回归，App a678bf4f7 不含本批源码，不能替代 WKWebView/实际 SSH 或完整目标验收。累计 22 个文件未达超过 30 文件先 review 后提交阈值，无提交/push/新包/正式发布或真实账号。

## 2026-10-10 远程发送的 Serve/shared client 链路（App 后续源码）

独立认证 POST/session-submit 接入精确 path/Controller epoch/revision 的原子用户消息接纳，并以现有后台 admission gate 防止 idle owner 关闭竞态；不 resume/切前台/解释管理命令或借用本地提交。typed shared Client 单次请求、exact 收据与未知结果/no retry，以及隔离真实 Agent/Serve 的前台/后台、旧版本/重复/认证/saved/retiring/外部 writer/不切前台回归和最终三包完整 race/vet 证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程发送的-serveshared-client-链路app-后续源码)。accepted 仅为接纳，不是回答/保存成功。未接 bridge/native/UI 与完整恢复/审批/接管/bot，现有 App a678bf4f7 不含本批源码，不替代原生或真实 SSH 验收；完整目标继续。累计 9 个未提交文件，未达超过 30 文件 review 后提交阈值，无提交/push/新打包。

## 2026-10-10 远程发送的原子 Controller 接纳基础（App 后续源码）

新增精确 path/Controller epoch/RuntimeState revision 下的 idle 原子接纳入口，拒绝旧页面、重复、撤销和 finishing 排队，不解释管理/shell 命令，也不 resume/切前台/获取写租约。最终专项 20 次 race、完整 Controller race/vet 与状态发布竞态证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程发送的原子-controller-接纳基础app-后续源码)。nil 仅表示接纳，不代表 provider/保存成功；尚未接远程 HTTP/shared client/native/UI，不能算完整发送功能或原生/真实服务验收。当前 App 为 a678bf4f7、不含本轮源码，完整目标保留；累计 4 个未提交文件，未达超过 30 文件 review 后提交阈值，无提交/push。

## 2026-10-10 已提交远程 Stop macOS App 与包级 smoke

已从干净提交 `a678bf4f7` 重建 arm64 ad-hoc 可运行 App，包含远程 Stop 后端、native IPC 和前端按钮；原完整 build gates、实际严格签名及 managed/explicit 临时档案包级启动、认证隔离、Global workspace、普通退出与 sidecar 清理通过。旧包完整保留，新包 SHA 和日志见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-已提交远程-stop-macos-app-与包级-smoke)。桌面仍锁屏，未宣称 WKWebView 远程 Stop/终端/图片或真实 SSH/bot 验收；跨平台、管理页、SQLite 和已知旧失败仍待收敛，完整目标不关闭。没有正式签名/公证/发布或默认下载切换。本轮仅两份文档追加包收据，未提交/push，以下保留历史阶段状态。

## 2026-10-10 远程 Stop 界面与超过 30 文件 review 收敛

达到 31 文件后暂停扩展，review 修复公共按钮 hidden 样式并补回归，最终 33 文件先 review 后提交，未 push。接入前端 owner-fenced Stop，不采样新轮次、不伪造终态，unknown 需手动刷新，旧结果不能影响新 surface；React/测试技能及 Go/Rust/完整 Transcript/Tauri/类型/构建预算、实际 Chrome 的桌面/窄屏交互与截图证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程-stop-界面与超过-30-文件-review-收敛)。当前 App 仍为 6a026104b，尚未重建；浏览器 fixture 不替代 WKWebView/实际 SSH 远程 Stop 验收，发送/审批/接管/bot 与完整目标其它门禁继续未完成，未发布或切换默认下载。

## 2026-10-10 远程 Stop 的 Go bridge 与原生 IPC（App 后续源码）

补齐 bridge token/已保存 SSH owner 下的固定 Stop route、typed schema 与生成/Go conformance、主窗口限定 Rust IPC；保持只读 projection，无本地 RuntimeManager/fallback/重试，句柄撤销或收据不确认不发表成功。Go bridge/shared/真实 Agent Serve 专项、Rust 293 项/clippy、协议生成/类型/绑定门禁见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程-stop-的-go-bridge-与原生-ipcapp-后续源码)。未接前端 lease/pool/按钮或验证原生交互，没有新 App/外部账号验收；发送/审批/接管/bot 与完整目标其它门禁仍未完成。当前 App 为 6a026104b，23 个文件未达超过 30 文件 review 后提交阈值，无提交/push/发布。

## 2026-10-10 远程指定轮次 Stop 的 Serve/shared client 链路（App 后续源码）

接入独立认证 HTTP endpoint 与一次性 typed shared Client Stop，不使用 legacy Cancel/resume/接管/重试；校验当前已持有 Controller 的 path/实例/轮次并在锁等待后检查请求撤销。隔离真实 Agent/Serve 的 foreground/detached context 与 interrupted terminal、旧轮/错误实例/saved/retiring/外部 writer/认证拒绝、客户端不确定结果和 no retry 三次 race/vet 见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程指定轮次-stop-的-serveshared-client-链路app-后续源码)。尚未接 native/UI，不将现有只读 projection 升为写权限；发送/审批/接管/bot/原生与其它完整目标门禁继续未完成。累计 10 个文件未达超过 30 文件 review 后提交阈值；当前 App 仍是 6a026104b，没有提交/push/重打包/真实账号或新的原生验收。

## 2026-10-10 指定远程轮次停止的 Controller 基础（App 后续源码）

补充精确 CancelScoped 原语，使用实际 Controller 实例 epoch/会话路径/活动轮次，原子校验并取消该 context、只清理该轮次交互；旧请求与旧 cleanup 不影响后续对话。普通 Serve 的实例 ID 与可为空的 Desktop 事件路由 ID 已区分，专项 20 次 race/vet 证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-指定远程轮次停止的-controller-基础app-后续源码)。尚未接远程 HTTP/shared client/native/UI，remote projection 不因此变成可写；当前 App 仍为 6a026104b，完整目标未完成。累计 5 文件，未触发超过 30 文件先 review 后提交阈值，无提交/push/重打包/真实账号或新的原生验收。

## 2026-10-10 已提交 admission/recovery macOS App 与包级 smoke

已从干净提交 `6a026104b` 重建 arm64 ad-hoc 可运行 App，包含上一批入库/host 续接及协议恢复修复；完整 build gates、严格签名和新实际包的 managed/explicit 临时档案启动、私有身份/认证边界/Global workspace/普通退出及 sidecar 清理均通过，旧包完整保留。新包 SHA 与日志见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-已提交-admissionrecovery-macos-app-与包级-smoke)。桌面仍锁屏，没有新的可见 UI/原生图片粘贴/终端/live 或真实远程账号/bot 验收，不能替代其它完整目标门禁；没有 DMG、正式签名/公证/发布或默认下载切换。本轮仅两文档追加收据，未提交/推送，以下保留各阶段状态。

## 2026-10-10 admission 与协议恢复批次 review 收敛

本批在 29 文件阶段先主动 review，发现并补齐 snapshot/projection 回归的专用 remote 脚本登记，最终 30 文件随本记录提交，未推送。当前完整 Go 八包 race/vet、Rust 290 项/clippy、完整 Transcript/Tauri/remote-controller/类型/lint/single writer 和实际新登记脚本通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-admission-与协议恢复批次-review-收敛)。已知 Wails 五项旧失败仍复现，未宣称其全量通过；当前 App 尚未重建、不含这批修复。接下来从已提交源码构建安全包，再继续远程控制及完整目标其余未完成门禁；以下保留各阶段当时状态。

## 2026-10-10 协议恢复 canonical 元数据可信边界（App 后续源码）

为实际 pending→consumed protocol metadata 改写增加原子 local-only 证明，在 durable checkpoint 后严格复原旧 prefix digest、校验身份/来源/其余 incident 字段，再推进新 digest；不忽略 metadata、不放行一般历史改写、不传证明到前端。真实 Agent/Serve/token SSE/shared client 的活动续接与固定分页已可读，同 ID 用户正文改写仍 409；三次真实回归、最终完整 Controller/wire race、相关 vet/生成检查见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-协议恢复-canonical-元数据可信边界app-后续源码)。当前 29 文件未超过提交阈值，未提交/推送/重打 App，不能据此宣称 native/SSH/bot/跨平台/管理页/SQLite 或全部目标验收通过；以下记录保留各阶段当时状态。

## 2026-10-10 真实协议恢复来源与 Serve 链路（App 后续源码）

修复协议恢复 context 的 host 来源被普通用户编排覆盖：同步/异步入口改用既有 synthetic 编排，真实 Agent/Controller/Serve/token SSE/shared client 验证 canonical 身份、无新增用户问题/checkpoint、私密续接不展示。最终真实链路三次 race 与相关 vet 通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-真实协议恢复来源与-serve-链路app-后续源码)。同时确认活动协议恢复的 pending→consumed 元数据改写触发严格 prefix fence/409：可信 canonical rewrite 恢复边界仍待实现，仅取消后的终态重新同步通过，不能算实时展示已完成。当前 25 文件未达超过 30 文件提交阈值，未提交/推送/重打 App；原生/真实账号及其它完整目标门禁仍未关闭。

## 2026-10-10 同订阅 host 续接展示与恢复（App 后续源码）

Renderer 已接 host readiness：完整 cut 分页后确认 canonical origin/ID，无新增用户问题或虚构 parent，同订阅恢复跨轮/早期 body、压缩及快速终态；错身份、混合来源、epoch 变化、legacy 无边界输出和 dispose 仍拒绝。专项、类型/lint、完整 transcript 与 remote-history/scroll writer 门禁、实际 React 的桌面/窄屏 fixture 交互与截图检查通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-同订阅-host-续接展示与恢复app-后续源码)。不是 WKWebView/真实远程服务或完整目标验收，当前 App 未包含上轮传输和本轮恢复修复。累计 22 文件，未达超过 30 文件提交阈值，未提交/推送；其它原生/控制/bot/跨平台/管理页/SQLite 门禁保持未完成。

## 2026-10-10 host 入库边界与 admission 真实载荷（App 后续源码）

新增独立、无正文的 host-input canonical 入库事件及同步 durable barrier，不将合成 continuation 当新用户问题；投影拒绝来源改写/重复身份。追查并修复 native 与 shared remote SSE/cut 原来仅允许 steer 携带 messageId、误拒绝真实用户 admission 的缺口，正向真实载荷、隐私剥离和缺失/非法 ID 负向回归通过。Go 完整相关 race/vet、Rust 290 项/clippy、类型及既有 renderer 回归见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-host-入库边界及-canonical-admission-传输收敛app-后续源码)。尚未接 renderer 的系统续接恢复，没有新原生/真实服务验收；最新 App 未含这些后续修复，完整目标不关闭。累计 18 文件，未达超过 30 文件提交阈值，未提交/推送。

## 2026-10-10 已提交源码 macOS App 与实际包 smoke

已从干净提交 `ebd1bcc50…` 重建当前 arm64 ad-hoc App，含最后的 admission wire/bot 路由修复；完整构建门禁、严格签名复核与这个实际包的 managed/explicit 临时档案启动、Global workspace/凭据身份隔离、普通退出与 sidecar 清理通过，旧 App 完整保留。证据与 SHA 见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-已提交源码-macos-app-与实际包-smoke)。桌面仍锁屏，没有重跑或宣称可见 UI 验收；这不是终端/live/图片/真实远程服务验证。完整目标及其它门禁继续保留，未推送/公证/发布。本轮仅新增两份文档的验收记录，未达超过 30 文件提交阈值；以下记录保持各阶段当时状态。

## 2026-10-10 超过 30 文件的 review 收敛

本批 32 个文件，停止功能扩展并按先 review 后提交规则收敛。Review 修复 bot 路由遗漏聊天类型/字段边界的身份碰撞；真实 hub 回归确认同 ID 群聊不能继承或解除私聊订阅/接管。远程投影与同步、admission wire 白名单、canonical content fence、React 生命周期及终端 opt-in 探针一起复核，Go/Rust/完整 transcript/类型/lint 等验证见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-超过-30-文件的-review-收敛)。独立 Wails desktop 全量有五项失败，全部在本批之前的 HEAD 临时快照复现（两项 checkpoint、三项旧 MiMo 断言），保留待独立处理，不宣称全量通过。最新 App 未重建，不含最后的 wire/bot 修复；原生可见性、完整 bot Desktop 接入及目标其它门禁仍未完成，不以本批源码回归替代。未推送/发布，以下阶段记录保留当时状态。

## 2026-10-10 增量 review 与 admission wire 收敛

只读检查确认当前桌面锁屏，与上轮 key window/可见性失败一致；原生绘制仍待解锁后的独立验收，未据此认定全部历史根因。Review 修复 admission wire 可能携带误附正文/审批身份的契约缺口，新增回归先复现再按六字段白名单收敛。关联七包完整 race、修复后三包 race、vet/生成器、Rust 289 项/clippy 和前端专项/生命周期通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-增量-review-与-admission-wire-白名单)。bot Desktop 仍需完整会话/watch/审批/显式接管链路，未用只读列表替代。最新 App 尚未含这一 wire 修复；29 个文件未达超过 30 文件先 review 后提交阈值，未提交/推送，完整目标继续保留。

## 2026-10-10 当前包原生终端复验

新增显式主屏测试选项（默认左屏约束不变）、固定布尔绘制诊断及原生激活/可见性前提；runner 4 项、格式和全量 build gates/签名通过。最新 arm64 App 已含 early admission 修复；旧包和中间诊断包完整备份。实际 managed/explicit 原生终端复验未通过可见文本，后续诊断证明页面当时不可见/不聚焦，最终候选在更早 native 激活前提失败，不能据此宣称 terminal transport 故障或成功，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-当前包原生终端复验与可见性前提)。未使用真实账号/数据/剪贴板，未公证/发布/提交/推送；29 个文件未达超过 30 文件先 review 后提交阈值。原生可见性、完整终端/图片/live 与完整目标剩余门禁继续保留。

## 2026-10-10 早期快照 buffered admission 修复（App 后续源码）

新增回归先复现 append 前空问题快照丢失已缓冲 canonical admission 的同步竞态，再修复为沿原身份 gate 消费缓冲并同订阅重取可信完整 cut；不发布空问题或轮询。专项/类型/lint/scroll writer 和隔离浏览器的实际竞态恢复、跨轮/压缩展示与 cleanup 通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-早期快照与已缓冲-admission-竞态app-后续源码)。最新 App 不含这一后续修复，仍须后续打包；legacy/synthetic 关联边界、原生和真实服务及完整目标剩余门禁保留。26 个变更文件，未达超过 30 文件先 review 后提交阈值，未提交/推送。

## 2026-10-10 当前远程 admission / 跨轮 / 压缩增量 App

已将当前 26 个 dirty 文件重建为 macOS arm64 ad-hoc 可运行 App，包含下述 admission、跨轮、内容 fence 与压缩后实时展示。全部现有 build gates、Go/Rust release、bundle budgets、包内 deep/strict 签名与完整 transcript 等价回归通过；实际包在两个全新临时档案的启动、Global workspace 隔离、普通退出和 sidecar 清理通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-admission--跨轮--压缩增量-macos-app)。旧 App 保留临时备份。未公证/发布/提交/推送；26 文件未达超过 30 文件先 review 后提交阈值。不能替代 WKWebView live/reader/窗口重建或真实服务验收，完整目标继续保留。

## 2026-10-10 压缩后的可信快照与实时展示（App 后续源码）

正常 provider projection 压缩完成后，同订阅重取可信完整 cut，校验通过才恢复共享卡片与回答流；直接和缓冲完成事件同路处理，真实 canonical rewrite 仍严格拒绝。专项/类型/lint/scroll writer/Kernel 53 项及桌面/窄屏隔离 Playwright 的卡片展开、后续回答和清理通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-压缩完成后的可信-cut-与实时展示app-后续源码)。不是 WKWebView 或真实服务验收，当前 App 未重建；真实 rewrite、legacy/synthetic continuation 与完整目标剩余门禁继续保留。26 个变更文件，未达超过 30 个文件先 review 后提交阈值，未提交/推送。

## 2026-10-10 prefix 内容 fence 与压缩语义（App 后续源码）

修复 canonical prefix 只核对 ID 的缺口：admission 原子保存身份及 canonical digest，投影拒绝 same-ID 内容改写，恢复原内容后可读，不干预引擎。完整 controller race、Agent 专项/vet/格式门禁通过；实际 CompactNow + 假 summarizer 证明 provider context projection 压缩保持 canonical IDs/digest，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-canonical-prefix-内容-fence-与压缩语义app-后续源码)。renderer 尚未接压缩后的可信新 cut，真正 base rewrite 与其它完整目标门禁仍未完成。26 个变更文件，未提交/推送/重打 App。

## 2026-10-10 同订阅跨轮 resnapshot（App 后续源码）

coordinator 等 canonical admission 再串行取得新完整 cut，同订阅/owner 不变，旧成功/失败/continuation 零发布；早期 active 空 suffix 等事件而非轮询。核对问题 ID、seq 与 runtime epoch，快速 terminal overlap 不误作新 turn，React 同步忙状态沿用 owner fence。专项/类型/lint/scroll writer/Kernel 53 项及桌面/窄屏隔离 Playwright 跨轮渲染通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-同订阅-canonical-admission-跨轮-resnapshotapp-后续源码)。compaction/base/legacy/合成 continuation 恢复和实际 App 的跨轮/reader 验收仍待完成，当前 App 不含本阶段源码。23 个变更文件，未达提交阈值；完整目标继续保留。

## 2026-10-10 canonical 用户 admission 边界（App 后续源码）

新增只含规范 messageId 的 `user_message_admitted`，在真实用户消息原子 append 后发布，Controller durable ledger 先于回调；合成 continuation 不发用户 admission，投影确认 suffix identity、不重复问题。Go race/vet、Agent/Controller 时序、native projector 4 项、契约与前端专项/类型/lint 通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-canonical-用户-admission-事件app-后续源码)。还需 coordinator 新轮 resnapshot 与 compaction/base 恢复；已交付 App 不含本阶段源码。累计 21 个变更文件，未达提交阈值，不关闭完整目标。

## 2026-10-10 当前增量 macOS 可运行 App

已将 `23a57a58b` 加 12 个未提交文件重建为 arm64 ad-hoc App；全部现有前端 build 等价门禁、Go/Rust release、包内 deep/strict 签名验证通过。实际 App 的 managed/explicit 临时档案启动、Global workspace 隔离、普通退出与 packaged sidecar 清理通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-当前远程-snapshotlive-增量-macos-app)。旧 App 已备份，没有公证/发布/推送或提交。不能把 package smoke 当作 WKWebView live UI 或真实远程服务验收；完整目标与恢复/跨平台/管理页/SQLite 门禁继续保留。

## 2026-10-10 远程 live history React 接入（未提交增量）

Serve-owned 历史页已接 ready-bound 快照、共享投影和实时事件；保存历史仍是静态只读。刷新/卸载退休旧订阅，reconcile 不被迟到 body 锁住，相同 controller 值不重订阅；三语文案更新。JSDOM 生命周期、投影/snapshot、类型/lint/scroll writer 与完整 transcript 等价回归通过，Playwright 自有夹具的桌面/窄屏快照/live/刷新/迟到回调/关闭和控制台检查通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-远程-live-history-react-接入未提交增量)。尚非原生或真实服务验收，跨轮/admission/compaction/legacy 恢复、App 打包与完整目标剩余门禁仍待完成；未推送。当前 12 个文件，未达提交阈值。

## 2026-10-10 shared reducer 远程 conversation 投影（未提交增量）

远程 fixed cut 已接纯 conversation 投影：复用共享 event reducer 与 history 展示，保留 backend/surface 身份、stream rollback 与 tool 结果；steer messageId 对应 saved notice，旧 local fallback 不变，输出不授予 local actions/commands。专项、shared stream 128 项、类型/lint 门禁通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-shared-reducer-远程-conversation-投影未提交增量)。尚未接实际 React 渲染或原生验收；admission、compaction、legacy steer 与跨轮仍须恢复设计，没有新 App 或推送。当前 7 个文件未达 review 后提交阈值。

## 2026-10-10 remote snapshot 同步层 review 提交

本批超过 30 个变更文件，已停止扩展并 review，按门禁收敛提交。新增前端 snapshot 窄 binding 与 ready 后读取/固定 cut 分页/live 缓冲协调层，贯通已完成的 shared client、bridge 与 native IPC；相关完整 Go race/vet、Rust 289 passed/6 ignored/clippy、前端专项/类型/lint 及契约门禁通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-10-remote-snapshot-同步层-review-提交)。仍未接实际渲染/统一 Item reducer，也未完成跨轮、admission/compaction/legacy steer 或原生滚动/窗口重建验收；没有新 App 或推送。完整目标继续推进，不能以这些源码/夹具回归替代原生或真实服务验收。

## 2026-10-09 ready 订阅绑定的 native snapshot IPC（未提交增量）

已注册 main-only snapshot command，只接受 ready 订阅身份及 bridge continuation；读前后核对 owner/surface generation，独立 socket 随订阅退休取消，每项订阅最多一个在途读取，结果携带完整 identity。专项 TCP 取消/ready/scope 门禁、完整 Rust 289 passed/6 ignored、clippy 与格式/生成器门禁通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-ready-订阅绑定的-native-snapshot-ipc未提交增量)。还需 renderer binding、快照/live reducer、同步边界和实际 App 验收；没有新 App、提交或推送。累计 26 个文件，未达超过 30 个文件先 review 再提交阈值。

## 2026-10-09 native projection transport 与类型契约（未提交增量）

补齐生成的快照契约与 native 初始/续页读取、历史身份及事件投影校验；同步拒绝远程 system/developer 提示词角色。完整 Rust 285 passed/6 ignored、clippy、生成器/Go DTO、类型与局部格式门禁通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-native-projection-transport-与类型契约未提交增量)。尚未注册 snapshot command；继续绑定 live ready/owner/surface generation、跨页固定 cut 及 renderer reducer，再做 App 原生验收。没有新 App、提交或推送；累计 19 个文件未达 review 后提交阈值。

## 2026-10-09 bridge host-owned projection 续页（未提交增量）

已补 bridge 首屏/续页：Serve token 留在 host，界面仅取绑定 controller/session 的临时句柄；缓存最多 64 项、不持历史正文/事件数组，两分钟原期限不延期，中间/最终页可显式重读。专项/完整两包 race 与 vet 通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-bridge-host-owned-projection-续页未提交增量)。native IPC、快照/实时 UI 合并及实际 App 验收仍待继续，没有新 App、提交或推送。当前 12 个变更文件未达 review 后提交阈值。

## 2026-10-09 bridge 初始远程 projection（未提交增量）

已接入认证 bridge 首屏只读快照入口，保留稳定身份、过滤未知私密字段并在 owner 关闭后取消/拒绝迟到读取。bridge 与 shared remote client 完整 race、vet 通过，范围与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-bridge-初始远程-projection未提交增量)。尚未完成 bridge 续页、native IPC、snapshot/live UI 合并或重建 App；实际原生与真实服务验收不以源码测试替代。当前累计 11 个文件，按超过 30 个文件先 review 再提交规则继续推进。

## 2026-10-09 shared remote projection client（未提交增量）

共享远程客户端已能读取 Serve 的初始 projection 与固定上界续页，校验 catalogue/scope/身份/序号/状态及字节/页数预算；过期或变化明确要求同步，不自动回退或接管。旧历史客户端也拒绝 system/developer 提示词行。完整相关三包 race、追加的有界解码/实际 Serve-client 专项及 vet 通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-shared-remote-projection-client未提交增量)。还需接 bridge/native snapshot IPC 与 renderer 统一归约，当前没有新 App 或实际原生验收；7 个变更文件未达到 review 后提交阈值。

## 2026-10-09 只读远程 projection 快照 review 提交

本批超过 30 个变更文件，已 review 并按提交门禁收敛。新增 Serve 只读 projection 首屏/续页接口，接入稳定 prefix、问题 suffix、固定事件 cut 和 steer 身份；句柄绑定原 controller/session，限制 64 槽与两分钟期限，不授予接管/确认/发送权限。Review 修复共用远程历史投影的 system/developer 提示词行泄露、重复身份和缓存强引用保留风险。完整相关 Go race、专项 HTTP/分页/隐私、vet、renderer 回归和类型/lint 检查通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-只读远程-projection-快照-review-提交)。以下增量标题保留当时状态；本批没有新 App、bridge/native snapshot IPC 或实时 UI 验收，仍须继续接入与处理 admission/compaction/legacy steer 同步边界。

## 2026-10-09 applied steer 保存消息关联（未提交增量）

运行中指导事件增加 display-only `messageId`，对应已保存的同一条消息；收件箱 `itemId` 保持独立，旧事件省略关联仍兼容。关联贯通 Agent、账本、wire、Go SSE 和 native typed projection。七包完整 Go race、专项 Serve/账本回归、vet/生成器 check、Rust 281 项与 clippy 通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-applied-steer-保存消息关联未提交增量)。尚未接到实际实时界面或重建 App，继续接历史前缀/分页/指导事件的远程快照与统一归约。当前 29 个变更文件，未达到超过 30 个文件的 review 后提交阈值。

## 2026-10-09 固定上界的 projection 重放分页（未提交增量）

补齐初始快照续页边界：固定首屏序号上界，校验 ledger/session/turn/epoch，不混入后到事件，并保留首屏 transcript 元数据。分页专项 race、vet 及最终完整 controller/ledger race 通过，证据与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-固定上界的-projection-重放分页未提交增量)。Serve/bridge/native 和 UI 尚未接入；下一步补 steer 保存消息身份与事件排序，compaction 恢复仍未完成。没有新 App 或实际原生验收，当前 16 个变更文件未达到提交阈值。

## 2026-10-09 Preview 试运行统一 runner（未提交增量）

Preview 试运行已改走统一 TaskTool runner，保持只读、临时不落盘、profile 提示词和取消边界，未放宽 child 构造白名单。完整 agent/controller/ledger 及 bridge race、ephemeral 专项与 vet 通过，解决下阶段记录的 agent 门禁失败；详见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-preview-试运行统一-runner未提交增量)。没有真实模型或新 App 验收；projection 路由与其他目标继续推进。变更超过 30 个文件先 review、验证再提交，当前 16 个文件未达到阈值。

## 2026-10-09 稳定历史 prefix 与 active suffix（未提交增量）

已补 admission 前的有界消息身份 fence 与内部只读 projection view，区分旧 prefix、后续用户消息与当前事件重放；不复制第二份 conversation，显示失败不影响引擎接收问题。专项、controller/ledger 完整 race 与三包 vet 通过；agent 完整回归仍被 Preview subagent 试运行绕过统一 runner 的既有枚举门禁拒绝，未加白名单绕过。详见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-admission-前缀身份与只读-projection-view未提交增量)。这是内部读取基础，尚未接 Serve/bridge/native DTO 或 UI；分页、compaction/steer 映射及原生验收仍未完成。

## 2026-10-09 active-turn 初始重放原子读取（未提交增量）

Go ledger/controller 已补同锁的 active-turn/status/replayAfter/首个有界事件分页读取，消除终止与新轮 admission 之间的混合 cursor；失败不变成空的 ready projection。完整 ledger/controller race 与 vet 通过，见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-原子-active-turn-重放边界未提交增量)。这不是 provider history 的原子快照；Serve/bridge/native 路由、后续分页、历史 prefix 与实时 suffix 合并及实际 UI/App 验收保持未完成。

## 2026-10-09 订阅客户端窄 scope（未提交增量）

在 `3c897f5da` 基础上补主窗口目标监听、频道白名单、显式请求字段与异步前 scope 捕获，并回归非法 generation 零 dispatch 和取消异常。未修改实际 UI 或重建 App；快照与事件的原子同步边界仍需实现，不能把 SDK mock 当作实时历史验收。范围与直接 Node 检查限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-09-renderer-订阅目标与窄请求未提交增量)。

## 2026-10-08 原生远程订阅 review 提交

本批 review 提交原生事件投影、订阅/取消 IPC、包内 sidecar 退出撤销及 renderer 消费 helper。Review 修复异步 unlisten 的未处理拒绝、测试类型兼容及既存历史文案的构建预算漏计，并将 renderer/native adapter 回归接入远程检查。Rust 280 项、clippy、Go race、协议生成检查、前端远程回归/测试类型/生产构建通过，详情见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生远程订阅-review-提交)。尚未挂接实时 UI，也未重建 App；实际 WebView、窗口重建、统一归约和可写入口的门禁保持未完成。

## 2026-10-08 包内 sidecar 终止 observer（未提交增量）

明确 child 终止 receipt 已主动撤销对应 remote subscription owner；晚绑定立即撤销、旧 owner 通知不影响新 owner，取消关闭实际 TCP 读取。管道故障不作为进程退出证据；显式 developer binary 仍未接同等 observer。源码回归与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-bundled-sidecar-终止订阅撤销未提交增量)，尚未重建 App 或关闭窗口重建/live UI 门禁。

## 2026-10-08 原生事件载荷与订阅 IPC（未提交增量）

已接入 Go-derived 32-kind 载荷 contract、native typed projection 和主窗口订阅/取消命令；返回身份与 opening/ready/ended 分开，替换/取消后的旧 worker 不发布迟到结果。完整 Rust 277 passed / 0 failed / 6 ignored、clippy 与构建契约通过，范围与限制见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生事件载荷投影与订阅命令未提交增量)。尚无此次实际 WebView IPC、新 App 或 live UI 验收；sidecar 意外退出即时撤销、窗口重建、快照/事件统一归约、恢复/发送/审批/模型归属和 bot 等门禁保持未完成。变更超过 30 个文件时先 review、验证，再提交；当前阶段不提前宣称完成或覆盖旧包。

## 2026-10-08 超过 30 文件的 review 提交

本批 review 提交共享原生历史图片验收修复、指定会话 SSE bridge/native transport 与 pending-open 取消、订阅 registry 核心。registry 防止旧 generation/worker 清理新订阅，限制 current/live/tombstone 数量；review 补 main Destroyed 之后的 admission 撤销、Supervisor stop 与 start 的 owner 锁归属、tombstone 上限准确恢复提示。四项 registry 回归、Go 五包 race/vet、完整前端 Tauri/测试类型、Python receipt 与协议镜像通过，详细范围见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-订阅注册表与超过-30-文件-review-提交)。订阅 IPC/载荷检查/实际 WebView 实时面仍未接入，本批源码提交不代表新 App 打包或整个 E 完成；已交付 App 不覆盖、不推送。下方未提交记录是各增量阶段当时状态。

## 2026-10-08 共享原生历史图片与会话事件传输（后续未提交源码）

- 实际 release App 的 managed/explicit 两种隔离档案已通过共享历史图片、可见 ImageViewer、Escape 保留底层历史、关闭清理和无未捕获异常门禁。修复 SSH acronym wire、Serve 首帧 admission、Rust 缺省字段省略、预览 Escape 冒泡。图片阶段证据见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-共享历史的真实原生图片验收0ebda4fd9-后续未提交源码)；已交付 `0ebda4fd9` App 未覆盖。
- 新增 shared controller `SessionEvents`，按明确 catalogue member 路径接收后台/前台会话事件，使用原 SSH owner/operation 的取消与 cookie transport；不把未标记或其他会话帧归给当前 surface。帧/行/UTF-8/序号/关联 ID 有界，未知字段通过 typed event 投影剔除；取消和关闭拒绝已缓冲结果，EOF 要求显式重开并补快照，不自动重试、接管或授予写入。
- shared client 五项顶层回归及生产 Serve token gate/真实 Controller/会话文件/broadcaster 路由用例通过；完整 controller/Serve race 和 vet 通过，日志 `/private/tmp/reasonix-native-history-ui.POmxwA/events-complete-race.log` / `events-vet.log`。生产 broadcaster 事件为自有 fixture，不声称已执行真实模型 turn、SSH/SSE 原生界面或审批恢复。
- 后续增加受监督 handle 的窄 `session-events` bridge route，实际自有 SSH tunnel + cookie HTTP + bridge HTTP 回归通过；验证路径过滤/typed projection、撤销清理、字段/鉴权/重放拒绝、每 handle 两订阅和取消释放，另用故障 writer 验证 request 尚未取消时的首写/事件输出失败清理。完整 bridge/controller/protocolgen race、vet、DTO/schema、镜像、前端类型和 Rust 编译检查通过。输出有写入预算，不进入本地 EventLedger；schema event 仍是 opaque，原生 payload 检查和 subscription generation fence 尚待接入，不能算实际包实时对话验收。证据 `events-bridge-final-race.log` / `events-bridge-complete-race.log`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程会话-sse-bridge-路由未提交增量)。
- 原生内部 transport 核心已接固定 HTTP/1.0 SSE、catalogue membership、controller/path fence、有界 header/frame、socket shutdown 取消和 Drop 清理，五项 transport/完整 remote-controller 15 项通过；实际 bridge HTTP/1.0 framing + socket 关闭清理 race 通过。尚未注册 command 或向 renderer emit opaque event；pending-open 取消、subscription registry、payload/capability 检查、generation/window/sidecar fence 仍待实现。不能把 TCP fixture 当作 WKWebView/App 联调。证据 `events-native-controller-final.log` / `events-native-http10.log`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生指定会话事件传输核心未提交增量)。
- 后续补 pending-open cancellation：同一 operation 的 socket lease 覆盖 catalogue 响应头/JSON body 和 SSE 响应头，取消主动 shutdown 而非等 I/O timeout；预先取消零 TCP dispatch，失败打开释放 lease，迟到打开不返回。两项顶层回归与完整 Rust 266 passed / 0 failed / 6 ignored 通过，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生会话事件-pending-open-取消未提交增量)。TCP connect 仍有 1 秒预算；subscription registry、载荷检查和窗口/sidecar/generation fence 尚待接入，不算 actual App 订阅验收。
- 下一步接原生订阅与 generation/surface fence，再接快照/实时事件统一归约、显式恢复/发送/审批/模型归属和重连。现有只读历史 handle 不能隐式升级为可写会话。系统剪贴板、bot Desktop、三平台专项、复杂管理页与 SQLite 等其余目标保持未完成；本批没有新的 App 重建或提交/推送。

## 2026-10-08 远程图片正向原生验收与 review 提交

真实 release App 的 managed/explicit 两种私有档案均通过 WKWebView → 注册 IPC → 包内 bridge → 临时密钥认证 SSH/SFTP → 生产 shell bootstrap → 实际 CLI Serve → saved workspace PNG 解码。验收没有 mock 图片响应或 controller factory，没有真实账号、模型请求、系统剪贴板操作、下载或远端安装；会话及 workspace 元数据保持逐字节不变。review 补正向收据严格布尔校验、fixture 输入/XOR 回归和 Serve 进程身份/清理确认；只有清理成功才生成最终收据。证据 `/private/tmp/reasonix-remote-image-positive.78LOTy/`、`/private/tmp/reasonix-native-ssh-2976458357/integration-receipt.json`。本批 review 后提交，再从干净 HEAD 重建 arm64 ad-hoc `.app`，以最终包复验记录为准。共享 Transcript 的原生完整界面、剪贴板粘贴发送、真实远端平台/代理/安装及完整 controller/bot 等门禁仍保留；原生 standalone 图片节点验收不能代替共享对话界面验收。下方“未提交/未执行”是各阶段当时状态。

## 2026-10-08 原生远程图片边界实际 App 验收（后续源码）

独立 release App 在 managed/explicit 两种临时档案中通过真实 WKWebView 图片 IPC 注册与 unknown field/空 source/未知 handle 拒绝、正常退出及无 sidecar/readiness 残留；没有操作系统剪贴板或真实账号。新增正向 PNG 探针仍需真实自有 SSH/Serve fixture，尚未执行，不把拒绝检查算作远程图片成功验收。首次 debug origin 拒绝保留失败证据；构建脚本已遵循 Cargo 独立 target 输出并验证实际候选签名，73 项构建契约及 Rust 257 passed/6 ignored、clippy、receipt verifier 通过。证据 `/private/tmp/reasonix-native-image-release.eUtbs7/`、`/private/tmp/reasonix-native-remote-image-boundary-bkdb2gjw/`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-原生远程图片边界探针与独立-release-候选后续源码)。本增量未提交，候选在独立临时目录；已交付 `eefdb4e55` App 未覆盖。完整目标继续保持未完成。

## 2026-10-08 远程图片预览与渲染证据（后续源码）

补远程历史图片的 shared ImageViewer 与 scoped resolver 生命周期，保持只读权限、不打开本机文件。新增 effect/旧请求/ABA/预览/清理回归，以及真实共享 Transcript 浏览器 desktop/narrow 图片解码、预览、reader refresh 与迟到图片验证，完整 Tauri/Transcript、类型及构建门禁通过。日志和截图在 `/private/tmp/reasonix-remote-image-ui.TknC1t/`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程图片共享预览与浏览器验收后续源码)。native invoke 使用模拟 adapter，不能当成原生 App/SSH/Serve 联调。当前交付 App 仍为 `eefdb4e55`；本增量未提交/未打包。下一步继续实际隔离原生链路，系统剪贴板 runner 未获授权，未执行；其余完整目标保持未完成。

## 2026-10-08 远程图片 IPC/UI review 与 macOS App

本轮接入 main-only 图片 IPC、remote lease 与共享历史图片 resolver，保持 backend workspace、只读会话归属和迟到结果 fence，不提供本机 opener/文件回退。已构建 arm64 ad-hoc 可运行 App，签名及实际 managed/explicit 临时档案启动和退出通过；review 补图片 scope、释放/迟到结果及私有字段回归。日志与上版 App 备份在 `/private/tmp/reasonix-package-review.QZYtvf/`，详见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程图片-nativeui-review-与可运行-app)。尚无新增图片 command 的实际 WKWebView/SSH/Serve 联调，不能把启动 smoke 当作该功能验收。真实剪贴板、完整 controller/bot Desktop、跨平台、管理页和 SQLite 门禁保持未完成；不发布、不替换正式应用。

2026-10-06 用户要求直接推进 E，明确不需要 updater。本轮不再以 D 全部验收为 E 的启动条件；D、A/B/C 未关闭项仍保留为正式发布门禁。正式发布、替换正式应用、切换默认下载均未授权。updater 从本轮范围排除。

2026-10-08 最新可运行候选与 review 记录见本文末尾及 [API audit](API_SURFACE_AUDIT.md#2026-10-08-远程历史增量-review-与-macos-可运行候选)。下列“未提交/未打包”保留各阶段当时状态，不覆盖后续交付记录。

| 功能面 | Wails 基线 | Tauri 起点 | 当前工作及验收 |
| --- | --- | --- | --- |
| remote 配置与认证 | desktop/remote_hosts.go、remote_prefs.go；SSH config、显式主机指纹、临时/持久凭据 | 读取/修改/删除、SSH alias 扫描、指纹确认和密码恢复已有 | 本批补连接请求归属、断开/改配置/退出取消、重开设置时的权威连接状态；源码 Go race / UI 回归、新包本机 SSH 信任/持久化/退出验收已通过 |
| remote 文件 | remote_listing.go；浏览、读取/写入及路径操作 | SFTP 浏览、文本预览、revision 保存已有 | 本批补目录/文件异步请求归属、取消/切换后旧结果丢弃；新包真实 SFTP 浏览/预览/revision 保存及旧版本冲突通过。本轮接入 mkdir、同目录 rename、删除（目录需显式递归确认），侧车拒绝根目录/登录主目录删除；Go race、Tauri 组件回归通过。此轮未重建 `.app`，不能把源码回归视为包内证据。 |
| remote tunnel / Serve | remote_serve.go、remote_server_stop.go；forward、bootstrap、状态/日志 | Serve 生命周期与 controller 浏览器入口已接；Tauri 内嵌 remote tab 未接 | 设置页可按已认证 SSH 会话读取状态、启动/复用、停止及读取最多 500 行、256 KiB 日志；可由原生宿主将fragment 登录 controller URL 直接交给系统浏览器，renderer 不接收 token/URL。tunnel 固定 `127.0.0.1:0`，断开 SSH 会随 client forward 一并关闭。源码 Go/UI/Rust 回归通过，但未用真实远端 Serve 或重建 `.app` 验收。自动启动可复用现有兼容 CLI，或按策略在远端 npm 安装；Preview 未携带可上传 CLI/发布下载回退，`local-proxy` 凭据显式拒绝。配置转发自动应用与 Tauri 原生 remote tab 后续批次，不把 SSH/SFTP 验证算作 Serve/controller 验收。 |
| remote 对话与工作区 | remote_tab*.go、remote_projects.go；恢复、事件/审批、模型归属、断线重连 | 可启动 remote controller 浏览器页；本地单会话 bridge 尚未完整对接远端 controller | 系统浏览器 controller 使用 Serve 的现有页面/会话能力；Tauri 内的会话恢复、事件/审批、模型归属与断线重连仍未迁移，不能借用本地会话或放宽模型/写入权限代替。 |
| bot 配置和账号 | bot_connection_app.go、bot_runtime_app.go；多账号、凭据、启停、状态、诊断 | 旧渠道配置/凭据、路由/队列/权限、适配器运行已有 | 本批补手动多账号创建/删除、受保护凭据、路由/订阅清理、运行时重启和配置应用状态；源码 Go race / UI、新包默认关闭/私有凭据/重启持久化/旧包读取回退通过；未连接真实 IM |
| bot 配对和管理 | internal/bot/pairing.go、desktop/bot_bridge*.go | 可开启配对，但没有待申请审批入口；Desktop=nil | 本批补本档案申请列表/批准/拒绝、失效连接拒绝、配置写失败保护；源码失败恢复及新包鉴权/连接内授权通过；桌面对话订阅/远程审批/接管仍待迁移 |
| bot 扫码安装与诊断 | Start/PollBotConnectionInstall、Diagnose/TestBotConnection | 尚无完整 Preview 安装/诊断流程 | 后续批次；真实 IM 平台连接须在可用账号环境验收 |
| 复杂管理页 | permissions/secrets/sandbox/network、skills/plugins/MCP、hooks/memory/subagents、data/diagnostics | 多数已有独立 Tauri 页面和 bridge | 本批修复 bot 管理轮询覆盖未保存路由草稿；其余页面逐项审计配置/异步归属/失败恢复/权限边界，不据页面存在判定完成 |
| updater | Wails 自动更新 | Preview 说明/手动下载 | 用户明确排除，本轮不迁移 |

验收分开记录：源码回归、真实组件浏览器、实际 `.app` 包内进程与本机协议服务、外部平台/原生窗口用户体验。旧包通过不自动升级为新包证据。测试不得使用真实凭据、发送测试消息到用户联系人或改动正式版数据。

复杂管理页首轮对照（Wails worktree `/Users/jerry/temp/app/reasonix-wails-1.38.3`，当前 HEAD `2185b8e8ff8abb2166bf8e55df0694d05a0f9d76`；本地基线包含既有补丁，不冒充官方归档身份）：

| 页面 | Wails 对照 | Tauri 当前实现与本次回归 | 仍需验收 |
| --- | --- | --- | --- |
| 权限/敏感信息 | app.go 的配置读写与权限规则 | 独立 permission/secrets bridge，项目覆盖与全局规则保持；补全局→项目读取乱序保护；Go 持久化、规则隔离与 UI 迟到响应回归 | 原生窗口实际工具执行与拒绝/恢复 |
| sandbox/network | app.go 配置、网络代理与 sandbox 策略 | 独立桥接；Go 配置保存、凭据脱敏和其他字段保护已回归 | 真实平台策略/代理切换后的工具与子进程行为 |
| skills/plugins | app.go SkillsSettings、plugin_packages_app.go | 来源、安装计划、revision、启停、归档/恢复、诊断；Go 计划/安装/拒绝风险缺失/项目隔离回归，已有 UI 回归 | 实际包中第三方安装、取消/重启/失败回退；无真实第三方执行证据 |
| MCP | app.go MCPServers/配置/认证与 mcp_* | 配置、运行时、认证、目录与设置 UI 回归已有；本次工作区/会话切换重新挂载管理页 | 实际打包子进程退出、OAuth/超时/异常重启与权限完整矩阵 |
| hooks | hooks_settings_app.go | 项目信任与配置范围、插件 hooks；已有请求归属保护；本次保持工作区/会话隔离 | 实际工具触发、取消和 hook 子进程权限 |
| memory | app.go Memory、memory_suggestions.go | 文档/fact/revision/建议、工作区边界；当前会话应用须空闲；本次避免工作区切换沿用旧页面状态 | 原生编辑、建议接受与真实模型召回 |
| subagents | subagents_app.go | profile CRUD/revision、试运行/取消及模型默认；本次页面归属跟随工作区/会话 | 真实模型试运行、取消/子进程退出/失败恢复 |
| data/diagnostics | app.go 档案与诊断 | 档案隔离、导入审阅与只读诊断；storage bridge 有有效目录回归 | 不继承旧包备份/导入证据，当前候选完整矩阵仍待验 |

本批源码提交 `7e6645bdc8f9011a4cf289d6885e998503e65651` 已构建直接可运行 `.app`，包内 sidecar 在临时档案与本机 SSH/SFTP 服务通过集成验收，旧包读取新配置并回到新包通过。原生宿主在屏幕 2 内侧通过两种档案的退出清理和只读通知权限查询；这不替代 D 的完整原生窗口矩阵。证据见 [本批验收记录](evidence/2026-10-06-e-lifecycle/README.md) 和 [包收据](evidence/2026-10-06-e-lifecycle/package-receipt.json)。

不将表中“已有”能力计作整个 E 已完成。下一批优先 Tauri 原生 remote tab/controller 与 bot Desktop 桥接；配置转发自动应用、扫码安装、完整管理页实际包矩阵仍保持未完成。仅 updater 属于用户明确排除。

## 2026-10-08 远程 controller 共用 HTTP 层（未提交源码增量）

- 将 Wails 的回环 Serve client / token handshake 提取到 `internal/remote/controller`；`desktop/remote_listing.go` 通过薄 adapter 使用它。共用层不依赖 Wails/CGO，不把 renderer URL、代理或本地模型配置用作远程 controller 归属。
- 仅接受显式端口的数字 loopback HTTP origin；拒绝 DNS 主机、userinfo、base query/fragment/path。独立 transport 禁用环境代理和自动重定向，并在每次请求前检查原始 IP/端口与 Host。Cookie jar 不隔离端口，因此跨端口请求必须在发送前拒绝，不能只依赖 cookie domain 或重定向保护。
- token 只在 `/auth/token` 的受限 JSON body 中交换，后续 API/SSE 走后端 cookie jar。握手有 10 秒取消预算，响应只丢弃最多 4 KiB，错误不返回响应正文、token 或 URL；JSON 转义后也符合 Serve 的 8 KiB 入站预算。SSE 没有 whole-client Timeout，stream body 由 owner context 取消。
- 共用层 7 项顶层回归覆盖地址拒绝、token/body→cookie API/SSE、环境代理关闭、307 和直接跨端口泄漏拒绝、Host 伪装、错误字节预算/脱敏、握手及 SSE 取消、无效 token/client 的零请求。另用 `internal/serve` 的生产 token middleware 验证未认证 401、错误 token 拒绝及认证后的 sessions/status/history/events；这些 API 响应是 fixture，不冒充实际 controller 生命周期。
- root 定向 race 与 `go vet` 通过，Wails `TestServe|TestRemote|Test.*CredentialProxy` race 回归通过（真实主机 smoke 开关显式禁用）。测试只访问自有回环服务，没有连接真实 SSH、模型或 IM 平台。证据保存在 `/private/tmp/reasonix-controller-transport.CywPLr/`。
- 此批未增加 Tauri remote tab/HTTP endpoint 或 bot Desktop 桥接，也未重建 App。下一步仍需 backend-owned host/workspace/connection generation、会话/模型归属、事件与审批恢复、断线重连和真正包级验证；共用 transport 通过不关闭 E 或图片原生粘贴门禁。

## 2026-10-08 远程 controller 连接归属与只读清单（后续未提交增量）

- 在上述共用 transport 基础上增加 bridge 内的 attach/list/close 接口，health 仅新增 `remote_controller_sessions_v1`，不声明远程对话完整可用。入站字段只有 saved host 与 workspace；地址与 token 来自后端 EnsureServe 和已认证 SSH tunnel，renderer 只得到随机 128-bit handle、主机/工作区及 `readOnly: true`。
- controller 绑定受监督 SSH client **及其当前底层 SSH 连接**，不能仅凭同一个 client 指针跨自动重连复用。附着 HTTP request 的结束不结束连接；断开、主机配置变更、替换连接、关闭 controller、sidecar 关闭会撤销 owner 并取消认证/读取。断线回调只取消 owner，迟到附着/清单再次检查当前归属，不能复活旧 handle。
- 同 host/workspace 的健康连接复用 handle，不同工作区隔离；已发布与正在附着的总数限制 16。附着预算 45 秒、等候 Serve 锁可取消；清单整个读取预算 15 秒、body 至多 8 MiB、条目至多 10,000。固定 DTO 丢弃未知字段，拒绝非法结构、重复路径、多 current、控制字符与非 JS-safe 计数；保留合法多行备用标题。401/403 cookie 失效废弃旧连接，后续需重新认证。
- 成功停止 Serve 后按请求或解析后的 workspace 撤销已发布 handle；同主机尚未完成解析的附着保守取消，防止路径别名导致停服后迟到发布，其他已发布 workspace 不受影响。停服完成还须通过当前 SSH 归属检查，不能撤销替换连接的新 controller。
- 新增 shared client 4 项、bridge controller 7 项顶层回归，使用自有 loopback SSH（真实 direct-tcpip）和 HTTP cookie 服务，覆盖复用、工作区隔离、鉴权/字段拒绝、上限、关闭/退出、断线取消、同 client 自动重连、迟到附着与可取消锁。另读取生产 Serve `/sessions`（真实 `control.Controller` + 临时 transcript + token gate）验证 current、计数、多行备用标题且不切换控制器；不再只有伪造 catalogue body 的兼容证据。未进行远端安装或模型调用。
- 最终定向 Go race、Go vet、Go DTO/schema、协议镜像一致性、前端 typecheck 和 Rust `cargo check --locked` 通过。证据在 `/private/tmp/reasonix-controller-ownership.aFopli/`，最终 race 为 `ownership-fenced-final.log`，静态检查为 `ownership-final-vet.log`。早期 fixture 失败和一次自动权限审核超时保留，不列作通过；修复夹具未读取 POST body、ID 格式后回归通过。
- 此批仍没有原生 IPC 命令、Tauri remote tab、远程历史/流/发送/审批/模型配置归属或 bot Desktop 联动；未重建 App、未提交/推送，也未操作系统剪贴板。清单中的 remote session path 是展示数据，不是本地 RuntimeManager、工作区或文件授权。下一步接原生窄 IPC 与只读 UI，再逐项迁移 controller 读写/事件与审批；E、真实 Serve 和包级门禁保持未关闭。

## 2026-10-08 只读 controller 原生 IPC 与设置页清单（后续未提交增量）

- 上述后端快照之后，接入 main-window-only 的 attach/sessions/close 三个 typed command。只有 saved host/workspace 或 canonical opaque handle 可入站；固定 HTTP 路由、后台 worker、严格字段/版本/响应预算及错误脱敏，不向 renderer 提供 Serve 地址/token，不持有 supervisor lock 等待网络。
- 设置页新增显式“查看远程会话”，懒加载、每页 50 条、显示当前/运行/接管状态与多行标题；目录仅展示，不链接到本地文件或本地 RuntimeManager，没有发送、恢复或模型/审批操作。React effect 依赖原始 host/workspace，切换后旧响应/错误/finally 均丢弃。
- 共享 lease 处理多个挂载与 StrictMode：最后消费者释放才关闭，同 scope 新附着等待旧附着与关闭完成；旧 finally 不删除新 owner。关闭投递不确定时保留 barrier，直到显式 SSH 操作已撤销后端 owner；未改变、仍保持 connected 的配置保存不清空池。
- Rust 新增 3 项 typed HTTP fixture 单测和 clippy 通过；pool/绑定/组件、完整 Tauri 前端回归、生产/测试类型检查、build 的现有门限通过。Browser plugin 不可用，使用已有 Playwright + 自有 headless Chrome，桌面/390px 窄屏的非空/overlay/console/分页/关闭重开/错误隐私/长路径布局通过；原生调用是 mock，不能算 `.app` 或真实远端验收。详细日志与截图见 [API audit](API_SURFACE_AUDIT.md#后续源码窄原生-ipc-与只读列表)。
- 未重建当前阶段的 App，未提交或推送，未操作真实 profile/SSH/模型/IM/系统剪贴板。下一步继续 controller 读写/事件与审批/模型归属及隔离包内验证；只读列表不关闭 remote tab、bot Desktop、图片原生粘贴或整个 E 的门禁。

## 2026-10-08 指定远程会话历史与 runtime 读取（后续未提交增量）

此前图片/终端/只读列表改造现已进入当前提交 `a9a9728fa`；本段是该提交后的独立源码增量，不把旧 App 或旧快照测试升级为新代码的包级通过。

- 新增 Serve 的显式 `desktop/session-view`，修正新远程读取链路会误用旧 status 自动 reclaim、旧 history 默认前台回退的问题；旧 API 保持原行为不变。新路由只读取选中的 canonical 会话：foreground/detached 使用各自 runtime/model，saved/external 使用各自文件，缺失/越界/退休不回退。
- shared client、认证 bridge 的 `session-view` 和 main-only native typed command 已接；远程清单是输入白名单，URL/token 不入站，30 MiB/100,000 条预算，未知配置及 replay 字段不出站，owner 关闭取消在途读取并拒绝迟到发布。只读视图不开放本地 RuntimeManager 或远程发送权限。
- 新增 Go client 3 项、生产 Serve 3 项、bridge 2 项顶层回归，以及 Rust 2 项 native client fixture；包含自有真实 SSH direct-tcpip/cookie、生产 Controller/磁盘历史兼容、工具/reasoning/search、后台模型隔离、不自动接管、缺失/symlink/未知字段拒绝、取消/脱敏/版本/预算。未调用真实模型或外部账号。
- 最终定向 4 包 Go race、shared client 全量 race、Go vet、DTO/schema 与生成镜像检查、前端 typecheck、原生 remote client 的 5 项 Rust 测试（其余 254 filtered）及 clippy 通过，证据 `/private/tmp/reasonix-remote-view.v5p1wr/`。最初 compile 因 sandbox 禁止写 Go cache 失败，获审核后最终编译/回归通过；不将最初失败记为成功。
- 未接前端远程历史面板，未增加稳定消息/turn 展示身份、发送、SSE/重连/审批或远程模型修改；未重建或启动 App，未提交/推送本批。接下来把视图接入独立 remote surface，保留生成代次与 transcript 单写入者，再补完整 controller 的事件和写入权限。bot Desktop、原生图片粘贴、Windows/Linux 和其余包级门禁保持未关闭。

## 2026-10-08 本批 review 与 macOS App 交付

- 用户要求先生成可运行 App，再 review 并提交；此前大批图片/终端/只读列表已在 `a9a9728fa`，本轮审阅剩余 20 项增量并加入生成器修复与回归。发现并修复 Rust 将 `sources_status` 误作驼峰字段而丢失搜索来源状态的问题，复现与修复后验证分开记录。
- 完整四包 Go race、Rust 253 passed/6 ignored、完整前端 Tauri 回归、typecheck、vet、clippy、协议镜像门禁通过。首次构建的 arm64 `.app` 与两种临时档案启动/退出 smoke 通过，交付包从本次干净提交重建，最终日志位于 `/private/tmp/reasonix-macos-review.J5DA05/`。
- 新增远程历史 UI、稳定消息身份、事件/发送/审批/模型改动仍未实现；完整 E 目标继续。未连接真实外部账号、改动正式数据或触碰系统剪贴板，本地 ad-hoc 签名不是正式发布签名/公证。

## 2026-10-08 远程历史稳定身份与共享读取（后续源码增量）

- 当前已提交/交付的 macOS App 基线是 `d0106adb8`；本段为其后的未提交源码，未覆盖该 App，也未升级旧包级证据。
- 指定会话 DTO 增加必需的 backend entry `id`。普通、steer/recovery 显示投影保留各自源身份；可见条目缺失/重复/越界 ID 时失败，不由数组位置生成。旧 `/history` 不变，旧格式存储通过现有 loader 确定性兼容且不回写；早期缺少 entry IDs 的远端快照明确要求升级 Serve。
- native binding 和 RemoteControllerPool lease 接入 `sessionView(path)`：派发前检查未释放/退役，完成后核对 controller、host、resolved workspace、path、只读标志和 ID 唯一性。共享消费者、release 到派发间隙、迟到读取、SSH 重置/新 owner、不同身份/路径、拒绝无本地/Wails fallback 均有回归。
- 源码验证：4 包定向 Go race、生产 Serve 的重复读取/追加/legacy 不回写/损坏身份/后台隔离回归、Rust 6 项筛选回归、完整 `test:tauri`、生产/测试 typecheck、vet、clippy、协议生成检查通过。证据 `/private/tmp/reasonix-remote-identity.8uASVb/`；补充 fixture 首次误用 Save 被 DAG 合法去重，修正为损坏旧格式后同一拒绝断言通过，失败日志保留。
- 尚未把远程历史接到共享 Transcript；下一步是独立 remote surface、远程文件/图片解析权限和 generation 归属，不复制位置键或另建滚动写入者。之后继续完整 controller 事件/恢复/发送/审批/模型与 bot Desktop、异平台/复杂管理页/SQLite 门禁；本次不是总体完成，也不是新增 native command 的 App/WKWebView 验收。

## 2026-10-08 只读远程历史界面与共享渲染安全（后续源码增量）

- 继上一阶段，远程列表现可显式打开独立只读历史 surface，懒加载并复用共享 Transcript/TimelineProjection/Kernel。所有展示键按 controller/workspace/path/backend entry 命名；刷新保留同一 surface，选中替换、读取失败和卸载拒绝旧结果。没有 composer、编辑/恢复、审批或模型修改入口，不将 runtime 快照冒充实时进展。
- 修补共享入口：UserMessage 附件遵从 scoped resolver，拒绝/异常不回落本地；远程 file URL/路径/代码行引用统一拦截点击、中键与本地路径菜单。远程文件/图片尚未接入，界面明确提示；现阶段不是媒体或完整 controller 完成。
- 浏览器复现了共享问题导航把“快照新增问题”误当“用户发送”抢回底部的真实问题。仅客户端明确提交才产生临时 `tailFollowRequested`，不会由历史读取赋予；共享 hook 与确定性 Kernel 回归覆盖普通刷新 reader 零写入、稳定锚点、主动提交恢复和补丁/前插不重放，未新增滚动写入者或重试时钟。
- 隔离浏览器最终验收通过：250 轮打开末尾、思考/工具展开、远程媒体/文件零本地调用与零直接图片请求、刷新可见锚点和 scrollTop 不变、旧请求切换拒绝、390px 无横溢出、错误隐私/空态/卸载释放。原生 invoke 为 mock，不替代 App/SSH/Serve 包级联调。证据 `/private/tmp/reasonix-remote-history.Aw6GtM/`，详细场景、失败日志和截图见 [API audit](API_SURFACE_AUDIT.md#2026-10-08-只读远程历史-ui-与共享权限阅读意图修复)。
- 完整 `test:transcript`、完整 `test:tauri`、生产/测试 typecheck、hooks lint、single-writer 与 build 体积门禁最终通过；新投影/媒体/异步归属/提交意图回归均纳入测试脚本。初稿撤掉所有被动跟随导致普通 tail 批量增长的挂载预算回归，修正为仅保留已有、非 native lease 的 tail 归属后原 53 项 viewport 断言通过，不调整 ≤40 或 4px 门限。失败与最终通过日志分开保留。
- 已交付 App 仍是 `d0106adb8`，本批未提交/推送、未重建 App。下一步继续远程媒体授权与实际 IPC 只读链路，再接 controller 事件/恢复/发送/审批/模型与 bot Desktop；原生图片粘贴、Windows/Linux、复杂管理页和 SQLite 门禁未关闭。

## 2026-10-08 远程历史批次 review 与可运行 App

- 已收敛上一阶段 42 项源码/测试/文档；尚未接通的远程图片读取入口不进入此次提交/包。共享阅读位置、稳定 ID 和本地/远程权限分流的回归保留，未新增滚动写入者、降低预算或开放远程写入。
- 当前 arm64 App 候选构建及 deep/strict ad-hoc 签名通过；实际 App 用独立临时 HOME 分别验证 managed/explicit 档案、私有身份、Global workspace、认证与 sidecar readiness、正常退出无残留通过。旧 App 可恢复副本与全部日志位于 `/private/tmp/reasonix-history-review.Ubhzkf/`。
- 本轮完整 Go 四包 race/vet、Rust 253 passed/6 ignored 与 clippy、完整前端 Transcript/Tauri、测试类型、协议生成与生产构建门禁通过。交付要求 review 提交后从干净 HEAD 重建，再运行相同包 smoke，最终收据是 `build-final.log` 与 `package-smoke-final.log`；不把 dirty 候选当作已提交版本。
- 未推送/正式安装或发布，不使用真实账号、用户数据/剪贴板。包启动不替代新增远程命令实际 WKWebView/SSH/Serve 联调，原生图片粘贴及跨平台等其余目标保持未完成。

## 2026-10-08 远程图片 Serve/client/bridge（后续源码增量）

- 上批已提交/交付基线 `de2c6050c` 保持不变；本批未提交/未重建 App。新增 scoped PNG 读取接口、共用 client、authenticated bridge、Go DTO/schema 与生成镜像；尚无原生 command 或 UI resolver，不将其记为完整远程图片功能。
- 全局 session catalogue 不等于 workspace grant。saved/external 按持久化 workspace（有界、无回写、root confinement）校验，owned runtime 按自身 workspace 校验；只允许同 Serve/SSH workspace，逻辑 alias 必须同 inode。旧元数据缺失、其他工作区、退休/替换、路径/文件 inode 改变、取消明确失败；不借本地工作区/cwd/原 opener，不自动恢复/接管历史。
- 入站 body/source、图片 bytes/pixels、输出 PNG/尺寸和两层 admission slots 有界；解码不持有 bind lock，发布再校验全部归属。公共图片复用现有专用 SSRF/proxy transport；controller Cookie、remote path/openHref、私有配置/诊断不交给 renderer。
- 当前回归：生产 Serve 实际 token/Controller/磁盘附件、客户端 PNG/字段/预算/错误/privacy、真实 loopback SSH bridge handle 生命周期与取消、held root/alias 替换及 metadata/admission 门禁通过。完整四包 race、最终新增用例两次 race、vet、协议 DTO/schema/镜像、Rust check/clippy、生产/测试 typecheck 通过；证据 `/private/tmp/reasonix-remote-image.rKqpbO/`，最终完整 race 为 `four-packages-final-race.log`。仅后端 seam 测试归属 race，不冒充真实网络图片或 native UI。
- 下一步：main-window-only IPC → remote lease scoped image resolver → 图片界面/滚动与隔离实际 App 验收；同时保留没有历史 workspace 元数据的兼容恢复检查。不关闭真实外部服务、原生剪贴板、完整 controller/bot Desktop、Windows/Linux/复杂管理页/SQLite 门禁。
