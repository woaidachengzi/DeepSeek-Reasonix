# Reasonix Tauri host

交互验收位置：用户确认左侧屏幕为屏幕2；后续先将私有验收父窗口恢复到左屏并检查bounds，再打开弹窗，避免干扰屏幕1。[当前46包左屏恢复及目录sheet验证](../../docs/tauri/evidence/2026-10-04-d-screen2-placement/README.md)。不读AX的Cmd+M/Minimize菜单观察仍无原生最小化完成，缺口保留。

当前46包补验：[真实目录、多文件与默认工作区重启](../../docs/tauri/evidence/2026-10-04-d-current-directory-multifile/README.md)。一份explicit私有档案真实Open/Cancel、两附件含中文空格路径、移除/重启持久化与两次正常Quit清理通过；其他档案/入口/保存错误场景仍待，D窗口稳定性仍未完成。

最新状态：[46c2c9da 选区普通重开与响应者诊断](../../docs/tauri/evidence/2026-10-04-d-selection-reopen-ready/README.md)。修正仅opt-in重开探针读取storage的就绪前提；当前包两档案实际配置录键、引用二次发送和普通重开/无draft回放通过。Rust231/5ignored、strict clippy、构建/DMG安装签名、七程序gate（含窗口48/48与全屏）及配置备份官方CLI回读通过。物理Cmd+M分发时key/查询target均main但无Mini完成；实际全屏菜单进/快捷键退/精确恢复与Quit通过，首次键进及最小化仍未通过。其他D/官方GUI全资料回退不继承旧包；D未完成，E逐项验收待D稳定，A/B/C及正式签名公证仍阻碍发布。历史现场不直接代表当前候选。

当前窗口补验：[完整矩阵失败与零显示器保护](../../docs/tauri/evidence/2026-10-03-d-zero-display-window/README.md)。b6首阶段原生最小化成功但零活跃显示器期间无新存档，整组保持失败；两档案实际零显示器旧状态保护、托盘语言6阶段及启动终止12项通过。同包累计17个程序门禁，恢复活跃显示器后完整矩阵及物理UI仍待验。
2026-10-02 实际窗口验收可用 `python3 tools/tauri/probe-launch-services-profile.py <installed-app> --interactive --observe`。
在全新私有档案启动并观察最多 300 秒；仅该显式模式启用只读原生窗口快照，
不替用户操作窗口。真实 Quit 后用确切 PID 的 kernel 回执核对退出码与 sidecar/ready 清理，
不要再读取失效 CUA binding。新包实际最小化与完整全屏往返仍未通过，
详情见 [本批证据](../../docs/tauri/evidence/2026-10-02-d-interactive-window/README.md)。

这是逐步替换 Wails shell 的 Tauri 2 host；它暂不替换稳定的 Go Agent、会话格式或
现有前端 API。开发构建启动时 host 从 `REASONIX_DESKTOP_BRIDGE_BIN` 读取由构建流程提供的
`reasonix-desktop-bridge` 可执行文件路径；release 打包启动时忽略该开发覆盖，由 Tauri 的 `externalBin` 从应用
包内定位同一个 bridge。两种路径都通过每次启动独有的 token、ready 文件和 launch nonce
监管它。token 经子进程 stdin 管道发送，bridge 读完后关闭管道并将标准输入切到空设备；
token 不放在 macOS 进程列表可见的启动环境或命令行中。

## 当前平台范围

当前安装包的实际配置备份回退可用 `python3 tools/tauri/smoke-backup-rollback.py '<installed-app>' --legacy-cli '<official-1.38.3-app>/Contents/MacOS/reasonix' --output /private/tmp/new-backup-evidence` 验收（Python 3.11+）。只接受官方内嵌 CLI 固定摘要；备份应用到独立私有目录，验证旧 CLI 回读、原配置/备份保持及正常退出。失败保留私有夹具、成功清理，不修改用户档案，也不代替完整资料或物理 GUI 回退。

安装包 D 门禁可用 `python3 -B tools/tauri/verify-installed-d.py '/path/to/Reasonix Tauri Preview.app' --output /private/tmp/new-evidence` 从仓库根串行复验。
可用 `--gates dialog-cancel package` 独立复验原生面板取消与正常启动退出；该切片包含菜单/文本剪贴板前置检查，不绕过或替代完整窗口门禁。摘要计算兼容系统 Python 3.9。
未跟踪的 desktop/internal/cmd 代码也会单独保存快照与摘要；只保存代码扩展名，不复制环境文件、私有档案或证据目录。
托盘语言可用 `--gates tray-language package` 独立复验；完整窗口门禁也包含菜单项语言与配置不变检查，不代替真实托盘弹出/点击。
输出目录必须全新；脚本记录候选摘要、签名、验收源码和逐项结果，遇到失败停止，后续项保持未运行。
程序化验收不代表物理交互或 D/E 整组完成；当前同包复验与剩余范围见
[2026-10-02 启动恢复证据](../../docs/tauri/evidence/2026-10-02-d-startup-refusal/README.md)。
新增 `identity` 门禁覆盖托管/显式档案的身份元数据拒绝与备份恢复；本候选完整窗口
仍在 Settings 最小化前提失败，不能宣称 D 已稳定或已验收。
随后增加 opt-in 原生呈现诊断；最新诊断包的原 API 与独立 `--presented` 探针仍未通过，
正常 package smoke 通过。具体时序和不同实验范围见
[窗口诊断证据](../../docs/tauri/evidence/2026-10-02-d-minimize-presentation/README.md)，不代替原完整窗口门禁。
独立 `--direct-only` 诊断随后确认实际主线程直接 NSWindow 调用仍失败，而最小 Cocoa
程序通过；不能只归因于 Tauri 调度。当前包摘要和精确范围见
[直接调用证据](../../docs/tauri/evidence/2026-10-02-d-direct-minimize/README.md)。
本轮另收敛遗留 Wails 几何/剪贴板直连，并取消 hook 卸载后尚未提交的几何观察。
最新包独立菜单/剪贴板 4/4 与正常双档案 smoke 通过；完整窗口失败仍保留。
具体回归、未提交适配器快照和取消边界见
[runtime 生命周期证据](../../docs/tauri/evidence/2026-10-02-d-runtime-lifecycle/README.md)。

2026-09-30 用户确认本轮只推进 macOS 的改造、测试和发布候选验收。Windows/Linux
因缺少实际测试环境延期；已有代码保留，但不承诺平台支持，也不要求其原生验收完成
后才能推进 macOS。D→E 的顺序及 macOS 待验项以 [迁移清单](../../docs/tauri/API_SURFACE_AUDIT.md) 为准。

## 数据隔离

Preview 使用 Tauri 应用数据目录下私有的 `REASONIX_HOME`。因此它不会静默读取、迁移或
写入稳定 Wails 客户端的配置、会话和缓存；稳定版可与它并存。导入稳定版数据会作为单独的
“先备份、再确认”的功能实现。开发者显式传入的 `REASONIX_HOME` 仍是有意识的覆盖选择。

