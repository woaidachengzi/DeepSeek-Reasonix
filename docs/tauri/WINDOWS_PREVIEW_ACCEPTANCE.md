# Windows Preview 手动验收

当前 `windows-pinnedloader-20261010` 候选在最后一次源码review之前构建；不包含随后追加的固定清单文件类型/外部symlink读取防护。不可将其标为最终review提交的安装包，下次继续时须重建；Windows实机门禁仍待验收。

## 2026-10-10 固定文件 turn loader 修复的新 x64 候选（实机待验收）

新安装包在 `desktop/tauri/target/windows-pinnedloader-20261010/x86_64-pc-windows-msvc/release/bundle/nsis/Reasonix Tauri Preview_0.1.0_x64-setup.exe`，38,247,762字节，SHA256 `b328df7304775dc191ba82c9e69667d8c3a242dbea102591bad89a1cce25154f`。base `de8bd83a…` + 十五文件dirty，包含共享Wails/Tauri安全读取、清单校验及Tauri每轮admission加载器，旧候选保留。原完整frontend门禁/预算通过，MSVC cargo-xwin release47.81s、NSIS正常exit0（session15375）；未签名/交叉构建/NSIS charset等既有警告保留。

解包session90345 exit0，夹具 `/private/tmp/reasonix-windows-pinnedloader-audit.o9gHdK`；x64 GUI host/Go sidecar通过，sidecar SHA256 `a94671b97b4fe61f9c88cee92e901e45416d984748b05499bc470cd25f63dff9` 与本次构建产物逐字节一致；整个host只有唯一预期UNK→NSS bundle marker差异，packed host SHA256 `ec67ceb058929fc897df760749d75af4c9addfd1b40ca1f1ec691595d5e2cf59`。配置/credential JSON/SQLite DB文件名清单检查通过，准确来源、build/audit脚本及日志在新target；不将文件名审计扩展为所有嵌入内容审计。

无Windows实机，未安装/签名/发布/推送。新增手动矩阵：仅独立合成档案及合法固定文件清单，在压缩后正常退出，修改夹具文件或将desired清单置空，重启提问，核对provider新manifest/正文或撤销修订、上次问答保留、host修订不显示为问题；原协议允许完整checkpoint或较短delta。还须验证WebView2、中文/IME、剪贴板、终端、bot、窗口/托盘与多屏DPI，不能由Mac准确包通过或PE解包替代。普通固定/取消固定管理入口本身仍未完整迁移，不能据包中loader存在宣称该入口可用。

## 2026-10-10 固定上下文历史过滤修复的新 x64 候选（实机待验收）

当前新候选在 `desktop/tauri/target/windows-pinnedhistory-20261010/x86_64-pc-windows-msvc/release/bundle/nsis/Reasonix Tauri Preview_0.1.0_x64-setup.exe`，38,242,763字节，SHA256 `353cfe6babea559c4edc9255fa0c625c41b2b9e65c02c98f4c36875a1172429d`。base `de8bd83ac…` + 十文件dirty增量，修复 bridge 将 host pinned revision 误投影为普通用户问题，不隐藏真正 user-origin 引用相同标签。旧projectionfix候选保留；不是clean release或已发布安装包。

完整原frontend门禁/预算通过，MSVC cargo-xwin release1m04s，构建session24688正常exit0。7zz解包session57130 exit0，十文件清单、PE x64 GUI host/sidecar、包内引擎与准确构建产物一致（SHA256 `376465d36dfe4558c045e6cbcda3add6296349c24fc7506e8e35a60f882322e5`）、整个host只有唯一预期UNK→NSS bundle marker差异通过；packed host SHA256 `874b07428d78065123568ae8302f5463451f7fdc5d9399c44e1e65758233e91e`，解包夹具 `/private/tmp/reasonix-windows-pinnedhistory-audit.YM6cRn`。Vite/cargo/appearance及NSIS未签名、非Win32字符集警告保留。未安装/运行/正式签名/推送/发布，无Windows环境，不能以macOS/Linux包测试代替实机验收。

追加手动门禁：独立合成会话带固定文件时，固定正文可供模型使用但不显示为用户问题，不污染列表预览；正常退出重启后再次提问，保留上次问答及固定正文，不重复修订。真正用户引用 `<pinned_context_revision>` 文本仍显示。固定文件编辑/撤销、covered pinned压缩重建和其它下文UI/剪贴板/终端/bot矩阵继续待实机；来源记录与日志在新target。

