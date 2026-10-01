# Reasonix Tauri host

这是逐步替换 Wails shell 的 Tauri 2 host；它暂不替换稳定的 Go Agent、会话格式或
现有前端 API。开发构建启动时 host 从 `REASONIX_DESKTOP_BRIDGE_BIN` 读取由构建流程提供的
`reasonix-desktop-bridge` 可执行文件路径；release 打包启动时忽略该开发覆盖，由 Tauri 的 `externalBin` 从应用
包内定位同一个 bridge。两种路径都通过每次启动独有的 token、ready 文件和 launch nonce
监管它。token 经子进程 stdin 管道发送，bridge 读完后关闭管道并将标准输入切到空设备；
token 不放在 macOS 进程列表可见的启动环境或命令行中。

## 当前平台范围

2026-09-30 用户确认本轮只推进 macOS 的改造、测试和发布候选验收。Windows/Linux
因缺少实际测试环境延期；已有代码保留，但不承诺平台支持，也不要求其原生验收完成
后才能推进 macOS。D→E 的顺序及 macOS 待验项以 [迁移清单](../../docs/tauri/API_SURFACE_AUDIT.md) 为准。

## 数据隔离

Preview 使用 Tauri 应用数据目录下私有的 `REASONIX_HOME`。因此它不会静默读取、迁移或
写入稳定 Wails 客户端的配置、会话和缓存；稳定版可与它并存。导入稳定版数据会作为单独的
“先备份、再确认”的功能实现。开发者显式传入的 `REASONIX_HOME` 仍是有意识的覆盖选择。

Go core 解析状态根和缓存根时，`REASONIX_STATE_HOME`、`REASONIX_CACHE_HOME` 分别优先于 `REASONIX_HOME`。托管的
Preview 在启动时会清除继承来的这两个变量，否则它们会覆盖上面的私有
`REASONIX_HOME`，把 Preview 的状态或缓存写入外部目录——这个错误是静默的，
所以由 `data_profile` 的单元测试固定。显式 `REASONIX_HOME` 属于非托管 profile，不做这项
覆盖，也不会启用自动导入。

窗口尺寸与最大化状态同样保存在 Preview 自己的 Tauri 应用数据目录，而非
`REASONIX_HOME`。损坏或不合理的状态会被忽略并使用默认窗口；导入、重置或删除 Go core
配置不会影响窗口偏好。

当前 Preview 只提供最小的显式配置导入：界面会显示默认稳定配置
`~/.reasonix/config.toml` 是否存在，用户确认后才复制它。复制前会在 Preview 私有目录创建
带时间戳的备份，目标 `config.toml` 必须尚不存在，绝不覆盖。会话、缓存、插件和 `.env`
均不会导入；导入后若 Provider 依赖环境变量，仍需由用户自行提供。显式设置
`REASONIX_HOME` 的开发环境不会显示该导入入口。
导入只接受不超过 16 MiB 的稳定版普通配置文件；备份与 Preview 副本均通过权限受限的临时文件
写入，再以无覆盖方式提交，避免导入期间短暂暴露配置中的凭据。

Preview 已开始采用工作台导航壳层：最近会话按工作区文件夹分组；可展开项目、为项目设置本地显示名称、切换到该项目最近会话，或直接在项目中新建对话。项目名称保存在 Preview 自己的项目目录中，只影响侧栏显示，不会重命名或移动磁盘上的工作区。首条用户消息生成本地可读标题；旧会话的缺失或默认占位标题通过只读摘要异步回填，手动标题优先，回填不改变最近使用顺序。侧栏采用 bridge 确认后的标题，避免并发手动改名被旧候选标题覆盖。切换会话通过
bridge 的显式 `switch_session` 完成。Global 对话没有项目归属，但使用当前档案内稳定的 `global-workspace`；顶部打开选择器由 host 查询实际工作区，避免采用进程启动目录。首版 bridge 仍只拥有一个 Go Controller，因此只允许
从 `idle` 会话切换；`running` 或 `paused` 的会话必须先结束或取消，绝不被 UI 静默替换。

侧栏可按会话标题、项目名称或项目路径筛选当前已加载的会话；目录尚有分页时会提示搜索范围。首次读取会话目录期间显示加载状态，读取失败时显示错误，避免把尚未加载的历史误报为空。

目前暴露给 WebView 的命令为 `bridge_status`、`restart_bridge`、`provider_summary`、`set_default_model`、`bridge_open_session`、
`bridge_session_snapshot`、`bridge_session_history`、`bridge_submit` 和 `bridge_cancel`。
token、loopback 端口和
bridge 原始请求不暴露给 JavaScript。`bridge_start_events` 在 Rust 内部订阅 SSE，并以
Tauri `bridge:event` 和故障时的 `bridge:connection-error` 事件转发给 WebView。

