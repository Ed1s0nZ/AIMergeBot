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

这些隔离fixture测试覆盖 WAL 数据、源文件保持、私有权限、租约拒绝、配置变化、哈希/损坏/manifest路径及不覆盖恢复，不能替代实际部署停服、恢复启动和业务数据演练。原生二进制恢复演练可用以下命令执行；进程托管、就绪监控和容量实证仍在本轮交付中完善。


```bash
go build -o /your/private/build/aimangebot .
python3 scripts/smoke-worker-recovery.py \
  --binary /your/private/build/aimangebot \
  --metadata-preview --lifecycle-preview --comment-preview \
  --verification-preview --backup-preview \
  --app-port 19236 --upstream-port 19237
```

使用空闲非生产端口及任务自建私有构建目录。演练仅连接本机合成GitLab/模型，实际启动应用、SIGKILL、等待原租约过期、停止写入、执行备份CLI、恢复到新目录，再用同一二进制启动。验证原账号会话与受限成员ACL、固定审计结果/检查点、人工复核/关联历史、评论状态保存，且模型/评论请求不重复。模型响应与关联移动报告是明确标记的合成fixture，不是实际PR审计质量证据。最终清理所有演练进程，私有源目录和恢复包保留本机；不要提交或上传这些目录。


## 实例就绪

监控和切换验收使用 `GET /readyz`：200返回 `{"status":"ready"}`，否则503返回 `{"status":"unavailable"}`。接口只查询本机DB，不检查远端GitLab/模型。需要当前Runner已启动且未停止、实例owner匹配且SQLite中的租约未过期；请求上下文设置2秒截止，响应禁止缓存，不返回私有配置或实例标识。旧 `/healthz` 保留兼容，不能用它代替当前租约归属证明。就绪是一次观察，任务继续由事务fence保护。

```bash
curl --fail --max-time 3 http://127.0.0.1:1234/readyz
```

新接口在本轮最终发布后生效；当前运行的旧二进制尚未替换。进程托管配置和自动恢复演练仍需单独验收。

## 服务配置生成与停止

`scripts/ops-service.py` 只生成配置，不安装或启动服务。输出父目录必须0700且目标文件不存在，文件0600；不读取config.yaml或嵌入任何凭据。部署工作目录应由服务用户持有且0700，配置0600，日志仅本机私有。先私下启动完成新数据库bootstrap；已有账号数据库无需把管理员密码写入服务配置。自动重启等待35秒，超过实例30秒租约；正常退出0不会自动重启，异常退出才重启。

macOS使用用户launchd域，示例中的路径由操作者替换，保留空格时仍传完整参数：

```bash
mkdir -m 700 /your/private/service-definitions
python3 scripts/ops-service.py launchd \
  --binary /your/deployment/aimangebot --directory /your/deployment \
  --label local.aimangebot \
  --output /your/private/service-definitions/local.aimangebot.plist
plutil -lint /your/private/service-definitions/local.aimangebot.plist
launchctl bootstrap gui/$(id -u) /your/private/service-definitions/local.aimangebot.plist
launchctl print gui/$(id -u)/local.aimangebot
curl --fail --max-time 3 http://127.0.0.1:1234/readyz
# 备份、升级或撤销托管前先卸载，避免自动重启：
launchctl bootout gui/$(id -u)/local.aimangebot
```

不要把日志或私有配置加入Git。用户launchd域需要已登录的用户会话，不宣称系统开机无人登录也启动。服务错误持续发生时先bootout再查本机日志，不重复安装多个标签争夺同一DB。验证重新出现的PID、readyz和任务状态，不能仅看到launchctl定义就宣称服务可用。

Linux生成systemd配置需明确已有非root用户/组；部署目录授予该用户访问，凭据保留在config.yaml。示例定义需审查并按实际安装路径手动安装，不由脚本sudo：

```bash
python3 scripts/ops-service.py systemd \
  --binary /srv/aimangebot/aimangebot --directory /srv/aimangebot \
  --user aimangebot --group aimangebot \
  --output /your/private/service-definitions/aimangebot.service
# 审查后由管理员安装至 /etc/systemd/system/aimangebot.service
systemctl daemon-reload
systemctl enable --now aimangebot
systemctl status aimangebot
# 备份、升级前停止并确认退出
systemctl stop aimangebot
```

