# D：当前包真实目录、多文件与工作区普通重启（2026-10-04）

源码未改，基于已提交 `6faa225c1`，使用当前真实 DMG 安装包 `/private/tmp/reasonix-d-reopen-ready-installed-r4q66pgv/Reasonix Tauri Preview.app`。host SHA256 `46c2c9da5f1d10a0c92a5d319b465b381a36256e9e770870e6b6933ec13c5256`，sidecar `31418d61fb58823fabe12e0b448f9d005eae4566381e0ff3ca868ef2c11d1fd4`；本轮未构建新候选，也未重复浏览器 canary。

## 基线与权限

Wails 1.38.3 工作区入口使用 OpenDirectoryDialog，取消不切换，默认位置经 nearestExistingDirectory 解析；附件 Composer 使用原生 file input 的 multiple。对应片段已冻结。当前 Tauri 工作区使用 directory=true/multiple=false，附件 directory=false/multiple=true，结果只能在明确选择后返回；默认工作区写入 Preview origin 的 localStorage，项目目录通过 host 保存。main-window capability 仅有 dialog:allow-open，没有因本轮扩展通用文件系统或 shell 权限。本轮不证明 Wails 默认目录解析、其他入口或文件读取范围完整等价。

## 当前物理验收

一份全新 explicit 私有档案 `/private/tmp/reasonix-launch-services-kct2la97`，第一轮 host91661/sidecar91668；正常退出后工具检查原 root/app/成功回执和 credential identity，再普通重启 host91918/sidecar91925。每轮绑定精确存活安装路径与私有 origin；退出后未读取死亡 CUA 对象。

- 真实 Choose Reasonix workspace 面板，经 Go To Folder 输入本夹具 workspace，点击 Open；项目列表、默认工作区及信息栏均显示确切路径。重新打开后点击 Cancel，原工作区保持。
- 真实 Add files to this conversation 面板，点击第一行后 Shift+Down，AX 明确两行 selected；点击 Open 后 UI 返回恰两份附件 canary.txt 与「附件 中文.txt」，绝对路径和中文/空格名称正确。再次打开并 Cancel，两份附件保持；逐项移除后 Send disabled。未发送模型请求。
- 两次正常 Cmd+Q，原 runner session 均 terminal exit0；kernel host/open exit0，sidecarRemaining=false、readyRemaining=false。普通重启恢复相同 origin、同项目与同默认工作区，显式移除的附件不回放。
- 验收后 strict codesign verify exit0，host/sidecar SHA 保持；两个确切 host PID 均不存活、Preview 不运行、该根自有 sidecar/ready 清理。

`physical-actions.json` 是 CUA 实际动作和 AX 观察的人工转录，不冒充导出的原始 AX trace。固定元数据、退出回执、虚构 canary 文件摘要、源码/基线片段保留；未复制私有 HOME/core、原始剪贴板、keychain 或无关目录列表。

## 保留失败与验收边界

Escape 在面板打开后立即发送未关闭面板；后续实际 Cancel 成功。Cmd+A 未扩展多选，Shift+Down 后 AX 两行 selected；不称 Cmd+A 通过。一次隐藏的 full AX 读取重新编号后，旧索引107点击被工具拒绝，刷新可见 AX 后用正确索引50；不将拒绝算作产品成功或产品故障。

只证明当前46包一份 explicit 档案上的这些动作。managed 档案、单文件独立路径、过滤器、消失/拒绝访问文件、保存/覆盖/失败恢复、主题/技能/插件/导出等其他入口仍待逐项验收；历史包保存证据不继承为当前包通过。最小化/首次全屏键及间歇窗口稳定性仍未解决，D 尚未完成，E 全面验收仍待 D 稳定。A/B/C 与正式签名公证、全资料官方 GUI 回退等缺口保持。无新增延期、push、正式发布或默认下载切换。