`provider_summary` 只投影 Preview 私有配置中的 Provider 名称/类型、模型 ID、模型数量、默认模型和凭据就绪布尔值；不把服务 URL、凭据变量名、请求 headers 或密钥传入 WebView。`set_default_model` 复用 Go 配置库校验和窄写入，只影响新会话，现有会话不会被重建。
系统钥匙串中的 Provider API key 只可通过保存和删除命令修改；WebView 无读取凭据值的命令，且不能用这些命令修改其他钥匙串条目。
凭据服务按 core profile 的持久随机身份隔离。早期 Preview 的 `keychain.dat` 不自动迁移；
设置页只显式复制选中的有效 Provider 凭据，保留原文件和旧原生服务，已有目标则拒绝覆盖。
来源必须是普通文件且不超过 1 MiB；所有条目先校验，写入和桥接同步失败则回滚。
`desktop/frontend/src/lib/tauriBridge.ts` 已为上述小范围命令提供类型化适配器，只会在
Tauri WebView 中激活；现有 `desktop/frontend/src/lib/bridge.ts` 仍是 Wails 默认实现，直到
对应功能面完成迁移。Tauri WebView 现会渲染 `TauriSessionPreview` 聊天工作台：左侧项目与
对话、欢迎页与建议入口、Markdown 消息流、底部输入框、顶栏工作区/新会话默认模型；运行时、
Provider 配置和桥接事件收纳在诊断抽屉。它仍是逐步迁移中的 Tauri 界面，尚不代表完整 Wails
功能对齐。助手正文按桥接 `text` 增量事件实时渲染；`reasoning` 与其他事件不进入聊天正文，
回合结束后由经过过滤的持久历史接管显示。
事件订阅可从 snapshot 的 sequence 开始，避免切换期间漏掉事件。sidecar 异常退出时，预览
提供受控重启：重新启动 bridge、重新打开当前 session、获取新 snapshot 后才恢复事件订阅和发送。
前端须等到 host 确认事件流已连接才允许发送；短暂断线会暂停发送，重连成功后恢复；若有界事件回放窗口已过期，host 会通知前端
重新获取权威 snapshot 和历史，再以新 sequence 订阅，而不会无限重试失效的旧游标。
`turn_done` 到达时 core 可能仍在 finishing 阶段；前端会等待空闲快照确认后才允许下一次
提交。快照持续失败或会话仍非空闲时保持禁止提交，并可在诊断面板手动刷新状态与记录。
新回合开始后，旧回合延迟返回的快照与历史不会覆盖当前状态；快速重复点击发送也只启动一次请求。

Preview 的 Runtime details 面板显示冻结的 1.38.3 基线与提交、当前 Preview/Tauri host
版本、打包时的 Preview 源码提交、bridge 协议版本和 live sidecar instance ID。当前 bridge 尚未提供独立的语义化发布
版本，因此界面不会把 instance ID 伪装成版本号。

选择 workspace 时，Preview 只提供系统目录选择器；`main-window` capability 只授予
事件监听、窗口拖动、系统文件选择器与文本剪贴板命令，不授予通用文件读写、保存对话框、shell、
更新器、托盘或菜单调用权限。选中的路径仍会通过既有 bridge 的 workspace 校验，
取消选择不会改变当前输入。
MCP 授权链接和“关于”页链接由 host 校验为不含 URL 用户名/密码的 HTTP(S) 地址后交给系统浏览器；WebView 不获得通用 shell 权限。

macOS 系统通知通过限定主窗口的专用 host 命令读取实际授权并返回提交错误，
只发送固定事件提示。点击使用档案内的随机 token 映射，由 host 重新核实会话与工作区；
前端忙碌时保留点击，同会话返回对话，失效会话不创建。未打包开发进程报告不可用。
Windows/Linux 的通知与凭据能力按当前平台范围延期；已有跨平台实现的证据和限制保留在迁移清单中。
当前实现、回归证据及真实系统 UI 待验收项见 [迁移清单](../../docs/tauri/API_SURFACE_AUDIT.md)。

开发环境还需要满足前端锁定的 Node 24 与 pnpm 10。使用前端目录中的本地 Tauri CLI
启动，脚本会把 Go sidecar 构建到被忽略的 `desktop/tauri/target/sidecar-dev/`，并仅向
Tauri host 注入其路径：

```bash
cd desktop/frontend
pnpm tauri:dev
```

这不依赖全局 `cargo tauri`，也不会将 sidecar 路径、token 或 loopback 地址暴露给 WebView。

## 构建 macOS 测试包

```bash
cd desktop/frontend
pnpm tauri:build
```

该命令先使用当前 Rust host target triple 构建 Go bridge，再由 Tauri 将它作为受管 sidecar
嵌入应用包。当前仅支持与构建机器相同的 target，不接受跨 target 打包；这是为了避免在
尚未建立 macOS 双架构签名与回归流程前制造未经验证的安装包。

本机构建没有 `APPLE_SIGNING_IDENTITY` 时会使用 ad-hoc 签名，适合本机测试但未公证；不要
将此类包作为正式下载发布。配置 Developer ID 与公证凭据后，正式发布流程必须保留 Tauri
的签名并完成 Apple notarization。

## 测试

依赖 Go bridge 的受监督生命周期测试在本地默认跳过。要让它们真正运行，先构建 bridge
并把路径传进去：

```bash
bridge_target="$(rustc --print host-tuple)"
mkdir -p desktop/tauri/binaries
go build -trimpath -o bin/reasonix-desktop-bridge ./cmd/reasonix-desktop-bridge
cp bin/reasonix-desktop-bridge "desktop/tauri/binaries/reasonix-desktop-bridge-${bridge_target}"
cd desktop/tauri && REASONIX_TAURI_BRIDGE_TEST_BIN="$PWD/../../bin/reasonix-desktop-bridge" cargo test
```

`CI` 环境变量存在时该路径是必需的：缺失会让这两个测试失败，而不是静默跳过。

桌面 CI 的前端测试计划会用专用 SVG、CSS 和 bridge stub 运行 Tauri 组件测试；macOS
host job 还会构建真实 Go sidecar、运行 Rust 集成测试、打出 Preview `.app`，并检查包内
host/sidecar 可执行文件及代码签名。包级 smoke 分别在临时 HOME 下使用托管 profile 与显式
`REASONIX_HOME` 启动真实 `.app`：检查 sidecar 实际继承的 profile 环境、ready 文件、父子进程
关系、无认证请求被拒绝和正常退出后的清理。测试故意注入旧 token 环境值，要求 sidecar 启动环境将其清空；
托管模式还注入外部状态与缓存覆盖，要求 host 在启动 sidecar 前清除。
集成测试使用临时 profile，bundle 使用开发签名；正式发布仍需
实际发布二进制兼容认证、Developer ID 签名与公证，以及停写后的真实数据恢复演练。

