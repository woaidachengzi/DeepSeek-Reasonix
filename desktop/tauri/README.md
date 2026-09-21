# Reasonix Tauri host

这是逐步替换 Wails shell 的 Tauri 2 host；它暂不替换稳定的 Go Agent、会话格式或
现有前端 API。开发启动时 host 从 `REASONIX_DESKTOP_BRIDGE_BIN` 读取由构建流程提供的
`reasonix-desktop-bridge` 可执行文件路径；打包启动时则由 Tauri 的 `externalBin` 从应用
包内定位同一个 bridge。两种路径都通过每次启动独有的 token、ready 文件和 launch nonce
监管它。

## 数据隔离

Preview 使用 Tauri 应用数据目录下私有的 `REASONIX_HOME`。因此它不会静默读取、迁移或
写入稳定 Wails 客户端的配置、会话和缓存；稳定版可与它并存。导入稳定版数据会作为单独的
“先备份、再确认”的功能实现。开发者显式传入的 `REASONIX_HOME` 仍是有意识的覆盖选择。

窗口尺寸与最大化状态同样保存在 Preview 自己的 Tauri 应用数据目录，而非
`REASONIX_HOME`。损坏或不合理的状态会被忽略并使用默认窗口；导入、重置或删除 Go core
配置不会影响窗口偏好。

当前 Preview 只提供最小的显式配置导入：界面会显示默认稳定配置
`~/.reasonix/config.toml` 是否存在，用户确认后才复制它。复制前会在 Preview 私有目录创建
带时间戳的备份，目标 `config.toml` 必须尚不存在，绝不覆盖。会话、缓存、插件和 `.env`
均不会导入；导入后若 Provider 依赖环境变量，仍需由用户自行提供。显式设置
`REASONIX_HOME` 的开发环境不会显示该导入入口。

目前暴露给 WebView 的命令为 `bridge_status`、`restart_bridge`、`bridge_open_session`、
`bridge_session_snapshot`、`bridge_session_history`、`bridge_submit` 和 `bridge_cancel`。
token、loopback 端口和
bridge 原始请求不暴露给 JavaScript。`bridge_start_events` 在 Rust 内部订阅 SSE，并以
Tauri `bridge:event` 和故障时的 `bridge:connection-error` 事件转发给 WebView。

`desktop/frontend/src/lib/tauriBridge.ts` 已为上述小范围命令提供类型化适配器，只会在
Tauri WebView 中激活；现有 `desktop/frontend/src/lib/bridge.ts` 仍是 Wails 默认实现，直到
对应功能面完成迁移。Tauri WebView 现会渲染 `TauriSessionPreview`：可开/读会话、发送、
取消，并显示 bridge 事件和连接故障；它是有意隔离的最小垂直切片，不会冒充完整 Wails UI。
事件订阅可从 snapshot 的 sequence 开始，避免切换期间漏掉事件。sidecar 异常退出时，预览
提供受控重启：重新启动 bridge、重新打开当前 session、获取新 snapshot 后才恢复事件订阅和发送。

选择 workspace 时，Preview 只提供系统目录选择器；`main-window` capability 仅授予
`dialog:allow-open`，而不授予文件读写、保存对话框或任意 shell 权限。选中的路径仍会通过
既有 bridge 的 workspace 校验，取消选择不会改变当前输入。

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