默认 `reasonix-core` 叶目录只接受普通目录；符号链接、悬空链接或同名文件会在创建
WebView/sidecar 之前拒绝，输出恢复说明并以退出码 1 结束，避免链接到旧 Wails 档案后
静默改写原件。新建目录为 0700。显式根仍遵循既有覆盖选择，不能据此宣称所有旧版
共享目录已互斥。真实 DMG 安装拒绝、正常启动/导入与 12 项启动中断证据见
[本轮目录边界验收](../../docs/tauri/evidence/2026-10-02-d-managed-root/README.md)。

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

定位某一种档案时可追加 `--profile managed` 或 `--profile explicit`；默认仍为
`--profile both`。单档案通过不能代替两种档案完整通过。每个阶段立即输出结果，
全部成功后删除该档案；失败仍返回非零，并在清理自有进程后保留 0700 私有目录及
原生结果/trace，输出现场路径和已完成阶段数。保留的档案包含测试凭据，只用于本地诊断，
不要作为公开附件。这些选项不改变窗口动作、超时、重试或成功断言。

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

普通档案的存储路径复制 UI 可独立验收：

```bash
python3 -B tools/tauri/smoke-native-ui-clipboard.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --seconds 600
```

runner 依次启动 managed/explicit 两个私有普通宿主，不启用原生窗口探针；控制文件
`/private/tmp/reasonix-ui-clipboard-control.json` 给出自有 PID、预期配置路径及只读 claim
命令。确认进程仍存活后绑定实际应用，进入设置→存储与路径，核对空工作区复制禁用，
实际点击配置目录复制按钮。立即执行该阶段的 claim 命令核对系统精确文本与代次，再
核对“已复制”反馈；返回空输入框，用实际 Cmd+V 验证完整路径，最后 Cmd+Q。不要发送
消息，也不要用 CUA paste 替代系统粘贴。退出后不可查询已退出的应用绑定；下一档案需
核对新 PID 并重新绑定。runner 的系统/生命周期通过不能代替实际 UI 证据。

系统复制前完整备份有界多格式剪贴板；路径来源限制在当前用户拥有的 nonce 私有档案，
拒绝符号链接、外部路径和公开控制文件。已展示路径的恢复必须同时匹配精确值、nonce
和独立登记的 changeCount；重新复制同样文字也视为外部变化，保留当前内容并报失败。
保护规则先在私有命名 pasteboard 自测。正常退出后原剪贴板每项/类型/顺序/字节均须恢复
并独立验证；成功夹具自动删除，失败夹具保留且清理自有进程，日志不输出原剪贴板。
`ada308408` 同一生产包两种档案的按钮、禁用状态、成功反馈及实际 Cmd+V 均已由 CUA
验证，两次正常退出、身份/原件/清理及原剪贴板恢复通过；日志
`/private/tmp/reasonix-path-clipboard-ui.log`。其他路径、消息、上下文菜单、实际失败反馈和
输入框键盘/右键完整编辑另见下方最新验收；其他路径与实际失败反馈仍待完成，窗口稳定性未因此变为通过。

实际产品输入框的键盘和 WebKit 原生右键编辑可用同一 runner 的 edit 场景：

```bash
python3 -B tools/tauri/smoke-native-ui-clipboard.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --scenario edit --profile both --seconds 600
# 每次实际 Copy/Cut 前记录代次；随后执行真实按键或菜单点击。
python3 -B tools/tauri/smoke-native-ui-clipboard.py --record checkpoint
# 实际 Copy/Cut 后必须核对精确值和严格新代次；检查点成功后消费。
python3 -B tools/tauri/smoke-native-ui-clipboard.py --record claim
# 实际 Paste 后核对系统内容及代次未改写，并另行核对产品输入框完整值。
python3 -B tools/tauri/smoke-native-ui-clipboard.py --record verify
```

用控制文件的 expectedText 在真实空输入框准备测试文字，确认实际完整值后分别执行
Cmd+A/C/X/V 和右键 Copy/Cut/Paste；每次 Copy/Cut 都需 checkpoint→真实操作→claim，
不能因为旧剪贴板已有同样文字而通过。`--record` 校验私有文件/目录、固定命令和进程
存活，只写私有代次回执，既不启动应用，也不写系统剪贴板。非默认 `--control` 必须同样
传给每次 record。`--profile managed|explicit|both` 可单独推进未完成档案，输出实际完成数。
源文字含中文、空格和 emoji；必要时用 CUA 输入框 setValue 准备，不以此证明输入法或
自动键入 Unicode。剪切后要求空值/发送禁用，空菜单 Copy/Cut 禁用、Paste 可用；粘贴后
核对完整文字。最后实际全选删除清空草稿，再 Cmd+Q，不发送消息。

`ada308408` 包两种普通档案已由 CUA 完成这些实际键盘/菜单动作，全部八次 Copy/Cut
的新代次/精确值、四次 Paste 的实际恢复/系统值代次保持通过；两次正常退出、身份/原件/
sidecar/readiness 清理及原剪贴板完整恢复通过。日志为
`/private/tmp/reasonix-composer-clipboard-explicit-ui.log`、
`/private/tmp/reasonix-composer-clipboard-managed-ui.log`。成功夹具删除。
首次托管观察失败的 `-3812` 及非正常退出不计通过，夹具和残留 bridge 目录保留；原剪贴板
未改写且自有进程已退出。失败 finally 现先等待 parent-loss watcher 最多五秒，再回收自有
残留 sidecar；该新增故障清理分支尚未实际故障验收。消息/其他路径/实际失败反馈、IME/
组合输入和窗口稳定性继续保留。
本地 ad-hoc 签名不等于正式签名/公证；外接屏延期和其他发布门禁见迁移清单最新记录。

普通产品的诊断导出 Save/Cancel/Replace 可独立验收：

```bash
python3 -B tools/tauri/smoke-native-ui-export.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --profile both --seconds 600
# 每步先完成真实面板/页面操作，再登记实际输出检查。
python3 -B tools/tauri/smoke-native-ui-export.py --record cancel
python3 -B tools/tauri/smoke-native-ui-export.py --record new-save
python3 -B tools/tauri/smoke-native-ui-export.py --record overwrite-cancel
python3 -B tools/tauri/smoke-native-ui-export.py --record overwrite
```

控制文件 `/private/tmp/reasonix-ui-export-control.json` 给出当前自有 PID、phase 和 outputFile。
确认进程存活再绑定应用，进入设置→诊断，实际启用记录、添加标记并停止导出。第一次
取消真实 Save 面板，确认页面仍“待导出”；再次导出同一报告到 outputFile，确认保存后
页面恢复未记录。然后开始另一份记录、添加标记、导出同名文件，取消真实 Replace 提示，
再取消返回的 Save 面板；确认保留待导出报告。最后再次导出该报告、接受 Replace 并
确认未记录状态，再实际 Cmd+Q。每步严格依序 record；不能只写回执就当面板通过。
路径/名称含中文和空格，使用实际 Go To Folder/Save As 字段；换档案须核对路径，避免
沿用系统面板记住的上一档案目录。`--profile managed|explicit|both` 默认 both；非默认
`--control` 必须位于 `/private/tmp` 且在启动前不存在，并同样传给所有 record。