原生窗口包级验收使用同一临时档案连续启动真实 `.app`，分别检查尺寸/位置、窗口隐藏和
最小化恢复、最大化重启后取消最大化、普通窗口重启、应用隐藏/取消隐藏、关闭后继续运行及
关闭即退出；还通过真实 AppKit Settings 菜单动作，检查隐藏窗口、最小化窗口和隐藏应用的
恢复，以及宿主恰好发出一次设置事件。还读取真实 AppKit 菜单的 Cmd/Ctrl 组合，核对共享
前端保留表；包括系统补充的 Emoji 组合，未知组合必须失败。还使用临时档案内的本地流式
provider，通过实际 Go core 验证运行时关闭窗口后继续收到内容并保存完整历史，经原生
Show 菜单恢复窗口，再运行第二个任务并通过实际 Quit 菜单退出，检查上游断开和 sidecar 清理。
还检查实际 AppKit 应用外观与持久偏好，验证 dark/light/auto 保存和重启恢复；auto 必须
清除显式 NSAppearance，而非仅与当前系统颜色碰巧相同。非法主题/样式保持原配置和原生外观。
还通过与生产保存相同的锁定事务注入外观失败，验证未配置标记、实际原生写入后的回滚、
过期配置回滚拒绝与失败提示；原生回滚失败时保留真实不一致状态，再由下一次整包启动恢复。
外观回滚使用 host/sidecar 内部鉴权接口，不新增 renderer 命令或权限。故障阶段卸载前端
页面以免自动同步掩盖错误，因此不代替设置页、标题栏视觉或系统拒绝的交互验收。
还通过实际 WKWebView 的 Tauri IPC 写入并读回系统剪贴板，要求图片读取及无权限的同源
隐藏窗口文本读写被拒绝。Swift helper 先完整备份有界的多格式剪贴板，再独立核对系统值
和 changeCount 并恢复原件；用户中途复制的不同内容保留，恢复不确定时保留私有恢复快照。
文件承诺、不可完整读取或超限内容会在写入前拒绝验收。helper 的多格式恢复、中途变化和
中断恢复保护先在独立命名 pasteboard 自测，不改动系统剪贴板。该项不代替输入框按钮或物理按键验收。
默认和显式 core 档案各执行 20 个场景，核对凭据身份
稳定、实际 sidecar 鉴权与退出清理：

```bash
python3 tools/tauri/smoke-native-window.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

菜单组合、剪贴板 IPC、严格原生编辑和面板取消也可独立运行，避免窗口最小化失败
阻止收集这些项目的证据：

```bash
python3 tools/tauri/smoke-native-window.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --independent --edit --dialogs
```

此切片在两种新私有档案各运行 4 项，共 8 项，保留真实 AppKit/WKWebView、active/key
焦点、权限拒绝、完整剪贴板恢复、档案身份和宿主/sidecar 清理断言。它不执行窗口几何、
最小化、Settings 恢复或任务/退出菜单场景，不能作为完整窗口门禁通过；默认门禁不变。
`--focus` 需要前置保存的窗口几何，不能与 `--independent` 组合。

已安装的 macOS 原生 Minimize 菜单有独立角色门禁：

```bash
python3 tools/tauri/smoke-native-menu-window.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

它核对实际 active/main key window、已安装 `performMiniaturize:` responder 角色、真实
最小化状态及 AppKit Will/Did Miniaturize 事件，再调用 Settings 恢复并要求 Did
Deminiaturize 事件和恰好一次设置事件；两种新私有档案还核对同档案重启身份与退出清理。
此门禁不调用 Tauri 的最小化 API；完整窗口 API/几何门禁继续保留，原生菜单动作也不
等同于物理点击或按键。

macOS 外观同步先在主线程核对实际 NSApplication 显式/有效外观及 Tauri 缓存，已符合
请求时跳过重复设置。auto 必须是显式 nil 且缓存符合有效系统主题，不能把显式浅色当作
auto；显式选择和过期缓存仍需更新。超时后的查询回调只读，不会产生延迟主题写入。
此收敛不代表窗口最小化根因已定位，完整窗口门禁仍须验证。

`--api-startups 4` 可在原生菜单检查前，分别对两种档案安排 4 个独立的原 Tauri API
启动样本；任一样本失败立即停止，不重试，不把后续未执行项记为通过。2026-10-01
新包 `6d405f8f8` 的完整 46 阶段与两种档案原生菜单门禁曾通过，但随后追加的原生菜单
及原 API 首个托管启动样本均失败，即使两次外观请求都已跳过重复写入。窗口稳定性仍
未验收；这也排除“重复外观写入是唯一原因”的判断。详情以迁移清单为准。

最新只读诊断包 `67c36323c` 增加实际应用启动完成状态与应用/窗口 occlusion 可见标志：
`--api-startups 2` 的 4 个 API 样本、2 个原生菜单恢复及2 个菜单重启阶段全部通过，
但完整门禁在首个 exercise 最小化失败（0/46 完整阶段通过）。失败时应用已完成启动、
系统报告窗口可见，实际最小化事件仍为 0；不能把等待启动完成或未被遮挡当作已证实修复。
随后 `ada308408` 只读追加 modalWindow、attachedSheet/isSheet/inLiveResize，完整门禁
23/46：托管全部通过，显式首项最小化失败，请求/超时均无模态窗口、sheet 或实时调整。
两种档案普通包生命周期通过。没有新 renderer API、激活/恢复重试或放宽最小化断言；
当前窗口稳定性未验收，失败状态和各包日志见最新迁移清单。

