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
| Node | `v24.21.0` | `>=24` | 已满足；经 `nvm install 24` 安装。 |
| pnpm | `10.34.5` | `>=10 <11`，锁定 `10.34.5` | 已满足；Node 24 下由 corepack 按 `packageManager` 提供。 |
| Wails CLI | 未安装 | `.wails-version` 指定 `v2.13.0` | 不能执行原 Wails 打包。 |
| 前端依赖 | 已安装（pnpm 10.34.5） | lockfile 安装 | 已满足；`pnpm install --frozen-lockfile` 通过。 |

Node 与 pnpm 两项原本是环境缺口，不是 1.38.3 代码失败，现已按 pin 补齐并记录
在上表。迁移期间仍不得以升级项目 lockfile 或依赖版本来绕过剩余缺口（Wails CLI
保持未安装，`wails build` 继续不在本基线范围内）。

## 已运行的基线验证

| 命令 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./...` | 通过 | 在未修改的 Go core 上运行；不包含嵌套 `desktop` Go module。 |
| `make build` | 通过 | 已生成被忽略的 `bin/reasonix` 与 `bin/reasonix-plugin-example`。 |
| `cd desktop && go test .` | 通过 | Wails/Desktop Go module 通过；macOS 链接器仅报告重复 `-lobjc` 的非致命 warning。 |
| `pnpm install --frozen-lockfile` | 通过 | Node v24.21.0 + pnpm 10.34.5；输出 `Lockfile is up to date` / `Already up to date`，未改动 `node_modules`。 |
| `pnpm build` | 通过 | 约 26s；8 项 bundle 预算全部 PASS。 |
| `pnpm test:all` | 通过 | 约 364s；`run-tests: all 337 suites passed`，断言 7116 passed / 0 failed。 |
| `wails build` | 未运行 | Wails CLI 未安装，且依赖前端固定工具链。 |

### 前端三项的运行条件

前端三项先在 `experiment/tauri` 的 `c5e7ed5d9` 上跑通，随后为取得不受并行改动
影响的纯粹证据，在官方基线提交 `fa018e4` 的独立 worktree 上重跑并作为本表依据：

```text
worktree: /Users/jerry/temp/app/reasonix-baseline-1383
commit:   fa018e4109268c912063c8cc619302fccdb57d74   (detached HEAD)
命令:     pnpm install --frozen-lockfile / pnpm build / pnpm test:all
```

三项结果与本表一致（baseline build 的 `initial raw` 实测 2434.1 KiB，与
`check-bundle-budget.mjs` 内记录的 2492541 B 归因值吻合）。该 worktree 不参与
日常开发，仅用于复核基线。

注意：`pnpm build` 触发的 Vite `emptyOutDir` 会删除被跟踪的
`desktop/frontend/dist/.gitkeep`，使工作树变脏；构建后需还原该文件再提交。

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
