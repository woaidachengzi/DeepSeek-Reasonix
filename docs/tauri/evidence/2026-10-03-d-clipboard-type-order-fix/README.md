# 剪贴板格式顺序恢复修复

当前 d295 首轮独立 native clipboard 主 IPC 与系统值/代次验证成功，但 fixture restore 严格顺序比较失败，整组 exit1，后续对话框/外链/通知未运行。原始失败记录在 ../2026-10-03-d295-native-services-run/，私有恢复原件保留在 /var/folders/j8/bqc5mc190_q6f4ytd9d32hlm0000gn/T/reasonix-clipboard-recovery-euf7n4kf，未复制其内容入仓库。

只读比较证明原件所有格式及每项字节完整一致，但 public.utf8-plain-text/public.utf16-external-plain-text 顺序互换；generation 在读取期间未变化。未认定首轮严格完整恢复通过。当前内容保留，不绕过 ownership guard 强制写入旧快照。

随机私有命名剪贴板固定假数据控制复现：NSPasteboardItem 的内存顺序正确，writeObjects 写入后将 UTF16 提到 UTF8 前；declareTypes + setData 保持原顺序。恢复 helper 对单项改用声明顺序/逐格式写入，仍以全部有序 item/type/bytes 严格比较。捕获前加入独立命名剪贴板恢复预检，无法完整恢复的多项归一化在写 general 剪贴板前拒绝；原件期间代次保护仍保留。

Swift 编译及私有命名板自检 exit0，新增真实 rich text/UTF8/UTF16 顺序恢复回归；Python四项恢复守卫回归通过，其中新增 restore状态3时调用方清理后原件仍保留。生产 host/sidecar 无改动，无须重建；同 d295 包修复后真实门禁另存 ../2026-10-03-d295-native-services-fixed-run/。窗口失败和D/E其他缺口不受本工具修复关闭。
