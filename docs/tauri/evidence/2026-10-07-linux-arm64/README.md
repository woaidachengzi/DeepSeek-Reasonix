# Ubuntu ARM64 交付与 review

## 包与环境

2026-10-07 在用户授权的 Parallels Ubuntu 26.04 ARM64 中，从独立、干净的源码副本
`bb0447047c826b61b46c6523e1609f8719943cce` 原生构建 Reasonix Tauri Preview 0.1.0。
Node 24.21.0、pnpm 10.34.5、Go 1.26.6、Rust/Cargo 1.93.1、WebKitGTK 2.52.6、GTK 3.24.52。
该环境及交付包不是 x64 或旧发行版兼容性证明。

交付目录：

- Mac：`desktop/tauri/target/Reasonix-Tauri-Preview-0.1.0-arm64-20261007/`
- Ubuntu：`/home/parallels/Downloads/Reasonix-Tauri-Preview-0.1.0-arm64-20261007/`

二进制包保留在被 Git 忽略的 target 目录，不提交安装包、构建缓存、用户档案或凭据。

| 包 | SHA256 |
| --- | --- |
| `Reasonix Tauri Preview_0.1.0_arm64.deb` | `4da654b7da1d3ff9e704677d4fdb5c50d035408b12ac49cf8a09941c78aff655` |
| `Reasonix Tauri Preview_0.1.0_aarch64.AppImage` | `c179daa18dab81e06bde8a1e25cffd82e2960c29fa96f05c6fcaaaa130c24f13` |

Mac 与 Ubuntu 副本均通过同一 SHA256 清单。deb 解包确认只有主程序、Go sidecar、图标及
desktop entry；主程序和 sidecar 均为 ARM64 ELF，sidecar 静态链接。主程序 `ldd` 无缺失库，
`apt-get --simulate install <deb>` 成功解析运行依赖，未由构建代理实际安装应用。
AppImage 提取成功，确认内含同架构主程序和 sidecar、图标、desktop entry、可执行 AppRun，
未发现 `.env`、`config.json`、`settings.json`；未复制 Mac 用户配置或个人 API key。

AppImage 首次封装因官方工具下载连接断开而失败；使用同一官方 GitHub 资产的 API 下载
入口取得完整工具后，复用已编译主程序封装成功。重试报告 bundle-type 标记已不存在
（前次失败封装已处理该标记）；本批不声明自动更新通道已验证。

## 自动测试与手动验收

- Ubuntu 冻结锁文件安装、69 项构建契约及构建脚本模拟校验通过。
- 完整前端构建、类型/样式门禁及 bundle 体积门禁通过，Rust release 原生编译完成。
- Ubuntu 上使用实际 ARM64 sidecar 的原生 Rust 回归：224 passed / 0 failed / 1 ignored。
- 用户随后在本对话中确认：**“ubuntu测试已通过，review并提交代码”**。
  记录为该台 Ubuntu 的手动整体验收通过。用户未提供逐项操作日志，因此不额外声明
  X11/Wayland 双环境、两种 AppImage 启动模式、Secret Service 故障注入、通知专项、
  卸载/稳定版隔离全矩阵均已分别验收。

## 本轮 review 修复与复验

Review 范围为 Linux 适配提交 `bb0447047` 的目标选择、构建脚本与包配置、工作流、
托盘降级、关闭行为偏好和相应回归，以及 Ubuntu 交付结果。

发现并修正一项 CI 缺陷：`tauri-linux-preview.yml` 的原生测试步骤未设置
`REASONIX_TAURI_BRIDGE_TEST_BIN`，而 `bridge::tests::bridge_under_test` 在 `CI` 存在且
路径缺失时明确失败。现传入打包生成的 x64 sidecar 绝对路径，先检查其可执行性，
并使用 `cargo test --locked`；不改变应用二进制或已验收包。

新增工作流回归覆盖正确的 sidecar 路径、缺失二进制必须失败、Xvfb/D-Bus 环境传递、
Cargo 参数与锁文件约束。Mac 本地模拟通过；Ubuntu 使用 Node 24 运行 69 项构建契约和
更新后的脚本/工作流模拟通过，再以 `CI=true` 和实际 ARM64 sidecar 复验原生测试：
224 passed / 0 failed / 1 ignored。测试保留已有 unused-import/dead-code 警告，未放宽门禁。
真实 GitHub x64 工作流未触发，不把本地模拟或 ARM64 测试当作 x64 运行结果。

未发现其他需要阻止本批提交的代码问题。其余平台、专项矩阵以及 macOS D/E 门禁仍以
[Linux 验收记录](../../LINUX_PREVIEW_ACCEPTANCE.md) 和迁移清单为准。
