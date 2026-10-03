# 窗口持久化坐标对照：固定历史种子，未复现2px漂移

上一轮已保留74候选完整窗口48/48通过与成功激活时序，本轮不重复完整矩阵。针对历史独立explicit对照保存y344→342的问题，使用原失败档案归档的window-state.json及normal.json固定种子，分别新建managed与explicit私有档案，各运行原restore-normal后second-instance，维持原焦点、几何及精确持久化断言。没有生产代码改动、偏移补偿、容差放宽或自动重试。

源码核对：锁定tao0.35.3 set_outer_position与set_inner_size分别投递主队列异步设置；当前Reasonix restore()在调用返回后解除restore_pending，setup与Focused路径随后capture。存在读取过渡状态的可能；未证明它导致历史2px漂移。Tao getter与setter的坐标换算均使用主屏高度，数学上互为对应，不支持直接硬编码±2px。锁定依赖源码片段所在完整文件归档为tao-window.rs及tao-util-*；其许可证头保留。

只读activation-timeline.swift补充CoreGraphics窗口列表元数据，仅为Preview进程保留窗口ID/PID、layer、onScreen和bounds；不复制标题、其他应用窗口详情或像素。此次能实际读到主窗口layer0边界，不能以空列表断言没有窗口，隐私限制/启动过渡均可能影响可见数据。原有run loop与工作区事件观察维持。临时控制脚本并行读取私有window-state.json，仅记录JSON内容变化，每50ms采样；不是每次写调用的日志，短暂过渡可能未采到，不能声称捕获所有中间写入。

固定已安装host74fefe10、sidecar16e3761d，准确摘要与源码/种子hash见identity.json；不是新构建包。managed与explicit各2阶段，共4/4通过，4个主宿主kernel exit0，观察器均由所属调用终止（-15）。各阶段state-before/after保持y344及x920/2000×1400/scale2完全一致，采样未看到JSON值变化。CG窗口主边界X460/Y172/Width1000/Height700，与该次2倍缩放后的原生几何一致；坐标单位解释仅针对当前双屏scale2现场。测试后严格签名通过，无匹配Preview进程。

未复现历史2px漂移，不算修复或稳定性达标；历史y342与间歇失焦仍保留。没有必要反复跑同一成功种子以覆盖失败，下一项需要在保存/退出路径捕获实际native frame与持久化前后值，并区分异步restore过渡和退出时窗口变化。D未完成/E未开启，其他物理UI、授权、多显示器及A/B/C缺口按清单保持。未发布、推送或切换默认下载。

归档不含私有HOME、钥匙串、用户剪贴板、观察器二进制；原窗口与内核JSON保留，manifest覆盖原始证据（不含说明自身）。观察器Swift编译及实际4阶段通过；源码diffcheck与归档校验通过。此为定向诊断，不替代完整当前候选验收。
