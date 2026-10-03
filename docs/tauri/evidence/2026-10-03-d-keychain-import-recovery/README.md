# 钥匙串迁移拒绝的精确恢复提示

上一轮实际 UI 已发现：已有凭据时再次迁移不会覆盖，但通用文案误导为系统授权失败。宿主 import command 现在只返回固定 code：existing_credential、missing_legacy_credential、unavailable；未知诊断、非主窗口拒绝和异步失败不携带原文。界面按白名单码选择中/繁/英恢复提示，明确已有凭据可通过手动填写/保存替换，缺失旧 Preview 凭据需重新填写；未知错误仍使用通用系统恢复提示。

Rust keychain 回归23通过/1忽略，包括固定错误码序列化、带私密内容诊断/追加字符串的脱敏；设置页回归通过，覆盖已有/缺失/未知拒绝、输入保留、未刷新/未覆盖与成功/并发路径。首次默认旧Node不支持--import失败保留；相同测试Node26通过。前端完整生产门禁、app/DMG构建和只读安装/strict ad-hoc签名通过。新候选host60076bd5，sidecar29d985c7；安装与DMG完整摘要见install.json。[新包门禁](../2026-10-03-d-keychain-copy-package-run/result.json)独立记录，未沿用旧包成功记录。

验收启动器新增受限 --reuse-root：仅允许已正常结束、真实无存活Preview/own sidecar/readiness的owned0700 explicit fixture；保持相同app及credential identity，旧日志/回执先归档，当前成功回执清除，信号退出单独记录。拒绝非观察模式、非夹具路径、相对路径均实际核对。原有fresh defaults不变。

旧249dfd15同私有档案重启确实建立host27634/sidecar27643且身份保持，但CUA两次应用读取和一次inventory超时；独立进程/原生状态仍存活且page finished。未据超时重启，精确SIGTERM清理得到kernel signal15、sidecar/readiness0，runner1。现场保存于old-candidate-restart-timeout；不算正常UI Quit、凭据恢复或删除通过。未锁登录钥匙串/读真实凭据。

新错误提示尚未真实新包GUI复验，同档案钥匙串恢复/删除、锁定/拒绝、managed和Wails服务兼容仍待验。当前生产候选已更新，旧249窗口/其他门禁不能升级为新600的通过。D窗口稳定性等仍未通过，E未开启，正式签名/公证/发布未执行。
