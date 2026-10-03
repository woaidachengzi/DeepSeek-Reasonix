# 当前真实包钥匙串成功迁移（2026-10-03）

当前安装包 host 249dfd15，新 explicit 私有档案/私有 HOME。仅迁移旧 Preview 文件中的假 `api_key_deepseek-flash`，未填写真实 API key、未测试连接或发送模型请求。CUA 设置→模型服务，首卡初始未配置；实际点击迁移后变为已就绪、显示已保存到钥匙串，输入框不回显凭据。再次点击被拒绝，已就绪保留，但通用错误文案不能准确说明已有凭据，需修正。

私有隔离依据 [Apple 偏好定位源码](https://github.com/apple-oss-distributions/Security/blob/main/OSX/libsecurity_keychain/lib/DLDBListCFPref.cpp) 和 [私有创建搜索列表规则](https://github.com/apple-oss-distributions/Security/blob/main/OSX/libsecurity_keychain/lib/StorageManager.cpp)。只写临时 HOME 的偏好 plist；从未调用用户默认值/搜索列表设置或锁定登录钥匙串。普通 sandbox 探针创建返回参数错误，失败原样保存；提升后的相同探针通过私有默认写入、指定夹具查找、删除与不存在检查。两者及实际 UI 前后正常 HOME 的默认/搜索列表输出和偏好文件摘要相同。

旧文件 sha/mode/size/mtime 完全保持。实际 CmdQ kernel0/open0，sidecar/readiness 无残留。夹具路径保留于 actions.json，供后续同档案重启和清理；不归档钥匙串二进制、不读取真实用户凭据。该切片并非锁定/拒绝、重启、删除、managed 档案或 Wails 服务兼容性全验收。D/E 未完成。
