#!/usr/bin/env python3
"""Private, stopped-service SQLite/config backup; restore only to a new directory."""
import argparse
from contextlib import closing
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import sqlite3
import stat
import time
from urllib.parse import quote


class BackupError(Exception):
    pass


def check_time(deadline):
    if time.monotonic() > deadline:
        raise BackupError("operation timed out")


def regular(path, private=False):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode):
        raise BackupError("source must be a regular file")
    if private and stat.S_IMODE(info.st_mode) & 0o077:
        raise BackupError("private file permissions required")
    return info


def read_file(path):
    regular(path)
    fd = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
    if not stat.S_ISREG(os.fstat(fd).st_mode):
        os.close(fd)
        raise BackupError("source must be a regular file")
    return os.fdopen(fd, "rb")


def digest(path, deadline):
    total, checksum = 0, hashlib.sha256()
    with read_file(path) as source:
        while True:
            check_time(deadline)
            chunk = source.read(1024 * 1024)
            if not chunk:
                break
            total += len(chunk)
            checksum.update(chunk)
    return {"sha256": checksum.hexdigest(), "size": total}


def copy_private(source, target, mode, deadline):
    with read_file(source) as origin, target.open("xb") as dest:
        os.chmod(target, mode)
        while True:
            check_time(deadline)
            chunk = origin.read(1024 * 1024)
            if not chunk:
                break
            dest.write(chunk)
        dest.flush()
        os.fsync(dest.fileno())


def db_connection(path, immutable=False):
    uri = "file:" + quote(str(path.resolve()), safe="/") + "?mode=ro"
    if immutable:
        uri += "&immutable=1"
    return sqlite3.connect(uri, uri=True, timeout=5)


def integrity(path, deadline):
    with closing(db_connection(path, immutable=True)) as connection:
        connection.set_progress_handler(lambda: int(time.monotonic() > deadline), 10000)
        if connection.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
            raise BackupError("database integrity check failed")
    check_time(deadline)


def stopped(connection):
    exists = connection.execute("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='platform_worker_instance'").fetchone()[0]
    if exists and connection.execute("SELECT COUNT(*) FROM platform_worker_instance WHERE julianday(lease_until)>julianday('now')").fetchone()[0]:
        raise BackupError("active worker lease; stop service and wait for lease expiry")


def new_directory(path):
    # Exclusive mkdir is the reservation; no existing directory is overwritten.
    path.mkdir(mode=0o700)
    os.chmod(path, 0o700)


def fsync_directory(path):
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def verify_bundle(bundle, deadline):
    info = bundle.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) & 0o077:
        raise BackupError("private regular bundle directory required")
    manifest_path = bundle / "manifest.json"
    regular(manifest_path, private=True)
    if manifest_path.stat().st_size > 65536:
        raise BackupError("manifest exceeds limit")
    with read_file(manifest_path) as source:
        manifest = json.load(source)
    if not isinstance(manifest, dict) or set(manifest) != {"version", "created_at", "files"} or type(manifest["version"]) is not int or manifest["version"] != 1:
        raise BackupError("unsupported manifest")
    datetime.datetime.fromisoformat(manifest["created_at"])
    files = manifest["files"]
    if not isinstance(files, dict) or set(files) not in ({"pr_agent.db", "config.yaml"}, {"pr_agent.db", "config.yaml", "aimangebot"}):
        raise BackupError("invalid manifest file set")
    if {p.name for p in bundle.iterdir()} != set(files) | {"manifest.json"}:
        raise BackupError("unexpected bundle contents")
    for name, expected in files.items():
        if not isinstance(expected, dict) or set(expected) != {"size", "sha256"} or type(expected["size"]) is not int or expected["size"] < 0 or not isinstance(expected["sha256"], str) or not re.fullmatch(r"[0-9a-f]{64}", expected["sha256"]):
            raise BackupError("invalid manifest digest")
        regular(bundle / name, private=True)
        if digest(bundle / name, deadline) != expected:
            raise BackupError("bundle content checksum mismatch")
    integrity(bundle / "pr_agent.db", deadline)
    return manifest


