#!/usr/bin/env python3
"""Fail closed on private artifact paths or credential-shaped staged content.
Never print matched values. Run immediately before each commit.
"""
import pathlib
import re
import subprocess
import sys

root = pathlib.Path(subprocess.check_output(["git", "rev-parse", "--show-toplevel"], text=True).strip())
names = subprocess.check_output(["git", "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"]).decode().split("\0")
private = []
for name in ("config.yaml", ".env.deploy"):
    path = root / name
    if path.exists():
        for line in path.read_text().splitlines():
            match = re.match(r"\s*(?:api_key|token|webhook_token|[A-Z_]*(?:KEY|TOKEN|PASSWORD))\s*[:=]\s*(.+)", line)
            if match:
                value = match.group(1).strip().strip("\"'")
                if len(value) >= 12:
                    private.append(value)
patterns = [r"sk-[A-Za-z0-9_-]{20,}", r"gh[pousr]_[A-Za-z0-9]{20,}", r"glpat-[A-Za-z0-9_-]{20,}", r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"]
failed = False
for name in filter(None, names):
    parts = pathlib.PurePosixPath(name).parts
    if pathlib.PurePosixPath(name).name in {"config.yaml", ".env.deploy", "pr_agent.db"} or any(p.startswith(".env") and p != ".env.example" for p in parts) or name.endswith((".db-wal", ".db-shm")):
        print("REJECT private artifact:", name)
        failed = True
        continue
    raw = subprocess.check_output(["git", "show", ":" + name]).decode("utf-8", errors="replace")
    if any(v in raw for v in private) or any(re.search(p, raw) for p in patterns):
        print("REJECT possible credential:", name)
        failed = True
print("staged secret check:", "FAILED" if failed else "passed")
sys.exit(1 if failed else 0)
