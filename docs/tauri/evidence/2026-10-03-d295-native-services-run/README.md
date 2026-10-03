# d295 原生服务首轮失败

dialog-cancel 组：managed 菜单与 clipboard-native 主 IPC/系统代次检查通过，但 fixture 严格原件格式顺序恢复失败，因此 exit1；真正 dialog-cancel 阶段尚未执行，explicit、links、notifications 均未开始。原日志/脚本快照保留；恢复原件仅在私有恢复目录保留，未复制入仓库。只读比较证明全部类型和每项字节一致、UTF8/UTF16顺序互换，仍不计完整恢复通过。修复工具后另行完整执行，见 ../2026-10-03-d295-native-services-fixed-run/。
