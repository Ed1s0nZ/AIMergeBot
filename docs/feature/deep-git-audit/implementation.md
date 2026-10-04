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
