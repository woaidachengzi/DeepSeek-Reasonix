# Wails 旧钥匙串迁移实现与验收缺口

Wails 1.38.3 的 credentials.go/credentials_keyring_default.go 使用 service `reasonix` 和 provider 的 `APIKeyEnv` 账号。旧 Preview 使用其他 service/账号；两者独立展示，新增“迁移 Wails 旧钥匙串”，原入口改为“迁移旧 Preview 凭据”。现代 Wails 的 Global .env 暂存不等于旧钥匙串，尚未据此迁移或验收。

账号由鉴权 sidecar 路由读取当前全局配置并按 provider 验证，renderer 仅提交 provider/source，不能指定任意钥匙串账号。路由不返回凭据或私有路径，不修改配置/.env；仅主窗口可调用导入命令。已有目标凭据在读取来源前拒绝，来源保留；目标写入与 core 同步共用现有事务/回滚。前端请求锁、固定错误反馈、草稿保留和导入后状态刷新均覆盖。

通过：Go TestProvider 定向回归；真实新 bridge 的 Rust keychain 24 项（3 项 native ignored，不算通过）、账号格式回归；组件 API key 测试、完整 test:tauri/typecheck、严格 clippy；完整 app/DMG 构建、只读挂载复制安装、严格 ad-hoc 签名。Go 首次默认缓存写入因沙箱失败，使用私有 GOCACHE 后通过。

新包 host `cbb590ba4a2dd45afa322508387ddc07383993550bd9a48e17fedb4fc617a384`，sidecar `cd7262010182d96253c0aa6e6a8038b18ce9f194e35cbfac5cc75106a1ba46b3`，DMG `82e900855c73cfd3799cff5c82e9bc864ac3e10d45b20068dbaeb41d2e8a8276`。构建后只补充 cfg(test) 的原生诊断和真实 bridge 映射断言；生产实现未再改。源码快照注明当前状态，真实包门禁有其运行时独立快照，不混用二者。

五个全新自有私有 keychain 的原生测试均失败，未进入导入或启动 sidecar。第一个 CLI fake record 使用测试二进制 ACL；第二个明确解锁；第三个仅自有随机假账号预授权；第四个验证原生 User domain 默认路径；第五个增加指定同一私有链的原生直接读取。最后默认选择 status=0，直接读取 status=-25293，platform load 返回固定 unavailable。不能据此判定系统锁定/ACL/环境的根因，不能宣称 Wails 原生导入成功。生产访问交互没有禁用；仅测试进程用 RAII 临时关闭交互并恢复。正常用户 default/search/preferences 前后均一致。

仅归档日志、状态/元数据和测试代码，不复制钥匙串 DB、凭据文件或假凭据私有 core。失败夹具保留 /private/tmp 原路径；没有绕过系统授权窗口。真实 Wails 凭据授权/取消、准确目标值、managed/跨档案、现代 .env 来源及真实 GUI 仍待验。

[六项新包门禁](../2026-10-03-d-wails-keyring-package-run/README.md)另存。D 尚未通过，E 尚未开启；正式签名/公证、完整回退与 A/B/C 缺口继续阻碍发布。