## 2026-10-10 旧会话完成升级修复候选（未做 Windows 原生验收）

本地基线 `de8bd83acbc4fd6fab6334dfcc5dac9ce4694979` + build开始时五文件dirty增量；共同Go保存层修复旧JSONL回合升级为DAG时遗漏 `turn_end`，只有回合身份及预先记录的canonical commit digest精确匹配才与回答同批落盘。不是clean release。新独立目录保留旧 `windows-current-20261010` 候选及所有来源记录，没有覆盖已交付旧包。

新NSIS：`desktop/tauri/target/windows-projectionfix-20261010/x86_64-pc-windows-msvc/release/bundle/nsis/Reasonix Tauri Preview_0.1.0_x64-setup.exe`，38,243,179字节，SHA256 `5833d7d3873f9bb56cbc57c6dc9cb2af597a954e62d2e4baed23b956d10f54d3`。原全部前端门禁与未放宽资源预算通过，Windows MSVC release56.37s、NSIS构建exit0；保留既有unused_mut/appearance方法、cargo config、交叉构建/未签名/NSIS字符集警告。

7zz解包成功，十个文件包含x64 GUI主程序、x64 Go sidecar、WebView2 bootstrapper和NSIS组件，不包含用户档案、模型配置、凭据或SQLite文件。准确sidecar SHA256 `3e92b52a422f6d56db85f4248dbeb68ce2b9609cc85b3bb143fb845f6a32c45d` 与构建产物逐字节相同；解包host `242370322611018865af072bbf31efd1be597187abc4233f5f52a6a96f1f9857` 与原host只差唯一预期 `__TAURI_BUNDLE_TYPE_VAR_UNK`→`__TAURI_BUNDLE_TYPE_VAR_NSS`。审计夹具 `/private/tmp/reasonix-windows-projectionfix-audit.sxdlTe`；来源/构建日志在新target目录。

没有Windows运行环境，不能把macOS/Linux上的恢复通过或PE解包当作Windows实机证明。手动矩阵追加：在独立档案导入合成旧会话并完成一轮回答→正常退出→重启→核对历史和后续模型上下文，不得误当中断输出；同时保留图片粘贴、终端、bot诊断、窗口/托盘/多屏DPI与安装/退出验收。没有安装、签名、发布、真实服务或真实用户迁移；完整目标active。

## 2026-10-10 当前诊断增量的新候选

基于 `7e11d70051715f0258e176a98171435fd50036dd` 加构建时 26 个未提交文件，包含只读 bot diagnostics native IPC/面板和窗口恢复共同路径改动。独立输出目录保留旧安装包，不是干净提交发布；没有签名、安装或 Windows 原生运行证明。

当前安装包：`desktop/tauri/target/windows-current-20261010/x86_64-pc-windows-msvc/release/bundle/nsis/Reasonix Tauri Preview_0.1.0_x64-setup.exe`，38,240,608 字节，SHA256 `6978cae495cc8e2b212d5d70ed0a8f8cbea0d2377ee49043a97f17d6654e59ba`。

原完整前端 lint/契约/TypeScript/Vite/资源预算通过，未放宽预算；Windows MSVC release 编译 1m55s、NSIS 封装正常 exit0。73 构建契约及 Linux preflight/CI sidecar 模拟检查通过。临时 runner 首次工作目录错误和第二次缺少本地工具路径分别 exit1，原失败日志保留；修正后第三次完成，不计失败尝试为通过。日志与构建时源码摘要：`desktop/tauri/target/windows-current-20261010/{build.log,build-corrected.log,build-toolpath.log,SOURCE_IDENTITY.json}`。

7-Zip 列表和解包成功；主程序为 x64 GUI PE、Go sidecar 为 x64 PE，仅 NSIS 组件、WebView2 bootstrapper 和程序文件，无用户档案、模型配置、凭据 JSON 或 SQLite 文件。解包 sidecar SHA256 `0e8bec4484f20e117a983ce5270c8cd710c7b2a6f2bfe8dcac877a1140a5398f` 与构建产物字节完全一致；解包主程序 SHA256 `c062fe4816fb3c14faaf02cdc86cf1025cfe1c23ec934d42e515c00e0d024c1a`，与原 host 唯一差异为正常 `__TAURI_BUNDLE_TYPE_VAR_UNK` → `__TAURI_BUNDLE_TYPE_VAR_NSS`。审计夹具保留 `/private/tmp/reasonix-windows-current-audit.3Jkjn8`。

