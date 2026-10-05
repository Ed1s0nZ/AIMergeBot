"""Native restore proof helper for the isolated smoke-worker-recovery fixture."""
from contextlib import closing
import hashlib
import http.cookiejar
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request


def fingerprint(run, finding):
    # Cross-checked against an actual server-produced fixture fingerprint before
    # constructing the synthetic persistence-only moved report below.
    fields = {"Version": "finding-fingerprint-v1", "Project": run["project_id"], "MR": run["mr_iid"], "Source": run["source_project_id"] or run["project_id"], "Side": finding.get("side") or "head", "File": finding["file"], "Anchor": finding.get("anchor_type") or "line", "Type": finding["type"], "Evidence": finding["evidence"], "Trigger": finding["trigger"]}
    return hashlib.sha256(json.dumps(fields, ensure_ascii=False, separators=(",", ":")).encode()).hexdigest()


def seed_move_report(db, prior_id):
    cursor = db.execute("SELECT * FROM platform_runs WHERE id=?", (prior_id,))
    record = dict(zip([col[0] for col in cursor.description], cursor.fetchone()))
    result = json.loads(record["result_json"])
    finding = result["findings"][0]
    assert fingerprint(record, finding) == finding["fingerprint"]
    old_path = finding["file"]
    record.pop("id")
    record.update(head_sha="f" * 40, base_sha=record["head_sha"], title="Synthetic persistence-only move fixture", worker_owner="", worker_lease_until="", retry_parent_id=0, retry_attempt=0, retry_at="", retry_info_json="null", trace_json="[]")
    finding["file"] = "moved/" + old_path
    finding["fingerprint"] = fingerprint(record, finding)
    finding["confidence"] = "candidate"
    for key in ("verification", "sequence_diagram", "observation_ids", "investigation_id"):
        finding.pop(key, None)
    blob = hashlib.sha1(b"blob 7\x00change\n").hexdigest()
    entry = {"mode": "100644", "type": "blob", "object_id": blob}
    result = {"findings": [finding], "summary": "Synthetic persisted report for restore only", "coverage_notes": ["Synthetic fixture; not Agent or Git rename quality evidence."], "metadata_changes": [{"kind": "rename", "old_path": old_path, "new_path": finding["file"], "base": entry, "head": entry}]}
    record["result_json"] = json.dumps(result, ensure_ascii=False, separators=(",", ":"))
    columns = list(record)
    cursor = db.execute("INSERT INTO platform_runs(" + ",".join(columns) + ") VALUES(" + ",".join("?" for _ in columns) + ")", [record[col] for col in columns])
    db.commit()
    return cursor.lastrowid


def business_digest(db):
    tables = ["platform_runs", "platform_users", "platform_sessions", "platform_projects", "platform_project_members", "platform_reviews", "platform_review_history", "platform_finding_association_history", "platform_comment_delivery"]
    output = {}
    for table in tables:
        columns = [row[1] for row in db.execute("PRAGMA table_info(" + table + ")") if row[1] not in {"worker_owner", "worker_lease_until"}]
        assert columns
        rows = db.execute("SELECT " + ",".join(columns) + " FROM " + table + " ORDER BY " + ",".join(columns)).fetchall()
        output[table] = hashlib.sha256(json.dumps(rows, ensure_ascii=False, separators=(",", ":")).encode()).hexdigest()
    return output


