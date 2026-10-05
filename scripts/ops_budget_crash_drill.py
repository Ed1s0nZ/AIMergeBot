"""Budget-enabled native SIGKILL assertions; synthetic upstream only."""
import json
import time


def run_budget_crash_drill(api, db, calls, start, primary, healthy, wait_for, root):
    run_id = api("/runs", {"project_id": 1, "mr_iid": 91})["id"]
    def row():
        return db.execute("SELECT status,trace_json,result_json,retry_info_json,audit_policy_json FROM platform_runs WHERE id=?", (run_id,)).fetchone()
    pending_error = "model request pending; token usage unknown"
    wait_for(lambda: calls[91] == 2 and any(t.get("error") == pending_error for t in json.loads(row()[1])))
    original = row()
    trace = json.loads(original[1])
    assert sum(t.get("total_tokens", 0) for t in trace if t.get("usage_reported")) == 20
    assert len([t for t in trace if t.get("name") == "model"]) == 2
    assert any(t.get("name") == "read_file" for t in trace)
    assert json.loads(original[4])["model_budget"]["max_tokens"] == 100
    owner, lease = db.execute("SELECT owner,lease_until FROM platform_worker_instance").fetchone()
    wait_for(lambda: db.execute("SELECT lease_until FROM platform_worker_instance").fetchone()[0] != lease, timeout=8)
    primary.kill()
    primary.wait(timeout=10)
    immediate = start("budget-immediate-restart")
    immediate.wait(timeout=10)
    assert immediate.returncode != 0 and "another" in (root / "budget-immediate-restart.log").read_text()
    assert row()[0] == "running"
    wait_for(lambda: db.execute("SELECT COALESCE(julianday(lease_until),0)<=julianday('now') FROM platform_worker_instance").fetchone()[0], timeout=35)
    replacement = start("budget-replacement")
    wait_for(lambda: healthy() or replacement.poll() is not None)
    assert replacement.poll() is None and healthy()
    wait_for(lambda: row()[0] == "failed")
    restored = row()
    assert restored[1:3] == original[1:3]
    assert json.loads(restored[3])["state"] == "model_usage_unknown"
    assert restored[4] == original[4]
    assert db.execute("SELECT COUNT(*) FROM platform_runs WHERE retry_parent_id=?", (run_id,)).fetchone()[0] == 0
    assert db.execute("SELECT owner FROM platform_worker_instance").fetchone()[0] != owner
    time.sleep(1)
    assert calls[91] == 2
    return {"synthetic_only": True, "run_id": run_id, "max_tokens": 100, "reported_tokens_before_crash": 20, "inflight_usage_unknown": True, "sigkill_performed": True, "immediate_restart_fenced": True, "original_lease_waited": True, "checkpoint_identical": True, "frozen_budget_preserved": True, "unknown_usage_stopped_retry": True, "retry_children": 0, "model_requests": 2}
