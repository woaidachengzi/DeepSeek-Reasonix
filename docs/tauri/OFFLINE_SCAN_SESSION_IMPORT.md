# 离线审核导入未认领会话

`reasonix-session-import` 只在**新建的隔离恢复副本**中登记扫描得到的会话。它不会修改来源 profile、原始 transcript 或正在运行的 Tauri Preview。旧 Wails 版本不使用新的 profile 锁；制作快照前必须退出所有使用来源资料的 Wails、Tauri 和 CLI 写进程，并在整个快照过程保持停写。`--writers-stopped` 是操作者的明确确认，不是对旧进程的自动检测。

下面的 `PROFILE` 是待核对的会话资料根目录（其中含 `sessions/`），`CATALOG` 是对应的 `workbench-sessions.json`，它必须位于该 profile 外。备份和恢复目录也必须位于来源 profile 外，且事先创建。

```sh
go run ./cmd/reasonix-session-import snapshot \
  --profile "$PROFILE" --catalog "$CATALOG" \
  --out-parent "$BACKUP_PARENT" --writers-stopped
go run ./cmd/reasonix-session-import stage \
  --snapshot "$SNAPSHOT" --out-parent "$STAGE_PARENT"
go run ./cmd/reasonix-session-import list \
  --stage "$STAGE" --selection-out "$SELECTION"
```

命令以 JSON 输出新建的 `snapshot` 和 `stage` 绝对路径。`list` 输出扫描候选与诊断，并以独占创建方式写入选择模板。每个候选默认 `selected: false`，`title` 和 `workspaceRoot` 为 `null`。逐项核对文件名、ID 和 SHA-256 后，把确实要导入的条目改为 `selected: true`，为其填写标题和项目归属。明确不设置标题或项目时填空字符串 `""`；省略字段或保留 `null` 会被拒绝。ID 必须与 `tauri-<ID>.jsonl` 文件名往返一致，不能手工另配 ID；不符合命名规则的文件仍在诊断中，不能通过此流程认领。选择模板、阶段标记和审核计划以独占创建、限制权限的方式落盘；读取时会校验普通文件、打开前后文件身份及大小，Unix 类系统还会拒绝组/其他用户可读写的文件。不要手动放宽这些文件的权限。

```sh
go run ./cmd/reasonix-session-import review \
  --stage "$STAGE" --selection "$SELECTION" --plan-out "$PLAN"
go run ./cmd/reasonix-session-import apply \
  --stage "$STAGE" --plan "$PLAN" --approve "$REVIEW_SHA256"
```

`review` 只读检查每个选中项，输出计划路径、条数和批准用 SHA-256；`apply` 要求计划文件的 SHA-256 与 `--approve` 完全相同，并在持有副本的 profile 锁期间重新核对 catalog、扫描来源、ID、标题、项目归属、顺序、transcript SHA-256 和路径冲突。任一选中项变化则整份计划不写入；成功时仅向 `$STAGE/profile/desktop/session-state-v1.sqlite` 登记身份行，transcript 字节不变。已登记的 ID 不会被扫描结果覆盖。计划为一次性操作；成功后重复提交会因候选已登记而拒绝。

恢复副本位于 `$STAGE/profile`，原始资料和快照保留。核对副本中的身份目录与 transcript 后，再按独立的资料切换流程决定是否使用它；此工具不会把副本自动合并回运行中的 Preview，也不会替换原 profile。

受管的 Tauri Preview 也提供“扫描并审核未认领 transcript”入口。它只在隔离的 Preview profile 中展示路径不外泄的候选 ID、文件名和 SHA-256；用户逐项选择并确认标题及项目归属后，桥接进程会重新检查 catalog 与 transcript，再一次性登记选中项。此入口不适用于设置了自定义 `REASONIX_HOME` 的非受管 profile。若来源仍可能被旧 Wails 版本或其他进程写入，旧 writer 不遵守新 profile 锁，必须先停写并采用上述离线快照与副本流程；不要对运行中的来源 profile 做并发导入。
