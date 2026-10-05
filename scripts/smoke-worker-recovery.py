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
    parser.add_argument("--lifecycle-preview", action="store_true", help="Verify recurring and absent findings across three pinned heads")
    parser.add_argument("--group-preview", action="store_true", help="Verify actual grouped25-file audit and synthesis")
    parser.add_argument("--metadata-preview", action="store_true", help="Also verify a metadata-only finding and note-only diagram")
    parser.add_argument("--verification-preview", action="store_true", help="Verify fresh independent metadata evidence; requires metadata preview")
    parser.add_argument("--comment-preview", action="store_true", help="Verify synthetic GitLab create and review update; requires metadata preview")
    parser.add_argument("--ui-preview", action="store_true", help="Keep fixture online after proof until SIGTERM for browser inspection")
    parser.add_argument("--backup-preview", action="store_true", help="Verify private backup/restore with the same native binary; requires metadata/lifecycle/comment previews")
    parser.add_argument("--app-port", type=int, default=19234)
    parser.add_argument("--upstream-port", type=int, default=19235)
    args = parser.parse_args()
    if args.backup_preview and not (args.metadata_preview and args.lifecycle_preview and args.comment_preview):
        parser.error("--backup-preview requires --metadata-preview --lifecycle-preview --comment-preview")
    if args.comment_preview and not args.metadata_preview:
        parser.error("--comment-preview requires --metadata-preview")
    if args.verification_preview and not args.metadata_preview:
        parser.error("--verification-preview requires --metadata-preview")
    def stop_fixture(*unused):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, stop_fixture)
    binary = str(Path(args.binary).resolve(strict=True))
    # Refuse ports commonly used by the actual workspace deployment.
    if args.app_port in (1234, 8080) or args.upstream_port in (1234, 8080) or args.app_port == args.upstream_port:
        raise ValueError("choose distinct temporary ports, not production1234/8080")
    root = Path(tempfile.mkdtemp(prefix="aimangebot-recovery-smoke."))
    calls = {90: 0, 91: 0, 92: 0, 93: 0, 94: 0}
    lifecycle_phase = [1]
    lock = threading.Lock()
    comments = {}
    comment_creates, comment_updates = {}, {}
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
            if path == "/api/v4/user":
                self.send_json({"id": 7})
                return
            match_comment = re.fullmatch(r"/api/v4/projects/1/merge_requests/(90|91|92|93|94)/discussions(?:/synthetic-(90|91|92|93|94))?", path)
            if match_comment:
                iid = int(match_comment.group(1))
                with lock:
                    body = comments.get(iid)
                if body is None:
                    self.send_json([])
                else:
                    discussion = {"id": f"synthetic-{iid}", "notes": [{"id": iid, "body": body, "author": {"id": 7}}]}
                    self.send_json(discussion if match_comment.group(2) else [discussion])
                return
            if path == "/api/v4/projects/1/repository/tree":
                object_id = hashlib.sha1(b"blob 7\x00change\n").hexdigest()
                head = "ref=" + head_sha in self.path
                if any("ref=" + ch * 40 in self.path for ch in "cde"):
                    body = b"prefix\nchange\n" if "ref=" + "d" * 40 in self.path else b"change\n"
                    object_id = hashlib.sha1(b"blob " + str(len(body)).encode() + b"\x00" + body).hexdigest()
                    head = True
                if not head:
                    object_id = hashlib.sha1(b"blob 4\x00old\n").hexdigest()
                self.send_json([{"path": f"f{i:02d}.any", "mode": "100644", "type": "blob", "id": object_id} for i in range(24)] + [{"path": "a.any", "mode": "100644", "type": "blob", "id": object_id}, {"path": "entry.any", "mode": "100755" if head else "100644", "type": "blob", "id": metadata_object}])
                return
            if "/repository/files/" in path:
                body = b"prefix\nchange\n" if "ref=" + "d" * 40 in self.path else b"change\n"
                self.send_json({"file_path": "a.any", "encoding": "base64", "content": base64.b64encode(body).decode(), "size": len(body)})
                return
            match = re.fullmatch(r"/api/v4/projects/1/merge_requests/(90|91|92|93|94)(/versions(?:/1)?)?", path)
            if not match:
                self.send_json({"message": "fixture endpoint unavailable"}, 404)
                return
            version = {"id": 1, "base_commit_sha": base_sha, "head_commit_sha": head_sha}
            if match.group(1) == "94":
                with lock:
                    phase = lifecycle_phase[0]
                version["head_commit_sha"] = {1: "c", 2: "d", 3: "e"}[phase] * 40
            suffix = match.group(2)
            if suffix == "/versions":
                self.send_json([version])
            elif suffix == "/versions/1":
                diffs = [{"old_path": "a.any", "new_path": "a.any", "new_file": True, "diff": "@@ -0,0 +1 @@\n+change"}]
                if match.group(1) == "92":
                    diffs = [{"old_path": "entry.any", "new_path": "entry.any", "diff": ""}]
                if match.group(1) == "93":
                    diffs = [{"old_path": p, "new_path": p, "diff": "@@ -1 +1 @@\n-old\n+change"} for p in ["a.any"] + [f"f{i:02d}.any" for i in range(24)]]
                if match.group(1) == "94":
                    diff = "@@ -1 +1 @@\n-old\n+change" if phase != 2 else "@@ -1 +1,2 @@\n-old\n+prefix\n+change"
                    diffs = [{"old_path": "a.any", "new_path": "a.any", "diff": diff}]
                version.update(state="collected", real_size=str(len(diffs)), diffs=diffs)
                self.send_json(version)
            else:
                self.send_json({"iid": int(match.group(1)), "source_project_id": 1, "title": "合成验证：重试与恢复", "web_url": "", "diff_refs": {"base_sha": base_sha, "head_sha": version["head_commit_sha"]}})

        def do_POST(self):
            match_comment = re.fullmatch(r"/api/v4/projects/1/merge_requests/(90|91|92|93|94)/discussions", self.path)
            if match_comment:
                iid = int(match_comment.group(1))
                request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
                with lock:
                    comments[iid] = request["body"]
                    comment_creates[iid] = comment_creates.get(iid, 0) + 1
                self.send_json({"id": f"synthetic-{iid}", "notes": [{"id": iid, "body": request["body"], "author": {"id": 7}}]}, 201)
                return
            if self.path != "/v1/chat/completions":
                self.send_json({"message": "fixture endpoint unavailable"}, 404)
                return
            request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            content = " ".join(str(message.get("content", "")) for message in request["messages"] if message.get("role") == "user")
            iid = 90 if '"mr_iid":90' in content else 92 if '"mr_iid":92' in content else 93 if '"mr_iid":93' in content else 94 if '"mr_iid":94' in content else 91
            with lock:
                calls[iid] += 1
                number = calls[iid]
            if iid == 94:
                moved = '"head_sha":"' + "d" * 40 + '"' in content
                absent = '"head_sha":"' + "e" * 40 + '"' in content
                line = 2 if moved else 1
                systems = [str(m.get("content", "")) for m in request["messages"] if m.get("role") == "system"]
                independent = any(m.startswith("Independently review") for m in systems)
                diagram = any(m.startswith("Generate a static sequence diagram") for m in systems)
                observations = [json.loads(m["content"]).get("observation_id") for m in request["messages"] if m.get("role") == "tool"]
                observations = [o for o in observations if o]
                if independent and observations:
                    message = {"role": "assistant", "content": json.dumps({"status": "rejected", "reason": "合成文本不构成漏洞，仅用于历史与恢复演练。", "limitations": ["合成响应不验证真实模型准确率。"], "observation_ids": observations}, ensure_ascii=False)}
                    finish = "stop"
                elif diagram:
                    message = {"role": "assistant", "content": json.dumps({"participants": [{"id": "caller", "label": "合成调用方"}, {"id": "source", "label": "合成文件"}], "steps": [{"from": "caller", "to": "source", "label": "固定版本的合成文本，仅用于演练", "kind": "note", "certainty": "cited", "risk": True, "evidence": [{"side": "head", "file": "a.any", "line": line, "snippet": "change"}]}], "limitations": ["未执行代码，不代表真实漏洞。"]}, ensure_ascii=False)}
                    finish = "stop"
                elif not observations and not absent:
                    message = {"role": "assistant", "content": None, "tool_calls": [{"id": "fixture_lifecycle_read", "type": "function", "function": {"name": "read_file", "arguments": json.dumps({"path": "a.any", "start": line, "end": line})}}]}
                    finish = "tool_calls"
                else:
                    findings = [] if absent else [{"side": "head", "file": "a.any", "line": line, "severity": "medium", "type": "synthetic lifecycle", "title": "合成历史候选：行号移动" if moved else "合成历史候选", "description": "仅验证跨版本历史，不代表真实漏洞。", "evidence": "change", "trigger": "合成固定触发条件", "suggestion": "人工核验当前版本", "confidence": "candidate", "observation_ids": ["observation-1"]}]
                    message = {"role": "assistant", "content": json.dumps({"findings": findings, "summary": "合成问题生命周期验证，未执行仓库代码。", "coverage_notes": []}, ensure_ascii=False)}
                    finish = "stop"
                self.send_json({"id": "fixture-lifecycle", "object": "chat.completion", "model": "synthetic-recovery", "choices": [{"index": 0, "message": message, "finish_reason": finish}]})
                return
            if iid == 93:
                synthesis = any(str(m.get("content", "")).startswith("Summarize a grouped") for m in request["messages"] if m.get("role") == "system")
                finish = "stop"
                if synthesis:
                    message = {"role": "assistant", "content": json.dumps({"summary": "合成跨组汇总：25个文件分为2组；保留候选，不代表模型准确率或运行复现。", "coverage_notes": []}, ensure_ascii=False)}
                elif number == 1:
                    finish = "tool_calls"
                    message = {"role": "assistant", "content": None, "tool_calls": [{"id": "fixture_group_read", "type": "function", "function": {"name": "read_file", "arguments": json.dumps({"path": "a.any", "start": 1, "end": 1})}}]}
                else:
                    findings = []
                    if number == 2:
                        findings = [{"side": "head", "file": "a.any", "line": 1, "severity": "medium", "type": "synthetic grouping", "title": "合成分组候选", "description": "仅验证分组合并与证据保留。", "evidence": "change", "trigger": "合成测试条件，不代表真实漏洞", "suggestion": "人工核验", "confidence": "candidate", "observation_ids": ["group-1-observation-1"]}]
                    message = {"role": "assistant", "content": json.dumps({"findings": findings, "summary": "合成分组结果", "coverage_notes": []}, ensure_ascii=False)}
                self.send_json({"id": "fixture-group", "object": "chat.completion", "model": "synthetic-recovery", "choices": [{"index": 0, "message": message, "finish_reason": finish}]})
                return
            if iid == 92:
                independent = any(str(m.get("content", "")).startswith("Independently review") for m in request["messages"] if m.get("role") == "system")
                if independent:
                    observations = []
                    for m in request["messages"]:
                        if m.get("role") == "tool":
                            output = json.loads(m["content"])
                            if output.get("observation_id"):
                                observations.append(output["observation_id"])
                    if not observations:
                        message = {"role": "assistant", "content": None, "tool_calls": [{"id": "fixture_verify_metadata", "type": "function", "function": {"name": "get_change_metadata", "arguments": json.dumps({"path": "entry.any"})}}]}
                        finish = "tool_calls"
                    else:
                        message = {"role": "assistant", "content": json.dumps({"status": "supported", "reason": "固定提交的独立读取确认执行位变化，仅支持条件性风险描述。", "limitations": ["合成模型响应，未验证真实模型准确率或部署可达性。"], "observation_ids": observations}, ensure_ascii=False)}
                        finish = "stop"
                elif number == 1:
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

        def do_PUT(self):
            match = re.fullmatch(r"/api/v4/projects/1/merge_requests/(90|91|92|93|94)/discussions/synthetic-(90|91|92|93|94)/notes/(90|91|92|93|94)", self.path)
            if not match or len(set(match.groups())) != 1:
                self.send_json({"message": "fixture endpoint unavailable"}, 404)
                return
            iid = int(match.group(1))
            request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            with lock:
                comments[iid] = request["body"]
                comment_updates[iid] = comment_updates.get(iid, 0) + 1
            self.send_json({"id": iid, "body": request["body"], "author": {"id": 7}})

    upstream = ThreadingHTTPServer(("127.0.0.1", args.upstream_port), Fixture)
    threading.Thread(target=upstream.serve_forever, daemon=True).start()
    config = {"listen": f"127.0.0.1:{args.app_port}", "gitlab": {"url": f"http://127.0.0.1:{args.upstream_port}", "token": "synthetic-token"}, "openai": {"url": f"http://127.0.0.1:{args.upstream_port}/v1", "api_key": "synthetic-key", "model": "synthetic-recovery"}, "projects": [{"id": 1, "name": "合成验证项目", "enabled": True}], "enable_polling": False, "enable_webhook": False, "enable_mr_comment": args.comment_preview, "audit_workers": 1, "audit_timeout_seconds": 90, "whitelist_extensions": [], "react": {"enabled": True, "temperature": 0.1, "max_steps": 16}, "mcp": {"enabled": False}, "git_audit": {"enabled": False}, "verify_findings": args.verification_preview, "generate_sequence_diagrams": args.metadata_preview}
    (root / "config.yaml").write_text(json.dumps(config))  # JSON is valid YAML.
    (root / "config.yaml").chmod(0o600)
    env = dict(os.environ, AIM_ADMIN_USERNAME="recovery-fixture-admin", AIM_ADMIN_PASSWORD="recovery-fixture-password")
    processes, logs = [], []
    base_url = f"http://localhost:{args.app_port}"
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    db = None

    def start(name, directory=None):
        log = open(root / (name + ".log"), "w")
        logs.append(log)
        process = subprocess.Popen([binary], cwd=directory or root, env=env, stdout=log, stderr=log)
        processes.append(process)
        return process

    def healthy():
        try:
            return urllib.request.urlopen(base_url + "/healthz", timeout=0.3).status == 200
        except Exception:
            return False

    def api(path, value=None, method=None):
        request = urllib.request.Request(base_url + "/api/v1" + path, data=json.dumps(value).encode() if value is not None else None, headers={"Content-Type": "application/json", "Origin": base_url}, method=method)
        with opener.open(request, timeout=5) as response:
            return None if response.status == 204 else json.load(response)

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
            assert calls[92] == (5 if args.verification_preview else 3)
            if args.verification_preview:
                verification = finding["verification"]
                assert verification["status"] == "supported" and verification["observation_ids"]
                assert all(v.startswith("verify-") for v in verification["observation_ids"])
                assert any(t.get("stage") == "verification" and t.get("observation_id") in verification["observation_ids"] for t in audited["trace"])
                proof.update(independent_verification="supported", fresh_verification_observations=verification["observation_ids"])
            proof.update(metadata_run=metadata_run, metadata_finding_verified=True, metadata_note_diagram="partial", preview_url=base_url+f"/#/runs/{metadata_run}")
        if args.group_preview:
            group_run = api("/runs", {"project_id": 1, "mr_iid": 93})["id"]
            wait_for(lambda: row(group_run)[0] == "succeeded", timeout=15)
            grouped = api(f"/runs/{group_run}")["run"]
            groups = grouped["result"]["audit_groups"]
            assert len(groups) == 2 and all(g["status"] == "completed" for g in groups)
            assert sum(len(g["files"]) for g in groups) == 25
            assert len(grouped["result"]["findings"]) == 1 and grouped["result"]["findings"][0]["confidence"] == "candidate"
            assert any(t.get("observation_id") == "group-1-observation-1" for t in grouped["trace"])
            assert any(t.get("stage") == "synthesis" for t in grouped["trace"]) and calls[93] == 4
            proof.update(group_run=group_run, completed_groups=2, grouped_files=25, grouped_finding_retained=True, preview_url=base_url+f"/#/runs/{group_run}")
        if args.lifecycle_preview:
            lifecycle_runs = []
            for phase in (1, 2, 3):
                with lock:
                    lifecycle_phase[0] = phase
                current = api("/runs", {"project_id": 1, "mr_iid": 94})["id"]
                lifecycle_runs.append(current)
                wait_for(lambda: row(current)[0] == "succeeded", timeout=15)
                detail = api(f"/runs/{current}")
                if phase == 1:
                    original_finding = detail["run"]["result"]["findings"][0]
                    api(f"/runs/{current}/findings/{original_finding['id']}/review", {"status": "fixed", "reason": "合成旧版本人工决定，不能自动继承。"}, method="PUT")
                elif phase == 2:
                    moved_finding = detail["run"]["result"]["findings"][0]
                    assert moved_finding["line"] == 2 and moved_finding["id"] != original_finding["id"] and moved_finding["fingerprint"] == original_finding["fingerprint"]
                    history = detail["finding_lifecycle"]["current"][0]
                    assert history["first_run_id"] == lifecycle_runs[0] and len(history["occurrences"]) == 2 and history["reviews"][0]["status"] == "fixed"
                    assert not detail["reviews"], "old human decision copied to new HEAD"
                else:
                    assert not detail["run"]["result"]["findings"] and not detail["reviews"]
                    assert detail["finding_lifecycle"]["not_reobserved"][0]["run_id"] == lifecycle_runs[1]
            assert calls[94] == (9 if args.verification_preview else 7 if args.metadata_preview else 5)
            proof.update(lifecycle_runs=lifecycle_runs, recurring_fingerprint_preserved=True, human_decision_not_copied=True, absence_not_auto_fixed=True, preview_url=base_url+f"/#/runs/{lifecycle_runs[1]}")
        if args.comment_preview:
            wait_for(lambda: api(f"/runs/{metadata_run}")["comment_sync"]["state"] == "sent")
            api(f"/runs/{metadata_run}/findings/{finding['id']}/review", {"status": "false_positive", "reason": "合成同步验证：未运行复现，仅验证评论更新。"}, method="PUT")
            wait_for(lambda: api(f"/runs/{metadata_run}")["comment_sync"]["sent_generation"] == 2)
            with lock:
                assert comment_creates[92] == 1 and comment_updates[92] == 1 and "误报" in comments[92]
            proof.update(comment_run=metadata_run, comment_single_create=True, comment_review_update=True)
        if args.backup_preview:
            from ops_restore_drill import run_backup_drill
            def upstream_counts():
                with lock:
                    return {"model": dict(calls), "creates": dict(comment_creates), "updates": dict(comment_updates)}
            proof["backup_restore"] = run_backup_drill(root=root, binary=binary, db=db, api=api, start=start, processes=processes, healthy=healthy, wait_for=wait_for, base_url=base_url, lifecycle_runs=lifecycle_runs, crash_parent=crash_parent, metadata_run=metadata_run, upstream_counts=upstream_counts)
            db = None  # The helper closed the source before restoring.
        (root / "proof.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2))
        (root / "proof.json").chmod(0o600)
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