保留两项源码警告（Windows 分支 DirBuilder unused_mut、未使用 restore_desktop_appearance）及旧 cargo config/交叉构建/未签名/NSIS 输出字符集提示。下方手动矩阵仍待 Windows 电脑；尤其 bot 页配置/运行观察分离、刷新、图片粘贴、终端、窗口/托盘、多屏 DPI、安装与退出不能由交叉编译或解包代替。真实远程/IM SDK、模型调用与用户数据导入没有执行。

## 2026-10-10 新候选（未做 Windows 原生验收）

从 `17ffeff7af01cfbc73fff3ddf7bd74ef21bffebf` 加当时 18 个未提交文件构建，包含远程 driving 收回观察与 Serve 观察锁竞争修正。完整前端门禁、Windows MSVC release 与 NSIS 构建通过；macOS 交叉构建不是 Windows 运行证明，也未正式签名或发布。

安装包：`desktop/tauri/target/x86_64-pc-windows-msvc/release/bundle/nsis/Reasonix Tauri Preview_0.1.0_x64-setup.exe`，38,206,373 字节，SHA256 `4d18669cf427f686392980c67e27a3304ac166274d536db55d5daa7bb4b6d4c1`。7-Zip 列表和解包通过，只有 NSIS 组件、WebView2 bootstrapper、x64 GUI 主程序及 x64 Go sidecar，没有用户档案、模型配置或 API key 文件。旧 NSIS 目录完整保留于 `/private/tmp/reasonix-remote-reclaim-package.ZLtaVX/windows-previous-nsis`；构建日志同目录 `windows-build.log`。以下是旧候选的历史记录，不能混用摘要或验收结论。

2026-10-07 用户授权 Windows 适配与安装包构建，原生运行由用户在 Windows 电脑验收。
E 提交基线：`d040be2b7917c2abe728166848a21241ecf86abe`。本批 Windows 构建适配在此基线上继续。

构建候选（尚未 Windows 原生运行）：`Reasonix Tauri Preview_0.1.0_x64-setup.exe`，
2026-10-07 配置页背景修复版，37,349,713 字节，SHA256：
`111525f6cb3f2ab0b35d965053e520be791637692fdff7306cec73d21f54f8e8`。
交付名为 `Reasonix-Tauri-Preview-0.1.0-x64-settings-bg-setup.exe`。
模型偏好分组、模型服务编辑区和服务卡片统一透出页面底色；输入框保留独立底色。
样式回归测试和配置页 API key 交互测试已通过，完整前端构建及 Windows 打包已通过。
实际配置组件的隔离浏览器预览通过 48 组背景检查：6 套主题 × 浅色/深色 ×
工作台/创作布局 × 1280/700px 窗口；添加自定义服务、编辑名称与输入框底色也通过。
该预览使用模拟 bridge，不是 Windows 原生验收，不读取或修改真实凭据。
上一版验收包保留在 `target/windows-handoff-20261007/`，不要混用安装包摘要。

已完成：完整前端构建及体积门禁、构建契约 47 passed、Mac 上 Rust 236 passed / 5 ignored、
Windows MSVC release 编译和 NSIS 打包。7-Zip 解包确认 x64 GUI 主程序、x64 sidecar 与
WebView2 bootstrapper 均在安装包内。解包 sidecar 摘要与构建产物完全一致；主程序
仅有 Tauri 正常补丁 `__TAURI_BUNDLE_TYPE_VAR_UNK` → `__TAURI_BUNDLE_TYPE_VAR_NSS`
的三个字节差异，解包主程序摘要为
`1043710d4d00c22d3313059f50dfbdc8da390fa33d3e0a798a7ec3fba131fae3`。
Windows 改动尚未提交，包内源码标识为 E 基线 + dirty；随包 `SOURCE_IDENTITY.json`
记录构建相关改动文件摘要，后续验收必须使用上面的安装包摘要。

## 构建

Windows 原生：Node 24+、pnpm、Go、Rust、Visual Studio C++ Build Tools，执行：