runner 仅接受当前用户 nonce 私有普通 0700 目录和有界普通 0600 控制/回执/报告，拒绝
符号链接、外部目标、跳步和失效进程。取消不得生成文件；新保存核对 schemaVersion=2、
报告身份和实际 marker，并私有备份；取消覆盖要求原报告字节/权限/mtime_ns/inode 完全
不变，接受覆盖要求第二报告 ID/内容不同且无额外输出。退出后核对原件/档案身份、正常
退出码和 sidecar/readiness 清理，成功夹具删除。命令不会操作 UI，也不读写系统剪贴板。
`ada308408` 同一生产包已由 CUA 完成两种普通档案的实际面板和状态验收，共两次正常
生命周期/八项文件检查通过；日志 `/private/tmp/reasonix-diagnostic-export-ui.log`，成功
目录/控制及四个自有进程均独立确认清理。Python AST 和 10 项有界拒绝检查通过；新增
故障清理分支未另外注入。本地文档/主题另存为、系统实际写入拒绝、其他 D 和窗口稳定性
待办继续保留；该项不代表正式发布签名/公证通过。

主题导出可用同一普通 runner 的 `--scenario theme`，默认仍为 diagnostics：

```bash
python3 -B tools/tauri/smoke-native-ui-export.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --scenario theme --profile both --seconds 600
# 非默认控制文件必须传给每一次 record；失败控制不会被下次启动覆盖。
```

实际进入设置→外观→浏览主题，创建基于石墨、无图片的 `Reasonix UI Theme`，不要应用或
删除主题。依次实际取消/新保存，编辑同一主题更名 `Reasonix UI Theme Revised`，再执行
取消覆盖/确认覆盖。四项 record 命令与 diagnostics 相同，从当前私有控制读取场景，
不接受另传场景绕过断言。主题目标为当前私有目录的 `主题 副本.reasonix-theme`；ZIP 必须
只有有界 theme.json，schemaVersion=2，ID/名称/明暗颜色/密度/圆角匹配固定 UI 样本。

`ada308408` 托管 UI 在设置点击后捕捉失败；显式实际创建/编辑及 cancel/new-save 两项
文件检查完成，但覆盖取消后捕捉失败，未执行确认覆盖，完整生命周期不计通过。失败
日志 `/private/tmp/reasonix-theme-export-ui.log`、
`/private/tmp/reasonix-theme-export-explicit-ui.log`，两份私有夹具/控制/原包保留；自有进程
退出、readiness 目录清理通过。主题覆盖/导入/图片及重启验收继续保留。

实际 UI 显示旗舰主题 0 后，新增 `pnpm test:tauri-theme-catalog` 复现并修复 glob 的运行时
判断错误。该门禁执行实际 Vite client 构建产物，核对所有八套主题、16 张资产字节、选中
状态及安全 URL 登记；直接 Node 组件测试不能代替这项。原生包重建和修复后 UI 证据
见迁移清单；D/E 与正式发布门禁保持。
新包已从干净 `de8afeeeccbffa0160835511a56a2538fb7f8f56` 完整构建，生产检查、bundle
budget、sidecar/arm64 包和本地严格签名核对通过；日志
`/private/tmp/reasonix-official-theme-package-build.log`。两种普通档案的实际画廊均显示
八套旗舰主题及图片，托管实际点击赤曜新城卡片后的详情/背景预览也通过。两种档案
实际创建/编辑无图片主题及 Save/Cancel/Replace 全流程完成，共两次正常生命周期/
八项文件检查通过；日志 `/private/tmp/reasonix-theme-fixed-ui.log`。成功 nonce 目录/控制
和四个自有进程已独立核对清理，原件及身份保持。旧包捕捉失败仍单独保留，不以此推断
捕捉错误根因已解决。实际主题导入/图片/应用重启、文档另存为和窗口稳定性仍待验收。

本地文件另存为也可使用同一普通 runner：

```bash
python3 -B tools/tauri/smoke-native-ui-export.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --scenario document --profile both --seconds 900 --control /private/tmp/reasonix-document-save-ui-control.json
```

读取当前 0600 控制里的固定 `prompt`，通过真实空输入框发送。唯一 provider 是本轮
`127.0.0.1` 服务，固定回复 Original A/B 两个私有文件链接；不使用外部模型或用户凭据。
实际右键 Original A→另存为，先 Cancel 并 `--record cancel`，再到 `outputFile` 目录
新保存并 `--record new-save`。改用 Original B，选择同名目标取消 Replace，再取消保存
面板，执行 `--record overwrite-cancel`；再次保存并确认 Replace，执行 `--record overwrite`。
所有 record 都须传相同 `--control`，最后实际 Cmd+Q；第二档案重复相同流程。
源目录/输出名包含中文和空格；两份原件的字节/权限/mtime/inode 始终保护，输出必须依次
为 A/B，取消覆盖保持已有副本的全部元数据。成功清理自有进程、服务和私有档案，失败
保留现场并返回非零。该命令管理夹具，不能替代实际面板和成功/取消反馈验收。
2026-10-02 当前 `851111594` 普通包两种档案已完成上述真实 UI，共两次正常退出/八项
文件检查，日志 `/private/tmp/reasonix-document-save-ui.log`。同文件/实际写入拒绝反馈和
Finder/editor 等操作另行验收，既有 theme/diagnostics 默认行为保持。

另存为的源文件错误切片：

```bash
python3 -B tools/tauri/smoke-native-ui-export.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --scenario document-errors --profile both --seconds 900 --control /private/tmp/reasonix-document-errors-ui-control.json
```

发送同一固定提示，实际右键 Original A→另存为，保留源目录/源文件名，实际 Save→Replace，
要求应用拒绝且显示源目标相同的解决步骤，执行 `--record same-source`。实际右键
Missing source→另存为，要求即时源文件不可用提示且没有 Save 面板，执行
`--record missing-source`，再实际 Cmd+Q；record 必须指定同一 `--control`。
每阶段要求两份源指纹不变、源目录无新增文件、输出为空；不会将“没有输出”单独认定为
错误提示/面板验收。两档案共四项拒绝，两次正常生命周期。`851111594` 的原生保护与旧文案已实际验证；
从干净 `5882a249d` 完整生产构建的新包，两种档案四项真实拒绝及简体中文解决步骤也已
通过，日志 `/private/tmp/reasonix-document-errors-fixed-ui.log`。原件/身份/退出清理保持，
成功夹具及自有进程已独立核对清理；目录/真实系统读取或写入拒绝另行验收。

真实源读取权限拒绝有独立切片：

