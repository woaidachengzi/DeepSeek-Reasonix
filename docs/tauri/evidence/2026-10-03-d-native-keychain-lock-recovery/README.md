# 原生钥匙串锁定拒绝与恢复（2026-10-03）

新增 macOS 显式 ignored 测试，使用生产 PlatformCredentialBackend。必须在单独测试子进程、/private/tmp/reasonix-locked-keychain-* 私有 HOME 运行，验证默认钥匙串为自己创建的 fixture.keychain，目录/实际数据库无符号链接，根目录0700；随机服务命名空间仅写假值，不修改正常用户钥匙串偏好或读真实密码。

初始单测编译成功，但原生执行45秒超时，runner1、normalUnchanged=true，原始失败保留；该轮 stdout 被 capture 至超时且未归档，不能确定卡住步骤或声称锁定测试通过。改为实时阶段日志，并在首次 backend 访问前禁止测试进程的可选授权UI、显式解锁自有夹具，避免夹具创建依赖交互；生产授权策略不变。

stage及添加路径无符号链接检查后的final两个全新夹具各1/1通过。实际 seed/read 后锁定，读取、替换和删除均返回 STORAGE_ERROR 脱敏恢复提示；解锁后原值保持，后续替换、删除和缺失读取通过。每次正常用户 default/search/偏好hash 前后不变；私有默认仍为夹具。native logs 保存完整阶段，未输出密钥。最终常规钥匙串回归23通过/2ignored，Clippy tests -D warnings及locked离线构建通过。

这证明锁定且禁止可选交互时原生后端拒绝与恢复；不是当前60076bd5安装包真实设置页的锁定弹窗、用户取消/授权拒绝或通知权限验收。尚需真实包GUI保存/替换、managed/cross-profile GUI与权限取消等验收。Wails基线服务 reasonix 与旧Preview com.reasonix.desktop不同，未将本测试算Wails凭据迁移。D窗口失败继续保留，D未完成/E未开启。
