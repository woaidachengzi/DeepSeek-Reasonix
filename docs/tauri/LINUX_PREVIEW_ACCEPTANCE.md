# Linux Preview 构建与验收

## 2026-10-10 共享固定文件 loader 新包验收及 review 边界

新 ARM64 deb/AppImage 位于 `desktop/tauri/target/linux-pinnedloader-20261010/packages/`，SHA256 分别为 `a886a8da0806c292b9b665592248b8f9d1576411bb237017d6d38a0ede49ed06`、`24805d9d88057249612a7787383759975ca39c1f3e102107c72ecb9660a972a9`；Mac副本核对一致。来源为 base `de8bd83a…` + 打包时十五文件dirty，源码归档 `b94feac8…`、guest冻结快照 `73702d76…`，私有root `linux-pinnedloader-20261010.UeDzHX`。原前端门禁/预算及73契约通过，Rust release1m28s，build session73203最终exit0。首次完整缓存复制被空间门禁拒绝（session33685 exit1），未删除旧目录或放宽空间门槛；后续只复制release依赖缓存，不复制debug/旧bundle。

audit session42699 exit0：准确host `6a9bd63c…`、sidecar `02c459af…`；ARM64、两包和构建sidecar字节一致、ldd无missing、apt仅simulate、配置/凭据JSON/DB文件名清单通过。deb入口及实际AppRun各managed/explicit，四次私有Xvfb/D-Bus启动正常RunEvent::Exit，401/profile及sidecar/readiness清理通过。全部bot/SQLite/platformargs及八类摘要恢复、坏pinned缓存、真实私有文件编辑撤销各十次race harness66.395s；历史/预览/usage十次1.346s、完成升级四类十次2.565s、共享loader十次1.106s；冻结Git结束clean。portal/PipeWire/D-Bus警告保留，未当作GUI或媒体证明。

`SOURCE_IDENTITY.json`及构建/审计日志在新target。没有新增标准Rust debug回归，旧包291 passed/2 ignored不得移算新包。用户随后要求review并提交、暂停目标；review追加清单FIFO非阻塞/外部symlink拒绝修复，**本候选不包含该打包后修复，不等同最终提交源码**。下一轮需重建三平台包；可见GUI/IME/Wayland/物理剪贴板、Windows实机及真实服务门禁继续保留。未安装/发布/推送或操作真实数据。

## 2026-10-10 covered pinned 缓存认证拒绝的双冷启动包门禁

准确现有deb sidecar `c57eaa1e…` 三正向恢复+三负向缓存各十次，15.019s，原session37786 exit0。负向先实际compress生成covered pinned检查点，退出后仅将派生缓存hash改错/删除或schema退为3；再次冷启动submit必须拒绝旧摘要、恢复完整原问答，同时固定正文仅一次且为user角色、上轮问答不丢、history不显示host修订、磁盘正文不重复、无额外模型请求。不是只验证缓存删除。使用只读overlay，harness SHA256 `4f7530db03ca3d84b32deaeb69ffb83e4450c60138d67a7cfe3aaa841b3840ff` 前后校验，guest冻结Git前后clean；未改源码、重建或安装。日志 `desktop/tauri/target/linux-pinnedhistory-20261010/package-pinned-cache-gates.log`，脚本 `pinned-cache-gates.sh`，来源记录追加准确范围。

race只覆盖测试宿主而非release child；仍不是canonical provenance篡改、文件实际编辑/撤销、压力自动压缩、原生WebView卡片/滚动或真实服务/桌面GUI证明。

## 2026-10-10 显式补跑标准回归的 PTY / 私有 D-Bus 两项 ignored 门禁

