# Linux Preview 构建与验收

2026-10-07 在已 review 提交 `d4e33ae0c`（Windows 适配与配置页背景修复）之后开始。
当前开发主机为 macOS，未发现可用 Linux/容器构建环境；本文件不声明 Linux Rust
编译、安装包生成或原生桌面验收通过。

本机已验证：构建契约 69 项和模拟脚本预检/参数/出包查找/缺失产物/验证失败退出码；合并 Linux 配置通过
本地 Tauri JSON schema 校验；工作流 YAML、仅手动触发与只读权限检查通过；
完整前端构建及体积门禁通过；macOS 上 Rust 239 passed / 5 ignored。
Go sidecar 交叉编译为 x64/ARM64 静态 ELF，摘要分别为：

- x64：`2f6f45e95ab40e45ee3cf58e80345b89a938c03b9a8c12c718df14106b7053b9`
- ARM64：`e71faa91a1edf142a89ce0833be441edbc2b6a06a4cb4cb56ae8d5d15e709a18`

以上 sidecar 不是可安装桌面包，未在 Linux 上运行；不能把这些检查替代下面的原生矩阵。

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

| 项目 | 操作与通过条件 | 结果 |
| --- | --- | --- |
| 构建/包结构 | 同架构主程序与 sidecar 均为 ELF；deb/AppImage 内有 sidecar、图标、desktop entry；无用户 .env/配置 | 待 Linux |
| 安装/启动 | Debian 包依赖可解析；AppImage 正常与提取模式启动；无空白 WebView | 待 Linux |
| 窗口 | X11/Wayland 分别启动、缩放、最大化/还原、拖动、重启窗口状态 | 待 Linux |
| 托盘/关闭 | 新档案关闭即退出；菜单打开/退出；无托盘不崩溃；第二次启动唤起旧窗口 | 待 Linux |
| 凭据 | 保存 key、重启恢复、删除；Secret Service 锁定/缺失不泄露且有可操作错误 | 待 Linux |
| 模型页 | 与其他页同底色，浅色/深色与两布局一致；API key、模型测试、服务增删 | 待 Linux |
| 输入/剪贴板 | IBus/Fcitx 中文 IME、Ctrl 快捷键、文本选区、系统剪贴板 | 待 Linux |
| 文件/打开器 | 中文空格路径、文件选择器、预览/保存冲突；xdg-open 和外部编辑器 | 待 Linux |
| 会话/对话 | 发送/流式/取消/审批、历史重启恢复、滚动与选区 | 待 Linux |
| SSH/Serve | 指纹确认、SFTP 读写/冲突/路径操作、Serve 日志/停止/controller | 待 Linux |
| 通知 | 系统授权、发送/失败、点击导航在当前桌面环境的实际表现 | 待 Linux |
| 卸载/隔离 | Preview 与稳定版资料分离，卸载不删除稳定版资料；退出清理 sidecar | 待 Linux |

托盘行为以 [Tauri 托盘限制](https://v2.tauri.app/learn/system-tray/) 为准：Linux 不发送托盘
鼠标事件，因此不是“点击图标直接恢复”，而是选择菜单中的“打开”。桌面可能需要
AppIndicator 扩展；若启用后台运行后看不到托盘，可再次启动应用通过单实例恢复窗口。
请记录发行版、架构、桌面环境、X11/Wayland、WebKitGTK 版本、安装包 SHA256。
