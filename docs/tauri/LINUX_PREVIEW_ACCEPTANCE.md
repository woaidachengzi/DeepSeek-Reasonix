# Linux Preview 构建与验收

## 2026-10-10 旧格式历史实际包读取

以本节下方SHA固定的lifecycle deb sidecar执行新增opt-in `TestSQLiteActualPackageLegacyHistory`，四类合成旧JSONL各十次通过（8.819s）：无ID中文/emoji/多行内容；legacy v1无manifest的host session-context和system/tool/reasoning不泄漏；240条精确最新200/start40/total240；损坏JSONL拒绝open500。无旧usage/时间/时长不虚构；401、准确身份/角色/顺序、两次启动及普通退出/readiness清理、JSONL字节不改、无phantom会话通过。不可达合成provider只用于open，不submit、不使用账号或真实历史。

同套实际macOS App sidecar十次17.349s通过。首轮探针误把*.events.jsonl等sidecar当会话，改用正式store.IsSessionTranscriptName；随后测试变量遮蔽包名导致编译失败，改名后复验通过，没有生产变更。日志`/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/sqlite-history-{macos-initial,macos-final,linux-final,macos-corrected,linux-corrected}.log`保留。guest仅复制新增测试、产品包未变，没有重打包/安装。测试宿主带race，实际release子进程不带race。

完整bridge/sessionidentity race和两包vet通过，回归日志`sqlite-history-regression.log`，默认opt-in skip不算包证据；gofmt/diff通过。四类样本不等于完整历史迁移/事件日志恢复矩阵或普通UI验收。当前28变更文件，未超过30文件review后提交门槛，无提交/push/真实数据/剪贴板/安装/发布，完整目标继续。

## 2026-10-10 实际 sidecar 扫描审阅导入

新增 opt-in `TestSQLiteActualPackageReviewedScan`，在独立私有档案用实际 lifecycle deb sidecar SHA256 `1171e74c945de7e918b0448ee8a1e2d11c783f2158c6905c2fc424120b66d519` 验证七类场景各十次（8.159s）：所选子集应用；文件改动/缺少确认元数据/重复选择/catalog 接管/删除时整批409；条件 trigger 在第一条插入后令第二条失败，500后身份及目录 revision整批回滚、同包重启无残留。401、path-free扫描、不自动登记、不改写源JSONL/catalog、未选文件保留和正常退出/readiness清理通过。guest仅复制新增测试，偏离私有源码快照`9daba875bc36ebc3a60dd2240e225bb3f5d7cb6a`；产品二进制未变，没有重打包/安装。

同套 macOS App sidecar SHA256 `ad6c56ef15da7be720cd5917eb1b6f649d113add9e18b5a6931bf6890bc76c4d` 十次通过（9.597s）。最初事务夹具要求HTTP暴露底层错误而失败，生产接口正确返回通用500；仅修正测试并断言无底层错误/私有路径泄漏。失败和最终日志 `/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/sqlite-reviewed-scan-{macos,linux}-{final,corrected}.log` 保留。完整 bridge/sessionidentity race及vet通过，日志`sqlite-reviewed-scan-regression.log`；默认测试会skip显式包用例，包级证据来自前述独立运行。宿主测试带race，实际release子进程不带race。

本次只有后端包内证据，不等同于普通界面扫描/导入/恢复或完整旧历史样本矩阵。累计27文件，未超过30文件review后提交门槛，无提交/push/真实账号/数据迁移/剪贴板/安装/发布，完整目标保持active。

## 2026-10-10 deb 普通 Tauri Exit 生命周期（新版候选）

将既有 `REASONIX_TAURI_PACKAGE_SMOKE=1` 入口扩展到 Linux，实际 release host 经正常 setup、Global workspace authenticated 操作和 `app.exit(0)` 进入普通 RunEvent::Exit，再执行原 sidecar/通知/图片状态清理。macOS 原有通知授权拒绝门禁保留；Linux XDG 没有对应 macOS 授权概念，记录真实 readonly Unknown/Unavailable 等状态，不请求或发送通知、不伪报投递验收。仅显式私有 probe 设置该 env；正常启动无变化。macOS cargo check 通过（39.50s），没有新 macOS 包级结果。

