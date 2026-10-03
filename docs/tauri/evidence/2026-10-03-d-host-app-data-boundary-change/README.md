# 两种档案的宿主应用数据目录保护

修复前 configure_preview_profile 的显式分支先返回，托管分支仅拒绝 reasonix-core 叶子链接，create_dir_all(app_data) 会跟随上层应用目录链接。新增源码回归修复前 exit101；d295实际包在应用数据目录链接到私有 .reasonix 的场景 exit0，没有拒绝，新增 reasonix-core/凭据身份/SQLite/窗口状态/theme-assets 等宿主文件，现场 /private/tmp/reasonix-app-data-boundary-612_3vtc 保留。仅使用固定假数据，原件内容没有归档。

现在先校验显式 override，再对两种档案共同检查 app_data 解析后的已知正式版目录重叠、应用目录叶子的普通目录属性，然后才能创建 UI/宿主元数据或启动sidecar。独立目录0700创建，已有目录权限不擅自修改；核心配置和环境路径保持原词法一致性。新增回归覆盖链接、悬空链接、冲突文件及父路径别名，档案回归21项、Clippy、完整前端/sidecar/Rust/.app/DMG构建通过。

实际安装新候选 host 0e607a62d9fc34d265e6a5964bb82c331b94953569bc6e69a81edcf3fc2b726f；sidecar29d985c7af09baefec4308339429eabfe921896c47db6f4c30584b742b7efd39；DMG3e9fb470779026fdff6b64016008675bc41fc89400a2a9fbeab500642ce23185。严格ad-hoc签名通过，不是正式签名/公证。

同一新包 app-data-boundary/parent-resolution/explicit-boundary/package/boundary/profile 六组真实门禁通过，见 ../2026-10-03-d-host-app-data-boundary-package-run/。不证明任意旧版本自定义共享目录互斥、对抗并发路径替换、完整备份应用或所有D功能。新包窗口/原生服务/lifetime/startup/GUI须重新验收；d295最小化失败及锁屏中断的目录确认仍未关闭。
