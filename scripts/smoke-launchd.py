#!/usr/bin/env python3
"""Isolated native launchd crash/restart proof. No production or remote traffic."""
import argparse
from contextlib import closing
import http.cookiejar
import json
import os
from pathlib import Path
import re
import signal
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--port", type=int, default=19240)
    args = parser.parse_args()
    if sys.platform != "darwin" or not Path(args.binary).is_absolute() or not 19000 <= args.port <= 19999:
        parser.error("requires macOS, absolute binary, isolated port 19000–19999")
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", args.port))
    root = Path(tempfile.mkdtemp(prefix="aimangebot-launchd-smoke."))
    root.chmod(0o700)
    cfg = {"listen": f"127.0.0.1:{args.port}", "gitlab": {"url": "http://127.0.0.1:9", "token": "synthetic-token"}, "openai": {"url": "http://127.0.0.1:9/v1", "api_key": "synthetic-key", "model": "synthetic"}, "projects": [], "enable_polling": False, "enable_webhook": False, "enable_mr_comment": False, "audit_workers": 1, "git_audit": {"enabled": False}, "react": {"enabled": True}}
    config = root / "config.yaml"
    config.write_text(json.dumps(cfg))
    config.chmod(0o600)
    base = f"http://127.0.0.1:{args.port}"
    domain = f"gui/{os.getuid()}"
    label = "local.aimangebot.smoke." + uuid.uuid4().hex
    target = domain + "/" + label
    def control(*parts, check=True):
        return subprocess.run(["launchctl", *parts], capture_output=True, text=True, timeout=10, check=check)
    def ready():
        try:
            with urllib.request.urlopen(base + "/readyz", timeout=1) as response:
                return response.status == 200 and json.load(response) == {"status": "ready"}
        except Exception:
            return False
    def wait(predicate, timeout=65):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if predicate():
                return
            time.sleep(0.2)
        raise RuntimeError("isolated launchd assertion timed out")
    def instance():
        with closing(sqlite3.connect(root / "pr_agent.db")) as db:
            return db.execute("SELECT owner,julianday(lease_until)>julianday('now') FROM platform_worker_instance WHERE id=1").fetchone()
    def pid():
        state = control("print", target).stdout
        match = re.search(r"^\s*pid = (\d+)\s*$", state, re.M)
        return int(match.group(1)) if match else 0
    foreground = None
    installed = False
    try:
        with open(root / "bootstrap.log", "x") as log:
            os.fchmod(log.fileno(), 0o600)
            foreground = subprocess.Popen([args.binary], cwd=root, env=dict(os.environ, AIM_ADMIN_USERNAME="launchd-fixture-admin", AIM_ADMIN_PASSWORD="launchd-fixture-password"), stdout=log, stderr=log)
            wait(lambda: ready() or foreground.poll() is not None)
            assert foreground.poll() is None and ready(), "bootstrap failed"
            foreground.terminate()
            foreground.wait(timeout=45)
            assert foreground.returncode == 0
        definition = root / "service.plist"
        subprocess.run([sys.executable, str(Path(__file__).with_name("ops-service.py")), "launchd", "--binary", args.binary, "--directory", str(root), "--label", label, "--output", str(definition)], check=True, capture_output=True, timeout=10)
        control("bootstrap", domain, str(definition))
        installed = True
        wait(ready)
        first_pid, first_owner = pid(), instance()
        assert first_pid > 0 and first_owner[1]
        started = time.monotonic()
        os.kill(first_pid, signal.SIGKILL)
        wait(lambda: pid() not in (0, first_pid) and ready())
        restart_seconds = time.monotonic() - started
        second_pid, second_owner = pid(), instance()
        assert second_owner[1] and second_owner[0] != first_owner[0]
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        request = urllib.request.Request(base + "/api/v1/auth/login", data=json.dumps({"username": "launchd-fixture-admin", "password": "launchd-fixture-password"}).encode(), headers={"Content-Type": "application/json", "Origin": base})
        with opener.open(request, timeout=3) as response:
            assert json.load(response)["username"] == "launchd-fixture-admin"
        control("bootout", target)
        installed = False
        wait(lambda: not ready(), timeout=10)
        assert control("print", target, check=False).returncode != 0
        proof = {"synthetic_only": True, "bootstrap_normal_exit": True, "launchd_sigkill_restarted": True, "different_pid": second_pid != first_pid, "different_valid_worker_owner": True, "readyz_after_restart": True, "account_preserved": True, "service_unloaded": True, "restart_observed_seconds": round(restart_seconds, 2)}
        receipt = root / "proof.json"
        receipt.write_text(json.dumps(proof, indent=2))
        receipt.chmod(0o600)
        print(json.dumps(proof))
    finally:
        if installed:
            control("bootout", target, check=False)
        if foreground is not None and foreground.poll() is None:
            foreground.terminate()
            foreground.wait(timeout=45)


if __name__ == "__main__":
    main()
