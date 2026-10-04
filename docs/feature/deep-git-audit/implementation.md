# Language-independent Git investigation

## F0 intake / workflow gate
User asks to implement the agreed language-independent repository tools and investigation loop, with main delivery and local port 1234 deployment. Existing main is clean at 8469b54. Phase: backend integration P8; existing React/Eino/API contracts and platform tests are available. Missing: bounded Git workspace and advanced tool contracts. Implementation allowed after this design; no language adapters or arbitrary command execution. Existing GitLab PR submission remains compatible.

## F1 requirements (authorized by user: 最佳实践帮我完成)
R1: Fixed base/head, tree and numbered reads, full-repository literal/regex search with resumable pagination and path filtering.
R2: Batch reads, diff/file comparison, history, blame and history string search, all language independent.
R3: Persistent tool observations plus hypothesis/progress recording and validated finding submission; no semantic-reference or runtime-verification claims.
R4: Configuration persisted through existing system settings/config.yaml; tests, main push and service deployment.
Optional sandbox execution, vulnerability databases and language-specific parsers are outside this agreed slice.

## Maintainability gate
Inspected agent (363 lines, tool handlers + orchestration + validation), repository (160), runner (180), settings (292). Medium coupling. Extract tools into dedicated modules and add a separate Git source adapter; preserve existing Repository interface and fixture compatibility. Tests cover fixed refs, removed-line evidence, Eino callbacks, settings and queue lifecycle. No broad legacy refactor.

## F2 design
GitLab remains metadata/comment transport. Per-run bare Git workspace fetches immutable commits with bounded history (default 200); forks fetch target base and source head separately. Read-only Git plumbing avoids checkout, hooks, submodules, LFS smudge and textconv. HTTPS origins must match configured GitLab; token passed only through subprocess environment, never URL or config. Standalone local repository adapter uses the same interface for tests and consumers. Tool command arguments are structured, refs fixed to snapshot or reachable history, paths literal, no shell. Output, fetch pack size, time, file size and calls have budgets. Workspace cleaned on success/error/cancellation. Shallow history explicitly marked limited.

Advanced tools expose cursor-based text pages rather than silently skipping files. Search uses Git grep over commit trees, binary files skipped. Regex is Git extended regex, not a promise of semantic resolution. Tool outputs include base/head SHA and retained observations, with errors and truncation. Investigation ledger is per-run, bounded and stored as trace output; hypothesis states investigating/supported/rejected, with evidence and counterevidence supplied by Agent. submit_finding reuses strict changed-line/snapshot evidence validation; final result remains validated even when Agent does not use submission tool. Neither a matching snippet nor model confidence establishes exploitability.

Settings add git_audit (enabled, history_depth, max_pack_mib, max_tool_calls). Defaults preserve existing configs while enabling native Git; settings JSON/YAML retain existing generic persistence. Docker runtime must include Git. Model max steps remains separately configurable.

Flow: queued run → capture config → obtain GitLab diff snapshot → prepare bare repository → native diff → Eino investigation → validate findings → persist result/observations → delete workspace → existing optional comment. No fallback that silently masks native-Git failure.

Official references: [Git grep](https://git-scm.com/docs/git-grep), [Git log](https://git-scm.com/docs/git-log), [Git config](https://git-scm.com/docs/git-config).

## F3 implementation plan
1. Add native Git adapter and guarded GitLab preparation; test real repositories, forks, renames/deletions, unusual paths and process limits.
2. Extract tool modules, register language-neutral exploration/history/ledger tools, retain schema compatibility; test full search pagination and evidence validation.
3. Wire per-run preparation and cleanup, settings and trace UI; test Eino HTTP fixture and frontend build.
4. Run repository/race tests, document limitations, merge/push main, build/restart the existing native port-1234 service with unchanged private config.
Rollback: previous main binary/commit; existing database JSON remains additive and old traces render without output. No repository code execution, writes or automatic clone credential reuse across origins.

## F4 implementation
Implemented native bare Git source, per-run fork-aware preparation/cleanup and default Git settings; extracted Agent tools into exploration/history/investigation modules. Registered 13 tools, fixed whole-tree search, retained observation output with pinned SHA, and added a factual investigation ledger plus strict submit validation. Changed policy key to eino-git-investigation-v2 so old completed runs do not suppress the new policy. Added React settings controls and investigation/observation display. Added standalone -audit-repo/-base/-head for local refs or HTTP(S) full SHAs without any GitLab API; optional AIM_GIT_TOKEN is environment-only. Standalone output is JSON, not inserted into the workspace database. SSH repositories use an existing local clone.
Checks before F4: go test ./... and platform race checks passed; frontend production build passed. Real Git fixtures cover 125 files, regex/base search, paging, removed guard evidence, history/blame/diff, smart-HTTP pinned fetch with scoped authentication and cleanup, and an Eino multi-tool standalone investigation. Fixture model responses demonstrate workflow, not actual model detection accuracy.

## F5 verification
- `go test ./...`: passed, including fixed-version API regressions and native Git integration.
- `go test -race ./internal/platform`: passed.
- `go vet ./...`, `git diff --check`: passed.
- `npm --prefix frontend run build`: passed (TypeScript + Vite).
- Smart-HTTP fixture verifies token authentication on Git requests, immutable object reads and cleanup; no token persists in Git config.
- Standalone executable smoke: local repository + two refs → JSON, 13 registered tools, fixed SHA, no platform API/database creation. Model is a local deterministic HTTP fixture.
- Browser fixture: authenticated settings save/restore history depth; YAML persistence and investigation/evidence expansion checked. Desktop and 390px mobile render checked. Screenshots settings.png/investigation.png are synthetic-model fixtures with real Git tool observations, not production vulnerability results.
- Docker build passed with backend tests and Git in non-root runtime. Final packaging smoke follows after merge.

Requirement audit: R1/R2 implemented and exercised with real Git; R3 multi-tool Eino fixture plus invented-evidence rejection, retained output and UI checked; R4 persistence/build/main/deployment steps tracked below. Limitations: lexical rather than semantic references; no execution/reproduction; shallow history, binary/LFS/submodule exclusions, resource limits, remote raw-SHA fetch support. Standalone mode currently CLI-only; existing web MR entry still uses GitLab metadata. Real private GitLab and real-model accuracy/throughput are not measured by these fixtures.