逐项读取隔离测试后，用准确新deb sidecar SHA256 `c57eaa1ed86eb97b4b66296c012ce92c8af69d2b30e91dbdb2664ba9341ff857`，在原冻结private snapshot执行两条精确名称 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml --bin reasonix-tauri <test-name> -- --ignored --exact`。脚本显式清理继承profile及DBUS_SESSION_BUS_ADDRESS，私有PTY子进程env_clear/file credential/新HOME/project，模型URL仅不可用loopback，不做模型请求；D-Bus测试自建独立 `/tmp/reasonix-dbus-*` broker与受控假通知服务，不连接用户session bus或显示桌面通知。原session14525正常exit0，结束guest git status clean，完整日志 `desktop/tauri/target/linux-pinnedhistory-20261010/explicit-native-gates.log`，包与源码均未修改。

- `terminal::tests::actual_bridge_pty_end_to_end`：1 passed/0 failed/0 ignored/292 filtered，0.20s。Rust TerminalClient经真实bridge HTTP到实际Go PTY，验证create/input同request身份去重、100×30 resize/stty、输出与同一shell PID、中文rename、模型切换保持原shell、终端输出不进入history；close后仅探测夹具自己的PID确已退出。第二个夹具shell自行SIGKILL后正确传递signed exit_code=-1。OwnedBridge Drop尝试正常shutdown并有精确child兜底；该测试没有单独断言是否使用了兜底，不能据此新增“正常host Exit”证明，正常包Exit仍依据此前四次startup门禁。
- `notifications::xdg::dbus_tests::real_broker_covers_delivery_actions_owner_restart_failure_and_shutdown`：1 passed/0 failed/0 ignored/292 filtered，2.12s。真实broker验证能力/投递wire、早到回调、重复/伪造/非默认/关闭后信号拒绝、服务owner重启及ID复用、失败/超时不破坏旧绑定、shutdown释放worker/callback；服务是受控mock，不证明普通桌面banner/portal或Wayland通知行为。

原标准回归仍记291 passed/2 ignored，以上是两条单独显式结果，不改写成标准套件293 passed。保留unused appearance警告，来源记录追加精确范围。本次不是release Rust测试宿主、WebView/xterm、物理键盘或剪贴板，Windows运行及其余完整目标仍未完成。

## 2026-10-10 同一新包的实际 compress / covered pinned 追加门禁

不重建包、不改冻结guest源码：只读Go overlay将最终SHA256 `87748481f4aa6fad68534a8eec00cae0c24865d7b41516e3433669005d401308` 包测试映射到private snapshot，前后校验摘要及准确deb sidecar `c57eaa1e…`，结束git status clean。三类 `TestProjectionActualPackageRestart` 各十次，含实际 `compress` 工具→loopback summarizer→生成认证covered-prefix pinned checkpoint→两次冷启动submit，最终7.946s、原session83054 exit0（初次7.132s通过）；release child非race。模型请求保留摘要/固定正文/上次问答，可见历史不泄漏固定修订，磁盘不重复；详细断言、Mac对应结果和仍未覆盖项见E清单本日“实际 compress 与 covered pinned 检查点”记录。日志 `desktop/tauri/target/linux-pinnedhistory-20261010/package-actual-compress.log`，来源记录已追加；不是可见WebView/压力自动压缩、固定文件编辑撤销、坏pinned hash拒绝、真实服务或Windows运行证明。

## 2026-10-10 固定上下文过滤修复的新 deb/AppImage（隔离包级验收）

新包在 `desktop/tauri/target/linux-pinnedhistory-20261010/packages/`：ARM64 deb 42,926,958字节、SHA256 `e6ae4462c1d609bb429e526cd0365d4d7613c082ec571036b61530d1bf191ab1`；ARM64 AppImage 118,966,792字节、SHA256 `c0852c8e1304328a4d5b27c1becbcc008be98e8473def47053e2d3dc92ab9183`。Mac副本与guest原件摘要一致；旧projectionfix及更早候选保留，没有安装或替换用户应用。

Ubuntu运行状态先确认，host `de8bd83ac…` + 十文件dirty源码归档SHA256 `c03048197b7f903df9865bab51923dfe6cd309efdd2eb7a64594e7a03a5894ce` 校验通过；新私有root `/home/parallels/reasonix-builds/linux-pinnedhistory-20261010.dGHbp9`，private snapshot `2ed49000aa529fb5d2380a33dc711df46f97e5bd`，不是GitHub clean release。完整原frontend门禁/预算、73构建契约及Linux preflight/CI模拟通过，release1m33s，原build session69914 exit0。macOS archive provenance、Vite及unused appearance警告留在日志，没有改门禁掩盖。

审计夹具 `package-audit.LmKVwf`：准确deb host SHA256 `28090bd7d3b0f2033bda93778ffff55602ecb65da1019cab47272c5feb241fab`，Go sidecar `c57eaa1ed86eb97b4b66296c012ce92c8af69d2b30e91dbdb2664ba9341ff857`；两包及构建sidecar字节一致、ARM64 ELF、ldd无缺失、apt仅simulate、配置/凭据/DB文件清单审计通过。deb入口与真实AppRun各managed/explicit四次独立Xvfb/D-Bus正常RunEvent::Exit通过，sidecar/readiness清理、401、profile/identity等边界通过。四个私有档案 `/tmp/reasonix-linux-package-ljjjw3u6`、`-9mqulm6n`、`-kkxbimam`、`-zxyvkiof` 保留。

准确包sidecar的全部SQLite actual-package系列、只读bot、平台参数及两类正向摘要恢复（无pinned/host pinned尾部，两次冷启动submit）各十次race harness58.263s通过；历史/预览/usage专项十次1.357s、完成/中断/digest fence四类十次2.500s通过，原audit session58117 exit0。release child本身非race。随后标准 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml --bin reasonix-tauri` 使用准确deb sidecar，own Xvfb/D-Bus，debug compile46.91s，291 passed/0 failed/2 ignored，测试2.72s，原session25224 exit0。忽略项不计通过；portal fallback/PipeWire及D-Bus结束警告保留。