def run_backup_drill(*, root, binary, db, api, start, processes, healthy, wait_for, base_url, lifecycle_runs, crash_parent, metadata_run, upstream_counts):
    member = api("/users", {"username": "restore-fixture-member", "password": "restore-fixture-member-password", "role": "member"})
    api(f"/projects/1/members/{member['id']}", {"role": "viewer"}, method="PUT")
    api("/projects", {"id": 2, "name": "Synthetic unauthorized project", "enabled": True})
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    def member_api(path, value=None):
        request = urllib.request.Request(base_url + "/api/v1" + path, data=json.dumps(value).encode() if value is not None else None, headers={"Content-Type": "application/json", "Origin": base_url})
        with opener.open(request, timeout=5) as response:
            return json.load(response)
    member_api("/auth/login", {"username": member["username"], "password": "restore-fixture-member-password"})
    admin_before = api("/auth/me")
    member_before = member_api("/auth/me")
    assert [p["id"] for p in member_api("/projects")["items"]] == [1]
    assert member_api(f"/runs/{metadata_run}")["run"]["id"] == metadata_run
    moved = seed_move_report(db, lifecycle_runs[0])
    suggestions = api(f"/runs/{moved}/associations")
    candidate = next(item for item in suggestions["items"] if item["prior_run_id"] == lifecycle_runs[0])
    decision = api(f"/runs/{moved}/associations/{candidate['id']}", {"decision": "confirmed", "reason": "Synthetic restore persistence proof only", "expected_revision": 0}, method="PUT")
    assert decision["revision"] > 0
    wait_for(lambda: db.execute("SELECT COUNT(*) FROM platform_comment_delivery WHERE state IN ('pending','sending','unknown')").fetchone()[0] == 0, timeout=20)
    details_before = {run_id: api(f"/runs/{run_id}") for run_id in [crash_parent, metadata_run, *lifecycle_runs, moved]}
    associations_before = api(f"/runs/{moved}/associations")
    counts_before = upstream_counts()
    for process in processes:
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=10)
    assert not healthy(), "source process still serving"
    baseline = business_digest(db)
    db.close()
    parent = Path(tempfile.mkdtemp(prefix="aimangebot-native-restore."))
    parent.chmod(0o700)
    bundle, restored = parent / "bundle", parent / "restored"
    script = Path(__file__).with_name("ops-backup.py")
    def command(*args):
        result = subprocess.run([sys.executable, str(script), *map(str, args)], capture_output=True, text=True, timeout=40)
        assert result.returncode == 0, "isolated backup tool failed; inspect private fixture"
        assert json.loads(result.stdout)["status"] == "ok"
    command("backup", "--database", root / "pr_agent.db", "--config", root / "config.yaml", "--output", bundle, "--service-stopped")
    command("verify", "--bundle", bundle)
    command("restore", "--bundle", bundle, "--output", restored)
    with closing(sqlite3.connect(restored / "pr_agent.db")) as restored_db:
        assert business_digest(restored_db) == baseline
    replacement = start("restored", restored)
    def ready():
        if replacement.poll() is not None:
            raise RuntimeError("restored binary failed; inspect isolated log")
        return healthy()
    wait_for(ready)
    assert api("/auth/me") == admin_before
    assert member_api("/auth/me") == member_before
    assert [p["id"] for p in member_api("/projects")["items"]] == [1]
    assert member_api(f"/runs/{metadata_run}")["run"]["id"] == metadata_run
    for project, expected in ((1, 403), (2, 404)):
        try:
            member_api("/runs", {"project_id": project, "mr_iid": 90})
            raise AssertionError("restored member access expanded")
        except urllib.error.HTTPError as error:
            assert error.code == expected
    for run_id, original in details_before.items():
        actual = api(f"/runs/{run_id}")
        for key in ("result", "trace", "base_sha", "head_sha", "audit_policy", "status"):
            assert actual["run"].get(key) == original["run"].get(key)
        assert actual["reviews"] == original["reviews"]
    assert api(f"/runs/{moved}/associations") == associations_before
    assert upstream_counts() == counts_before, "restore duplicated model or comment request"
    with closing(sqlite3.connect(restored / "pr_agent.db")) as restored_db:
        assert business_digest(restored_db) == baseline
    proof = {"same_binary_sha256": hashlib.sha256(Path(binary).read_bytes()).hexdigest(), "bundle_verified": True, "new_directory_restore": True, "business_tables_identical": True, "admin_and_member_sessions_preserved": True, "member_acl_preserved": True, "fixed_reports_and_checkpoint_preserved": True, "risk_reviews_preserved": True, "association_decision_preserved": True, "association_fixture_seeded_not_quality_evidence": True, "comments_preserved_no_duplicate": True, "model_requests_not_repeated": True, "restored_binary_healthy": True, "synthetic_only": True}
    receipt = parent / "proof.json"
    receipt.write_text(json.dumps(proof, indent=2))
    receipt.chmod(0o600)
    return proof
