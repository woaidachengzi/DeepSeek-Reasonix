# Tauri 实际消息选区菜单：review 与安装包检查点

2026-10-03。源码基线 `c9ad428d1fa2760eaef2d0b1bd0694d5483c6dfa` 加本目录 `gates/source.patch` 与 `gates/source-untracked`；新增 Python 工具保存在 `gates/scripts`。最终安装包在测试期间身份和严格 ad-hoc 签名保持不变。测试源码不等于干净 HEAD 构建；本次不发布、不推送、不切换默认下载项。

## Review 与改动

- Wails 基线共享菜单曾要求 `window.runtime`；实际 Tauri 入口为 `TauriSessionApp`/`TauriChatWorkspace`，此前没有挂载该菜单。仅修改 runtime 判定不能修复实际页面。现在在实际入口 lazy 挂载共享菜单，窄匹配真实 Tauri Markdown 正文，保留输入框系统菜单与会话/覆盖层禁用边界。
- 文本复制复用现有原生剪贴板边界，没有新增 renderer command、capability 或依赖。拒绝时展示既有错误提示、保留原生/逻辑选区，不能回退到 navigator/execCommand 绕过原生拒绝。
- 共享 Add to Chat 模式的 pointerdown 不再提前清除 Copy 所持选区。Review 发现新增 Tauri 测试最初没有启用该模式监听器，已补强实际事件路径；88 项菜单断言与完整 transcript 套件通过。Tauri 本轮没有接入 Add to Chat 回调，不能声称该动作或其快捷键完成迁移。
- 新增 opt-in 原生验收从实际 textarea 提交到私有本机假 provider、等待真实 assistant Markdown、程序化 contextmenu/Copy，再读实际系统剪贴板。动作只执行一次，等待只观察结果；没有物理鼠标/键盘、IME 或系统拒绝 UI 操作。

## 固定安装身份

| 对象 | SHA-256 |
| --- | --- |
| 最终 host | `f259ddc90191a09d7f5418b30579c6c9ca89e754e9ad456ccf5001d241ee7ca3` |
| 最终 sidecar | `156f1d447b179408fd2a803ee496d1be96d31a8c9b870f0deb637d02b3335311` |
| 最终 DMG | `f1bdf9e05c4ab05f0192f7e803ef0ffafa1e86b1744e084204f3bcac402a600e` |

只读挂载 DMG 后用 ditto 复制到 `/private/tmp/reasonix-d-message-menu-installed-crpdlm9f/Reasonix Tauri Preview.app`；没有替换用户安装。完整构建和签名见 `build-reviewed.log`、`install-reviewed.log/json` 与 `gates/signature.txt`。

## 验证结果

- `pnpm test:transcript` 完整通过；选区菜单 88/88。`pnpm test:tauri` 完整通过；生产前端 lint、类型/契约、CSS 与原有 bundle 预算随两次完整构建通过。
- 最终源码 `cargo clippy --offline --locked --all-targets -- -D warnings` 通过；使用最终安装 sidecar 的 Rust 回归 **227 passed / 0 failed / 5 ignored**，不把 ignored 视为通过。
- `verify-installed-d.py --gates message-copy package startup lifetime` 四组通过，原始结果/日志/脚本/源码身份见 `gates/result.json`。消息复制 managed/explicit 各一次 provider 请求、实际 UI/原生 IPC/系统精确文本、选区释放、完整有序剪贴板字节恢复、kernel exit0、sidecar/readiness 清理通过。
- package 双档案通过；startup 双档案 × SIGTERM/SIGKILL × 三个启动点共12项通过；lifetime 双档案 × idle/streaming × SIGTERM/SIGKILL 共8项通过，sidecar kernel exit0、原件与同档案重启保持。

## 首轮失败必须保留

首包 host `542f68f077410240b548694030088a3a29cab7930a194990853ea4184eedf118`、DMG `103355b36203973779b193480ede1770023754cd28e9d902286bdc5622b46e38`（sidecar 与最终一致）。managed 运行到了菜单点击，但探针立即 `read_text()?`，遇到尚无可读文本就失败，kernel exit2；explicit 未执行。原剪贴板完整恢复，失败 fixture `/private/tmp/reasonix-native-message-copy-kk_h7jd1` 保留。`first-failure` 保存安装身份、原探针、日志和原生 result；没有归档用户剪贴板快照或私有 HOME。

修正为固定5秒内观察一次异步写入是否达到精确文本（包含空/非文本初态），不再点击菜单、不放宽精确值/代次/恢复断言。全新安装和全新双档案通过；该失败不包装为产品复制权限已通过或所有 UI 已验收。

## 未关闭范围与回退

本包只验上述四组，不能继承旧 e7f5f9ae 的13组、48阶段窗口、通知、凭据导入与官方回退结果。旧固定安装包及其证据仍保留，原 Wails 默认下载项未改；本轮没有迁移用户数据。当前新包的完整窗口/其余服务/官方备份实际回退仍需按目标继续验。

D 未完成，E 未开启。现场菜单/快捷键/IME、Add to Chat、系统授权取消/旧 Wails 钥匙串服务、显示器拔插/混合缩放、通知横幅和点击/冷启动、选择/保存对话框、任意旧自定义目录互斥以及历史失焦/2px问题仍未关闭。A 的协议事件错误覆盖、B 的旧 Global 图片及大历史兼容、C 的 terminal/Browser/worktree 和真实 MCP/plugin 生命周期，以及正式 Developer ID/公证仍阻碍正式发布。仅 Windows/Linux 曾获用户延期，本轮没有新增延期。
