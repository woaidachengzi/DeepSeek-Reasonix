# 当前候选实际原生窗口与快捷键

只读DMG安装候选host249dfd15、sidecar29d985c7，strict签名经launcher验证。新explicit HOME/TMP/core/cache隔离档案，确切PID23796/sidecar23804，CUA绑定安装路径与该进程的Preview私有origin。仅已内置opt-in只读observer采样，不代执行UI。

Window→Minimize（AX role performMiniaturize:）、AX最小化按钮点击尝试、composer焦点后Cmd+M与全屏恢复后Cmd+M均未观察到nativeMiniaturized=true；will/did-mini/did-demini全0，窗口仍visible。不能把AX动作返回视为目标selector内部实际执行计数，更不能记为最小化通过。

额外观察：截图原生交通灯位置可见紫色共享形状控件，而AX仍列出普通按钮。该控件来源/捕获状态未验证，黄色按钮尝试未证明命中实际黄色按钮；不在用户系统设置中修改共享、权限或其他应用。后续需要控制此观察条件，不能单凭它判定系统共享导致失败。

View→Toggle Full Screen尝试未看到原生fullscreen位或geometry变化，不记成功。随后实际Ctrl+Cmd+F进入：style32783→49167，geometry2560×1640@(-3200,176)→3840×2160@(-3840,0)，AX交通灯消失；同快捷键退出后style/geometry精确回到原值、nativeVisible/key/active=true，AX按钮恢复。这一explicit快捷键进入/退出切片通过，不代替菜单或整个稳定性矩阵。

退出全屏后一次Cmd+M因“用户切换应用”防过期拒绝，未记执行；按照要求getAXState刷新后一次新Cmd+M仍无原生最小化事件。实际Cmd+Q后未再读取死亡绑定；确切kernel0/open0，runner0，无sidecar/readiness残留。所有前后原生snapshot、动作summary与回执保存，不归档用户其他窗口。

生产窗口代码未改，当前包与上一完整门禁为同hash。最小化仍无可靠通过：[完整门禁](../2026-10-03-d-current-window-after-phase/README.md)managed24/24、explicit第9项失败不得被本全屏切片覆盖。D与Preview稳定性未完成，E尚未开启。