新来源记录和native-build/package-audit/native-rust日志在target。没有可见普通GUI/IME/Wayland/媒体/通知/系统图片剪贴板或真实模型、IM、SSH证明；pinned covered-prefix重建、固定文件编辑撤销、实际compactor及其它完整门禁继续待验收，不能因当前合成包级通过而关闭完整目标。未安装、正式签名/发布、推送或操作真实用户数据。

## 2026-10-10 旧会话完成升级修复的新 deb/AppImage

冻结本地 `de8bd83acbc4fd6fab6334dfcc5dac9ce4694979` + 五文件dirty增量，归档SHA256 `e98640f7da413c3ee4c07906784c13c335951870b6215412e13a6386361dc188`；新私有Ubuntu root `/home/parallels/reasonix-builds/linux-projectionfix-20261010.YX7XXe`、快照commit `075459eaddffd57d221959cfafd0ab07dfc50b84`。快照不是GitHub clean release。旧guest源码和旧包保留，仅复制旧私有编译缓存与经package.json/lock逐字节核对的既有依赖。macOS provenance扩展属性解压警告保留，源码归档校验通过，不更改源码内容或屏蔽构建门禁。

原完整前端门禁及资源预算、73构建契约、Linux preflight/CI模拟通过；Rust release1m39s，deb/AppImage构建原session64936终态exit0。主机副本在 `desktop/tauri/target/linux-projectionfix-20261010/packages/`，分别与guest原包摘要相同：

| 包 | SHA256 |
| --- | --- |
| `Reasonix Tauri Preview_0.1.0_arm64.deb` | `63b1e1c6934ded3d2c31c2f0e713b3d09b05ff1dc858f6a1acea16cb195e8545` |
| `Reasonix Tauri Preview_0.1.0_aarch64.AppImage` | `eef7ec0e909716fc42a061fdd2cbe56e4c68ed22eab6e026253e27ebdcb5f03f` |

审计root `package-audit.2iPL5W`：deb host ELF ARM64摘要 `94c0b8472efa555a7f435a2b6585f5d8c6c9aa409abdc449fc0131a2d898a325`，静态Go sidecar `15cfed15cedc2483b553b34f84969f0c799425bfb113616d2c476b933500ee7f`；两包及本次构建sidecar字节相同。ldd无missing、apt仅simulate成功，完整文件清单不含.env/config.toml/凭据JSON/SQLite/DB。deb入口和真实AppRun分别managed/explicit共四次独立Xvfb/D-Bus正常RunEvent::Exit通过，准确子进程pidfd退出、readiness清理、401及私有身份/profile/清空继承token与忽略开发binary override门禁保留。portal/PipeWire/D-Bus与unused appearance警告保留，不当作可见UI/声音验收。

准确deb内sidecar的合法摘要恢复（两次冷启动submit，模型摘要与canonical历史分离）、九类旧历史、其它全部SQLite actual-package系列、只读bot与平台参数，各十次race46.821s通过；Controller完成升级/负向digest fence/已完成DAG/真正中断专项各十次2.501s通过。宿主race，release子进程非race；loopback模型夹具无真实凭据，不接真实模型服务。审计session79852终态exit0，日志和固定runner在新target目录。随后标准Linux Rust回归使用这个准确包内sidecar、独立Xvfb/D-Bus/CARGO_BUILD_JOBS=2，原session74141终态exit0：debug编译31.68s、291 passed/0 failed/2 ignored（2.39s），警告与末尾D-Bus提示保留。

这关闭本次共同引擎完成升级缺陷的Linux包验证切片，不代表全部压缩/pinned上下文/损坏恢复或可见UI/IME/Wayland/物理剪贴板、真实SSH/IM等完整目标。未安装、签名/发布、推送或操作真实用户数据。

## 2026-10-10 当前包合法 DAG 分支的正向读取

