#!/usr/bin/env python3
"""Isolated binary proof: upstream delay, SIGKILL checkpoint recovery, lease fencing.

Run after building: python3 scripts/smoke-worker-recovery.py --binary /tmp/aimangebot
Only synthetic localhost services and a temporary database are used. No real model,
GitLab, repository code execution or production settings are involved.
"""
import argparse
import hashlib
import base64
import http.cookiejar
import json
import os
from pathlib import Path
import re
import signal
import sqlite3
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import urllib.request


def wait_for(check, timeout=15):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(0.1)
    raise RuntimeError("fixture condition did not become true")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--metadata-preview", action="store_true", help="Also verify a metadata-only finding and note-only diagram")
    parser.add_argument("--ui-preview", action="store_true", help="Keep fixture online after proof until SIGTERM for browser inspection")
    parser.add_argument("--app-port", type=int, default=19234)
    parser.add_argument("--upstream-port", type=int, default=19235)
    args = parser.parse_args()
    def stop_fixture(*unused):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, stop_fixture)
    binary = str(Path(args.binary).resolve(strict=True))
    # Refuse ports commonly used by the actual workspace deployment.
    if args.app_port in (1234, 8080) or args.upstream_port in (1234, 8080) or args.app_port == args.upstream_port:
        raise ValueError("choose distinct temporary ports, not production1234/8080")
    root = Path(tempfile.mkdtemp(prefix="aimangebot-recovery-smoke."))
    calls = {90: 0, 91: 0, 92: 0}
    lock = threading.Lock()
    base_sha, head_sha = "a" * 40, "b" * 40
    metadata_content = b"untrusted(command)\n"
    metadata_object = hashlib.sha1(b"blob " + str(len(metadata_content)).encode() + b"\x00" + metadata_content).hexdigest()
    metadata = {"kind": "modify", "old_path": "entry.any", "new_path": "entry.any", "base": {"mode": "100644", "type": "blob", "object_id": metadata_object}, "head": {"mode": "100755", "type": "blob", "object_id": metadata_object}}
    metadata_evidence = json.dumps(metadata, separators=(",", ":"))
    metadata_finding = {"anchor_type": "git_metadata", "side": "head", "file": "entry.any", "line": 0, "severity": "medium", "type": "execution policy", "title": "合成候选：文件执行模式变化", "description": "这是界面与证据链验证用的候选，未确认实际部署会执行此文件。", "evidence": metadata_evidence, "trigger": "部署环境按执行位启动此文件，且未做额外入口校验时", "suggestion": "核验实际启动入口与执行策略", "confidence": "candidate", "observation_ids": ["observation-1"]}

    class Fixture(BaseHTTPRequestHandler):
        def log_message(self, *unused):
            pass  # Never log request headers, credentials or model messages.

        def send_json(self, value, status=200, retry=None):
            body = json.dumps(value).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            if retry is not None:
                self.send_header("Retry-After", str(retry))
            self.end_headers()
            try:
                self.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError):
                pass

        def do_GET(self):
            path = self.path.split("?", 1)[0]
            if path == "/api/v4/projects/1/repository/tree":
                object_id = hashlib.sha1(b"blob 7\x00change\n").hexdigest()
                head = "ref=" + head_sha in self.path
                self.send_json([{"path": "a.any", "mode": "100644", "type": "blob", "id": object_id}, {"path": "entry.any", "mode": "100755" if head else "100644", "type": "blob", "id": metadata_object}])
                return
            if "/repository/files/" in path:
                self.send_json({"file_path": "a.any", "encoding": "base64", "content": base64.b64encode(b"change\n").decode(), "size": 7})
                return
            match = re.fullmatch(r"/api/v4/projects/1/merge_requests/(90|91|92)(/versions(?:/1)?)?", path)
            if not match:
                self.send_json({"message": "fixture endpoint unavailable"}, 404)
                return
            version = {"id": 1, "base_commit_sha": base_sha, "head_commit_sha": head_sha}
            suffix = match.group(2)
            if suffix == "/versions":
                self.send_json([version])
            elif suffix == "/versions/1":
                diffs = [{"old_path": "a.any", "new_path": "a.any", "new_file": True, "diff": "@@ -0,0 +1 @@\n+change"}]
                if match.group(1) == "92":
                    diffs = [{"old_path": "entry.any", "new_path": "entry.any", "diff": ""}]
                version.update(state="collected", real_size="1", diffs=diffs)
                self.send_json(version)
            else:
                self.send_json({"iid": int(match.group(1)), "source_project_id": 1, "title": "合成验证：重试与恢复", "web_url": "", "diff_refs": {"base_sha": base_sha, "head_sha": head_sha}})

        def do_POST(self):
            if self.path != "/v1/chat/completions":
                self.send_json({"message": "fixture endpoint unavailable"}, 404)
                return
            request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            content = " ".join(str(message.get("content", "")) for message in request["messages"] if message.get("role") == "user")
            iid = 90 if '"mr_iid":90' in content else 92 if '"mr_iid":92' in content else 91
            with lock:
                calls[iid] += 1
                number = calls[iid]
            if iid == 92:
                if number == 1:
                    message = {"role": "assistant", "content": None, "tool_calls": [{"id": "fixture_metadata", "type": "function", "function": {"name": "get_change_metadata", "arguments": json.dumps({"path": "entry.any"})}}]}
                    finish = "tool_calls"
                elif number == 2:
                    message = {"role": "assistant", "content": json.dumps({"findings": [metadata_finding], "summary": "合成验证：元数据候选与时序图，不代表实际审计准确率。", "coverage_notes": []}, ensure_ascii=False)}
                    finish = "stop"
                else:
                    diagram = {"participants": [{"id": "deploy", "label": "部署环境"}, {"id": "entry", "label": "入口文件"}], "steps": [{"from": "deploy", "to": "entry", "label": "执行位由普通文件变为可执行文件；实际启动仍需核验", "kind": "note", "certainty": "cited", "risk": True, "evidence": [{"anchor_type": "git_metadata", "side": "head", "file": "entry.any", "line": 0, "snippet": metadata_evidence}]}], "limitations": ["合成示例，未执行仓库代码，未证明部署中的执行关系。"]}
                    message = {"role": "assistant", "content": json.dumps(diagram, ensure_ascii=False)}
                    finish = "stop"
                self.send_json({"id": "fixture-metadata", "object": "chat.completion", "model": "synthetic-recovery", "choices": [{"index": 0, "message": message, "finish_reason": finish}]})
                return
            if iid == 90:
                self.send_json({"error": {"message": "synthetic rate limit"}}, 429, retry=300)
                return
            if number == 1:
                message = {"role": "assistant", "content": None, "tool_calls": [{"id": "fixture_read", "type": "function", "function": {"name": "read_file", "arguments": json.dumps({"path": "a.any", "start": 1, "end": 1})}}]}
                finish = "tool_calls"
            else:
                if number == 2:
                    time.sleep(60)  # Simulate a model request in flight at the crash.
                message = {"role": "assistant", "content": json.dumps({"findings": [], "summary": "合成模型：仅验证任务恢复，不代表实际代码审计。", "coverage_notes": []}, ensure_ascii=False)}
                finish = "stop"
            self.send_json({"id": "fixture-completion", "object": "chat.completion", "model": "synthetic-recovery", "choices": [{"index": 0, "message": message, "finish_reason": finish}]})

    upstream = ThreadingHTTPServer(("127.0.0.1", args.upstream_port), Fixture)
    threading.Thread(target=upstream.serve_forever, daemon=True).start()
    config = {"listen": f"127.0.0.1:{args.app_port}", "gitlab": {"url": f"http://127.0.0.1:{args.upstream_port}", "token": "synthetic-token"}, "openai": {"url": f"http://127.0.0.1:{args.upstream_port}/v1", "api_key": "synthetic-key", "model": "synthetic-recovery"}, "projects": [{"id": 1, "name": "合成验证项目", "enabled": True}], "enable_polling": False, "enable_webhook": False, "enable_mr_comment": False, "audit_workers": 1, "audit_timeout_seconds": 90, "whitelist_extensions": [], "react": {"enabled": True, "temperature": 0.1, "max_steps": 16}, "mcp": {"enabled": False}, "git_audit": {"enabled": False}, "generate_sequence_diagrams": args.metadata_preview}
    (root / "config.yaml").write_text(json.dumps(config))  # JSON is valid YAML.
    env = dict(os.environ, AIM_ADMIN_USERNAME="recovery-fixture-admin", AIM_ADMIN_PASSWORD="recovery-fixture-password")
    processes, logs = [], []
    base_url = f"http://localhost:{args.app_port}"
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    db = None

    def start(name):
        log = open(root / (name + ".log"), "w")
        logs.append(log)
        process = subprocess.Popen([binary], cwd=root, env=env, stdout=log, stderr=log)
        processes.append(process)
        return process

    def healthy():
        try:
            return urllib.request.urlopen(base_url + "/healthz", timeout=0.3).status == 200
        except Exception:
            return False

    def api(path, value=None):
        request = urllib.request.Request(base_url + "/api/v1" + path, data=json.dumps(value).encode() if value is not None else None, headers={"Content-Type": "application/json", "Origin": base_url})
        return json.load(opener.open(request, timeout=5))

    def row(run_id):
        return db.execute("SELECT status,trace_json,result_json,retry_child_id FROM (SELECT *,COALESCE((SELECT child.id FROM platform_runs child WHERE child.retry_parent_id=r.id LIMIT 1),0) AS retry_child_id FROM platform_runs r) WHERE id=?", (run_id,)).fetchone()

    try:
        primary = start("primary")
        def primary_ready():
            if primary.poll() is not None:
                raise RuntimeError("primary failed to start; inspect isolated log")
            return healthy()
        wait_for(primary_ready)
        db = sqlite3.connect(root / "pr_agent.db", timeout=5)
        api("/auth/login", {"username": env["AIM_ADMIN_USERNAME"], "password": env["AIM_ADMIN_PASSWORD"]})
        rate_parent = api("/runs", {"project_id": 1, "mr_iid": 90})["id"]
        wait_for(lambda: row(rate_parent)[0] == "failed")
        rate_child = row(rate_parent)[3]
        child = api(f"/runs/{rate_child}")["run"]
        info = child["retry_info"]
        assert child["status"] == "pending" and info["kind"] == "rate_limit" and info["source"] == "model" and info["delay_seconds"] >= 299
        crash_parent = api("/runs", {"project_id": 1, "mr_iid": 91})["id"]
        wait_for(lambda: calls[91] == 2 and len(json.loads(row(crash_parent)[1])) >= 1)
        checkpoint = row(crash_parent)[1:3]
        owner, before = db.execute("SELECT owner,lease_until FROM platform_worker_instance").fetchone()
        wait_for(lambda: db.execute("SELECT lease_until FROM platform_worker_instance").fetchone()[0] != before, timeout=8)
        lease = db.execute("SELECT lease_until FROM platform_worker_instance").fetchone()[0]
        primary.kill()
        primary.wait(timeout=10)
        duplicate = start("immediate-restart")
        duplicate.wait(timeout=10)
        assert duplicate.returncode != 0 and "another" in (root / "immediate-restart.log").read_text()
        assert row(crash_parent)[0] == "running" and row(crash_parent)[3] == 0
        # Wait for the real persisted expiry; no database clock/lease rewriting.
        wait_for(lambda: db.execute("SELECT COALESCE(julianday(lease_until),0)<=julianday('now') FROM platform_worker_instance").fetchone()[0], timeout=35)
        replacement = start("replacement")
        wait_for(healthy)
        wait_for(lambda: row(crash_parent)[3] != 0)
        crash_child = row(crash_parent)[3]
        wait_for(lambda: row(crash_child)[0] == "succeeded", timeout=15)
        retained = row(crash_parent)
        assert retained[0] == "failed" and retained[1:3] == checkpoint
        assert db.execute("SELECT owner FROM platform_worker_instance").fetchone()[0] != owner
        recovered = api(f"/runs/{crash_child}")["run"]
        assert recovered["retry_attempt"] == 1 and recovered["retry_info"]["kind"] == "worker_interrupted"
        assert calls[90] == 1 and calls[91] == 3, "unexpected hidden/early HTTP retry"
        proof = {"fixture": str(root), "rate_parent": rate_parent, "rate_child": rate_child, "crash_parent": crash_parent, "crash_child": crash_child, "retry_after_seconds": info["delay_seconds"], "heartbeat_renewed": True, "sigkill_restart_rejected_until_expiry": True, "original_lease_waited": True, "parent_checkpoint_identical": True, "recovered_child_succeeded": True, "model_calls": calls, "preview_url": base_url + f"/#/runs/{rate_child}", "synthetic_only": True}
        if args.metadata_preview:
            metadata_run = api("/runs", {"project_id": 1, "mr_iid": 92})["id"]
            wait_for(lambda: row(metadata_run)[0] == "succeeded", timeout=15)
            audited = api(f"/runs/{metadata_run}")["run"]
            finding = audited["result"]["findings"][0]
            assert finding["anchor_type"] == "git_metadata" and finding["line"] == 0 and finding["metadata"]["head"]["mode"] == "100755"
            assert finding["sequence_diagram"]["status"] == "partial" and finding["sequence_diagram"]["steps"][0]["evidence"][0]["metadata"] == finding["metadata"]
            assert calls[92] == 3
            proof.update(metadata_run=metadata_run, metadata_finding_verified=True, metadata_note_diagram="partial", preview_url=base_url+f"/#/runs/{metadata_run}")
        (root / "proof.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2))
        print(json.dumps(proof, ensure_ascii=False), flush=True)
        if args.ui_preview:
            while True:
                time.sleep(1)
    finally:
        for process in processes:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
        if db is not None:
            db.close()
        upstream.shutdown()
        upstream.server_close()
        for log in logs:
            log.close()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        pass