```bash
python3 -B tools/tauri/smoke-native-ui-export.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --scenario document-permissions --profile both --seconds 600 --control /private/tmp/reasonix-document-permissions-ui-control.json
```

runner 将 Original A 设为 mode-000，启动前要求当前用户路径打开实际 PermissionError，
能绕过权限时拒绝验收。唯一固定回环 provider 仍只生成文件链接。实际发送固定提示，
右键 Original A→另存为，要求源不可读取提示、无保存面板，然后
`--record read-denied --control /private/tmp/reasonix-document-permissions-ui-control.json`，
实际 Cmd+Q；第二档案重复。记录时核对不可读原件的 mode/inode/mtime/size、第二原件
完整指纹和零输出；退出后通过只留在 runner 的原只读描述符确认第一原件精确字节。
不会恢复文件权限或把描述符交给应用。`5882a249d` 普通包两种档案已通过两次实际读取
拒绝与正常退出，日志 `/private/tmp/reasonix-document-permissions-ui.log`。写入拒绝等
剩余范围另行验收，默认正常导出及其他错误切片阶段不变。

文档目的地写入拒绝使用 `smoke-native-ui-export.py --scenario document-write-denied`。
该切片的源 A/B 可读，目标为 0500 目录；启动前实际创建必须得到 PermissionError。
发送控制中的固定提示，实际 Original A→另存为→Go To 指定输出目录→Save，观察
写入拒绝提示后 `--record write-denied`，Cmd+Q。两种档案各一阶段，要求目标权限、
mtime/inode 保持、零输出和源完整指纹不变；正常退出验收后才恢复私有目录权限以清理。
基线 `5882a249d` 两种档案均通过，日志 `/private/tmp/reasonix-write-denied-ui.log`；
`30c311130` 新三语提示的完整生产构建和双档案包级 smoke 通过；当前实际托管首次
绑定连续 `-3811`，重置采集会话仍失败，新提示原生复验未完成。只终止自有进程组并
保留现场/控制和日志 `/private/tmp/reasonix-write-denied-fixed-ui.log`。无 UI 回执/输出，
两个 PID 已退出，但组信号包含 sidecar，失败现场保留 readiness 文件；这不是正常退出
或应用自行崩溃的证据，不以组件回归或包级 smoke 代替提示验收。
当前包追加单独宿主异常退出门禁 8/8 通过，覆盖两种档案、SIGTERM/SIGKILL、空闲/
真实流式任务，sidecar 正常退出与 readiness 清理、同档案重启通过；日志
`/private/tmp/reasonix-write-denied-host-lifetime.log`。

主题导入的普通私有档案 runner：

```bash
python3 -B tools/tauri/smoke-native-ui-theme-import.py 'desktop/tauri/target/release/bundle/macos/Reasonix Tauri Preview.app' --profile both --seconds 900 --control /private/tmp/reasonix-theme-import-new-control.json
python3 -B tools/tauri/smoke-native-ui-theme-import.py --record cancel --control /private/tmp/reasonix-theme-import-new-control.json
# 实际操作后按顺序 record import、duplicate、invalid；Cmd+Q 后同档案自动重启。
# 实际重新进入画廊核对两份主题及首页/工作区图片，再 record restart，Cmd+Q。
# 每次 record 均传同一 --control；失败控制和夹具不覆盖。
```

实际 Import 先取消，再选择当前控制的 sourceFile，重复导入该包，再导入 invalidFile
并观察错误。两份主题名均为 `Imported UI Theme`，ID 分别为 `user-ui-import` 和
`user-ui-import-2`；首页/工作区使用不同的现有官方 WebP。不要应用/删除主题。
runner 只核对有界磁盘证据和私有宿主生命周期，图片显示及错误提示必须单独观察。
完整流程为两种档案各导入与重启，共四次正常生命周期。

首轮 `de8afeeec` 托管实际导入成功但两张预览不显示，未通过。已修正重复 bundle
identifier 的 asset scope，并增加当前 Tauri FsScope 的允许/拒绝回归；完整包 UI 另验。
现有偏好 `0644` 可在私有 HOME 中接受，权限、原件及无效导入/重启不变检查保持。

干净 `9972828f4` 已完整重建；托管实际 Cancel/双图片导入/重复导入/无效包/重启五项
文件检查及两次正常生命周期通过，重启后两张卡片分别显示正确首页/工作区图。
显式仅实际 cancel 通过，Go To Folder 返回后捕捉连续 `-3812`，未点击 Open；同宿主
重新绑定失败后只终止自有宿主，整体 runner exit 1。日志
`/private/tmp/reasonix-theme-import-fixed-ui.log`，失败夹具/控制保留。六个自有进程及
readiness 均独立确认已清理；显式完整验收及窗口稳定性仍未通过。

`851111594` 中英文主题导入失败提示已增加处理步骤，完整 Tauri 前端回归和干净
macOS 包构建通过。新托管实际取消通过，第二次路径面板捕捉再次 `-3812`，新文案
原生 UI 尚未验收；失败日志 `/private/tmp/reasonix-theme-import-feedback-ui.log`，原件
及进程/readiness 清理已独立核对。另用 IAB 渲染当前真实设置组件和产品样式，中英
提示、失败后取消清除、成功重试恢复及 900×620 长文案可读性通过；IPC 为固定样本，
仅计组件 QA。证据 `/private/tmp/reasonix-theme-feedback-browser-9fg8wcnz/qa-evidence.json`。


2026-10-02 当前干净 `5882a249d` 生产包已用新私有显式档案补齐上述五阶段及两次正常
UI 生命周期：实际 Cancel/双图片导入/重复导入/无效包提示/重启；两份主题逐张选择并
切换首页/工作区图片均恢复，偏好、原件、身份及退出清理通过。日志
`/private/tmp/reasonix-theme-explicit-oct02.log`；重启截图
`/private/tmp/reasonix-theme-explicit-restart-second-home.png` 与
`/private/tmp/reasonix-theme-explicit-restart-second-task.png`。四个自有 PID 已独立确认退出，
成功夹具/控制删除。重复导入后的两次 `-3812` 观察由同宿主只读截图/AX 恢复，未重复
执行导入；显式导入待验项关闭，当前中文失败提示原生验收通过。托管仍引用此前
`9972828f4` 成功记录，历史失败保留；捕捉根因、窗口稳定性与主题应用仍待验。


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


2026-10-02 配置备份目录保护：`backups` 若为符号链接或非目录，导入拒绝；Unix
新建备份根/批次用 0700，配置与备份保持 0600。修复前重定向回归失败，修复后
配置档案 13 项及严格 clippy 通过；从干净 `05cf57e1d` 完整构建后，真实包
`smoke-profile-import.py` 的 import/restore/explicit 三阶段通过，包括新目录权限、
稳定原件/备份保持和退出清理。日志 `/private/tmp/reasonix-backup-root-package-import.log`。
这不等同旧官方 Wails 已遵守新目录锁，也不代替设置页物理导入或已知截图故障的修复。


