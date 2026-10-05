# 单实例运维

本项目使用单服务实例及 SQLite WAL，不支持同时运行多个实例服务同一数据库。升级和恢复时先停止进程托管、应用及所有使用相同配置/数据库的写入者；等待有效 Worker 租约释放或过期。崩溃后租约最长约30秒。备份恢复工具不会停止服务或自动切换部署。

## 私有恢复包

Python 3.9+ 标准库提供 `scripts/ops-backup.py`。使用 SQLite Backup API 生成独立完整数据库，包含已经提交但尚在 WAL 中的记录；不用复制主DB文件来替代一致快照。配置和数据库需要停写后成套保存，`--service-stopped` 是操作者对停止写入的声明，工具额外拒绝有效实例租约，不能替代确认所有进程已停止。

先创建源目录外的私有备份父目录。以下以 `/srv/aimangebot` 为部署目录举例，实际目录由操作者指定；目标包目录必须不存在：

```bash
mkdir -m 700 /srv/aimangebot-backups
# 先按实际进程托管方式停止服务及其他写入者
python3 scripts/ops-backup.py backup \
  --database /srv/aimangebot/pr_agent.db \
  --config /srv/aimangebot/config.yaml \
  --binary /srv/aimangebot/aimangebot \
  --output /srv/aimangebot-backups/release-before-upgrade \
  --service-stopped

python3 scripts/ops-backup.py verify \
  --bundle /srv/aimangebot-backups/release-before-upgrade
```

源配置必须只有所有者访问权限，普通文件不接受 symlink；输出不得放进源文件父目录内部。包目录0700，数据库/配置/manifest0600，可选二进制0700。manifest只包含版本、时间、固定文件名和大小/SHA256，不包含配置内容或源路径。配置前后校验变化时失败；完整manifest最后写入，失败清理本次自建目录。SQLite目标执行integrity_check，并转换为独立DELETE日志形式；服务启动仍会使用WAL。哈希证明内容一致，不证明备份来源真实性，只使用受信任的私有本机备份。

恢复只写到不存在的新目录，不覆盖任何现有目录，不执行包中的二进制或自动启动：

```bash
python3 scripts/ops-backup.py restore \
  --bundle /srv/aimangebot-backups/release-before-upgrade \
  --output /srv/aimangebot-restored
```

恢复完成后核对版本、监听地址、凭据与项目配置，在停服务窗口以兼容二进制从恢复目录启动；检查登录、项目权限、历史任务与人工复核。保留原目录直到新实例验收完成。恢复包不会清空任务、评论状态或租约；服务自身按固定策略恢复未完成记录。回滚需数据库、配置及兼容二进制成套恢复，旧版权限实现不能直接读取已有跨仓库结果的新数据库。备份包含会话、凭据和私有仓库证据，存放在本机私有目录，不加入Git或公共报告。

工具默认30秒，在复制/SQLite备份步骤检查截止时间，可通过全局 `--timeout 60`（1–300秒）调整。输出只有成功/失败状态和文件数量，失败不打印秘密配置、数据库记录或原始异常；必要时操作者本机检查权限、有效租约、SQLite完整性和磁盘空间。目录fsync与文件fsync提供落盘请求，不宣称在任何文件系统/硬件故障下保证恢复。manifest缺失、未知文件、symlink、校验和不符、损坏数据库和权限不私有均拒绝验证。

## 验证边界

```bash
python3 -m unittest discover -s scripts -p test_ops_backup.py -v
```

这些隔离fixture测试覆盖 WAL 数据、源文件保持、私有权限、租约拒绝、配置变化、哈希/损坏/manifest路径及不覆盖恢复，不能替代实际部署停服、恢复启动和业务数据演练。原生二进制恢复演练、进程托管、就绪监控和容量实证仍在本轮交付中完善；当前文档不将它们标记为已完成。
