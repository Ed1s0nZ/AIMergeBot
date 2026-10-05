#!/usr/bin/env python3
"""Generate private service definitions; never installs or reads credentials."""
import argparse
import json
import os
from pathlib import Path
import plistlib
import re
import sys


def absolute(value):
    path = Path(value)
    if not path.is_absolute() or any(ord(c) < 32 or ord(c) == 127 for c in value):
        raise ValueError("invalid absolute path")
    return str(path)


def unit_quote(value):
    return '"' + value.replace('\\', '\\\\').replace('"', '\\"').replace('%', '%%') + '"'


def render(kind, binary, directory, label, user=None, group=None):
    binary, directory = absolute(binary), absolute(directory)
    if not re.fullmatch(r"[A-Za-z][A-Za-z0-9_.-]{0,100}", label):
        raise ValueError("invalid label")
    if kind == "launchd":
        return plistlib.dumps({"Label": label, "ProgramArguments": [binary], "WorkingDirectory": directory, "RunAtLoad": True, "KeepAlive": {"SuccessfulExit": False}, "ThrottleInterval": 35, "ExitTimeOut": 45, "Umask": 0o077, "StandardOutPath": str(Path(directory) / "service.stdout.log"), "StandardErrorPath": str(Path(directory) / "service.stderr.log")})
    for identity in (user, group):
        if not identity or identity == "root" or not re.fullmatch(r"[a-z_][a-z0-9_-]{0,31}", identity):
            raise ValueError("non-root user/group required")
    # systemd prefixes have semantics even inside quotes; require an ordinary
    # absolute executable path and escape specifiers rather than evaluating them.
    text = f'''[Unit]
Description=AIMergeBot audit service
After=network.target
StartLimitIntervalSec=300
StartLimitBurst=8

[Service]
Type=simple
User={user}
Group={group}
WorkingDirectory={unit_quote(directory)}
ExecStart={unit_quote(binary)}
Restart=on-failure
RestartSec=35
TimeoutStopSec=45
UMask=0077
NoNewPrivileges=true
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
'''
    return text.encode()


def write_private(path, data):
    parent = path.parent
    if not parent.is_dir() or parent.is_symlink() or parent.stat().st_mode & 0o077:
        raise ValueError("private output parent required")
    created = False
    try:
        with open(path, "xb") as output:
            created = True
            os.fchmod(output.fileno(), 0o600)
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
    except Exception:
        if created:
            path.unlink(missing_ok=True)
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", choices=["launchd", "systemd"])
    parser.add_argument("--binary", required=True)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--label", default="local.aimangebot")
    parser.add_argument("--user")
    parser.add_argument("--group")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        data = render(args.kind, args.binary, args.directory, args.label, args.user, args.group)
        write_private(Path(absolute(args.output)), data)
    except (OSError, ValueError):
        print(json.dumps({"status": "error", "error": "service definition rejected"}))
        return 1
    print(json.dumps({"status": "ok", "kind": args.kind}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