guest 源码快照 `9daba875bc36ebc3a60dd2240e225bb3f5d7cb6a` 是上一候选加 main.rs/探针，不是 GitHub 提交。73 项构建契约、原完整 frontend 门禁及 Linux Rust release 通过。新 deb SHA256 `02de8581d32939c603f2f3313a6cff9b56b0c01dd86c5fe99481703e2336ebf8`；解包 host `90670ac6c2247c7d83928a658d95e9d2b988670fd6c0dc4156d12e4ed760228c`、sidecar `1171e74c945de7e918b0448ee8a1e2d11c783f2158c6905c2fc424120b66d519`。准确 sidecar 与本次构建逐字节一致；新 deb 复制到单独 `desktop/tauri/target/Reasonix-Tauri-Preview-0.1.0-arm64-20261010-lifecycle/`，与前轮包分开，未安装或覆盖用户应用。

在独立 Xvfb/D-Bus/最小环境/private HOME/XDG/file credential store 中，真实新 deb managed/explicit 两次普通 Exit 通过：host 返回0、同一 pidfd sidecar 已退出、readiness 删除，准确 protocol/session/Global 路径与0700非 symlink目录保留；实际子进程 private profile/credential identity、401、继承 token 清空和 release 忽略无效开发 sidecar 路径均通过。探针只在失败 finally 才终止自有进程，此路径不能产生通过结果。反例使用前轮缺 Linux Exit hook 的真实 deb，20秒 timeout 按预期失败，之后的自有清理不把结果转为通过。该入口证明普通 Tauri Exit 清理，不等同于点窗口关闭、托盘退出或键盘菜单动作。

本轮原生 Rust 283 passed / 0 failed / 2 ignored（3.00s），新 deb 实际 sidecar 的十二类 SQLite 场景与三平台参数十次 race 通过（13.085s）；race插桩为测试宿主，sidecar为release。证据 `/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/{linux-lifecycle-build,macos-lifecycle-check,linux-lifecycle-negative,linux-deb-normal-exit,linux-lifecycle-native-rust,linux-lifecycle-deb-sqlite}.log`。保留D-Bus/portal/PipeWire/X11退出提示，成功runner均exit0。

新 AppImage 最初封装无新日志时，仅核对原活 linuxdeploy/appimagetool 并等待，没有重启。随后原构建成功完成两包，链内原生 Rust 再次283 passed/2ignored（2.46s）。新 AppImage SHA256 `6deb43a6602a7dfa686e8e3354076c08a7d69496c7d96a40c3b0a221f114ef04`，正常 AppRun 与 deb sidecar 逐字节一致。首个 Parallels 执行返回 Invalid argument，准确新 audit 目录不存在且无探针，确认未执行后一次重试。

最终两包整组普通 Exit 验收到第四项时中断：deb managed/explicit、AppImage managed 先各获得真正通过，AppImage explicit 已创建 fixture `/tmp/reasonix-linux-package-4kbwf8ds` 和 Global 标记，但已知 host67223以及所有相关 guest进程消失、readiness残留，宿主客户端仍等待。固定只读脚本核实guest真实进程表无相关进程，近期服务日志14:16:38出现停止/重启但没有据此断言OOM或产品根因。仅定位并再次核对本任务准确UUID/私有脚本路径的唯一宿主prlctl88647，终止这个已无guest工作的客户端；handle以143结束，原中断不算通过。不关闭VM、用户应用或其它任务。

