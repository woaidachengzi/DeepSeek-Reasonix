# c71ab66c 与官方 Wails 1.38.3 真实数据回退

smoke-legacy-rollback.py --workspace-data 实际 exit0。当前真实 Preview、官方旧 Desktop/内嵌 CLI 固定摘要验证通过；本次使用官方内嵌 CLI 5b1ab314，其路径按原验证字典核验；二进制/脚本摘要另存；本次不继承a62运行结果，实际exit0、成功fixture正常删除，测试后签名和包摘要另行核对。

官方 CLI 本机 serve 和固定 loopback provider 生成真实 Global 文件、文本与图片附件引用、文件编辑及检查点；旧 GUI 导入前后实际读回历史/tab/model，活跃会话 writer 拒绝，原生正常退出及清理。Preview 导入、配置修改重启持久化与显式拒绝通过；整个私有 Wails 原树字节/权限/mtime保持。

原版继续同会话、解析真实文件/附件并生成第二检查点；最新 code rewind 恢复实际文件 preimage，较早冲突拒绝、原 transcript/附件保持。最后旧 CLI 只读 currency=CNY、固定provider请求计数、配置及 canary 保持。成功私有夹具自动删除，无用户资料/真实provider网络。

这是所生成历史/附件/检查点的实际旧版回退，不是所有资料快照恢复、Preview 图片渲染、物理旧 GUI 点击/输入、任意自定义共享目录互斥或真实钥匙串授权迁移。单份配置备份实际应用有独立回执。D窗口/GUI/钥匙串、A/B/C/E和正式签名/公证未关闭。