该脚本拒绝操作已运行的同标识 Preview，使用原生宿主 API，不依赖屏幕录制权限。
菜单动作检查不能代替真实鼠标/键盘操作或 WebView 设置界面的渲染验收。测试先确认窗口的
隐藏/显示转换，再建立严格的最小化前提；启动就绪后直接最小化未成功的时序仍保留记录。
第二实例键盘焦点使用额外的严格门禁 `--focus`，通过 LaunchServices 打开真实第二个进程，
同时要求原窗口可见、几何正确且 `is_focused()` 为真，原 sidecar 保持唯一。此前完整包的
两种档案焦点门禁已通过，当前包覆盖以最新迁移清单为准；默认窗口门禁不能代替该项验收，
也不代表菜单/托盘点击、键盘编辑或
显示器拔插已经验收。macOS CI 已接入默认窗口门禁，远端执行结果待确认。

浏览器链路有独立门禁，不依赖外接屏或 Preview 键盘焦点：

```bash
python3 tools/tauri/smoke-native-links.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

它在默认和显式私有档案下，通过主 WKWebView IPC 验证两个现有链接入口的非法 URL
拒绝，再实际打开默认浏览器，并要求两个本机 canary 页请求到达、宿主/sidecar 正常退出。
不加载外部资源、不继承用户凭据环境、不增加 renderer 命令或权限，也不关闭既有浏览器
标签页；新建的测试标签页可手动关闭。失败保留私有记录，成功删除测试档案。
这项不代替真实点击、邮件客户端、OAuth 登录/回调或页面视觉验收。
2026-10-01 从干净提交 `185b7223d` 构建的本地 ad-hoc 包，已通过此门禁两种档案
及普通包生命周期验收；正式 Developer ID 签名/公证仍待完成。
2026-10-01 用户仅延期不同缩放显示器/外接屏拔插测试，其余 macOS D 门禁仍保留。

通知提交与实际 Notification Center 送达有独立 macOS 门禁：

```bash
python3 tools/tauri/smoke-native-notifications.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

该门禁只使用已有系统授权；在托管/显式新私有档案各经生产 host 命令发送 3 类固定
提示，通过 UserNotifications 查询自己生成的 identifier 及实际标题/正文，移除并确认
该 identifier 不再送达。后两次发送先确认应用已隐藏；第一次发送如实记录是否 active，
不强制激活，也不默认视为前台。清理只针对本次随机 token，不移除其他通知；
不打印已有通知或 token。还核对 canary 原件、sidecar/readiness 清理及同档案重启身份。
授权未开启时失败且不主动请求授权。此门禁不经过 renderer IPC，不证明可见横幅、
物理通知点击、冷启动点击或真实系统拒绝，相关交互验收继续保留。

2026-10-01 干净提交 `67e7fcd8b` 的真实 arm64 包已通过两种私有档案的 6 次实际送达：
2 次 active=true、4 次 hidden=true/active=false，固定标题/正文、精确清理、原件与重启
身份均保持。横幅视觉和实际点击仍待执行；不能将此切片作为全部通知能力通过。

指定应用失败反馈另有独立门禁：

```bash
python3 tools/tauri/smoke-native-app-failure.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

它在私有 HOME 创建缺少 executable 的测试 Ghostty bundle，先严格确认生产 catalog
解析到该私有目标，再经主 WKWebView IPC 要求未知应用 ID 和真实 LaunchServices 拒绝
均返回错误；核对特殊文档路径的内容/权限/mtime 不变和两种档案的退出清理。
不会操作真实已安装 Ghostty，也不代替正常应用打开、终端 cwd 或物理 UI 验收。
2026-10-01 干净提交 `c83f947e5` 的本地 ad-hoc 包已通过两种私有档案的实际主 IPC
启动拒绝门禁；同包默认浏览器门禁也已复验通过。

系统 Terminal 实际工作目录使用独立门禁（需要本机 Clang/SDK）：

```bash
python3 tools/tauri/smoke-native-terminal.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

主 WKWebView 通过原有指定应用入口打开私有目录及其文档，要求两个新系统 Terminal
shell 的原始 kernel cwd 精确匹配包含中文/换行/引号/`$` 的目录。独立私有 helper
使用 macOS libproc，核对同用户 UID，不读取终端文本/命令参数，不依赖截图或 Apple Events。
回收前重新核对 PID 出生时间、祖先链、可执行名称和 cwd；可能留下可手动关闭的已结束
测试窗口。该门禁不代替其他应用、物理菜单点击或项目/Global 会话的 UI 打开验收。
2026-10-01 干净提交 `a9217f828` 的本地 ad-hoc 包已通过两种档案的实际系统 Terminal
目录/文档入口和 kernel cwd 验收；每档案均要求两个新会话根 shell，排除启动子 shell。

自定义 native/bridge 命令总入口要求 Tauri 注入的原生调用窗口为 `main`；其他窗口
在命令分发前被拒绝，前端参数不能伪造窗口身份。插件仍执行原有 capability 检查。
真实调用窗口和可执行文档边界使用独立门禁：