```powershell
powershell -ExecutionPolicy Bypass -File desktop/tauri/scripts/build-windows.ps1
```

macOS 交叉构建：安装 LLVM、LLD、NSIS 和 cargo-xwin，添加 Rust
`x86_64-pc-windows-msvc` target，把相关工具目录加入 PATH 后执行：

```sh
cd desktop/frontend
pnpm tauri:build -- --target x86_64-pc-windows-msvc
```

依据 [Tauri Windows 安装文档](https://v2.tauri.app/distribute/windows-installer/)。
构建脚本按目标映射 Go 的 GOOS/GOARCH、`.exe` sidecar 名称，并在跨平台 Windows
构建时选用 cargo-xwin。Tauri 自动合并 `tauri.windows.conf.json`。
安装程序在 `desktop/tauri/target/x86_64-pc-windows-msvc/release/bundle/nsis/`。

## 安装及环境

- 此候选针对 Windows 10/11 x64；ARM64 本批没有构建或验证。
- 安装名为 Reasonix Tauri Preview，采用当前用户安装，标识为
  `io.reasonix.desktop.preview`；开始菜单有独立 Preview 文件夹。
- 安装包包含 WebView2 bootstrapper。若电脑缺少 WebView2，安装运行时需要联网。
- 这是未签名的 Preview；核对随包 SHA256 后运行，Windows 可能显示发布者未知。
- 默认数据目录为 `%APPDATA%\io.reasonix.desktop.preview\reasonix-core`。
  首次验收从空 Preview 档案开始，保留稳定版资料。导入配置只在用户明确操作后执行。
- 验收前记录 Windows 版本、WebView2 版本、显示缩放和屏幕数量。

## 手动矩阵

| 项目 | 操作与通过条件 | 结果 |
| --- | --- | --- |
| 安装/启动 | 中英文安装器、中文空格安装路径、开始菜单启动；主界面完整，sidecar 不弹控制台 | 待 Windows |
| 数据隔离 | 设置中查看 Preview 路径；稳定版配置/会话不变；Preview 与稳定版可分别启动 | 待 Windows |
| 窗口 | 最小化、最大化、还原、关闭后托盘恢复；重启尺寸恢复；125%/150% 缩放及跨屏移动 | 待 Windows |
| 单实例 | 再次启动只唤起已有 Preview，不多开窗口或 sidecar | 待 Windows |
| 输入 | 中文 IME 组合期间 Enter 不误发送；Ctrl+C/V、Ctrl+Enter、设置快捷键、文本选区复制 | 待 Windows |
| 文件 | 中文空格工作区、多文件附件；打开/取消选择器；预览、保存及已有目标覆盖确认 | 待 Windows |
| 原生打开 | 浏览器链接、系统文件管理器、外部编辑器；目录/文件参数不被拆分 | 待 Windows |
| 对话 | 配置测试 provider 后发送、流式输出、取消、审批；重启会话可读；滚动/选区正常 | 待 Windows |
| 配置页背景 | 模型偏好、模型服务与其他配置页整体底色一致；浅色/深色、工作台/创作布局下无大块白色卡片；添加服务输入框仍有清晰边界 | 待 Windows |
| SSH/SFTP | 首次指纹确认、临时凭据、浏览/保存冲突、新目录/重命名/删除；symlink 删除不删目标 | 待 Windows |
| Serve | 对 Linux/macOS 远端启动/状态/日志、系统浏览器 controller；重复打开复用端口；停止、SSH 断开 | 待 Windows |
| 退出 | 托盘退出后 Preview 主进程与其 sidecar 消失；可再次正常启动 | 待 Windows |
| 卸载 | 应用/快捷方式移除，稳定版安装与资料不变；记录 Preview 数据是否保留 | 待 Windows |

Serve 的远端目标仍限 Linux/macOS，`local-proxy` 凭据未接入。
Windows 通知点击导航、Tauri 原生远端 tab、updater 不在本批通过声明中。
这里的“待 Windows”必须以真实电脑的结果替换；Mac 上交叉编译成功只证明构建成功。

遇到失败时记录复现操作、错误文案、Windows/WebView2 版本和候选 SHA256；
通过设置中的诊断导出入口保存证据，反馈前去掉凭据、token 与私人会话内容。