此后才使用新的私有档案单项重验同一 AppImage explicit（新增 --profile 选择，无重新构建）：实际普通Exit0、同一sidecar退出/readiness删除、Global/身份/401及覆盖值边界全部通过，runner exit0。因此两包managed/explicit四种组合均有真正普通Exit证据，不将整组中断粉饰为全程绿。AppImage复制到上述独立lifecycle目录，SHA与guest原包一致；旧两包保留原目录不覆盖。当前runner源码在构建后加强校验及单项选择，SOURCE_IDENTITY记录独立runner摘要，编译产品源码不变。

后续证据 `/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/{linux-packages-normal-exit,linux-packages-normal-exit-retry,linux-appimage-explicit-normal-exit}.log`，失败/中断私有fixture保留；Ubuntu当时是否被人工操作已询问，未得到答案前不猜测原因。普通API Exit不是菜单/窗口退出、媒体/GStreamer（保留appsink/DRI3警告）、通知投递或可见UI完整验收。本分支累计26变更文件，未超过30文件先review再提交门槛，无提交/push/真实账号/数据迁移/安装/正式发布，Linux完整专项和完整目标继续。

## 2026-10-10 新 ARM64 包已产出，私有启动通过

上一节 Node 24 的原资源门禁阻塞已解决：只移除旧模型服务编辑器的 23 条无调用方文案，三语言同步，不缩短现用文案、不放宽预算、不改变 locale loader/UI 行为。新增回归扫描当前源码与测试无引用、三语言键集一致及现用错误恢复文案/占位符保留；23 个模型服务/文案相关测试 suite、生产与测试类型检查通过。新测试初次使用 ES2022 Object.hasOwn 不符合项目 ES2021 目标，改为等价 hasOwnProperty 后复验通过，失败日志保留。Ubuntu 原 Node 24 全部原 frontend 门禁通过，zh 79.8/80.0 KiB、zh-TW 80.4/80.6 KiB。原生 Rust release 与 deb/AppImage 完成；没有换压缩器或跳过 beforeBuildCommand。

guest 精确源码快照 `8a83cbbc50c4db8cd589e67e17c1952ddc22637e`，由前节私有快照加三份 locale、文案回归和 SQLite 启动参数夹具修正组成；不是 GitHub 提交或当前分支干净 release。新交付目录 `desktop/tauri/target/Reasonix-Tauri-Preview-0.1.0-arm64-20261010/`，`SOURCE_IDENTITY.json` 记录档案、后续源文件及包摘要。guest 原包保留在独立目录的 `source/desktop/tauri/target/aarch64-unknown-linux-gnu/release/bundle/`，Mac 交付副本摘要一致：

| 包 | SHA256 |
| --- | --- |
| `Reasonix Tauri Preview_0.1.0_arm64.deb` | `15bcec8f422c09279378476960e0654d5d4092a55228096f923a5209d2b7d633` |
| `Reasonix Tauri Preview_0.1.0_aarch64.AppImage` | `b9fd66965b5d38ba68de3ac1b680acfdd7756113cd910d863bb5a733e6dd8714` |

deb 解包为 ARM64 GUI 主程序和静态 Go sidecar、图标与 desktop entry，ldd 无缺失；apt-get --simulate install 解析成功但没有安装。AppImage 提取成功，AppRun 可执行；两包 sidecar 与本次构建二进制逐字节一致，SHA256 `6809880b33c090a78bc54b776f9645a8edc08073578d37ba63c01de86489b1f9`，没有配置或凭据文件。用最终 deb 解出的 sidecar 完整复验十二类合成 SQLite 场景与三平台启动参数十次 race 通过（12.138s）；实际 sidecar 为 release，race 插桩是测试宿主。

新 Linux 探针在独立 Xvfb/D-Bus、最小环境、私有 HOME/XDG/managed 或 explicit core、file credential store 中实际启动打包入口：deb 与提取 AppRun 共四次启动通过，真实子进程/readiness、401、准确档案目录和凭据身份验证，不读取真实账号/历史/剪贴板。最初错误地绕过 AppRun 直接启动 AppImage 的 usr/bin，WebKit 子进程相对路径失败；保留日志，纠正到正常 AppRun 后通过，同一包未改。探针仅终止并回收自己持有的 host/sidecar，不冒充正常窗口退出或完整 GUI/Wayland/IME/托盘/通知矩阵。私有日志保留 `/tmp/reasonix-linux-package-*`，D-Bus/portal/PipeWire/授权提示保留但探针及 runner exit 0。