```bash
python3 -B tools/tauri/smoke-document-scope.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

主窗口必须拒绝可执行文档及其符号链接别名，同时能读取真实系统应用 catalog。
同源隐藏 WKWebView 必须拒绝 8 个文档/工作区应用命令以及偏好/bridge 状态查询，即使
传入伪造的 `window:'main'`。检查原文件内容/权限/mtime、未执行 executable、宿主及
sidecar 清理；失败保留私有记录。这项不代替正常 Finder/编辑器打开或物理对话框操作。
2026-10-01 干净提交 `2cbed72bb` 的本地 ad-hoc 包已通过两种档案的双 WKWebView
权限门禁及独立剪贴板插件 ACL/原件恢复；本轮 42 阶段回归在最小化设置窗口前提停止，
尚不能标记全部通过，详见迁移清单。
同一包后续在私有普通档案通过 CUA 的产品输入/全选删除/撤销重做、实际 Edit→Redo、
Cmd+M 后原生 Settings 点击恢复、中文/空格目录与文本文件面板选择、工作区/项目重启恢复、
深色模式点击/渲染、Cmd+Q 与关闭即退出按钮及 sidecar/readiness 清理。
这些是真实产品 UI 自动交互证据，不能替代完整 `--edit`/`--focus` 或人工硬件键盘验收。
独立 LaunchServices 启动仍卡在自动最小化前提，未调整窗口断言；外接屏按用户要求延期。
退出后不要用 CUA 查询已退出应用；此操作可能重新启动普通档案，应改查已持有的进程句柄
和私有文件。详细范围和本轮新启动进程的清理记录见迁移清单。
后续干净诊断包 `e3e030586` 的完整 `--dialogs --edit --focus` 46 阶段全过，
包括两种档案的原生编辑和第二实例严格焦点；普通生命周期的系统授权实读均为 granted。
这是当前包的通过证据，不能推断旧包最小化失败的环境原因。
全新私有 HOME/core 曾继承旧默认工作区：macOS WebKit 默认存储不随 HOME 切换。
干净生产包 `673c30023` 已按持久档案身份绑定 `reasonix-preview` UI origin，主窗口
导航限定当前档案，协议只提供编译资产并保留 Tauri MIME/CSP；没有新增文件或网络权限。
同档案重启/目录移动保留 origin，不同档案隔离；开发服务器 URL 不变。

```bash
python3 -B tools/tauri/smoke-native-ui-storage.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

同一包 18 个 UI 存储阶段、完整 `--dialogs --edit --focus` 46 阶段、两种档案的
同 origin 文档/bridge 权限与普通生命周期均通过；Rust 198 项通过、2 项既有忽略。
UI 存储门禁核对实际工作区按钮重启渲染、跨档案返回及仅清理自有 canary，不清空系统
WebKit 仓库。旧 `tauri://localhost` 共享 UI 偏好保留，新 origin 首次启动使用默认偏好；
当前包已接入设置→存储与路径→旧 Preview 界面偏好：显式只读预览、核对确认导入和
带记录的撤回。仅迁移 22 个固定界面 key，先保存回退记录再写入，保留源记录及后续修改；
读取隐藏窗口不运行产品 UI，没有 capability，关闭偏好仅作用于主窗口。

```bash
python3 -B tools/tauri/smoke-native-ui-legacy-read.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

干净包 `3225f43c3` 的两种档案重复旧 UI 只读 IPC、读取窗口销毁、双窗口权限与
普通生命周期均通过；实际设置页预览、未确认按钮禁用及当前工作区不变通过 CUA 验证。
合成偏好导入/撤回、重挂载恢复和存储失败保护的组件回归通过，Rust 199 项通过/2 项忽略。
受控私有来源的三项偏好完整真实 UI 导入/撤回已在 `ada308408` 包完成，其他偏好及异常
UI 交互仍待验收。当前已提供以下两个入口：

```bash
# 只读三项固定测试偏好；两种档案各重启一次，要求每次新回执和正常退出。
python3 -B tools/tauri/smoke-native-ui-legacy-read.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --private-source
# 普通产品 UI 的六次生命周期，需操作员实际预览、确认、导入/撤回及 Cmd+Q。
python3 -B tools/tauri/smoke-native-ui-migration.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --seconds 600
```

私有来源限定为当前用户拥有的 0700 测试目录、0600 控制文件；固定工作区、深入模式和
`large` 字号仅写入已核对 `isPersistent()==false` 且为空的隐藏 WKWebView 数据仓库，
不修改系统共享旧 origin。`--private-source` 的实际主 IPC、三项精确值、读取窗口销毁和
重启身份/原件保护门禁在 `f72a9b7a2` 包 **4 次启动/8 次读取通过**；不带标志的正常旧
origin 只读门禁也通过。完整 UI runner 每次启动移除旧回执，三次均需重新预览。
托管档案先导入前预览/确认/导入并退出，再验证重启值/预览/撤回并退出，最后验证默认值、
回退记录消失/预览但不再导入并退出；随后对显式档案重复。控制 JSON 给出自有 PID 和阶段，
绑定原生 UI 前需确认进程仍存活，退出后不可再次读取旧应用绑定。
runner 成功只证明来源、身份、原件和退出检查，UI 应用/恢复必须另有实际界面证据。
`--profile managed|explicit|both`（默认 both）可只推进独立尚未完成的档案，输出实际通过的
生命周期数。显式测试只设置 REASONIX_HOME/cache，不添加独立 state override。
首次字号夹具 `18` 已纠正；此前捕捉失败保留。当前两种档案的工作区、深入模式和较大字号
均已通过 CUA 实际预览/确认/导入/重启/撤回/再重启，共六次正常退出及新来源回执、身份、
原件保护和进程清理检查通过。托管即时撤回观察曾报 `-3812`，没有重复点击，新进程实际
确认三项恢复与回退记录消失；显式即时反馈“已恢复 3 项”也通过。两种成功档案已清理。
托管完成后的显式夹具环境检查曾失败，移除多余 state 变量后仅重跑未完成显式三阶段通过；
不把设置失败或全部 22 项/异常 UI 当作通过。详见迁移清单最新记录。
此前通知准备在发消息前停止，尚无横幅/点击证据。
本地 ad-hoc 签名不等于正式签名/公证；外接屏延期和其他发布门禁见迁移清单最新记录。

macOS 受管 bridge 跟随实际 kernel parent 生命周期。宿主异常退出时取消任务/HTTP/SSE，
处理宿主日志管道断开产生的 SIGPIPE，并释放目录锁、清理自己的 readiness 文件和空目录。
不带 `--host-pid` 的独立 bridge 客户端行为保留。

```bash
python3 -B tools/tauri/smoke-host-lifetime.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

