# 钥匙串真实等待现场与提示修复（2026-10-03）

60076bd5旧当前候选，私有explicit档案 _0une03g 重启：真实设置页输入不可认证假值、点击保存后按钮持续禁用，草稿仍为遮罩显示，没有成功或失败提示。CUA全局清单超时并重置不能视为应用退出；独立确认PID32207仍活，1秒只读采样定位生产保存线程在SecKeychainAddGenericPassword/Security服务等待。指定私有fixture.keychain查询statusBits2/unlocked=false。显式解锁自有夹具返回0，但等待请求未完成。SecurityAgent实际运行；CUA明确拒绝操作com.apple.SecurityAgent，未查看其弹窗，不能确定弹窗内容或用其他输入技术绕过限制。

真实CmdQ后kernel0/open0，sidecar32214/readiness均无残留。保存/替换/重启验收未通过，保存是否发生不能据按钮禁用推断。旧成功迁移及删除证明不替代本轮。screenshots已在工具会话返回，未独立归档；actions.json记录输入/点击及限制，样本不含真实密钥。

修复设置页等待反馈：中英繁显示“正在处理钥匙串请求。若出现系统授权提示，请完成或取消。”，沿用每provider状态，不添加原生重试或假超时、不释放并发写保护、不改变权限。真实组件回归验证60秒等待提示不消失、草稿保留、写入仍一次、完成替换为结果；原有拒绝/迁移恢复回归也通过。

首次构建目录选错已保留；正确前端完整生产门禁及原生app通过，但普通执行环境DMG hdiutil设备未配置失败。允许磁盘映像操作后完整app/DMG构建通过；只读挂载复制到私有安装目录并卸载，strict签名通过。新host d1c9fd5a，sidecar29d985c7，DMG2be16391，详见install.json。新包package/lifetime门禁独立记录在 ../2026-10-03-d-keychain-pending-package-run/result.json，两项均退出0：双档案package、8次SIGTERM/SIGKILL闲置/流式及重启清理通过；未把600旧包的通过升级为新包证明。

新等待提示尚未取得真实安装包GUI显示证据；真实保存/替换/重启、managed/cross-profile GUI、系统授权取消等仍未完成。窗口最小化仍失败，D未通过/E未开启；正式签名、公证、发布和默认下载切换均未执行。