探针 review 补齐：等待 readiness 时持续持有同一 pidfd，不丢失子进程引用；host 极早退出时仅从本 subreaper 的内核子进程中匹配准确 packaged sidecar，回收自己的孤儿进程，不按全局名称/PID 猜测。最终同一两包四种私有启动再次通过，runner exit 0，Python 语法与 diff 检查通过。正常 UI quit 仍不因此被标成完成。

证据 `/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/{linux-native-build-locales,linux-native-build-locales-fixed,provider-locale-regression,linux-package-audit-startup,linux-package-audit-startup-retained,linux-package-audit-startup-apprun,linux-package-audit-startup-final,linux-deb-sqlite}.log`。本批累计 25 文件，未达超过 30 文件 review 后提交门槛，无本分支提交/push、安装、发布或默认下载切换。Windows/macOS 当前已构建候选早于本次文案精简，不能混用 Linux 摘要；完整目标及各专项仍待继续。以下保留本阶段早期失败记录。

## 2026-10-10 当前源码的 Ubuntu 隔离复验（尚未出新包）

用户明确授权恢复并隔离验收，随后确认 Ubuntu 已恢复。仅使用新目录 `/home/parallels/reasonix-builds/linux-20261010-isolated.ikxqey`，以普通 parallels 用户执行；不安装新应用、不读取真实账号或迁移历史。源码档案由 tracked/非忽略 untracked 文件组成，不含 `.git`、用户配置或构建缓存；SHA256 `eb530d37e85d58b48da8912fbc4fb2ea075831497cef6f7dbe8b3812ff3f6830`。它对应 `17ffeff7af01cfbc73fff3ddf7bd74ef21bffebf` 加当时 18 文件变更，guest 本地快照提交 `893f9196a02bbfe11c01407e00c37bfaf1fa9048` 不是 GitHub 提交或原基线的干净 release。

Node 24.21.0 / pnpm 10.34.5 / Go 1.26.6 / Rust 1.93.1，73 项构建契约和 Linux 构建脚本模拟通过。原 frontend 构建的类型、样式、边界等门禁通过，但中文资源 level-9 gzip 为 80.3 KiB，超过既有 80.0 KiB；完整打包退出失败，没有新版 deb/AppImage。保留原预算，没有把旧包或 sidecar 交叉编译当作新版 GUI 包。

实际 Ubuntu ARM64 sidecar SHA256 `77ec241fee513b9c8c307e20a06d800efed0701c1b94143d8c4f8e2ce42241e0`。SQLite 测试首轮错误地传入 macOS 专用 `--host-pid`，生产启动校验正确拒绝；仅修正夹具的跨平台参数并补三平台参数回归，未改产品校验。通过 guest shell 复制测试修正后，同一 sidecar 的十二类合成私有兼容/故障/恢复场景十次 race 通过（11.953s）。测试宿主带 race，实际 release sidecar 不带 race；仅测试文件偏离上述 guest 快照，sidecar 未变。macOS 修正后的实际包复验两次权限审核超时未执行，不能算通过。

Xvfb/独立 D-Bus 中的原生 Rust 回归使用上述实际 sidecar：283 passed / 0 failed / 2 ignored（2.58s）。退出时有 D-Bus connection 提示，测试与 runner 均 exit 0；保留已有编译警告。这不是可见 GUI 交互或完整 Linux 专项矩阵证明。日志 `/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/{linux-native-build,linux-native-sqlite,linux-native-sqlite-platform-fixed,linux-native-rust}.log`，失败日志保留。后续先解决跨工具链资源体积门禁，再原生出包及包级验收；以下旧包手动通过结论不适用于本候选。

