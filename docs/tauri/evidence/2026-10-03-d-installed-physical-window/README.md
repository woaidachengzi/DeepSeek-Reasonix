# 当前真实安装包原生窗口交互

当前候选 host `bfb47fd84156acba8e9b530889d6a215145291d0a0722ab5f2f0518f2a17b4fa`。严格签名验证后，LaunchServices 启动独立 explicit HOME/TMPDIR/core/cache 档案，PID17006/sidecar17014。只读 interactive-observe 在产品窗口采样状态，不代执行 UI；CUA 成功绑定确切安装路径。

实际 CUA 打开 Window 菜单并点击 Minimize（performMiniaturize:）、点击原生黄色最小化按钮、按 Cmd+M：窗口仍 visible，nativeMiniaturized=false，will/did-mini/did-demini 均0，AX未出现最小化完成。保留每次后状态。本样本不能证明每次内部 selector 的执行计数，不能拿动作尝试本身作成功验收。

同进程 View→Toggle Full Screen 两次：进入后 style mask32783→49167（FullScreen位），几何2560×1640@(640,142)→3840×2160@(0,0)，AX原生按钮消失；退出后 style mask与几何精确回到初值，visible/key=true，AX按钮恢复。实际菜单进入/退出这一条切片通过，未覆盖按钮/快捷键与稳定性矩阵。

实际 Cmd+Q 后，不再读取死亡绑定。确切 kernel exit0/open0，runner terminal0，sidecar与ready无残留。结果与安装包哈希见 summary.json、result.json、各状态快照和动作记录。CUA状态过渡两次防过期拒绝，均重新读取状态后才继续，未绕过；一次 sandbox ps拒绝后，用获准的确切PID查询确认进程存活。

D 最小化门禁保持失败，整体D/E、Preview稳定性未宣布通过；生产源码未修改。
