# Git资源边界实测（F2输入）

2026-10-16；codex/sidebar-workflow；承接Confirmed需求和S4，用户“没想清楚先不实现”。资源预算与进程隔离不是同一概念，不能从HTTP bytes、100ms workspace扫描或进程组取消推导所有资源硬限额。

## Workflow / Maintainability / Lifecycle Gate

- P6/F2，本轮仅本机Darwin与已确认本地Docker Linux的合成研究，生产实现no；不修改Runner/gitCommand/Settings或恢复入口。
- 已取得：现有Git30s/输出8MiB/文件256KiB与16MiB工具上限、取消修复、网络额外URI反例与proxy实验、预算和API设计；缺少可验证OS资源能力。
- 允许独立scripts/probes/resource-limits的C子进程及Python监督器、最小Docker测试镜像；同一Unix Docker socket固定，远端daemon拒绝；只把该probe目录作为build context，不打包项目配置/凭据/用户仓库。
- 镜像来源为项目现有Dockerfile同源的官方debian:bookworm-slim，增加编译probe和测试实际Git/prlimit所需的官方apt包。仅隔离研究镜像，不部署应用。记录实际image ID，不宣称tag等于所有版本。
- 只在probe子进程设置rlimit，不修改Python/服务/用户shell限制。只尝试虚拟mmap不触碰大块RAM；文件总量最多128KiB。CPU子进程1s soft/2s hard，监督器10s timeout并杀进程组。无core dump，不执行仓库内容。
- Docker测试使用只读rootfs、network none、cap-drop ALL、no-new-privileges、10001用户、1MiB tmpfs、256MiB memory、16 pids及1CPU；容器唯一名、--rm，取消时只清理本次容器，不prune用户资源。
- 维护风险low：单职责独立probe，root Go无新依赖/包；不往生产多职责文件加分支。验收为实际能力分类而非所有平台强制“通过”，不支持必须记录，不假装fallback已等价。

## 实验和反例

| 场景 | 受控动作 | 要证明/不能混淆 |
| --- | --- | --- |
| AS | 子进程设128MiB地址空间，尝试512MiB匿名mmap，不触页 | setrlimit成功不等于mmap实际被拒；AS不是RSS/cgroup内存 |
| FSIZE | 子进程64KiB单文件限额，写128KiB，再写1字节 | errno/文件大小证明限制；不是整个工作区quota |
| aggregate | 同样FSIZE写两个各64KiB文件 | 总128KiB仍允许，明确反驳用FSIZE当workspace硬cap |
| NOFILE | 子进程64 FD上限，开/dev/null到EMFILE | 限额/失败可观测，退出清理；不是进程数量上限 |
| inherit | 降硬NOFILE后exec同一probe并尝试抬高 | exec继承与无特权不能抬高，不用父服务setlimit |
| CPU | 设置soft1/hard2后有限监督下忙循环 | SIGXCPU/SIGKILL和退出证明，只是单进程CPU，非group累计 |
| Git(prlimit/Linux) | 固定限制wrapper执行合成seed→bare fetch→show/fsck | 成功路径能用，不把C probe等同Git可运行 |

所有状态JSON只含平台/能力/次数/有限errno类别，不输出环境、原始路径、用户或密钥。实验目录退出删除；测试镜像可留作可复现工件，不删除用户已有image。

## 资料边界

[Linux getrlimit](https://man7.org/linux/man-pages/man2/getrlimit.2.html)区分AS、CPU、FSIZE和继承；[Apple归档手册](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/getrlimit.2.html)只作接口参考，不当作当前Darwin强保证。实际本机结果与Linux容器结果分别记录。

后续根据证据写正式资源合同：准确额度/单位、per-process vs group/run/workspace、支持环境、无能力时行为、cleanup和覆盖。不得因为Linux成功就默认Mac/Windows等价；也不能因为平台不同永久改成API-only或缩小GitHub/跨项目最终目标。当前native管理保持只读，S4–S6及原完整REQ/AC未完成，不合并main/部署。