systemd模板定义异常重启、35秒间隔、45秒停止超时、0077 umask、禁止新增权限；没有声称Linux部署已实测。需要独立的就绪监控，管理器进程状态不等于业务健康。当前1234服务尚未切换为本配置；隔离自动重启演练及最终上线验证仍待完成。

### 隔离launchd异常重启演练

macOS已登录用户可运行：

```bash
go build -o /your/private/build/aimangebot .
python3 scripts/smoke-launchd.py --binary /your/private/build/aimangebot --port 19240
```

只接受19000–19999测试端口，需空闲；刚完成一次演练时TCP状态可能暂时占用，选另一空闲测试端口。脚本创建自己的私有临时目录、合成账号和随机服务标签，不读取生产config/DB，不连接外部GitLab或模型。前台bootstrap正常退出后交由launchd，强制终止测试PID，核对35秒等待后的不同PID、不同有效租约owner、readyz和原账号登录；finally卸载自己的服务。stdout只输出布尔结果和观察耗时，证明0600；日志/DB/配置保留私有目录，不提交Git。失败需查私有日志，不能把服务定义存在当作验收成功。此演练证明本机用户域异常自动恢复，不证明Linux系统服务、真实模型质量或生产容量。

### 隔离并发容量检查

```bash
python3 scripts/smoke-worker-recovery.py \
  --binary /your/private/build/aimangebot --capacity-preview \
  --app-port 19242 --upstream-port 19243
```

先执行原生SIGKILL/租约恢复基础场景，再阻塞专属合成MR的模型响应，使队列在检查窗口保持占用。40个不同MR通过真实HTTP同时入队；全局未完成配额4、项目/用户运行配额1、已有重试pending占1，期望3个新建和37个明确429。满队列重复请求复用旧任务；取消pending后新任务可入队。采样一秒的DB计数与提交延迟输出只描述本次本机边界，不证明生产最大吞吐量或全部时间内的运行并发。使用临时端口、DB和合成上游，finally清理，私有证明不提交Git。真实吞吐量还取决于模型限流/延迟、预算、仓库大小及磁盘，需要按实际部署单独测量。

### 启用预算的崩溃边界

```bash
python3 scripts/smoke-worker-recovery.py \
  --binary /your/private/build/aimangebot --budget-crash-preview \
  --app-port 19246 --upstream-port 19247
```

该选项单独执行：预算100，首响应报告20 tokens，第二请求发送前已持久化pending未知usage，然后真实SIGKILL。立即重启必须被有效租约拒绝，等待原租约到期后重启，保留相同检查点与预算；父任务标记失败且model_usage_unknown，不创建retry子任务，也不重复模型请求。未知消耗不能按零计价或重新领取预算，因此这里主动停止自动重试。此为本机合成SDK/Worker故障实证，不测真实模型质量、账单或生产吞吐量。

## GitLab只读验收采证

`scripts/gitlab-acceptance-receipt.py` 对指定run/project/MR/full HEAD读取readyz、审计详情和GitLab MR/discussion，核对当前MR未漂移、任务已结束、评论sent且desired/sent generation一致、note/discussion对应。只GET，不触发审计或修改MR。凭据提前在当前私有shell环境设置 `AIM_ACCEPT_SESSION`（现有aim_session值）与 `AIM_ACCEPT_GITLAB_TOKEN`，不要写入命令参数、终端历史、脚本或证明；脚本不输出凭据。远端地址要求HTTPS，本机可HTTP，重定向一律拒绝。

```bash
python3 scripts/gitlab-acceptance-receipt.py \
  --app-url http://127.0.0.1:1234 --gitlab-url https://gitlab.example.com \
  --run-id 123 --project-id 456 --mr-iid 7 \
  --head-sha FULL_LOWERCASE_HEAD_SHA \
  --output /your/private/acceptance/after.json
```

输出父目录0700，文件0600且不覆盖，证明只含版本/任务和评论标识、generation/body SHA256，不复制正文或会话。此收据证明一次指定对象的一致性，不证明没有其他重复评论、Webhook触发、更新历史、冲突恢复或真实审计质量。专用MR写入验收需明确授权；可在人工复核更新前后采两份，后续完整程序再验证同评论更新和冲突，不能用这份只读收据冒充完整R7。
