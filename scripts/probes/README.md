# Native repository research probes

`git_http_bridge.go` is an isolated feasibility experiment, excluded from normal
Go packages by a build tag. It is not imported by AIMergeBot and is not a
production proxy, sandbox, repository factory, or authorization layer.

Run from the repository root with Git and `git-http-backend` installed:

```sh
go run -race scripts/probes/git_http_bridge.go
```

The probe creates two synthetic repositories in a temporary directory, exports
them through a local TLS `git-http-backend` fixture, and runs real Git fetches
through a loopback Go handler. Temporary files and listeners are removed on exit.
It never reads project configuration, user repositories or credentials, and
never connects to a hosting platform/model/notification service.

Checks include smart HTTP v0 and actual v2 advertisement, per-repository
synthetic token isolation, private local-header capability, header filtering,
shared request exhaustion, forbidden push/dumb routes, redirect rejection,
revocation during discovery, byte-limit failure and absence of upstream secrets
from audit-side Git environment/workspace. The JSON output contains only
classifications/counts/tool version. Fixture-only local seed pushes explicitly
allow file transport; audit-side clients do not inherit that exception.

This does **not** prove production DNS/CIDR enforcement, malicious pack handling,
concurrent hard byte quotas, OS process limits, alternate URI egress prevention,
provider identity/ACL/lease integration, or native audit readiness. The small
experimental ledger performs post-read accounting; production byte reservation
and whole-run semantics still need a separate reviewed design and tests.
