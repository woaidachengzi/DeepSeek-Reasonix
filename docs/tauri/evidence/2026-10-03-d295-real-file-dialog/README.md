# d295 真实文件选择通过，目录确认被锁屏中断

当前 d295 显式私有普通档案，实际 CUA 点击添加文件 → AppKit Open → CmdShiftG 输入私有中文/空格路径 → Open。文本附件准确显示「附件 中文.txt 待发送」与完整私有路径。再次添加文件 → 实际 Cancel，草稿已有附件与可发送状态保持。未发送消息，附件文件 bytes hash/mode/mtime 保持，见 fixture/observations。

目录选择器已实际打开；确认 Open 时 CUA 明确报告 Mac locked，需要用户手动解锁。未证明确认成功、持久化或重启恢复，未尝试绕过锁屏。观察 runner300秒没有收到真实Quit的内核回执而exit1，精确私有进程已由runner清理。不能把文件切片成功升级成完整目录/退出验收；后续须在解锁后继续，并使用当时实际候选。