干净包 `715f8743c` 的 8 项全部通过：两种私有档案 × SIGTERM/SIGKILL × 空闲/实际流式任务。
runner 只终止已核实的自有宿主；kernel 确认 sidecar 正常退出码 0，provider 实际断开，
ready 文件/目录清理、原件保护和同档案重启通过。握手完成前的强制终止还需单独验收。
普通包生命周期两种档案和使用新 sidecar 的 Rust 199 项通过/2 项忽略。
本轮完整窗口门禁停在 `exercise` 的最小化前提，尚未全部通过，保留为独立待办。
独立正常路径每档案 5 项、共 10 阶段通过：菜单组合、真实后台任务/Menu Quit、
系统剪贴板、四类面板取消及严格原生编辑。任务使用实际保存窗口 frame 检查恢复；
不覆盖最小化/最大化或第二实例焦点，不替代完整窗口门禁。

启动阶段另有门禁：Rust 在 spawn 前发布私有、无认证 token 的启动记录，Go 在读取
stdin token 前观察实际 kernel parent，取消可打断半截 token 读取。清理仅作用于
匹配记录/instance ID 和原 inode 的空 0700 目录；不删除替换目录或其他原件。

```bash
python3 -B tools/tauri/smoke-native-startup.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
```

干净包 `30a93592d` 的 12 项通过：两种档案 × SIGTERM/SIGKILL × 父校验前、
token 读取前及读取后尚未启动 core。kernel exit 0、清理、原件与同档案重启通过；
同一新包就绪后 8 项也复验通过。范围为 sidecar 已经启动，不覆盖 spawn 前全部目录回收。
同一新包完整窗口门禁为 31/46：托管 23 项全过，显式在 Settings 最小化前提停止。
新增固定计数证明该失败没有额外恢复/Reopen 请求；最小化根因仍待定位。

同包的普通托管私有产品输入框已通过实际 CUA Cmd+C/X/V：剪切后为空、粘贴后
精确文本恢复；独立 AppKit helper 核对系统值和代次。清除测试草稿后实际 Cmd+Q
退出码 0，档案身份/原件保护和 sidecar/ready 目录清理通过，所有原剪贴板格式已恢复。
仅覆盖输入框，不推广为消息复制按钮、错误反馈、上下文菜单或显式档案全部 UI。
本轮 Cmd+M 的 AX 观察不足以确认最小化，仍保留严格原生门禁失败。

原生 responder 编辑另有严格门禁 `--edit`：默认 40 个场景之外，每档案增加一个
`menu-editing` 阶段。它复用完整剪贴板保护，要求应用 active、主窗口是实际 key window
且 WKWebView 接受 first responder；核对已安装 Copy/Paste/Cut/Select All/Undo/Redo 的
selector 和 nil target，调用前执行菜单验证。临时 textarea 的值/选择、粘贴 input 事件、
剪贴板独立读回及粘贴/剪切后的原生撤销重做都必须匹配；Copy/Cut 前先种入不同的唯一
测试值，不能因旧剪贴板内容相同而误通过。此项不是物理按键、鼠标或产品输入框验收。

```bash
python3 tools/tauri/smoke-native-window.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --edit
```

真实系统对话框取消使用独立 `--dialogs` 门禁，每档案增加一个 `dialog-cancel`
阶段。宿主分别调用实际文档另存为、诊断导出、主题导入和目录选择入口，观察当前
进程唯一可见的 NSSavePanel/NSOpenPanel 后调用 AppKit cancel；保存面板还核对
私有文件名。要求生产回调返回取消结果、面板关闭、主题未导入、原文件内容/时间戳/
权限不变且无额外文件，沿用鉴权、档案身份和退出清理检查。不伪造插件回调或选中
路径，不增加 renderer 权限；这不证明鼠标/键盘取消、实际选择/保存或系统错误已验收。
macOS CI 已加入该门禁，本机干净包的 42 个阶段通过，远端 CI 结果待确认。

```sh
python3 tools/tauri/smoke-native-window.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --dialogs
```

当前本机 macOS 27.0.1 的原生编辑 `--edit` 门禁停在严格焦点前提：responder 接受、窗口可见且应用未隐藏，
但应用 inactive、key window 不存在。产品共享恢复路径及显式当前 AppKit activate 请求
都未建立激活；尚未执行后续编辑动作，不能标记为通过，也不能据此确定产品编辑有缺陷。
待桌面能够实际激活 Preview 后再运行；默认门禁通过不能覆盖该项。

配置导入包级验收使用临时 HOME 中的正式版和 Preview 档案，调用设置页同一已确认宿主入口。
验证配置/项目目录导入、真实 bridge 读取、Preview 修改默认模型后 sidecar 重启、整包重启
恢复、备份与原件保护、拒绝覆盖/显式目录导入及退出清理。原件比较包含字节、权限和修改
时间；项目仅导入 root/title，不复制旧会话排序、sessions/cache/plugins 或 `.env`。

```bash
python3 tools/tauri/smoke-profile-import.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app'
# Optional: construct a valid legacy fixture, then verify it remains readable.
python3 tools/tauri/smoke-profile-import.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --legacy-cli /absolute/path/to/legacy/reasonix
```

可选旧版 CLI 会在保存基线前自行升级测试配置；Preview 退出后的旧版查询必须不再改写
原件。该验收不代替设置页真实点击、旧 Wails GUI/目录锁或完整会话回退验收。macOS CI
执行三个标准场景，不依赖开发机上的旧版二进制；远端执行结果待确认。

