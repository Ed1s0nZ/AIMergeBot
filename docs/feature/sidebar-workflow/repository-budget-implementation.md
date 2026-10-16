# N1b：共享预算格式与Schema3基础

2026-10-16；codex/sidebar-workflow。输入：Confirmed requirements、S4第7轮技术评审Pass、budget/execution/legacy-transition合同。N1a已实现scope v2；完整需求不缩减。

## Workflow / Lifecycle Gate

P7/P8、F4，允许实施已审N1内部基础。此片实现严格预算格式、AuditPolicy克隆/摘要与旧路径拒绝、真实schema3迁移及数据库约束；不开放v51 producer、网络grants、工厂或C3。下一片必须完成admission/run/retry的同账本事务与计量；只有表和格式不能算资源预算生效。

## Maintainability Gate

high集成风险，文件规模可控：Store290行、schema guard71行、identity migration111行。Store只新增委托，budget policy/migration各独立文件；不将SQL/transport/Runner混在一起。allowed adapter_extraction/narrow_fix，无需先大重构。身份迁移固定为schema2，再单独升级3，避免重复登记/旋转安装HMAC key。现有v50捕获不自动加预算字段。

## 实施与验收

- AuditPolicy预算可选，无字段时旧JSON/digest不变；有字段必须完整、严格v1并匹配冻结Git/timeout公式。limits JSON上限4096 bytes（服务端紧凑固定字段，小于该上限），purpose只由服务端选择；policy只允许audit公式。
- schema3新增budget、run映射、reservation三表，约束基础类型、FK、principal user/service、非负计量及终态；运行时授权、limits/owner/CAS仍须后续Store协议，不能拿SQL CHECK替代。
- schema1/2停机迁移同事务；2→3不重建身份/HMAC、不回填旧预算、不重写旧任务。当前3缺表/列拒绝而不自修；未来版本及坏metadata零schema写。停止旧worker并备份后才可部署，本轮仅临时SQLite测试。
- 专项真实DB迁移/重开/失败回滚/活动worker、约束反例、严格JSON、旧policy与拒绝通道；随后全Go/race/vet和精确CI。保持501/legacy guard，不合并main/部署/真实外发。

## 实际结果

N1b内部基础已实现：repository_budget_policy.go负责14个required int64字段和6种固定purpose公式；AuditPolicy包含可选预算、严格解析/克隆/digest，legacy捕获不新增字段。带预算但没有正式factory的快照/持久任务均被旧路径拒绝，不能把描述性limits当成已附着账本。

repository_budget_migration.go独立创建三表，Store只委托；身份schema2升级和budget schema3升级分开。绝对admission_deadline持久化，不从每次claim重算；run映射使用WITHOUT ROWID防止NULL run_id被隐式生成为别的运行。JSON计量上限检查显式拒绝缺失/null/非整数，避免SQLite CHECK的NULL结果放过非法计量。JSON主体、角色/授权及digest的运行时验证仍是后续协议职责，SQL不能证明service请求可信。

验证环境Go1.27.2/Darwin arm64、Xcode编译器PATH；最终源代码：

- `go test ./...` exit0，internal/platform 131.136s；全包通过/无测试。
- `go test -race ./internal/platform -run 'TestRepositoryBudget|TestPlatformSchema|TestRepositoryIdentityMigration|TestRepositoryScope' -count=1` exit0，16.368s，无race。
- `go vet ./...`、`git diff --check` exit0。没有前端/依赖/生成资源变化，不重复无关Vite构建。
- 专项验证真实SQLite schema1/2→3与重开、失败事务回滚/活动worker/当前缺表缺列/版本表丢失、HMAC与登记代次及历史保留、负数/实数/上限/NULL约束、FK和不可退款request/text receipt；严格budget JSON/purpose/frozen公式与旧路径拒绝。

上一代码05c7edaa447c37284abcf07697f6cc481aa2e697的CI38860147252已查询completed/success后才推新代码。当前精确HEAD CI另行记录，旧成功不替代本片验证。全部新DB为测试临时文件，未迁移用户运行库、启用C3或外部写。N1仍未完成：缺principal/owner受控Store、原子Attach与retry映射、扣减/结算/恢复及所有transport/consumer，不能将本片宣称为共享预算已经生效。
