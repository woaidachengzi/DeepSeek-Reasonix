# cbb590ba 单实例、启动拒绝与 sidecar 生命周期

当前真实安装候选的 identity、startup、lifetime 三组原门禁全部 exit0；包、签名、脚本与源码身份见 [result.json](result.json)。未削弱超时、退出状态或原件保护断言。

identity：私有 managed/explicit 的档案身份与竞争拒绝，具体场景见 identity.log。startup：私有启动边界拒绝/清理，具体场景见 startup.log；不是所有 spawn 前临时目录回收证明。

lifetime：managed/explicit × idle/实际 streaming × 宿主 SIGTERM/SIGKILL，共 8 个场景；kernel 确认 sidecar 退出码 0、ready/启动记录与目录锁清理、原件保持及同档案重启均通过，见 lifetime.log。

这是程序门禁，不是物理托盘/菜单退出、完整窗口稳定性、任意旧版自定义共享目录或全部 D 验收。新包其他原生服务和 GUI 仍待验；D 未完成/E 未开启。