2026-10-01 已使用官方 CLI `v1.38.3` 原生 arm64 发布包完成同一门禁，三个导入/
重启/显式档案阶段和最后旧版回读均通过，原件字节、权限、mtime 不变。归档校验值
与官方 release API 一致，二进制构建信息对应基线 `fa018e4`、`vcs.modified=false`。
这补充了前轮有修改工作区 CLI 的证据，范围仍是配置回读，未代替旧 Wails GUI/
历史目录互斥或完整会话附件回退。版本、摘要和边界详见迁移清单。

官方 Wails `desktop-v1.38.3` 归档也已核对完整大小、SHA-256 与沙箱外严格
Developer ID 签名；进一步使用官方 CLI 生成真实文本历史后，旧 GUI 在 Preview 导入/
重启前后的会话租约、拒绝第二 writer、原生退出及原历史字节保护均已通过。
空会话的 JSONL 延迟保存，不能用其文件出现作为旧 GUI 启动就绪条件。

独立旧版回退门禁需要原生 arm64 macOS、Swift 编译器和已取得的官方 1.38.3 二进制，
固定摘要拒绝本地修改版本。成功清理临时档案，失败保留私有记录；默认覆盖文本
历史，`--workspace-data` 另外覆盖私有 Global 文件/文本与图片引用、实际文件编辑及
检查点：旧版鉴权 loopback serve 完成引用解析，Preview 导入/重启不改原件；旧版
恢复后再次解析/编辑并生成第二检查点，实际最新代码回滚恢复文件。旧版随后向早期
检查点回滚会以 `file conflicts detected` 拒绝，门禁核对该拒绝没有修改文件或历史；
不能据此认定连续跨检查点回滚通过。两种模式均不代替图片渲染/像素送模型、完整
历史数据、真实 GUI 点击、物理按键或宿主目录生命周期互斥验收：

```sh
python3 tools/tauri/smoke-legacy-rollback.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' \
  --legacy-app '/absolute/path/to/official/Reasonix.app' \
  --legacy-cli '/absolute/path/to/official/reasonix' --workspace-data
```

本轮平台范围仅为 macOS，Windows/Linux 等有测试环境后恢复。

2026-10-01 从干净提交 `91cbff49cbe10a0faab0a7570d880bad87ffc126` 完整构建 macOS `.app`，
严格本地 ad-hoc 签名校验、两种临时档案共 14 个原生窗口/关闭场景和原有包级启动/退出
smoke 均通过；同时通过前端 Tauri 回归与使用真实 bridge 的 189 项 Rust 回归（2 项默认忽略）。
第二实例焦点严格门禁失败，真实界面点击与正式签名/公证继续保留待办。

2026-10-01 设置菜单修复从干净提交 `6284dd44cbd663cb68c69613b6c668bf05fdb415` 完整构建
macOS `.app`；严格本地 ad-hoc 签名校验、两种档案共 20 个原生窗口/Settings/关闭场景及
原有包级启动/退出 smoke 均通过，构建与验收后工作树干净。Rust 使用真实 bridge 的 189 项
回归、严格 clippy 与前端 Tauri 回归通过。原生 Settings 动作验证宿主事件和窗口恢复，
不代表 WebView 设置覆盖层、键盘编辑或第二实例焦点已完成现场验收。

2026-10-01 菜单快捷键冲突修复从干净提交 `b9fccd040334ea1d68da5d02d358962a818c648c`
完整构建 macOS `.app`；严格本地 ad-hoc 签名校验、两种临时档案共 22 个原生菜单/窗口/关闭
场景及原有包级启动/退出 smoke 均通过，构建与验收后工作树干净。Rust 189 项（真实 bridge）、
严格 clippy、前端 Tauri 及设置组件回归通过。独立 Chrome 页面还验证录制拒绝、换键保存、
重置冲突与重置全部；Browser 插件不可用、Playwright WebKit 未安装，未安装新依赖。
这些证据不代替真实 WKWebView 编辑、原生键盘路由、第二实例焦点或正式签名/公证。

2026-10-01 后台任务验收从干净提交 `c2da30b25cd3219e59d1562a340abb0ceb0381ca` 完整构建
arm64 macOS `.app`；严格本地 ad-hoc 签名、两种档案共 24 个原生场景及原有包级生命周期
smoke 均通过，构建与验收后工作树干净。运行任务时关闭窗口后仍收到真实 Go 流事件并保存
完整历史，原生 Show 恢复普通几何；第二个任务运行中实际 Quit 菜单终止上游流并清理
host/sidecar/readiness。Rust 189 项真实 bridge 回归及严格 clippy 通过。这是程序化原生动作
验收，物理 Cmd+Q/托盘点击、第二实例焦点、外接屏及正式签名/公证仍待完成。

2026-10-01 配置导入验收从干净提交 `ccfa44177a1b5a68d9945b4d74464f85fd3ab81d` 完整构建
arm64 macOS `.app`；严格本地 ad-hoc 签名、三个标准导入场景、三个本地旧版配置场景及
最后 CLI 查询、原有 24 个原生场景和两种档案启动/退出 smoke 全部通过，工作树干净。
原件文件集合/字节/权限/修改时间和备份保持不变，Preview 修改经 sidecar/整包重启保留。
本地旧版 CLI 的 `vcs.modified=true`，不证明正式发布 artifact、Wails GUI/目录锁、完整
会话回退或设置页物理点击已认证；证据与 SHA-256 见迁移清单。