2026-10-02 Hooks JSON 剪贴板：Copy/Paste 使用共享原生文本入口，不再直接依赖
`navigator.clipboard`。复制等待实际写入后显示成功；读取拒绝/空剪贴板保持草稿，
异步旧 workspace/scope 结果不覆盖当前编辑器；不保存配置或执行 hooks。编辑器使用
新增严格读取入口，其他调用者的空串读取契约保持。新组件回归已纳入 `pnpm test:tauri`，
并覆盖系统调用样本、拒绝/空值和上下文变化。真实 Hooks 按钮点击与完整系统剪贴板
恢复另验，组件回归不代替 macOS 物理 UI；现有文本 ACL 与回退策略不变。
生产代码 `912c87a4e` 的完整 macOS 构建及双档案独立菜单/剪贴板门禁 4/4 通过，
实际文本 IPC、权限拒绝、系统剪贴板原件恢复及退出清理通过；日志
`/private/tmp/reasonix-hooks-clipboard-native.log`。不扩大为 Hooks 物理点击或完整窗口稳定性。


2026-10-02 剪贴板拒绝边界：在 Tauri 中，原生文本写入拒绝返回 false，严格读取
传播拒绝，兼容读取仍为空串；原生拒绝/native busy 均不尝试浏览器、Wails 或
execCommand。可通过用户重试恢复；Wails/browser 模式的原有回退保持。
`pnpm test:clipboard` 覆盖可成功替代 transport 的零调用和原有模式兼容，完整
`pnpm test:tauri` 通过；设置页测试夹具已明确响应原生插件命令，原路径断言不降。
当前 `7a83505ce` 生产包双档案独立菜单/clipboard-native 4/4 通过，权限拒绝及
系统剪贴板原件恢复、退出清理通过；日志 `/private/tmp/reasonix-clipboard-boundary-native.log`。
原生 ACL 与 helper 拒绝后零回退分别由真实包/组件回归证明，不当作物理失败 UI 验收。

2026-10-02 同一 `7a83505ce` 生产包完整窗口门禁 `--dialogs --edit --focus`
46/46 通过（两种档案各 23 项），日志 `/private/tmp/reasonix-window-boundary-current.log`。
包含此前易失败的最小化恢复，但历史偶发根因仍未确定，稳定性与正式发布保持待验。
随后保存写入拒绝提示的普通 UI 补验首次绑定仍遇 ScreenCaptureKit `-3811`，未输入或
执行保存，不能计 UI 通过。只终止自有宿主，确认 host/sidecar 退出、ready 清理、
原件字节及元数据保持、零输出；失败现场和日志
`/private/tmp/reasonix-write-denied-after-window-ui.log` 保留。该测试主动停止不是正常
Cmd+Q 验收或应用自行崩溃；物理 UI 验收仍需恢复可靠的观察能力。

2026-10-02 搜索来源：链接点击（含 Command/中键）使用原生外部链接封装，禁止
WebView 默认导航；复制使用原生文本封装，显示实际成败，打开失败提供复制恢复。
不增加权限，原生拒绝不走浏览器回退，来源过滤/去重保持。真实组件回归修复前失败、
修复后通过，已纳入 `test:external-links`；完整外链回归与生产构建通过。
两种私有档案的新包原生链接拒绝与默认浏览器本机回执通过，日志
`/private/tmp/reasonix-search-sources-native-links.log`。这不代替消息中来源物理点击
验收；ScreenCaptureKit 观察问题及其余 D/E、正式发布门禁继续保留。

2026-10-02 诊断报告复制：实际 Tauri 设置消费者改用共享原生文本入口，成功反馈
等待实际完成；拒绝清除旧成功并提示重试，复制错误不覆盖报告加载错误。刷新/切换
runtime 选项使旧复制失效，pending 禁止重复操作，卸载清理计时器和请求。
新增真实组件回归、原诊断测试及完整 `test:tauri` 通过；产品 `81b229d4a` 完整
生产构建和双档案独立菜单/剪贴板 4/4 通过，权限拒绝、系统原件恢复及退出清理通过，
日志 `/private/tmp/reasonix-diagnostics-native-clipboard.log`。没有增加 capability 或
自动读取；此证据不代替真实诊断设置页点击、其他 D/E 或正式发布验收。

2026-10-02 历史目录互斥负向探测：`probe-legacy-profile-gate.py` 核验固定官方
1.38.3 CLI 摘要，在生成的私有 config/state 目录持有两处当前排他锁，独立确认
第二个参与者无法取锁。旧 CLI 仍实际把 CNY 配置改成 USD，探测 exit 1，日志
`/private/tmp/reasonix-legacy-profile-gate-negative.log`；这是明确失败证据，不是验收
通过，也不证明旧 Wails GUI 的全部写入路径。默认 Preview 独立目录保持，不能把
当前 Wails/bridge 的新锁推广为历史二进制已参与；生产包与历史回退验收范围保持。

2026-10-02 Mermaid 外部链接：修复右键 auxclick 误打开，仅左键/中键打开；原生
拒绝给出三语失败反馈和复制链接恢复，复制使用共享文本入口，拒绝不走浏览器回退。
反馈更新保持原 SVG DOM，不重置缩放状态。新增真实组件回归在修复前失败，修复后
通过；原 Mermaid 渲染 105/105 和完整外链回归通过，已纳入 `test:external-links`。
产品 `1b6a7706d` 完整生产构建、双档案实际原生浏览器回执/协议拒绝通过，日志
`/private/tmp/reasonix-mermaid-native-links.log`。这不代替消息中图表的物理点击或
完整邮件/OAuth、D/E 和正式发布验收。
同包双档案独立菜单/剪贴板 4/4 通过，最小权限拒绝、原剪贴板恢复和退出清理通过，
日志 `/private/tmp/reasonix-mermaid-native-clipboard.log`，独立确认无 Preview 残留。

2026-10-02 当前 `1b6a7706d` 包完整窗口门禁 `--dialogs --edit --focus` 46/46
通过（两档案各 23 项），日志 `/private/tmp/reasonix-mermaid-full-window.log`，独立
确认无 Preview 残留。历史最小化偶发根因仍未知。新私有托管档案的保存写入拒绝 UI
补验首次绑定/同宿主只读重绑仍报 ScreenCaptureKit `-3811`，未输入或保存；只停止
确认自有宿主，sidecar/ready 清理、原件和只读目标目录保护通过。失败现场和日志
`/private/tmp/reasonix-write-denied-mermaid-ui.log` 保留，不计正常 Cmd+Q 或 UI 通过。