当前 deb 精确 sidecar `b155dd0a…` 追加独立 schema-2 DAG wire fixture：checkpoint/main/fork 回答故意不同，显式选中fork。实际open/history只返回所选两条消息及准确角色/顺序/原时间戳，无未选分支混入或checkpoint回退；缺失usage/duration不虚构。完整七类旧历史矩阵（旧Unicode、隐藏协议、最新分页、损坏JSONL、未知schema、未知DAGentry、selected DAG）各十次race12.069s正常exit0，每类两次启动/退出，原件不改写、401和无phantom等门禁保留。宿主race/release子进程非race，没有模型调用/真实数据/GUI。

仅同步测试文件到私有guest源码，原件备份 `/home/parallels/reasonix-builds/linux-current-20261010.OYyUda/selected-dag-regression.1Sy2Nt/original-test.go`；未重建或安装包，私有快照已有测试增量。日志与runner为 `desktop/tauri/target/linux-current-20261010/selected-dag-regression.{log,sh}`。不证明全部DAG/压缩损坏恢复或普通恢复界面。

## 2026-10-10 当前包不支持事件格式的拒绝边界

当前 deb 准确 sidecar SHA256 `b155dd0a6a7cb48b7db04908fc71e69667cc3b0ca3c7cb290e0f5af55df97a92` 追加 schema_version=999、schema 2 未知 DAG entry 两类实际包测试。合法主 JSONL不能遮盖不支持的权威事件日志；authenticated open500、401、错误脱敏、两次启动/退出、原件字节保留、无 `.damaged` salvage 或 phantom transcript，各10次race通过，3.785s、正常exit0。宿主race/release子进程非race，不调用模型、真实账号或用户数据。

只同步测试文件至原私有guest源码，原件保留 `/home/parallels/reasonix-builds/linux-current-20261010.OYyUda/event-format-regression.xI0kFr/original-test.go`；快照此后含测试增量，不重建/安装已交付包。日志和固定runner在 `desktop/tauri/target/linux-current-20261010/event-format-regression.{log,sh}`。不是合法 DAG、全部压缩/损坏恢复、可见界面或完整 SQLite gate 证明。

## 2026-10-10 当前 Linux Rust 原生回归完成

继续观察原 session11073，而非重启测试；以 0 结束。使用当前 deb 包内 sidecar SHA256 `b155dd0a6a7cb48b7db04908fc71e69667cc3b0ca3c7cb290e0f5af55df97a92`，准确 frozen source `8525393798…`、标准 `cargo test --locked --manifest-path desktop/tauri/Cargo.toml --bin reasonix-tauri`、独立 Xvfb/D-Bus/CARGO_BUILD_JOBS=2：debug test 编译 2m42s，291 passed/0 failed/2 ignored（2.43s）。保留 unused appearance 方法警告和末尾 D-Bus 提示；默认忽略项不算通过。

因此同一当前源码/包已有原完整前端门禁与预算、实际 deb/AppImage 审计和四次普通 Exit、真实 release sidecar 诊断及 SQLite 十轮 race、Linux Rust 回归证据；仍不等同于可见 UI/IME/Wayland/媒体/通知或真实 SSH/IM 服务验收。日志 `desktop/tauri/target/linux-current-20261010/native-rust.log`；交付包与下节摘要未变，未安装或发布。26 个变更文件，无提交/push，完整目标 active。

## 2026-10-10 当前 deb/AppImage 已交付，实际包门禁通过

上一节的同一构建 session51437 以 0 结束，未因封装静默重启；只读核对准确私有目录时 linuxdeploy/appimagetool 确实在运行，随后原任务完成。Rust release 4m50s，保留 Linux-only unused appearance 方法警告。源码仍为档案 `7fa70880…` 与 guest 私有快照 `8525393798…`，不混用旧候选、不声称原 GitHub 基线的干净 release。

实际包复制在独立 `desktop/tauri/target/linux-current-20261010/packages/`，宿主副本摘要与 guest 原包逐字节一致，旧包保留：

| 包 | SHA256 |
| --- | --- |
| `Reasonix Tauri Preview_0.1.0_arm64.deb` | `b40ee1393a83561bacdfc58eb35e66d80725ba9cda8ae925120b8a4ede52938d` |
| `Reasonix Tauri Preview_0.1.0_aarch64.AppImage` | `78cd4f2836de0eb97991e89789b45d54d30f3405ca219b3600b39b3f972779a8` |

新私有 audit `/home/parallels/reasonix-builds/linux-current-20261010.OYyUda/package-audit.G8ehIS`：deb 架构 arm64、主程序/静态 Go sidecar 均实际 ELF ARM64，host `cad7a2c4dd18047d9c10e4df2d2272bf78e494a71d36134db9397b9664b4ed79`、sidecar `b155dd0a6a7cb48b7db04908fc71e69667cc3b0ca3c7cb290e0f5af55df97a92`；host ldd 无 missing，apt-get --simulate install 成功（未安装）。AppImage 正常提取、AppRun 可执行，两包 sidecar 与本次构建 binary 精确一致；文件清单未包含 .env/config.toml/credentials JSON/SQLite/DB，未打入真实账号或数据。