2026-10-07 在已 review 提交 `d4e33ae0c`（Windows 适配与配置页背景修复）之后开始。
随后在用户授权的 Parallels Ubuntu 26.04 ARM64 中完成原生构建、测试及 `.deb` / AppImage
交付，安装包源码为 `bb0447047c826b61b46c6523e1609f8719943cce`。用户在本对话中确认
“ubuntu测试已通过”。该结论记录为这台 Ubuntu 的手动整体验收通过；未提供逐项操作日志，
不据此推定下面全部专项、x64、其他发行版或 macOS 待验项通过。

实际环境、包摘要与 review 结果见 [Ubuntu ARM64 交付证据](evidence/2026-10-07-linux-arm64/README.md)。
Linux 原生 Rust 回归使用实际 Go sidecar，224 passed / 0 failed / 1 ignored；
完整前端构建、体积门禁、69 项构建契约和 Linux 脚本校验通过。

适配初期的 macOS 检查记录：构建契约 69 项和模拟脚本预检/参数/出包查找/缺失产物/验证失败退出码；合并 Linux 配置通过
本地 Tauri JSON schema 校验；工作流 YAML、仅手动触发与只读权限检查通过；
完整前端构建及体积门禁通过；macOS 上 Rust 239 passed / 5 ignored。
Go sidecar 交叉编译为 x64/ARM64 静态 ELF，摘要分别为：

- x64：`2f6f45e95ab40e45ee3cf58e80345b89a938c03b9a8c12c718df14106b7053b9`
- ARM64：`e71faa91a1edf142a89ce0833be441edbc2b6a06a4cb4cb56ae8d5d15e709a18`

以上是初期交叉编译 sidecar 的摘要，不是随后交付的安装包摘要；不能代替原生矩阵。

## 本批范围

- 原生 GNU Linux x64/ARM64 目标与无扩展名 Go sidecar；显式 GOOS/GOARCH、CGO=0。
- 独立 `tauri.linux.conf.json`，默认 `.deb`、AppImage，PNG 图标和 Debian 托盘依赖。
- 构建脚本检查 Node 24+、Rust、Go、pnpm、WebKitGTK/GTK/AppIndicator/D-Bus 开发库，输出包摘要。
- Linux 托盘采用菜单恢复；新档案默认关闭即退出，已有明确偏好不改写。
- 托盘初始化失败不阻止应用启动；没有托盘时不允许保存“关闭后后台运行”，已有该偏好则关闭时退出。
- 无托盘时设置页显示实际生效的“退出”，不改写已保存偏好；托盘恢复后原明确偏好仍有效。
- 手动 GitHub 工作流只构建 x64 预览附件，无自动触发、release 发布或模型凭据。

## 原生构建