2026-10-02 凭据恢复：单个 Provider 的读取/同步失败不再阻断其他有效条目恢复，
部分恢复返回固定重试指引，不返回名称、密钥或底层诊断。全局身份损坏保留恢复
元数据备份的原指引，恢复函数不访问 sidecar；原生存储及保存/删除/迁移回滚保持。
修复前真实 sidecar 回归已复现后续有效 Provider 未恢复；最终相关回归 22 通过、
1 原生 OS 测试默认忽略，覆盖部分重启、后续恢复和档案不落盘密钥，严格 clippy
通过。最终产品 `ebae594f4` 完整生产构建、双档案包级启动/身份/权限/退出通过，
日志 `/private/tmp/reasonix-keychain-partial-final-package-smoke.log`，独立确认无残留。
故障样本不等同真实 OS 锁定/拒绝或迁移 UI 验收，D/E 和正式发布门禁继续保留。

2026-10-02 通知点击竞态：在途 pending 查询返回旧空快照时，不再丢弃期间收到的
真实点击唤醒。新增确定性回归修复前失败、修复后通过；存储失败不循环重试，原单
消费者、卸载 fence、导航失败保留点击及持久 ack 规则保持。完整 test:tauri 通过，
日志 `/private/tmp/reasonix-notification-wake-tauri-tests.log`。产品 `2e4837c26` 完整
生产包构建和托管/显式双档案基础 smoke 通过，日志
`/private/tmp/reasonix-notification-wake-build.log`、
`/private/tmp/reasonix-notification-wake-package-smoke.log`；独立确认无 Preview 残留。
这不等同真实通知横幅、点击、冷启动和 OS 拒绝恢复通过；旧版互斥、其余 D/E 与
正式签名/公证门禁继续保留。

2026-10-02 组合输入快捷键保护：产品 `c07a78ac3` 将输入框已有的 isComposing/229
检查共用到 Tauri 匹配、全局 Escape 和设置录制入口。匹配回归修复前失败；完整
test:tauri 修复后通过，覆盖 43 个动作两种 IME 标记、设置录制不改绑定和工作区
面板保持，日志 `/private/tmp/reasonix-shortcut-ime-tauri-tests.log`。同提交完整生产
构建及双档案基础 smoke 通过，日志 `/private/tmp/reasonix-shortcut-ime-build.log`、
`/private/tmp/reasonix-shortcut-ime-package-smoke.log`；独立确认无 Preview 残留。
CUA 只读 inventory 查询仍超时，未输入；真实 macOS 候选输入/取消和物理自定义
快捷键验收继续保留，不以组件或基础启动检查替代。其余 D/E、旧版目录互斥和正式
发布门禁保持。

2026-10-02 消息复制反馈：产品 `79e3adac6` 的共享 CopyButton 不再保留失败前的
成功状态；原生拒绝提供三语重试/手动复制提示，待处理双击合并，来源变化/卸载
隔离旧生成与写入结果。clipboard 修复前回归失败，修复后及完整 test:tauri 通过，
日志 `/private/tmp/reasonix-copy-button-after.log`、
`/private/tmp/reasonix-copy-button-tauri-tests.log`。同提交完整 macOS 构建及双档案
独立菜单/剪贴板 4/4 通过，日志 `/private/tmp/reasonix-copy-button-build.log`、
`/private/tmp/reasonix-copy-button-native-clipboard.log`；原剪贴板恢复且无 Preview
残留。这不替代物理消息按钮/实际 OS 拒绝反馈或完整窗口验收，D/E 及发布门禁保留。

2026-10-02 sidecar 退出回执：产品 `c0e3871c5` 不再把 shell Error 或无回执通道
关闭当成进程退出，保留实际子进程状态，只有 Terminated 清零。Error 可来自管道
读取失败已按锁定 shell 2.3.6 源码核对；事件序列修复前 1 失败、1 通过，修复后
3/3 通过及严格 clippy 通过。完整 macOS 构建和双档案正常退出通过，日志
`/private/tmp/reasonix-sidecar-terminal-build.log`、
`/private/tmp/reasonix-sidecar-terminal-package-smoke.log`。当前包宿主 TERM/KILL、
空闲/流式双档案 8/8 通过，kernel 八次 sidecar exitCode 0、锁/ready 清理及同档案
重启通过，日志 `/private/tmp/reasonix-sidecar-terminal-host-lifetime.log`；无 Preview
残留。事件回归不是实际 OS 管道故障注入，不代替物理菜单/托盘或完整窗口验收；
D/E、历史旧版互斥和正式发布门禁继续保留。

2026-10-02 当前产品 `c0e3871c5` 完整窗口门禁为 19/46，在托管原生编辑前提
停止（firstResponderAccepted true，但 applicationActive/keyWindow false），显式
档案未开始，日志 `/private/tmp/reasonix-sidecar-terminal-full-window.log`。
只读诊断 `ad0d62200` clippy、固定字段 parser 和完整生产构建通过；新托管独立
编辑切片 2/3，再次焦点失败，日志 `/private/tmp/reasonix-edit-focus-trace-probe.log`。
实际启动/page 已完成、可 key 且在活动 Space，无 modal/sheet/live resize；恢复
请求/完成 1/1，但无 became-key，应用/窗口 occlusion-visible false。根因未确定，
没有改激活动作、等待或断言，不把切片或历史 46/46 替代当前完整通过。两次原
剪贴板恢复且无宿主/sidecar/启动记录残留，失败现场保留。真实 UI、历史旧版互斥、
其余 D/E 和正式发布门禁保持。

2026-10-02 LaunchServices 激活对照：新增独立 `probe-edit-launch-services.py`，
同一诊断包由 open -n -W 启动，显式生成私有 HOME/config/state/cache；核对私有
launch-owner、kernel 父子归属、身份、鉴权拒绝与空 token。首次 state 验证前提
不匹配停止，不计产品结果；改为核对本探测的显式私有 state，未改正常 smoke。
最终实际桌面启动仍原生编辑焦点失败，日志
`/private/tmp/reasonix-edit-launch-services-final-probe.log`；page/启动完成、可 key/
活动 Space、无模态/sheet，恢复 1/1、无 became-key，应用激活/key window/occlusion
均 false。缺少直接启动的桌面上下文不能解释全部失败，根因仍未知。原剪贴板恢复
且无进程/启动记录残留，失败现场保留。open 等待器不是宿主退出码；当前完整窗口
19/46、物理 UI、其余 D/E 及正式发布门禁保持，不将诊断当作验收通过。


2026-10-02 外接屏恢复：原生 API 已识别两块 2× 屏幕（主屏工作区
0,60,3840,1966；副屏 -3840,60,3840,2100）。`b96367355` 的独立
LaunchServices 编辑探测通过，运行期策略 Regular=0、应用/key window 已激活；
完整窗口复测仍在托管第 9 项设置最小化前提失败，前 8 项通过，显式未开始。
日志 `/private/tmp/reasonix-monitor-policy-launch-probe.log` 与
`/private/tmp/reasonix-external-display-full-window.log`；历史失败根因未据此证明。
新增真实副屏定位/重启验收：
`python3 -B tools/tauri/smoke-native-displays.py <macOS app>`。
只在 opt-in 验收模式操作私有窗口，两种档案核对实际位置/尺寸/缩放、生产保存文件、
整包重启及身份/退出清理；`f51979947` 严格 clippy 和完整构建通过，真实包
4/4 通过，日志 `/private/tmp/reasonix-secondary-display-package-smoke.log`。
不同缩放、实际拔插、物理拖动/WebView 和完整窗口稳定性仍待验，观察工具仍超时，
不将 API 几何通过标为物理 UI 全通过；其余 D/E/发布门禁及 Windows/Linux 延期保持。