独立 Xvfb/D-Bus、最小环境/private HOME/XDG/file credential store 中，deb 入口与 AppRun 各 managed/explicit 共四次真实普通 RunEvent::Exit 通过：host0、同一 sidecar pidfd 已退出、readiness 删除，401、准确 core/cache/credential identity、继承 token 清空及忽略无效开发 binary override 门禁均通过。私有 fixture `/tmp/reasonix-linux-package-{o7cikmsm,vvzhuh0f,s8przdfq,p5odf31b}` 保留；不是可见 GUI、窗口关闭/托盘动作或物理剪贴板证明。D-Bus/portal/PipeWire 提示原样保留，runner 与整组审计均 exit0。

最终 deb 内实际 sidecar 的 bot diagnostics（渠道关闭/网关关闭，两次正常启动）、所有五个 TestSQLiteActualPackage 系列（原子导入/WAL突退/兼容及离线恢复/审阅扫描/旧历史）与三平台参数门禁十次 race 通过（29.495s）。宿主测试带 race，实际 release 子进程不带 race；无模型 submit/IM adapter 启动或真实账号联调。审计 session64909 exit0，完整脚本与日志 `desktop/tauri/target/linux-current-20261010/{audit.sh,package-audit.log}`。

当前源码 Linux Rust 标准回归随后以包内实际 sidecar、独立 Xvfb/D-Bus、CARGO_BUILD_JOBS=2 启动，session11073 真实编译中，尚无全量通过结论；原进程须继续观察，不能重启替代。日志 `native-rust.log`，来源/包摘要与当前状态见同目录 SOURCE_IDENTITY.json。当前26变更文件，未提交/推送/安装/正式发布；可见 UI/IME/Wayland/媒体/通知及真实服务等完整专项仍未完成。

## 2026-10-10 当前诊断增量的 Ubuntu 新私有构建（进行中）

重新只读确认授权的 Ubuntu UUID `{12152c42-4575-4de9-8a87-d116cb8a48b5}` running。Parallels 嵌套 shell 参数首次解析不符合预期，改为固定脚本入口并用 `id` 明确验证 parallels uid1000；没有以 root 构建或操作用户应用。源码档案只从 git tracked/非忽略 untracked 导出，无 `.git`、真实 `.env`、node_modules、构建缓存、绝对或上行路径、tracked symlink；原有 `.env.example` 与 credential 源代码保留。档案 SHA256 `7fa70880c14baad736a04dead9c087f3971640c8e3af206bccebeb2383b6dc92`，对应 host `7e11d7005` 加当时 25 个未提交文件，含新 bot diagnostics 与 macOS 时序修复源码。

新目录 `/home/parallels/reasonix-builds/linux-current-20261010.OYyUda` 私有 0700，旧 `linux-20261010-isolated.ikxqey` 源码/产物不覆盖。先逐字节核对 package.json/pnpm-lock.yaml 与旧依赖缓存一致，再复制 node_modules，不安装或更新依赖；使用已有 Node24.21.0/pnpm10.34.5/Go1.26.6/Rust1.93.1，GOENV=off/GOPROXY=off，无真实服务请求。guest 私有快照提交 `8525393798316517bb8f17151eb2974b3cf16745` 不是 GitHub 提交或原基线的干净 release。

第一次构建调用 Parallels 返回 Invalid argument（255）；宿主开始标记/日志均不存在，guest 准确目录前缀也没有匹配，确认未开始后仅重试一次。原构建现在真实运行，宿主 exec session51437；没有观察超时重启。73 项构建契约/Linux 脚本模拟、原完整前端类型/样式/边界等门禁与预算已通过：CSS120.5/120.9KiB、zh79.8/80.0、TW80.4/80.6，未改预算。已进入 native Rust release 编译，当前尚无新版 deb/AppImage 或包级通过结果。

脚本、完整原构建日志与源码档案保留在 `desktop/tauri/target/linux-current-20261010/{build.sh,probe.sh,native-build.log,source.tar.gz,guest-root.txt}`，独立目录不含用户配置/凭据。后续继续观察同一进程，再审计实际 deb/AppImage/sidecar 摘要、依赖与普通 Exit；不能以源码契约或原旧包成功代替本候选包验收。新增本文件记录后当前 26 个变更文件，未提交/推送/安装/发布，完整目标 active。

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
