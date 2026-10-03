# ecac5751 当前候选：全屏与真实官方回退

2026-10-03，生产源码为 8f7cdd791 的隐藏启动修复；使用此前实际 DMG 安装的同一 app，本轮未修改/重新构建生产代码。完整包/DMG身份见 install.json。host ecac5751dde271a1c2d4c5cb758e76ad80da5be991d98a348f365e979b1f7248，sidecar db15d5210d7008e8bb38baab27d06fdd5e7462a47d9dceea9fd9a390cf952805。不是干净 HEAD 构建，未公证；本轮补验不能替代上一轮的构建来源说明。

## 全屏：两档案完整往返

`verify-installed-d.py --gates fullscreen` 实际 exit0、result.json passed。两档案各一次 exercise → restore-maximized → restore-normal → menu-fullscreen → restore-normal，共10阶段，无重试，原几何/最小化断言不变。

真正 AppKit Toggle Full Screen 角色动作，nativeFullscreen/Tauri fullscreen 同时进入并退出、didEnter/didExit各1；几何 **2000×1400/x920/y344/scale2 → 3840×2160/x0/y0 → 精确原框**。全屏期间与退出后的普通存档原字节一致，重启与凭据身份保持。fullscreen.log保存六个完整原生状态回执，fullscreen-receipts.json按两档案整理；相应源码、原有严格判据和包签名/摘要见 fullscreen/scripts 与 result.json。成功夹具由既有工具清理；未宣称留有已清理夹具的独立原始trace文件。属于程序化菜单操作，不是物理按键/点击；未据此关闭历史间歇问题。

## 单份配置备份实际应用

新增 `tools/tauri/smoke-backup-rollback.py`，将此前一次性脚本整理为可重跑 CLI（Python3.11+）。当前 app 实际导入配置、修改模型后重启，确认源包及sidecar退出，将唯一 config.toml 备份复制到独立0700旧版HOME/.reasonix、文件0600，再由固定摘要的官方内嵌CLI读回。真实exit0：恢复模型 native-import/alpha、currency=CNY；备份与恢复SHA相等，原Preview配置/备份树以及恢复树的字节/hash/mode/mtime保持，正常显式拒绝与旧CLI原目录回读也通过。详见 backup/rollback-receipt.json、artifact.json、result.json 与 backup.log。

门禁在失败时保留私有夹具，成功清理；共有fixture helper新增可选retain_failed，既有调用默认清理语义保持。失败原字节/0700权限与成功清理的Python回归1项通过，日志另存。真实current package运行验证了import/restore/explicit三阶段，无用户档案应用、真实provider网络或GUI一键回退宣称。

## 官方历史、Global附件与检查点回退

`smoke-legacy-rollback.py --workspace-data` 使用当前 Preview、官方 Wails1.38.3 GUI和内嵌CLI实际exit0。GUI摘要869b02d8、CLI5b1ab314按原固定摘要验证；完整身份见artifacts.json。本机假provider产生真实历史、Global文件、文本/图片引用、文件编辑和检查点；官方GUI在导入前后读回原历史/tab/model，活跃会话writer拒绝、原生Quit/清理通过。Preview导入/修改/重启与显式拒绝通过，旧版原树的字节/mode/mtime保持。

官方CLI继续同会话，解析真实文件/附件并生成第二检查点；最新code rewind实际恢复preimage，较早冲突拒绝，transcript/附件保持且无provider请求。最后旧CLI只读currency=CNY；成功fixture清理。全过程原始日志与执行脚本另存，不是所有历史资料快照、Preview图片渲染或物理旧GUI输入验收。

回退后两包digest和strict签名保持，无匹配实例。初次postcheck误用了猜测的Preview ID com.reasonix.desktop.preview，该检查不足以证明Preview已退出；其记录保留为artifacts-before-bundle-id-correction.json。随后从实际plist读取 io.reasonix.desktop.preview 与官方 com.wails.reasonix-desktop 并重新检查，均已停止，正确结果见artifacts.json。没有用旧错误断言作为最终证据。

## 仍待完成

本轮补齐当前包上述全屏与回退范围，不继承旧候选.env、系统通知、权限/多显示器等结果。可信页面始终未Finished的可见恢复、物理菜单/快捷键/IME/AddToChat、真实文件/目录确认/保存、通知banner/click/deny-regrant、钥匙串人工授权/取消/旧服务、混合缩放/拔屏/无活动屏幕回归、任意官方旧版自定义共享目录互斥和历史窗口间歇问题仍未完成。仅Windows/Linux沿用延期，无其他新增延期。

D未完整验收，E remote/bot/updater/复杂页未开启。A协议/事件/错误、B旧Global图片/大历史、C实际工具/进程生命周期，以及正式签名公证等仍阻碍发布。未push、正式发布或切换默认下载。归档只含工具/源码/日志/状态与SHA，不含私有HOME/core、钥匙串/原剪贴板快照或二进制。