2026-10-02 最小化前提对照：`f43825cc9` 为 Settings 两种最小化路径增加真实活动
应用/key window 等待，沿用 5 秒上限和原生事件断言；trace 固定最多 5 行/16 KiB。
严格 clippy、字段边界检查和完整构建通过，但完整窗口仍托管 8 项后失败，显式未开始，
日志 `/private/tmp/reasonix-settings-key-precondition-full-window.log`。新快照证明请求前
确为活动 key window；独立真实 Minimize 菜单也最小化失败，日志
`/private/tmp/reasonix-settings-native-role-probe.log`。等待 key 并未修复问题，失败不限于
Tauri minimize API。两份现场无自有进程/启动记录残留；未据此认定系统或产品根因，
已请求其他应用的最小化对照，完整 D/E 和发布门禁保持。


2026-10-02 页面完成后对照：`63984aca9` Settings 验收统一等待可信主页面
PageLoad::Finished，沿用 5 秒上限和活动 key/真实最小化断言。clippy 与完整构建通过；
独立私有最小化切片仍失败，日志 `/private/tmp/reasonix-settings-page-ready-probe.log`。
请求前页面完成/活动/key 均 true，仍无最小化事件；页面初始化未完成不能单独解释问题。
没有重跑完整门禁，也不记作产品修复；现场无进程/启动记录残留，最新完整 8 项后失败
与其余 D/E/发布门禁保持。


2026-10-02 跨屏恢复修复：窗口多数标题栏在副屏、少数伸入主屏时，旧恢复逻辑
会按主屏优先顺序选错目标。`37674eea7` 改为选择可见标题栏最多的可达区域，
同分仍主屏优先，其他边界约束/缩放/缺屏回退和旧文件兼容保持。窗口状态 8 项回归
及严格 clippy 通过；完整 macOS 包构建通过。
`smoke-native-displays.py <app> --straddle` 增加本夹具跨屏保存状态恢复及再次重启。
修复前真实包 savedX=-1400 实际回到主屏 x=0，日志
`/private/tmp/reasonix-display-overlap-package-before.log`；修复后两种档案均正确落在
副屏边界约束位置 x=-2000，普通定位/重启与跨屏恢复/再次重启合计 8/8 通过，日志
`/private/tmp/reasonix-display-overlap-package-after.log`。失败现场保留、成功夹具删除，
无 Preview 残留。本轮两屏均 2×，混合缩放/实际拔插/物理拖动与最小化稳定性仍待验，
不将此修复推广为完整 D/E 或发布通过。


2026-10-02 存储复制反馈修复：`a8b06f7ea` 为实际 Tauri 存储页增加同步重复请求
拦截和来源/请求代次；旧异步结果不能给新路径显示已复制、旧错误或解除新请求忙碌。
刷新/路径/语言变化与卸载使旧反馈失效；加载错误独立保留，已进入 native 的写入不
宣称可撤销。专项回归先复现重复写入，再验证修复、拒绝、来源竞争、刷新错误与零回退；
完整 Tauri/共享剪贴板回归及 macOS 构建通过。当前包独立原生菜单/文本 IPC 两档案
4/4 通过，日志 `/private/tmp/reasonix-storage-copy-native-clipboard.log`，原剪贴板恢复、
无 Preview 残留。组件模拟和实际 native ACL 门禁不能代替存储页物理点击/拒绝 UI；
最小化稳定性、其余 D/E 和发布门禁仍待完成，桌面观察工具仍超时。


2026-10-02 通知批次交接修复：`af7cf185d` 解决处理首批 32 条期间补入的点击
在批次结束后遗留的问题。原生 32 条是同时排队上限；完整确认一批后 consumer 会
继续一次 fresh query，空队列/不可导航/停止/故障仍停止，故障不自动重试。
专项回归旧代码实际只确认 32 次；修复后原队列始终 ≤32，补入点击在 33 次打开/
各 token 一次确认后处理完成，最后一次空查询停止。完整 Tauri 回归和 macOS 构建通过。
当前包两种档案各 3 条通知的实际 OS 提交/固定文案/隐藏发送/精确删除/原件与重启
清理通过，日志 `/private/tmp/reasonix-notification-batch-native-delivery.log`；无 Preview
残留。依赖层批次模拟与 OS 送达分别记证据，横幅/物理点击/冷启动/拒绝恢复和其余
D/E/发布门禁保持。


2026-10-02 当前包旧版回退复验：同一 `af7cf185d` macOS 生产构建包与固定摘要
官方 Desktop/CLI 1.38.3，在生成私有目录执行 `smoke-legacy-rollback.py --workspace-data`
全部规定阶段通过，日志 `/private/tmp/reasonix-current-candidate-legacy-rollback.log`。
旧 GUI 前后两次原历史/同会话 writer 拒绝/原生退出、Preview 配置导入/重启/显式
三阶段原件保护、旧版 Global 文件/文本+图片引用与附件/检查点、回退后真实续聊及
第二检查点通过。最新代码检查点实际恢复 preimage；更早冲突拒绝不算回退成功。
独立确认两种宿主退出、成功夹具删除。此证据不等于完整旧历史已迁入 Preview、
图片像素传模型、旧客户端共享目录并发或物理导入/回退 UI 已验收。当前仍不能正式发布；
完整窗口稳定性、物理 D、B 图片/历史迁移、C 工具生命周期、E 和正式签名/公证继续待验。


2026-10-02 当前 `af7cf185d` 候选包的独立宿主丢失 **20/20** 复验通过：
两种私有档案的 TERM/KILL 空闲/流式 8 项，以及父检查前/token 前/ready 前
启动边界 12 项；kernel sidecar 均正常退出码 0，原件/锁/启动清理与同档案重启通过。
日志 `/private/tmp/reasonix-current-candidate-host-lifetime.log`、
`/private/tmp/reasonix-current-candidate-startup-lifetime.log`。
当前托管普通私有档案的配置路径按钮/已复制反馈/精确系统代次、实际 Cmd+V、
清空输入及 Cmd+Q 也通过，原剪贴板逐项恢复、成功夹具删除；日志
`/private/tmp/reasonix-current-candidate-ui-second-observation.log`。首个 60 秒超时
夹具不记通过。退出后已退出应用绑定的只读 AX 查询返回了新的普通档案窗口，
已停止操作且未关闭该普通实例；只确认自有夹具清理，不声称全局无 Preview。
后续退出后只读取 runner/进程回执，每次 UI 输入前核对自有存活 PID 和私有 origin。
当前显式档案物理复验、实际失败反馈、完整窗口最小化稳定性及其余发布门禁保留。


