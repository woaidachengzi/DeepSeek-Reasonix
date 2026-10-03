# 当前60076bd5真实钥匙串重启与删除验收

全新explicit私有HOME/档案/fixture.keychain，仅旧Preview文件假api_key_deepseek-flash，无真实API key、无测试连接、无模型请求。原生工具inventory先确认恢复后才启动；保持准确PID和origin，三次正常退出后均不读死app绑定。

第一次实际设置页迁移成功，已就绪/已保存到钥匙串，空输入框不回显。再次迁移准确显示已有凭据不会覆盖及填写新密钥保存的解决办法；deepseek-pro无旧值准确显示未找到旧Preview凭据及重新填写解决办法。候选新错误码/文案经过真实主WebView IPC与OS后端验证。

同包同root重启，身份不变，设置页已就绪且输入框仅配置占位、主模型选择器启用。实际删除后未配置/密钥已删除；再次删除明确不存在。第三次同档案启动后仍未配置且模型选择禁用，保留的旧文件没有被自动导入。随后只查指定私有keychain/随机profile service/account，Security返回项目不存在，未读取密码。

28531、28639、28729三轮均实际CmdQ kernel0/open0，sidecar/readiness无残留；前两轮日志/完整回执已自动归档于launch-PID，第三轮在根目录。旧文件hash/0600/size/mtime完全保持；正常用户默认钥匙串/搜索列表输出和偏好hash与较早归档基线相同（不是宣称本轮另有新起始snapshot）。安装bundle严格签名/hosthash复核保持60076bd5。

actions.json记录实际CUA结果。屏幕图像在会话工具结果中，未单独保存；AX树截断，状态/按钮文案结合实际截图确认。夹具保留供锁定/拒绝等后续验收。此为explicit旧Preview假值入口的真实成功/拒绝/重启/删除切片，不等于所有钥匙串入口或Wails reasonix服务兼容性；managed、UI保存/替换、跨档案GUI隔离、OS锁定/拒绝仍待验。D窗口稳定性等未完成，E未开启。