def backup(database, config, output, binary, service_stopped, deadline):
    if not service_stopped:
        raise BackupError("stop all service writers and specify --service-stopped")
    regular(database)
    regular(config, private=True)
    if config.stat().st_size > 1024 * 1024:
        raise BackupError("configuration exceeds limit")
    sources = [database, config] + ([binary] if binary else [])
    for source in sources:
        regular(source)
        if output.resolve().is_relative_to(source.parent.resolve()):
            raise BackupError("output must be outside source directories")
    before = digest(config, deadline)
    created = False
    try:
        # First validate the lease; avoid leaving output on admission failure.
        with closing(db_connection(database)) as origin:
            stopped(origin)
            new_directory(output)
            created = True
            db_path = output / "pr_agent.db"
            fd = os.open(db_path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
            os.close(fd)
            with closing(sqlite3.connect(db_path)) as target:
                origin.backup(target, pages=128, progress=lambda *args: check_time(deadline), sleep=0.01)
                target.execute("PRAGMA journal_mode=DELETE")
            stopped(origin)
        integrity(db_path, deadline)
        with db_path.open("rb") as dest:
            os.fsync(dest.fileno())
        copy_private(config, output / "config.yaml", 0o600, deadline)
        if digest(config, deadline) != before or digest(output / "config.yaml", deadline) != before:
            raise BackupError("configuration changed during backup")
        names = ["pr_agent.db", "config.yaml"]
        if binary:
            copy_private(binary, output / "aimangebot", 0o700, deadline)
            names.append("aimangebot")
        manifest = {"version": 1, "created_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "files": {name: digest(output / name, deadline) for name in names}}
        with (output / "manifest.json").open("x") as dest:
            os.chmod(output / "manifest.json", 0o600)
            json.dump(manifest, dest, sort_keys=True)
            dest.flush()
            os.fsync(dest.fileno())
        fsync_directory(output)
        verify_bundle(output, deadline)
        return manifest
    except BaseException:
        if created:
            shutil.rmtree(output)
        raise


def restore(bundle, output, deadline):
    manifest = verify_bundle(bundle, deadline)
    if output.resolve().is_relative_to(bundle.resolve()):
        raise BackupError("restore must be outside backup directory")
    created = False
    try:
        new_directory(output)
        created = True
        for name, expected in manifest["files"].items():
            copy_private(bundle / name, output / name, 0o700 if name == "aimangebot" else 0o600, deadline)
            if digest(output / name, deadline) != expected:
                raise BackupError("restored content checksum mismatch")
        integrity(output / "pr_agent.db", deadline)
        fsync_directory(output)
        return manifest
    except BaseException:
        if created:
            shutil.rmtree(output)
        raise


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--timeout", type=int, default=30, help="total operation deadline, 1–300 seconds")
    sub = parser.add_subparsers(dest="command", required=True)
    create = sub.add_parser("backup")
    create.add_argument("--database", type=Path, required=True)
    create.add_argument("--config", type=Path, required=True)
    create.add_argument("--output", type=Path, required=True)
    create.add_argument("--binary", type=Path)
    create.add_argument("--service-stopped", action="store_true")
    for command in ("verify", "restore"):
        action = sub.add_parser(command)
        action.add_argument("--bundle", type=Path, required=True)
        if command == "restore":
            action.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not 1 <= args.timeout <= 300:
        parser.error("timeout must be 1–300 seconds")
    deadline = time.monotonic() + args.timeout
    try:
        if args.command == "backup":
            manifest = backup(args.database, args.config, args.output, args.binary, args.service_stopped, deadline)
        elif args.command == "verify":
            manifest = verify_bundle(args.bundle, deadline)
        else:
            manifest = restore(args.bundle, args.output, deadline)
        print(json.dumps({"operation": args.command, "status": "ok", "files": len(manifest["files"])}))
    except (BackupError, OSError, sqlite3.Error, ValueError, TypeError, KeyError):
        # Never emit raw configuration, database rows or exceptions containing
        # private paths/content. Detailed investigation stays with the operator.
        print(json.dumps({"operation": args.command, "status": "failed", "reason": "validation, lease, permission, integrity or timeout check failed"}))
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
