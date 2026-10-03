# 当前 d1 候选：预先解锁私有钥匙串的 GUI 保存、替换、重启与删除

实际从只读 DMG 安装的 d1c9fd5a 候选，使用独立 explicit HOME 和 fixture.keychain，启动前明确解锁。正常用户钥匙串没有作为默认目标或查询目标。测试值均无真实服务访问能力，未点击测试连接。

CUA 实际设置页首次保存、替换均显示已保存到钥匙串/已就绪并清空输入。CmdQ 后使用原工具 --reuse-root 正常重启，身份不变、仍已就绪且不回显。随后实际点击删除，显示已删除/未配置；最终仅查询指定私有 service/account 的元数据，exit 44 确认该项不存在，不读取秘密。两次退出均 kernel/open 0，sidecar/ready 无残留。旧 Preview keychain.dat 摘要、权限、mtime 保持。

observations.json 是本会话 CUA 操作与返回 AX/截图观察的转录摘要，并非自动测试输出；截图未另存文件。替换成功反馈及重启就绪已观察，但没有原生读回替换值，因此不宣称精确值读回验收。保存原失败现场和先前锁定等待记录，不以本次解锁条件成功认定锁定问题消失。

managed/跨档案 GUI、真实系统授权取消、处理中提示 GUI、删除后再次 GUI 重启和 Wails service 兼容仍待验。D/E 未完成，未正式发布。
