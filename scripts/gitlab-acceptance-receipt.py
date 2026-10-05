#!/usr/bin/env python3
"""Read-only private receipt for an explicitly selected GitLab audit run."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import urllib.parse
import urllib.error
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def base_url(value):
    parsed = urllib.parse.urlsplit(value)
    local = parsed.hostname in ("localhost", "127.0.0.1", "::1")
    if parsed.username or parsed.password or parsed.query or parsed.fragment or not parsed.hostname or (parsed.scheme != "https" and not (local and parsed.scheme == "http")):
        raise ValueError("invalid endpoint")
    return value.rstrip("/")


def get_json(base, path, headers):
    opener = urllib.request.build_opener(NoRedirect())
    try:
        response = opener.open(urllib.request.Request(base + path, headers=headers), timeout=10)
    except urllib.error.HTTPError as error:
        error.close()
        raise
    with response:
        data = response.read(2 * 1024 * 1024 + 1)
        if len(data) > 2 * 1024 * 1024:
            raise ValueError("response too large")
        return json.loads(data)


def receipt(detail, mr, discussion, project, iid, run_id, sha):
    run = detail["run"]
    sync = detail["comment_sync"]
    if (run["id"], run["project_id"], run["mr_iid"], run["head_sha"]) != (run_id, project, iid, sha) or run["status"] not in ("succeeded", "incomplete"):
        raise ValueError("run mismatch")
    if mr["iid"] != iid or mr["project_id"] != project or mr["sha"] != sha:
        raise ValueError("MR changed")
    if sync["state"] != "sent" or sync["sent_generation"] < 1 or sync["sent_generation"] != sync["desired_generation"] or discussion["id"] != sync["discussion_id"]:
        raise ValueError("comment not converged")
    notes = [note for note in discussion["notes"] if note["id"] == sync["note_id"]]
    if len(notes) != 1 or notes[0].get("system"):
        raise ValueError("note mismatch")
    return {"version": 1, "read_only": True, "run_id": run_id, "project_id": project, "mr_iid": iid, "base_sha": run["base_sha"], "head_sha": sha, "status": run["status"], "discussion_id": sync["discussion_id"], "note_id": sync["note_id"], "sent_generation": sync["sent_generation"], "note_body_sha256": hashlib.sha256(notes[0]["body"].encode()).hexdigest(), "readyz": True, "snapshot_matches_current_mr": True, "comment_generation_converged": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("app-url", "gitlab-url", "head-sha", "output"):
        parser.add_argument("--" + name, required=True)
    for name in ("run-id", "project-id", "mr-iid"):
        parser.add_argument("--" + name, required=True, type=int)
    args = parser.parse_args()
    try:
        if min(args.run_id, args.project_id, args.mr_iid) <= 0 or not re.fullmatch(r"(?:[a-f0-9]{40}|[a-f0-9]{64})", args.head_sha):
            raise ValueError("invalid target")
        app, gitlab = base_url(args.app_url), base_url(args.gitlab_url)
        session, token = os.environ["AIM_ACCEPT_SESSION"], os.environ["AIM_ACCEPT_GITLAB_TOKEN"]
        if not session or not token or any(c in session + token for c in "\r\n"):
            raise ValueError("invalid credentials")
        if get_json(app, "/readyz", {}) != {"status": "ready"}:
            raise ValueError("not ready")
        detail = get_json(app, f"/api/v1/runs/{args.run_id}", {"Cookie": "aim_session=" + session})
        prefix = f"/api/v4/projects/{args.project_id}/merge_requests/{args.mr_iid}"
        headers = {"PRIVATE-TOKEN": token}
        mr = get_json(gitlab, prefix, headers)
        discussion_id = urllib.parse.quote(detail["comment_sync"]["discussion_id"], safe="")
        discussion = get_json(gitlab, prefix + "/discussions/" + discussion_id, headers)
        proof = receipt(detail, mr, discussion, args.project_id, args.mr_iid, args.run_id, args.head_sha)
        output = Path(args.output)
        if not output.is_absolute() or not output.parent.is_dir() or output.parent.is_symlink() or output.parent.stat().st_mode & 0o077:
            raise ValueError("private output parent required")
        with output.open("xb") as stream:
            os.fchmod(stream.fileno(), 0o600)
            stream.write(json.dumps(proof, indent=2).encode())
            stream.flush()
            os.fsync(stream.fileno())
    except (OSError, ValueError, KeyError, TypeError):
        print(json.dumps({"status": "error", "error": "acceptance receipt unavailable"}))
        return 1
    print(json.dumps({"status": "ok", "read_only": True}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
