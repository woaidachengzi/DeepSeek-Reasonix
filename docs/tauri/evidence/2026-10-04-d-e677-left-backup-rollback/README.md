# D：e677 当前包配置备份实际回退（左屏）

源HEAD b5e870786；本轮仅工具改动，产品仍为e677候选，精确host/sidecar与官方Wails CLI身份在artifact.json。官方CLI SHA必须为5b1ab31424d45c8bf3cfe6a60b11da527df2252aee963d1c0c56352f57945962，否则启动前拒绝；没有把源码构建或其他CLI当官方包。

## 工具修改

配置备份回退可选受限六字段窗口模板，传给每个私有Preview profile launch。首次exclusive创建0600window-state；普通重启/显式档案沿用已有完全一致的normal state，错屏、额外字段或symlink拒绝且不覆盖。native interactive-observe只提供读观察，原profile import探针和控制文件仍负责动作；每次导入动作开始前require实际native geometry等于模板，并独立保存phase/hostPid/geometry固定回执。缺快照/位置错仍失败；先清旧观察文件，不能用上次启动快照过关。不扩大产品权限，不复制用户状态。

默认未传模板的调用行为保持，历史全资料回退工具仍按原参数调用。本次选左屏x=-3200/y142/2560×1640/scale2，import/restore/explicit三次原生位置先通过，之后才开始对应profile操作。两项新增guard回归通过：不同或扩展状态与symlink在Popen之前拒绝，原件保持，不创建core；原4项通用模板回归亦通过。真实包正向验收覆盖三phase，不用纯mock宣称启动通过。

## 实际回退

当前包真实import生成唯一原配置备份；Preview已停止、其source sidecar无残留之后，将backup以exclusive/0600复制到新私有Wails目录。官方Wails1.38.3嵌入CLI config currency退出0并输出CNY，恢复配置模型native-import/alpha。backup与restored SHA均30c2e6c6771f68e26613c91ca621e07471dc369998ec7419de1013ea0e61b36b。恢复目录在CLI查询前后不变；Preview与备份树不变；原Wails fixture也在全部Preview import/restore/explicit及官方CLI读后不变。保留原import/restart身份及real bridge设置/配置与backup0600/sidecar就绪清理门禁。

整轮exit0、result passed、恰一次backup回读；三个host正常退出清理，postcheck无匹配Preview运行实例、包strict/deep签名0、host/sidecar SHA保持。未归档HOME/core/原配置/凭据，只归档固定回执和工具。CLI输出来自固定本机canary配置，不含用户资料。

仅证明单个配置backup实际应用与官方CLI只读恢复，**不证明官方GUI全部历史/Global附件/检查点回退**。最小化/CUA菜单及间歇窗口稳定未解决；D未完成、E完整验收仍待；A/B/C及正式DeveloperID/公证等缺口按清单保留。Windows/Linux沿用用户延期，未新增其他延期，未push/发布/切换默认下载。
