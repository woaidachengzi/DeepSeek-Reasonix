<p align="center">
  <img src="docs/logo-ghost-wave-effect.svg" alt="Reasonix" width="360"/>
</p>

# Reasonix Tauri

[English](./README.md) · [Tauri 宿主说明](./desktop/tauri/README.md) · [迁移清单](./docs/tauri/E_MIGRATION_CHECKLIST.md)

基于 Reasonix 独立维护的 Tauri 桌面改造分支。重点改造桌面交互、配置管理与 macOS、Windows、Linux 原生打包，沿用现有 Go Agent 引擎。

当前仓库：[woaidachengzi/DeepSeek-Reasonix](https://github.com/woaidachengzi/DeepSeek-Reasonix) · 当前分支：`experiment/tauri`

## 维护者与项目定位

本改造分支由 [woaidachengzi](https://github.com/woaidachengzi) 独立开发维护。Tauri 迁移和下述改进属于当前分支的工作；原有 Go 引擎及继承功能来自上游 Reasonix。

桌面应用名为 **Reasonix Tauri Preview**，当前版本 `0.1.0`，使用独立应用标识 `io.reasonix.desktop.preview`。它不替换稳定版 Wails 应用；提供 Preview 安装包，不代表全部迁移功能或正式发布门禁已完成。

## 当前分支的改造内容

- **Tauri 桌面宿主**：Rust 外壳管理 Go sidecar、原生窗口、菜单与系统能力，前端通过类型化 bridge 通信。
- **桌面工作台**：项目与会话管理、模型选择、流式对话、Markdown、附件及会话生命周期管理。
- **模型与配置页面**：服务商和模型设置、凭据保存/删除；模型偏好与模型服务页面在不同主题、布局下统一整体底色。
- **远程管理**：SSH 认证和主机指纹确认、SFTP 浏览/编辑/路径操作，以及 Serve 状态、日志、启停和系统浏览器 controller 入口。
- **机器人与管理页迁移**：账号管理、配对审批，以及权限、插件、MCP、hooks、memory、subagents、数据和诊断等配置面；迁移和真实服务验收仍逐项推进。
- **原生安装包**：Windows x64 NSIS、Linux ARM64 原生 deb/AppImage Preview，以及平台对应的托盘和窗口关闭行为。
- **档案隔离与验证**：托管 Preview 资料与稳定版分离；记录源码身份、回归结果、安装包摘要和验收证据。

上述是已接入的功能面，不代表与 Wails 完全对齐。Tauri 原生远程 tab、剩余机器人流程、管理页实际包验收等见 [E 迁移清单](./docs/tauri/E_MIGRATION_CHECKLIST.md)。自动更新不在本轮迁移范围内。

## 平台状态

截至 2026-10-07：

| 平台 | Preview 安装包 | 当前验证情况 |
| --- | --- | --- |
| macOS Apple Silicon | 开发签名 app / DMG | 已构建 Preview 并完成部分包级验证；原生窗口与正式发布门禁仍有未完成项 |
| Windows x64 | NSIS 安装器 `.exe` | 已完成构建和包结构检查；完整原生手动矩阵尚未逐项记录 |
| Ubuntu 26.04 ARM64 | `.deb` / `.AppImage` | 原生构建、包审计完成，Rust 224 项通过 / 1 项忽略；维护者已确认手动验收通过 |
| Linux x64 / 其他发行版 | 同架构构建适配及手动 x64 打包工作流 | 不能继承 Ubuntu ARM64 的通过结论 |

详见 [Windows 验收](./docs/tauri/WINDOWS_PREVIEW_ACCEPTANCE.md)、[Linux 验收](./docs/tauri/LINUX_PREVIEW_ACCEPTANCE.md) 和 [Ubuntu 包摘要](./docs/tauri/evidence/2026-10-07-linux-arm64/README.md)。

目前是本地交付的 Preview 包，不是已发布、已签名的稳定下载渠道。上游 npm 包、官网、下载页及签名声明不代表本分支的 Tauri 安装包。

## 使用 Preview

使用与你的系统、架构对应的交付包，运行前核对随包 SHA256。安装并启动 **Reasonix Tauri Preview** 后，在应用内配置模型服务即可。

Ubuntu ARM64 交付包可在安装包目录打开终端执行：

```bash
sha256sum -c SHA256SUMS
sudo apt install "./Reasonix Tauri Preview_0.1.0_arm64.deb"
```

也可选择 AppImage：

```bash
chmod +x "./Reasonix Tauri Preview_0.1.0_aarch64.AppImage"
"./Reasonix Tauri Preview_0.1.0_aarch64.AppImage"
# 没有 FUSE 支持时使用提取模式：
APPIMAGE_EXTRACT_AND_RUN=1 "./Reasonix Tauri Preview_0.1.0_aarch64.AppImage"
```

安装包不携带维护者的个人 API key 或配置。默认模型条目不等于已保存凭据，请配置自己的服务凭据。Linux 系统凭据保存需要可用的会话 D-Bus 和 Secret Service。

默认托管 Preview 档案与稳定版 `~/.reasonix` 分离，旧资料仅在明确操作后导入。显式设置 `REASONIX_HOME` 会覆盖默认目录；没有备份时，不要让测试包直接使用稳定版资料。

## 从当前分支构建

需要 Node 24+、pnpm 10.34.5、与 `go.mod` 匹配的 Go（当前固定工具链 1.26.6）、Rust 1.89+，以及目标系统的原生构建和 WebView 依赖。Tauri 使用前端目录内的本地 CLI，不需要 Wails CLI。

```bash
git clone --branch experiment/tauri https://github.com/woaidachengzi/DeepSeek-Reasonix.git
cd DeepSeek-Reasonix
pnpm --dir desktop/frontend install --frozen-lockfile
pnpm --dir desktop/frontend tauri:dev
```

macOS 打包：

```bash
pnpm --dir desktop/frontend tauri:build
```

Windows 原生构建，准备 Visual Studio C++ Build Tools 后，在仓库根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File desktop/tauri/scripts/build-windows.ps1
```

Linux 在同架构 GNU Linux 上准备 GTK/WebKitGTK 等依赖后执行：

```bash
bash desktop/tauri/scripts/build-linux.sh
# 仅生成 Debian 包：
bash desktop/tauri/scripts/build-linux.sh deb
```

Linux 产物位于 `desktop/tauri/target/<本机 Rust triple>/release/bundle/`。此打包路径不支持 macOS→Linux 或 Linux 跨架构构建。详细说明及 Windows 交叉编译前提见 [Tauri 宿主文档](./desktop/tauri/README.md)。

继承的 CLI 仍可用 `make build` 构建；旧 Wails 构建方法保留在 [desktop/README.md](./desktop/README.md)。它们与 Tauri Preview 的构建方式不同。

## 文档入口

- [Tauri 宿主、开发和档案边界](./desktop/tauri/README.md)
- [E 迁移范围与未完成项](./docs/tauri/E_MIGRATION_CHECKLIST.md)
- [API 功能审计与发布门禁](./docs/tauri/API_SURFACE_AUDIT.md)
- [Windows Preview 验收](./docs/tauri/WINDOWS_PREVIEW_ACCEPTANCE.md)
- [Linux Preview 验收](./docs/tauri/LINUX_PREVIEW_ACCEPTANCE.md)
- 继承的引擎文档：[指南](./docs/GUIDE.zh-CN.md) · [CLI](./docs/CLI.zh-CN.md) · [ACP](./docs/ACP.zh-CN.md) · [扩展开发](./docs/EXTENSIONS.zh-CN.md) · [配置路径](./docs/CONFIG_PATHS.zh-CN.md)

引擎和旧桌面端文档可能描述上游/Wails 行为，当前 Preview 的实际范围以 Tauri 迁移和验收记录为准。

## 项目来源与许可证

本项目基于 [esengine/DeepSeek-Reasonix](https://github.com/esengine/DeepSeek-Reasonix) 开发，感谢上游作者和贡献者提供原有引擎及项目基础。沿用的项目 logo 来自 [Bernardxu123](https://github.com/Bernardxu123)。

上游贡献者属于原项目的历史贡献者，不是本 Tauri 改造分支的开发人员名单。本首页不将上游贡献榜或收款入口展示为当前分支的信息。

原有版权声明和 MIT 许可文本保留在 [LICENSE](./LICENSE)。
