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


## Alternate URI egress experiment

```sh
go run -race scripts/probes/git_uri_egress.go
```

This standalone probe runs only synthetic repositories and loopback services. It
uses real Git-generated blob packs and bundles, a CGI Git origin, a separate URI
origin and an experimental fixed forward proxy. Verified locally with Apple Git
2.50.1 (Apple Git-155); other versions require their own evidence. It is excluded
from normal packages by its build tag and is not executed by application CI.

Positive controls must download a pack. Configured fetch.bundleURI is reported
separately: Apple Git 2.50.1 downloads the bundle/list; Debian Git 2.39.5 in the
tested fixture does not. That absence is a capability observation, not proof of
bundle protection. A partial bundle/list control fails the probe. The normal server path
then checks empty command-level URI settings against a polluted local config,
and reads the expected fixed blob. A malicious server path replays the complete
successful CGI response, including correct sideband framing, to a client which
did not request URI downloads. The observed
`unsolicited_packfile_uri_bypasses_disable: true` is a counterexample to relying
on configuration alone. **Exit zero means the experiment completed; it does not
mean the configuration is safe.** Raw Git/credential errors are not printed.

Finally the same response is tested with a forced local HTTP forward proxy:
normal fixed-object reads succeed, alternate HTTP addresses are rejected, and
HTTPS CONNECT is rejected before a TLS connection. HTTPS uses a temporary trusted
fixture CA and a successful direct-download control; TLS connection counts avoid
mistaking certificate failure for network isolation. The reported zero extra
HTTP requests/TLS connections applies only to these cases. No production factory,
bridge assembly, OS sandbox, filesystem or malicious-pack resource isolation is
validated.

The extended matrix replays the same real response with localhost, decimal
loopback and percent-encoded loopback HTTP authorities. Each can download the
pack directly; the fixed proxy rejects them without a pack-origin arrival.
An actual synthetic pack file URI fails to complete fetch even with file
protocol enabled; this is not a syscall-level claim that no local file was
opened. A loopback FTP connection succeeds when explicitly permitted, is not
made when protocol policy denies it, and is rejected by the proxy when FTP is
deliberately permitted for the control.

An intentional child NO_PROXY/no_proxy `*` control **bypasses the forced proxy**.
The whitelist child environment then strips a polluted parent NO_PROXY and
restores denial. A dead proxy yields zero Git-origin requests; no direct fallback
is observed. Exit zero includes these counterexamples and must not be called
"all proxy settings are safe". The JSON reports each positive control and denied
case separately; the parent process environment is restored on exit.

The matrix also ran as a CGO-disabled Linux/arm64 binary under the existing
isolated Debian fixture image with Git 2.39.5, nonroot, no network, read-only root
and bounded tmpfs. Only that temporary binary was mounted, never the project,
configuration or credentials. Native -race and Linux no-race are distinct proof.
Other versions, URI/userinfo/parser variations and production gateway/OS
isolation remain separate verification; two Git builds are not all-platform proof.


## API text matching candidate

```sh
cd scripts/probes/text-diff
go run -mod=readonly -race .
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -show verbose ./...
```

This separate module pins go-difflib v1.0.0 without changing application
dependencies. It creates synthetic files and asks actual local Git to apply the
generated patches and compare exact target bytes. Offset/fuzz correction is
rejected, with a shifted-hunk positive/negative control. It checks 13 named cases,
64 deterministic mixed-text cases, simple line anchors, repeated-line precision,
CRLF/Unicode/empty content, missing final LF, input/output bounds, and pre-cancel.
All temporary files are removed; no remote repository or model is accessed.

The default autojunk heuristic can label 1000 lines added/removed for one edit
in a 1999-line repetitive file while still producing a valid patch. The precise
candidate explicitly disables autojunk. The raw library writer's no-final-LF
failure is recorded as a negative control. Exit zero means the experiment ran,
not that defaults are appropriate or the production API adapter is complete.
The candidate has no mid-match context interruption; recorded microseconds are
local observations, not a runtime guarantee. Normal application CI excludes
this nested module; invoke it explicitly. Full tree/PR/provider/budget/BuildDiff
integration remains separate verification.

