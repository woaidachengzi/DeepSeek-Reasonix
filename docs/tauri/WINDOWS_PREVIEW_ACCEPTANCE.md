# Windows Preview 手动验收

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
