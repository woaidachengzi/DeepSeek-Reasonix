# 当前包钥匙串不可用反馈与独立原生隔离测试

当前真实候选249dfd15、sidecar29d985c7，隔离explicit HOME/core/cache，PID24725/24732。私有app-data仅生成假值keychain.dat，目标服务按随机credential-profile身份隔离。初次helper错误种入deepseek；看到真实默认provider为deepseek-flash后，仅更正自有旧文件且在任何凭据操作前保存原始manifest，之后才执行实际迁移。未填写真实API key或调用外部模型。

CUA实际设置页→模型服务，首项deepseek-flash未配置/输入为空；实际点击迁移旧凭据后，截图持续显示“迁移失败，请检查旧凭据与系统授权，或重新填写。”，首项仍未配置/输入为空。AX树在该长页面只返回前段，错误的可见性依据CUA本轮原生截图，未伪称AX完整捕获。控件坐标依据刚读取的本应用截图，未操作其他应用。

host启动已报告credential restore失败。只读security default-keychain -d user：私有HOME退出1、无默认钥匙串；normal HOME退出0、报告login路径。没有改变钥匙串默认值、搜索列表、锁定状态或权限。因此本轮覆盖实际存储不可用的错误反馈，不能冒称系统授权拒绝/锁定或迁移成功。

旧文件hash/mode/size/mtime前后精确相同；实际CmdQ kernel0/open0，runner0、sidecar/readiness无残留。退出后不读取死CUA绑定。另一个当前源码显式native_keychain_profile_isolation_and_cleanup测试使用normal HOME与随机两个命名空间/固定假值，真实读写、替换、隔离、删除1/1通过；此unit不是当前包GUI成功迁移证据。

源码边界：host只允许main窗口凭据命令，renderer无钥匙串/fs/shell通用权限；当前旧Preview复制使用com.reasonix.desktop，目标使用按档案生成com.reasonix.desktop.profile.*。冻结Wails核心旧keyring服务为reasonix（credentials.go），不能把旧Preview文件夹具当成Wails凭据兼容性全验。迁移不覆盖已有目标、保留旧源的源码测试已存在，本轮实际GUI未完成其成功链路。

下一步构造拥有自身安全存储的隔离夹具，再验GUI迁移/重启/拒绝覆盖/删除；不得通过锁定用户login钥匙串或改用户默认存储来制造场景。D锁定/授权拒绝、通知点击/拒绝、窗口稳定性及其余矩阵仍未完成，E保持待D。