2026-10-02 Hooks 异步归属修复 `3fc6eb16e`：scope/工作区切换与卸载使旧
加载/复制/粘贴/保存/应用结果失效；旧完成只可释放自己的 busy，不能替换新
view/path/draft 或显示旧成功，手动刷新同步持锁。已发出的系统/后端操作不能撤销。
专项回归先在旧代码失败，修复后旧 paste/save/reload 与新上下文/新 busy 竞争通过；
日志 `/private/tmp/reasonix-hooks-request-before.log`、
`/private/tmp/reasonix-hooks-request-after.log`。最终完整 Tauri 回归与 macOS 包构建
通过，日志 `/private/tmp/reasonix-hooks-request-tauri-final-tests.log`、
`/private/tmp/reasonix-hooks-request-build.log`。普通 Preview 仍运行，本轮没有启动
私有 native smoke 或操作该普通实例；实际 Hooks UI/拒绝和新包原生验收仍待完成。
本地 ad-hoc 签名未公证，完整 D/A/B/C/E 与正式发布门禁保留。


2026-10-02 托盘主点击 `55fd3a7d7` 对齐稳定 Wails：Left/Up 始终走
主线程 show_main_window，与 Show 菜单/Dock/第二实例共用应用 unhide、窗口
unminimize/show/focus；已可见窗口点击不再隐藏。右键菜单/退出过滤和关闭偏好保留。
严格 Rust clippy 与完整 macOS 候选构建通过，日志
`/private/tmp/reasonix-tray-open-clippy.log`、`/private/tmp/reasonix-tray-open-build.log`。
普通 Preview 仍运行，已请求保存工作后退出，未操作/关闭它；本轮没有私有原生 smoke。
实际托盘点击、Show/Quit、隐藏/最小化恢复和 Dock 仍待验，不将编译视为物理通过。
其他 D/A/B/C/E、完整窗口稳定性与正式签名/公证/发布门禁保留。


2026-10-02 钥匙串反馈 `247212b09`：失败和已写入但刷新失败的恢复提示保留至
下次显式操作/卸载，以 role=alert 呈现；普通成功仍 2 秒清除并用 role=status。
三语言补充解锁/访问权限/重试及关闭、重开设置刷新步骤，原始错误/密钥不进提示。
受控时钟回归旧代码失败，修复后普通成功过期、错误持久、下次操作替换与原有
草稿/互斥/原件保护断言通过；完整 Tauri 回归 exit 0。日志
`/private/tmp/reasonix-keychain-feedback-before.log`、
`/private/tmp/reasonix-keychain-feedback-after.log`、
`/private/tmp/reasonix-keychain-feedback-tauri-tests.log`。
首次构建繁体 locale 超预算 2.8 bytes；`253abf30a` 精简重复措辞后 81197 bytes
通过原 81203.2 上限，未调整预算。最终完整 macOS 构建通过，日志
`/private/tmp/reasonix-keychain-feedback-final-build.log`。普通 Preview 仍运行，
没有操作用户钥匙串/普通窗口；真实 OS 锁定/拒绝/迁移点击和新包 smoke 继续待验。


2026-10-02 当前 `253abf30a` 包 sidecar 的独立钥匙串事务复验 **22 通过**，
包括真实 bridge 的迁移/保存/重启/删除/不落盘及单 Provider 读取失败后的其余恢复；
日志 `/private/tmp/reasonix-current-keychain-transactions.log`。默认忽略的 macOS
原生 test 已独立显式执行 **1/1 通过**，两份临时随机 service/固定假值实际读写、
隔离、替换、删除及重复删除缺失通过，日志 `/private/tmp/reasonix-current-keychain-native.log`。
原生测试只操作自有身份；普通 Preview 原 PID 保持，未操作窗口或真实用户凭据。
该结果不替代包级设置页迁移、实际 OS 锁定/拒绝/重新授权 UI，相关门禁保持待验。


2026-10-02 另存为源替换保护 `033803e32`：系统面板期间源路径被替换后，
旧句柄 inode 比较不足以拒绝覆盖新源。新增确定性回归旧代码失败；生产复制前和
最终提交前核对原源路径仍指向已打开句柄，无法确认则拒绝并提示重新打开。
取消、已有目标/源字节保护、无临时残留和重新打开后的正常保存，以及既有文件
动作共 12 项 Rust 回归通过；严格 clippy、完整 external-links 回归和 macOS 包构建
通过，日志 `/private/tmp/reasonix-save-source-before.log`、
`/private/tmp/reasonix-save-source-after.log`、`/private/tmp/reasonix-save-source-clippy.log`、
`/private/tmp/reasonix-save-source-final-links.log`、`/private/tmp/reasonix-save-source-build.log`。
身份校验不能锁住外部 writer，不承诺同 inode 内容变化的一致快照或彻底消除最终
校验/rename 外部竞态。本轮未执行新包真实 Save/Replace 面板，普通 Preview 未被操作；
物理 D 和其余发布门禁保留。


2026-10-02 通知队首保护 `ad3eb7070`：32 pending 溢出原先淘汰正在处理的
队首，256 target 溢出也会移除其映射；两个旧代码回归均确定失败。现在保留首个
有效 pending/映射，淘汰其他最旧候选；清除过期/无映射 pending，容量与 TTL 不变。
最终 9 项 Rust 通知回归、真实 bridge 路由/不重建、前端点击回归、严格 clippy 和
完整 macOS 包构建通过。日志 `/private/tmp/reasonix-notification-head-before.log`、
`/private/tmp/reasonix-notification-target-head-before.log`、
`/private/tmp/reasonix-notification-head-final-tests.log`、
`/private/tmp/reasonix-notification-head-frontend.log`、
`/private/tmp/reasonix-notification-head-clippy.log`、`/private/tmp/reasonix-notification-head-build.log`。
仍为有界队列，超量等待项可被淘汰；不保证所有超量点击保留，不扩大 ACK 权限。
普通 Preview 未被操作，新包实际 OS 送达/横幅/点击仍未复验；最小化根因未确认，
其余 D/A/B/C/E 与正式签名/公证/发布门禁保留。


2026-10-02 当前候选 `ad3eb7070` 的完整 Rust suite 使用包内 sidecar **215 通过、
2 默认忽略**，真实 bridge 生命周期/工作区/会话/钥匙串/通知路由实际执行；日志
`/private/tmp/reasonix-current-candidate-rust-full.log`。显式 macOS 钥匙串和 D-Bus 两项
默认忽略各自保留既有独立证据/延期范围，不记为本次通过。最终工作树完整 Tauri
前端 suite exit 0，日志 `/private/tmp/reasonix-current-candidate-tauri-full.log`。
测试 sidecar 无额外残留，普通 Preview 原进程未被操作。完整源码回归不代替当前包
窗口/托盘/通知等物理 UI；最小化失败、其他 D/A/B/C/E 及正式发布门禁仍保留。