依据 [Tauri 系统依赖](https://v2.tauri.app/start/prerequisites/) 与
[AppImage 构建基线](https://v2.tauri.app/distribute/appimage/)，以 Ubuntu 22.04 /
Debian 12 作为初始基线候选。实际兼容范围需要原生运行证明，不能由构建环境版本推定。
AppImage 不等于完全静态链接，构建系统的 glibc 版本仍限制旧发行版兼容性。

Debian/Ubuntu 先准备开发库（需要管理员权限，脚本不会自动安装）：

```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config curl wget file \
  libwebkit2gtk-4.1-dev libgtk-3-dev libxdo-dev libssl-dev \
  libayatana-appindicator3-dev librsvg2-dev libdbus-1-dev patchelf
```

另需 Node 24+、pnpm 10.34.5、与根 `go.mod` 匹配的 Go、Rust 1.89+（建议当前 stable）。

```bash
pnpm --dir desktop/frontend install --frozen-lockfile
bash desktop/tauri/scripts/build-linux.sh
# 或只生成 Debian 包：
bash desktop/tauri/scripts/build-linux.sh deb
```

输出：`desktop/tauri/target/<本机 Rust triple>/release/bundle/{deb,appimage}/`。
ARM64 必须在 ARM64 Linux 上构建；本批拒绝 macOS→Linux、Linux 跨架构和 musl 打包。
手动工作流 `.github/workflows/tauri-linux-preview.yml` 需先由用户推送并明确触发，本轮未触发。
Review 已修正工作流的真实 sidecar 测试环境变量，并增加可执行文件预检及锁文件约束；
缺失 sidecar 必须失败，不能跳过真实进程回归。本地执行该步骤的夹具校验通过，
但不等于 GitHub x64 工作流已运行。

安装/运行示例：

```bash
sudo apt install ./Reasonix*.deb
chmod +x ./Reasonix*.AppImage
./Reasonix*.AppImage
# 没有 FUSE 支持时按 AppImage 工具的提取模式运行：
APPIMAGE_EXTRACT_AND_RUN=1 ./Reasonix*.AppImage
```

API key 由用户在目标机配置。Linux 系统凭据需要可用的登录会话 D-Bus 和 Secret Service
（如 GNOME Keyring/KWallet 的兼容服务）；无服务或锁定时保留错误，不回退到明文文件。
默认 Preview core 目录为 `${XDG_DATA_HOME:-$HOME/.local/share}/io.reasonix.desktop.preview/reasonix-core`，
与稳定版 `~/.reasonix` 隔离；显式 REASONIX_HOME 仍遵守既有档案隔离规则。

## 原生验收矩阵

Ubuntu 26.04 ARM64 手动整体验收已由用户确认。未收到逐项日志的专项保留为待分项记录，
而非替用户补写测试过程；X11/Wayland 双环境、故障注入与其他发行版仍需独立证据。

| 项目 | 操作与通过条件 | 结果 |
| --- | --- | --- |
| 构建/包结构 | 同架构主程序与 sidecar 均为 ELF；deb/AppImage 内有 sidecar、图标、desktop entry；无用户 .env/配置 | Ubuntu ARM64 包审计通过 |
| 安装/启动 | Debian 包依赖可解析；AppImage 正常与提取模式启动；无空白 WebView | deb 依赖解析通过；用户整体验收通过；两种 AppImage 启动模式未分别记录 |
| 窗口 | X11/Wayland 分别启动、缩放、最大化/还原、拖动、重启窗口状态 | 待专项记录 |
| 托盘/关闭 | 新档案关闭即退出；菜单打开/退出；无托盘不崩溃；第二次启动唤起旧窗口 | 待专项记录 |
| 凭据 | 保存 key、重启恢复、删除；Secret Service 锁定/缺失不泄露且有可操作错误 | 待专项记录 |
| 模型页 | 与其他页同底色，浅色/深色与两布局一致；API key、模型测试、服务增删 | 待专项记录 |
| 输入/剪贴板 | IBus/Fcitx 中文 IME、Ctrl 快捷键、文本选区、系统剪贴板 | 待专项记录 |
| 文件/打开器 | 中文空格路径、文件选择器、预览/保存冲突；xdg-open 和外部编辑器 | 待专项记录 |
| 会话/对话 | 发送/流式/取消/审批、历史重启恢复、滚动与选区 | 待专项记录 |
| SSH/Serve | 指纹确认、SFTP 读写/冲突/路径操作、Serve 日志/停止/controller | 待专项记录 |
| 通知 | 系统授权、发送/失败、点击导航在当前桌面环境的实际表现 | 待专项记录 |
| 卸载/隔离 | Preview 与稳定版资料分离，卸载不删除稳定版资料；退出清理 sidecar | 待专项记录 |

托盘行为以 [Tauri 托盘限制](https://v2.tauri.app/learn/system-tray/) 为准：Linux 不发送托盘
鼠标事件，因此不是“点击图标直接恢复”，而是选择菜单中的“打开”。桌面可能需要
AppIndicator 扩展；若启用后台运行后看不到托盘，可再次启动应用通过单实例恢复窗口。
请记录发行版、架构、桌面环境、X11/Wayland、WebKitGTK 版本、安装包 SHA256。
