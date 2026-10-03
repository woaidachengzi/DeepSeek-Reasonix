# macOS 全屏完成通知与精确恢复：当前包切片

2026-10-03。源码基线 `62de8e7fb` 加 first-gates/source.patch 与工具快照；不是干净 HEAD 构建。生产窗口/菜单行为、capability 和依赖没有改动：新增的是 opt-in AppKit 全屏角色探针、四种全屏通知计数/快照，以及独立安装包门禁。Wails 1.38.3 的 createAppMenu 使用原生 App/Edit/Window 角色，基线 menu.go 已保存；Tauri 使用已锁定 muda 的 toggleFullScreen: 原生角色（Ctrl+Cmd+F），没有改成 JS 模拟窗口。

## 固定身份与源码检查

- 安装 `/private/tmp/reasonix-d-fullscreen-installed-y5w4n0kj/Reasonix Tauri Preview.app`：host `22ee53cbfe6324f63a1f63d73d477cb3017be21cd5866ee4dfab1f50efc20e35`，sidecar `6f7f6352af9eed4043abe9600b6ad80712088d9363b4a94d69d824628035e447`，DMG `c7e5c54f31ff9d35f569f192bbd2c99a1e1c199ed73626a67e5b64b0cbab6106`。
- app/DMG 完整构建、只读挂载/ditto 全新复制安装、严格 ad-hoc 签名通过；未替换用户应用。首次 clippy 因现有大 snapshot JSON 增字段触发宏递归限制，已保留源码patch和日志；字段移到现有 presentation 合并块后 strict clippy 通过，未提高 crate 递归限制。
- 最终原生源码使用本安装 sidecar，Rust **227 passed / 0 failed / 5 ignored**。构建内前端 lint、类型/契约、CSS与现有体积预算通过；ignored不记通过。

## 首轮 suite 不能判通过

原新工具每档案跑 exercise→restore-maximized→restore-normal→menu-fullscreen→restore-normal。

托管五阶段均通过，真实原生菜单进入与退出各一次：FullScreen样式位/Tauri状态位同步，did-enter/did-exit完成计数各1；几何 **2000×1400、x920/y344、scale2 → 3840×2160、x0/y0 → 精确原frame**。全屏期间save未替换普通窗口存档，退出后save字节与前状态完全一致，再次重启恢复正常frame。原始三个完整fullscreen快照在 first-gates/fullscreen.log。

随后显式 exercise 最小化失败，kernel exit2；will/did miniaturize=0、nativeMiniaturized/minimized=false，超时后 key/main=false，restoreRequests/Completions=2、mainPageFinished=true。请求时日志显示active/key=true、mainPageFinished=false。不根据页面加载、失焦或恢复计数自行归因；历史最小化问题仍未关闭。显式后续四阶段当时未执行。first-gates/result.json 的 fullscreen为failed，package/failure-exit/lifetime为not-run，保持原记录。

## 同包显式独立切片与边界门禁

在确认停止、固定二进制SHA/严格签名、私有0700目录/所有者、原native普通几何种子与凭据身份后，复用 `/private/tmp/reasonix-native-fullscreen-ofeh6dsq`，独立运行 restore-normal→menu-fullscreen→restore-normal，**3/3、kernel exit0**。实际fullscreen位/完成通知/存档字节/精确恢复与托管一致，原始trace/result见 explicit-control；不是重新运行原失败的最小化，也不补记原suite通过。

另在全新 boundary-gates 运行同包 package/failure-exit/lifetime **三组通过**，后者双档案×idle/streaming×SIGTERM/SIGKILL共8项、sidecar kernel exit0/清理/同档案重启保持；每项之后签名和包/工具身份均复核。没有系统剪贴板访问，没有重跑浏览器canary。

## 证据限制与修正

复用显式失败档案前未先复制原生 result/trace，后续launch重置了这两份live文件。原始完整失败日志、三阶段精选快照与超时完整状态、原始exercise kernel退出回执、原geometry种子仍保留；**不存在独立保留的原失败完整trace/result文件**，不能拿后来成功的文件伪装失败时序。该失误已向用户说明。

工具现于phase失败后立即把原始 result/trace/normal/该phase kernel exit复制到独立 `failed-phase` 0700目录，再抛原错误；不重试动作。新增 fake-file 回归通过，模拟后来live文件被覆盖，断言原失败回执仍逐字节不变、只有白名单四文件、无私有HOME数据和自动重试。该最终Python工具版本在根目录保存，首次gate的旧版本在first-gates/scripts保留；Python工具变更不影响已安装二进制。既有5项剪贴板恢复保护回归也通过。

## 尚未完成

本轮两种档案全屏分别通过不代表完整suite、实际键盘/鼠标、跨Space/显示器、从全屏隐藏/关闭/异常退出或长期稳定性通过。最小化失败及历史失焦/2px保持未关闭；下一步按请求时真实native状态与事件链定位最小化，不加延时/重试/偏移补偿或放宽断言。CUA近期30秒超时，现场证据不足；剪贴板不可读格式前置及待用户确认也未绕过。

不继承旧f259/e7的完整窗口/通知/凭据/官方回退为22ee验收。当前包其余D及官方备份实际回退仍待继续，D未完成/E未开启。A协议/事件/错误覆盖、B旧Global图片/大历史兼容、Cterminal/Browser/worktree/实际MCP插件生命周期及正式签名公证仍阻碍发布。仅Windows/Linux已有延期；没有新增延期、push、正式发布或切换默认下载。
