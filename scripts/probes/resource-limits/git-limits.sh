#!/bin/sh
set -eu
export HOME=/work
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0
limited_git() {
    prlimit --as=268435456:268435456 --cpu=2:2 --fsize=1048576:1048576 --nofile=64:64 -- \
        git --no-pager -c core.hooksPath=/dev/null -c credential.helper= -c pack.threads=1 -c gc.auto=0 "$@"
}
for audit_probe_format in sha1 sha256
do
    audit_probe_seed=/work/seed-$audit_probe_format
    audit_probe_bare=/work/target-$audit_probe_format
    limited_git init --quiet --template= --initial-branch=main --object-format="$audit_probe_format" "$audit_probe_seed"
    limited_git -C "$audit_probe_seed" config user.name Fixture
    limited_git -C "$audit_probe_seed" config user.email fixture@example.invalid
    printf 'synthetic-resource-evidence\n' > "$audit_probe_seed/evidence.txt"
    limited_git -C "$audit_probe_seed" add -- evidence.txt
    limited_git -C "$audit_probe_seed" commit --quiet -m fixture
    audit_probe_sha=$(limited_git -C "$audit_probe_seed" rev-parse HEAD)
    printf 'synthetic-new-tip\n' > "$audit_probe_seed/evidence.txt"
    limited_git -C "$audit_probe_seed" add -- evidence.txt
    limited_git -C "$audit_probe_seed" commit --quiet -m next
    limited_git -C "$audit_probe_seed" config uploadpack.allowReachableSHA1InWant true
    limited_git init --quiet --bare --template= --object-format="$audit_probe_format" "$audit_probe_bare"
    limited_git -C "$audit_probe_bare" -c protocol.file.allow=always fetch --quiet --depth=1 --no-tags "$audit_probe_seed" "$audit_probe_sha:refs/heads/pinned"
    audit_probe_content=$(limited_git -C "$audit_probe_bare" show refs/heads/pinned:evidence.txt)
    test "$audit_probe_content" = synthetic-resource-evidence
    limited_git -C "$audit_probe_bare" symbolic-ref HEAD refs/heads/pinned
    limited_git -C "$audit_probe_bare" fsck --connectivity-only >/dev/null
 done
printf '{"synthetic_only":true,"prlimit_git_sha1_sha256_fixed_fetch":true}\n'