### Interruptible line matcher comparison

```sh
cd scripts/probes/text-diff
go test -mod=readonly -race -v .
go test -mod=readonly -run '^$' -bench '^BenchmarkBoundedLCSWorstCase$' -benchtime=1x -benchmem .
```

These commands exercise the separate bounded-LCS candidate. `go run .` above
continues to exercise the original library comparison; its report that
mid-match interruption is unavailable refers to go-difflib, not this candidate.
The candidate is project research code based on the standard LCS recurrence,
not a third-party maintained library or a production adapter.

The tests retain exact line bytes, including CRLF and missing final LF; replay
77 named/fixed-seed patches with actual Git and reject offset/fuzz. They check
3,969 tiny sequence pairs against an independent minimum insert/delete oracle,
repetitive-line anchors, reversed/alternating/long-line maximum-size inputs,
in-progress cancellation and output/input limits. The synchronous cancellation
observer cancels a real context during DP; no timeout goroutine is used.

DP compares exact-line integer IDs with checks before each at-most-2,000-cell
row. A full matrix is bounded to 8,008,002 bytes, which is not a total-memory or
RSS guarantee. A single-owner 32-million-cell allowance spans eight worst-size
files; the ninth fails with no partial hunk. The local allowance is neither
concurrent-safe nor durable across retries/restarts. Production needs the
separate shared-budget/session design and cannot reset this allowance per file.
Benchmarks report local allocations and elapsed time, not an SLA or CPU quota.

## Fixed GitLab object protocol

```sh
go run -race scripts/probes/gitlab_objects.go
go vet scripts/probes/gitlab_objects.go
```

This ignore-tag probe uses the application's existing go-gitlab v0.90.0 SDK,
temporary real SHA-1/SHA-256 repositories and a local TLS protocol fixture.
Actual Git creates 107 directory entries in each format, including regular,
executable, symlink and gitlink metadata. SDK reads then run with Git removed
from PATH. Two-page directories match actual Git exactly; raw empty/non-UTF8/
symlink/executable bytes are checked against Git blob hashes.

Wrong hashes, oversized raw bodies, provider errors, missing/duplicate pages,
invalid metadata modes, mixed object formats and missing fixed refs are rejected.
A shared transport allowance around SDK response buffering stops the next
request; pre-cancel sends nothing, and mid-request cancellation reaches the
fixture. Errors returned by the adapter omit the synthetic provider-secret
marker. The local allowance is an experiment, not the production durable ledger
or a proof of same-body concurrent Read/Close safety.

No actual GitLab server, account, model, publication or production adapter is
used. In particular, accepting 64-character OIDs in this fixture does **not**
prove SHA-256 support in every real GitLab version. Normal application CI
excludes ignore probes; run these commands explicitly.

## Legacy SDK endpoint identity

```sh
GOTOOLCHAIN=go1.27.2 go run -race scripts/probes/legacy_sdk_endpoint.go
GOTOOLCHAIN=go1.27.2 go vet scripts/probes/legacy_sdk_endpoint.go
```

This ignore-tag probe uses only synthetic TLS requests with the current GitLab
SDK. Distinct `BaseURL` strings can produce the same wire prefix for encoded
dash/slash paths. Path case and encoded-dot remain distinct. Constructing an SDK
request with an empty relative path yields the effective prefix observed in all
eight actual project requests. No real GitLab account or repository is used;
this is evidence for the proposed endpoint-key rule, not a production adapter.

## Child resource capabilities

```sh
python3 scripts/probes/resource-limits/probe.py --docker
```

See [resource-limits/README.md](resource-limits/README.md) for prerequisites,
cleanup and exact scope. This experiment compares native child rlimits with a
local nonroot Linux container and actual Git under prlimit. Exit zero is not a
cross-platform guarantee: Darwin address-space and hard CPU enforcement gaps
are retained in the result. Single-file limits intentionally do not constrain
aggregate workspace size. No production executor is changed or validated.
