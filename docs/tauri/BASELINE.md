# Tauri 迁移基线

## 冻结点

| 项目 | 值 |
| --- | --- |
| Stable tag | `desktop-v1.38.3`、`v1.38.3` |
| Commit | `fa018e4109268c912063c8cc619302fccdb57d74` |
| Tauri experiment branch | `experiment/tauri` |
| Worktree | `/Users/jerry/temp/app/reasonix-tauri` |
| 原 Wails worktree | `/Users/jerry/temp/app/reasonix-wails-1.38.3` |

`experiment/tauri` 从 `work/stable-hardening` 创建；该祖先仍精确指向上述
1.38.3 发布提交，迁移计划文档是唯一额外祖先提交。不得将 `main-v2` 或
任意 1.38.4+ 版本合并进本分支。

## 本机工具链检查

| 组件 | 本机检测值 | 基线要求 | 结论 |
| --- | --- | --- | --- |
| Go | `go1.27.1 darwin/arm64` | 模块要求 Go 1.26，toolchain `go1.26.6` | 可通过 Go toolchain 选择机制测试。 |
| Node | `v22.23.2` | `>=24` | 不满足 Desktop 前端构建要求。 |
| pnpm | `11.21.0` | `>=10 <11`，锁定 `10.34.5` | 不满足 Desktop 前端构建要求。 |
| Wails CLI | 未安装 | `.wails-version` 指定 `v2.13.0` | 不能执行原 Wails 打包。 |
| 前端依赖 | `node_modules` 不存在 | lockfile 安装 | 尚未安装，不能执行前端测试/构建。 |

这些是环境缺口，不是 1.38.3 代码失败。迁移期间不得以升级项目 lockfile 或
依赖版本来绕过它们；需要时应在隔离的本机工具链中安装精确版本。

## 已运行的基线验证

| 命令 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./...` | 通过 | 在未修改的 Go core 上运行；不包含嵌套 `desktop` Go module。 |
| `make build` | 通过 | 已生成被忽略的 `bin/reasonix` 与 `bin/reasonix-plugin-example`。 |
| `cd desktop && go test .` | 通过 | Wails/Desktop Go module 通过；macOS 链接器仅报告重复 `-lobjc` 的非致命 warning。 |
| `pnpm install --frozen-lockfile` | 未运行 | 当前 Node/pnpm 版本不符合项目 pin。 |
| `pnpm build` / `pnpm test:all` | 未运行 | 依赖前项完成。 |
| `wails build` | 未运行 | Wails CLI 未安装，且依赖前端固定工具链。 |

## 数据边界（迁移前不改）

默认 Reasonix home 为 macOS/Linux 的 `~/.reasonix`，Windows 为
`%APPDATA%\\reasonix`。`REASONIX_HOME` 会把全部配置、状态、缓存和数据
收敛到指定目录；`REASONIX_STATE_HOME` 只移动运行时状态，不移动全局配置或
Provider 凭据。

迁移中必须特别保护：

- `<Reasonix home>/config.toml` 与 `.env`（Provider 凭据）；
- sessions、archive、memory、projects；
- Desktop topic SQLite 元数据；
- session/history/task/usage catalog 等可重建投影；
- 窗口状态、更新器元数据和桌面标签状态。

Tauri Preview 未经用户明确选择不得迁移或重写任何一个目录；首次连接既有数据前
必须生成可定位的备份并阻止 Wails 与 Tauri 并发写入同一 state root。
