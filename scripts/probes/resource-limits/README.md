# Child resource capability experiment

Run from the application repository root:

```sh
python3 scripts/probes/resource-limits/probe.py
python3 scripts/probes/resource-limits/probe.py --docker
```

Requires Python 3 and a C compiler. The optional Linux comparison requires a
running local Unix-socket Docker daemon, official Debian image/package access
for building the isolated fixture, and Docker's init support. No project files
other than this directory's C/shell fixture enter the Docker build context.
The build can contact package registries; fixture containers use network none.
No model, hosted repository, application configuration or credential is used.

The native executable is compiled in a temporary directory. Limits apply only
inside its child processes. The memory test maps virtual address space without
touching the large mapping. Disk writes total at most 128 KiB. CPU tests have
an outer ten-second deadline with owned-process-group cleanup. A supervisor kill
is explicitly excluded from evidence of kernel CPU enforcement.

Docker uses a dedicated nonroot user, init, read-only root, a one-MiB work tmpfs,
no capabilities, no new privileges, 256 MiB memory, 16 pids, and one CPU. Each
uniquely named container is removed; the local test image is kept. The C fixture
must run under init rather than as PID 1: Linux's PID-1 default signal behavior
otherwise confounds the soft CPU signal observation.

Exit zero means the experiment completed and Linux, if requested, exhibited
the expected capabilities. **It does not mean every native platform supports
every limit.** Inspect `local.capabilities` and `supervisor_timeout`. The final
JSON is classified evidence; progress goes to stderr. Redirect stdout to retain
the result. No raw subprocess stderr, environment, credentials or local paths
are included in that JSON.

The Linux Git test uses prlimit around actual Git commands, creates two synthetic
object-format repositories, fetches an older fixed commit, reads its exact blob,
and checks connectivity. File protocol is allowed only for this local fixture;
it is not a production transport recommendation.

Scope limits:

- RLIMIT_AS constrains per-process virtual address space, not total resident RAM.
- RLIMIT_CPU is per-process CPU, not process-group cumulative CPU or wall time.
- RLIMIT_FSIZE is per-file. Two permitted files exceeding the single-file cap
  are an intentional counterexample to treating it as a workspace quota.
- FD limits inherit through exec; lowering a hard limit cannot be undone by
  the nonroot fixture. This is not a per-run process-count limit.
- Container restrictions surround this fixture; production cgroup, pack bomb,
  hostile repository, concurrent-run and cleanup/restart behavior are untested.
- This does not change or validate the application's production Git executor.

Results and design implications are recorded in
`docs/feature/sidebar-workflow/repository-git-resource-research.md`.
