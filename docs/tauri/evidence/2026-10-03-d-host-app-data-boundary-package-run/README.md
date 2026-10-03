# 0e607a62 新包六项档案门禁通过

原 verify-installed-d 顺序 app-data-boundary、parent-resolution、explicit-boundary、package、boundary、profile，全部exit0。每阶段后核对固定真实包hash、严格签名和全部脚本依赖hash，来源patch及未追踪代码已冻结。

app-data-boundary：managed/explicit 各五场景，应用数据根链接到正式版、其他私有目录、悬空目录、普通冲突文件和父路径别名。全部在UI/sidecar前exit1并给出固定恢复提示；正式版原树及其他目标目录metadata/bytes、链接/冲突项metadata保持；无子进程或readiness。原d295反例保留。

其余五组核对Rust/Go alias/..词法位置一致、原有override拒绝、普通双档案凭据身份/401/Global工作区/通知授权读取/正常退出、managed核心根拒绝及配置修改/重启/备份原件保护。profile里的restore是配置修改持久化，不是应用备份回退证明。完整窗口、物理GUI、进程异常与原生服务等尚未在新包复验；不得继承d295/d1证据为当前已通过。
