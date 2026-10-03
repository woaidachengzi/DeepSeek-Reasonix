# 私有固定假 Wails 来源的 CLI 读取对照

保留private keychain pikhkolf的自有root/700/文件身份检查，显式仅查询该链，元数据只在进程中筛选Wails fake随机账号，不复制密码或属性。unlock-keychain使用已知fixture密码exit0，security find-generic-password精确固定dummy匹配true/exit0；正常default/search/preferences摘要前后保持。

该来源值存在且CLI可读；此前同fixture Rust Security-framework原生status=-25293失败不能归为缺少来源。未重试原生测试、未修改ACL/partition或正常凭据、未绕过SecurityAgent。调用方/进程环境仍未确认；不能据CLI成功放行产品Wails迁移或真实授权/取消。
