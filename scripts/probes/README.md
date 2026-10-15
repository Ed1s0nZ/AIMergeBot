# Native repository research probes

`git_http_bridge.go` is an isolated feasibility experiment, excluded from normal
Go packages by a build tag. It is not imported by AIMergeBot and is not a
production proxy, sandbox, repository factory, or authorization layer.

Run from the repository root with Git and `git-http-backend` installed:

```sh
go run -race scripts/probes/git_http_bridge.go
```

The probe creates three synthetic repositories (two SHA-1 and one SHA-256) in a temporary directory, exports
them through a local TLS `git-http-backend` fixture, and runs real Git fetches
through a loopback Go handler. Temporary files and listeners are removed on exit.
It never reads project configuration, user repositories or credentials, and
never connects to a hosting platform/model/notification service.

Checks include concurrent fixed non-tip SHA fetches, smart HTTP v0 and actual v2 advertisement, per-repository
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


## Pinned library comparison

`go-git-v6/` is a separate module pinned to go-git `v6.0.0-beta.1`. It does not
change or participate in the application module. Run separately:

```sh
cd scripts/probes/go-git-v6
go run -mod=readonly -race .
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -show verbose ./...
```

The same type of synthetic TLS Git backend serves three repositories. The probe
uses a distinct HTTP client per operation and checks concurrent credentials,
fixed non-tip SHA-1/SHA-256, depth-one repositories read by native Git, request
exhaustion, redirect rejection, revocation, response-limit failure and in-flight
HTTP cancellation. The reported cancellation duration is a local observation,
not a production latency guarantee. It also records whether an injected upstream
error contains a synthetic token; raw library errors must not become public
application errors. Only the Boolean classification is printed.

The candidate's default dependency graph resolved x/net v0.59.0. This probe pins
v0.60.0 to retain the application's existing security baseline. Module advisories
and reachable findings are reported separately; a zero reachable result is not
proof that every dependency is vulnerability-free. Neither probe establishes
concurrent hard byte quotas, malicious pack resilience, process isolation, or
production provider/ACL integration. Normal root-module tests do not run this
nested module; invoke the commands above explicitly.
