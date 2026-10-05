"""Real HTTP admission assertions for the isolated native recovery fixture."""
from concurrent.futures import ThreadPoolExecutor
import json
import threading
import time
import urllib.error


def run_capacity_drill(api, db, wait_for):
    def submit(iid):
        started = time.monotonic()
        try:
            result = api("/runs", {"project_id": 1, "mr_iid": iid})
            return iid, result, time.monotonic() - started
        except urllib.error.HTTPError as error:
            body = json.load(error)
            assert error.code == 429 and body["code"] == "audit_quota_exceeded"
            assert body["scope"] == "outstanding_global" and body["limit"] == 4
            return iid, None, time.monotonic() - started
    ready = threading.Barrier(40)
    def concurrent(iid):
        ready.wait(timeout=10)
        return submit(iid)
    with ThreadPoolExecutor(max_workers=40) as executor:
        results = list(executor.map(concurrent, range(100, 140)))
    accepted = [(iid, result) for iid, result, _ in results if result is not None]
    assert len(accepted) == 3, "concurrent admission exceeded or lost capacity"
    wait_for(lambda: db.execute("SELECT COUNT(*) FROM platform_runs WHERE status='running'").fetchone()[0] == 1)
    for _ in range(20):
        outstanding, running = db.execute("SELECT SUM(status IN ('pending','running')),SUM(status='running') FROM platform_runs").fetchone()
        assert outstanding == 4 and running == 1
        time.sleep(0.05)
    iid, original = accepted[0]
    duplicate = api("/runs", {"project_id": 1, "mr_iid": iid})
    assert duplicate["id"] == original["id"] and not duplicate["created"]
    # Prefer cancelling pending work so the capacity release test does not depend
    # on cancellation delivery to an intentionally blocked synthetic upstream.
    cancelled = next(result["id"] for _, result in accepted if db.execute("SELECT status FROM platform_runs WHERE id=?", (result["id"],)).fetchone()[0] == "pending")
    api(f"/runs/{cancelled}/cancel", {}, method="POST")
    assert db.execute("SELECT status FROM platform_runs WHERE id=?", (cancelled,)).fetchone()[0] == "cancelled"
    extra = api("/runs", {"project_id": 1, "mr_iid": 149})
    assert extra["created"]
    assert db.execute("SELECT COUNT(*) FROM platform_runs WHERE status IN ('pending','running')").fetchone()[0] == 4
    for run_id in [result["id"] for _, result in accepted] + [extra["id"]]:
        if run_id == cancelled:
            continue
        api(f"/runs/{run_id}/cancel", {}, method="POST")
    elapsed = sorted(seconds for _, _, seconds in results)
    return {"synthetic_only": True, "concurrent_http_submissions": 40, "accepted": 3, "quota_rejections": 37, "outstanding_limit": 4, "running_observed": 1, "duplicate_reused_at_capacity": True, "cancel_released_capacity": True, "sampled_window_seconds": 1, "submission_p95_seconds": round(elapsed[37], 3), "production_throughput_not_measured": True}