2026-10-01 原生外观修复从干净提交 `f6f38bd32aaeac24dc159e6ea24c5c7b6b8d51c4` 完整构建
arm64 macOS `.app`；严格本地 ad-hoc 签名、32 个原生场景、三个配置导入场景及两种档案
启动/退出 smoke 全部通过，工作树干净。保存 dark/light/auto 后读取实际 AppKit 外观，
整包重启恢复，auto 要求清除显式外观；非法 theme/style 保持原配置和原生属性。
189 项真实 bridge 回归、严格 clippy 与生产门禁通过；真实界面/系统外观切换和原生失败
回滚的故障注入仍待验收。同次审计新增 Markdown 图片 resolver 的 A/B/C 发布缺口，见清单。

2026-10-01 剪贴板验收从干净提交 `b8e420d6c0f22f998c943f5a40ef5a8f0645efa6` 完整构建
arm64 macOS `.app`，源码标识无 dirty 标记；严格本地 ad-hoc 签名、34 个原生场景、三个
配置导入/重启场景及两种档案启动/退出 smoke 全部通过，工作树干净。实际 WKWebView
文本读写、图片及无权限同源窗口拒绝、独立系统值/代次核对和完整剪贴板恢复通过。
189 项真实 bridge 回归、严格 clippy 与生产门禁通过；输入框/消息复制按钮、物理按键及
responder 编辑仍待验收，正式发布签名/公证尚未执行。

2026-10-01 原生编辑门禁从干净提交 `f7c30746ff42aeafc6f09a8140def8c0a5ab8277` 完整构建
arm64 macOS `.app`，构建源码标识无 dirty 标记；严格本地 ad-hoc 签名、默认 34 个原生
场景、三个配置导入/重启场景及两种档案启动/退出 smoke 全部通过，工作树干净。
189 项真实 bridge 回归与严格 clippy 通过。独立 `--edit` 的实际探测仍停在 inactive/key
window 缺失前提，未通过的编辑动作未计入默认统计；完整包复核未在相同条件下重复尝试。
待桌面能实际激活 Preview 后复验，物理输入/点击、其他 macOS D 和 E 及正式发布门禁仍保留。

2026-10-01 外观回滚修复从干净提交 `1cb40d135b5e3be7fed9d7134d364d518223d64c` 完整构建
arm64 macOS `.app`，构建源码标识无 dirty 标记；严格本地 ad-hoc 签名、默认 40 个原生
场景、三个配置导入/重启场景及两种档案启动/退出 smoke 全部通过，工作树干净。
首次 native 失败保留未配置标记，实际写入后的回滚、过期回滚拒绝及 native 回滚失败后的
整包恢复通过。配置层/Go bridge 全量回归、定向 go vet、189 项真实 bridge Rust 回归及
严格 clippy 通过；真实系统/磁盘拒绝、设置 UI/系统外观切换及原有 D/E 发布门禁仍保留。

2026-09-27 本机以独立 bundle ID 构建并 ad-hoc 签名测试包，包级 smoke 两种 profile 都通过：host 从临时 HOME
读取应用数据、启动包内 sidecar、验证实际继承的环境和 ready 文件，并在正常退出后清理子进程和临时目录。
旧测试包的托管模式曾被该门禁准确检出继承外部缓存覆盖；重建当前代码后通过。该测试包
只验证本机生命周期；正式 Preview 标识的 CI 包仍需在独立 macOS runner 上通过同一门禁。

2026-09-27 又从提交 `dcf4c5889587e94ba9fa40f596f1a68365e94dc9` 的干净 worktree 运行完整构建：
锁定依赖安装、Tauri 前端测试、Go bridge 测试、Rust 格式与 clippy、对真实 bridge 的 95 个 Rust 测试均通过。
仓库的 `tauri-build.mjs` 从该提交生成正式 Preview 标识的 `.app`，包内含同一源码提交标识，
ad-hoc 签名通过严格校验，构建后 Git 仍干净。随后复用这次完整前端产物与 sidecar，构建独立
bundle ID 的测试 `.app`；托管和显式 profile 的真实启动 smoke 均通过。正式标识的远端 CI
启动结果、旧发布二进制兼容、真实用户数据恢复，以及 Developer ID 签名和公证仍需分别验证。

2026-09-27 从提交 `7acf1358998d92f2eeb886a402e07a22a878dc1d` 的干净 worktree 再次运行
`tauri-build.mjs -- --bundles app --ci`，得到正式 Preview 标识的 `.app`；严格签名校验与
托管、显式 profile 的真实包级启动和退出 smoke 均通过，构建后 Git 保持干净。

2026-09-27 标题回填与并发保护提交 `cb752b831f1e198f2f62b4d4060b036618b69352` 也从干净 worktree
重建为正式 Preview `.app`；前端构建预算、严格签名校验和两种 profile 的真实包级 smoke 均通过。

2026-09-30 原生通知授权与会话定位切片从干净提交
`cedc0c7d6d0a3925a5cd14fc1e7fbf4995dea310` 构建正式 Preview 标识 `.app`；
前端构建、Tauri 回归、36 项打包契约、161 项真实 bridge Rust 回归和严格 clippy 通过。
本地 ad-hoc 签名校验及两种档案的真实包级 smoke 通过，增加只读 macOS 原生通知授权查询；
未执行授权弹窗、真实通知显示或点击，相关 UI 验收继续保留。

2026-09-30 Global 工作区切片从干净提交
`065b1ae5f676e38a1f84a2a1bfab8f08a85bd7cc` 构建正式 Preview `.app`。
Rust 163 项、Go bridge/运行时/协议、前端 Tauri 回归、生产构建、严格 clippy、Go vet
与 Wails Global 能力基线通过；严格本地 ad-hoc 签名及两种档案包级 smoke 通过，
增加实际 Global 会话目录解析及 `0700` 权限检查。未执行外部应用 UI 交互。
旧 rootless 会话在旧启动目录创建的文件不自动搬迁，其文件引用/附件/检查点兼容
仍需验收，详见迁移清单。
