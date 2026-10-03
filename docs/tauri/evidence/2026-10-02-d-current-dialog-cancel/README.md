# 初始化失败，原生门禁未开始

系统 `/usr/bin/python3` 为 Python 3.9，runner 在 artifactSha256 计算时抛出 `AttributeError: module hashlib has no attribute file_digest`。只完成签名和脚本/补丁快照，未生成 result.json、未启动任何 native gate。该失败目录保留；修复和实际运行见 `../2026-10-02-d-current-dialog-cancel-run/README.md`。
