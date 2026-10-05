#!/usr/bin/env python3
"""Writes only to an explicitly authorized dedicated test MR; leaves review/conflict evidence."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

spec = importlib.util.spec_from_file_location('receipt', Path(__file__).with_name('gitlab-acceptance-receipt.py'))
receipt = importlib.util.module_from_spec(spec)
spec.loader.exec_module(receipt)


def request(base, path, headers, method='GET', body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, headers={**headers, 'Content-Type': 'application/json'}, method=method)
    try:
        response = urllib.request.build_opener(receipt.NoRedirect()).open(req, timeout=10)
    except urllib.error.HTTPError as error:
        error.close()
        raise
    with response:
        raw = response.read(2 * 1024 * 1024 + 1)
        if len(raw) > 2 * 1024 * 1024:
            raise ValueError('response too large')
        return None if not raw else json.loads(raw)


def run(args, credentials, record, send=request, pause=time.sleep, clock=time.monotonic):
    app, gitlab = receipt.base_url(args.app_url), receipt.base_url(args.gitlab_url)
    if not args.allow_test_mr_writes or min(args.project_id, args.mr_iid) <= 0 or not re.fullmatch(r'(?:[a-f0-9]{40}|[a-f0-9]{64})', args.head_sha) or not 1 <= args.wait_seconds <= 1800:
        raise ValueError('invalid authorized target')
    session, token, webhook = credentials
    if any(not value or any(c in value for c in '\r\n') for value in credentials):
        raise ValueError('invalid credentials')
    ah, gh = {'Cookie': 'aim_session=' + session}, {'PRIVATE-TOKEN': token}
    prefix = f'/api/v4/projects/{args.project_id}/merge_requests/{args.mr_iid}'
    deadline = clock() + args.wait_seconds

    def current_mr():
        mr = send(gitlab, prefix, gh)
        if (mr['project_id'], mr['iid'], mr['sha']) != (args.project_id, args.mr_iid, args.head_sha):
            raise ValueError('MR changed')
        return mr

    def wait(test):
        while clock() < deadline:
            value = test()
            if value:
                return value
            pause(min(1, max(0, deadline - clock())))
        raise ValueError('acceptance deadline')

    def trigger():
        current_mr()
        record('webhook_request_started')
        return send(app, '/webhook', {'X-Gitlab-Token': webhook, 'X-Gitlab-Event': 'Merge Request Hook'}, 'POST', {'object_kind': 'merge_request', 'project': {'id': args.project_id}, 'object_attributes': {'iid': args.mr_iid, 'action': 'open'}})

    if send(app, '/readyz', {}) != {'status': 'ready'}:
        raise ValueError('not ready')
    current_mr()
    record('preflight_complete')
    created = trigger()
    if created.get('created') is not True or type(created.get('id')) is not int or created['id'] <= 0:
        raise ValueError('fresh run required')
    run_id = created['id']
    record('webhook_run_created', run_id=run_id)
    path = f'/api/v1/runs/{run_id}'

    def detail():
        d = send(app, path, ah)
        r = d['run']
        if (r['id'], r['project_id'], r['mr_iid'], r['head_sha']) != (run_id, args.project_id, args.mr_iid, args.head_sha):
            raise ValueError('run mismatch')
        if r['status'] == 'incomplete':
            record('audit_incomplete_stop', run_id=run_id, deduplication_not_verified=True)
            raise ValueError('complete audit required for terminal deduplication')
        if r['status'] in ('failed', 'cancelled'):
            raise ValueError('audit failed')
        if (d.get('comment_sync') or {}).get('state') in ('blocked', 'conflict', 'stopped'):
            raise ValueError('comment unavailable')
        return d

    def converged(min_generation=1):
        d = detail()
        s = d.get('comment_sync') or {}
        return d if d['run']['status'] == 'succeeded' and s.get('state') == 'sent' and s.get('sent_generation', 0) >= min_generation and s.get('sent_generation') == s.get('desired_generation') else None

    first = wait(converged)
    duplicate = trigger()
    if duplicate != {'id': run_id, 'created': False}:
        raise ValueError('webhook dedup mismatch')
    record('webhook_deduplicated', run_id=run_id)
    sync = first['comment_sync']
    discussion_id, note_id = sync['discussion_id'], sync['note_id']
    dp = prefix + '/discussions/' + urllib.parse.quote(discussion_id, safe='')

    def note():
        discussion = send(gitlab, dp, gh)
        receipt.receipt(first, current_mr(), discussion, args.project_id, args.mr_iid, run_id, args.head_sha)
        selected = [n for n in discussion['notes'] if n['id'] == note_id]
        return selected[0]

    original = note()['body']
    match = re.match(r'<!-- AIMergeBot:[a-f0-9]{64} -->', original)
    if not match:
        raise ValueError('missing publication marker')
    marker = match.group(0)

    def unique_marker():
        count = 0
        for page in range(1, 11):
            items = send(gitlab, prefix + f'/discussions?per_page=100&page={page}', gh)
            if not isinstance(items, list):
                raise ValueError('invalid discussions')
            count += sum(n.get('body', '').startswith(marker) for d in items for n in d.get('notes', []))
            if len(items) < 100:
                if count != 1:
                    raise ValueError('duplicate publication marker')
                return
        raise ValueError('discussion coverage incomplete')

    unique_marker()
    findings = first['run']['result']['findings']
    if not findings or not isinstance(findings[0].get('id'), str) or not findings[0]['id']:
        raise ValueError('finding required')
    review_path = path + '/findings/' + urllib.parse.quote(findings[0]['id'], safe='') + '/review'
    current_mr()
    record('review_update_started', run_id=run_id)
    send(app, review_path, ah, 'PUT', {'status': 'false_positive', 'reason': 'Dedicated test MR acceptance: synchronization only; not a real risk decision.'})
    updated = wait(lambda: converged(sync['sent_generation'] + 1))
    us = updated['comment_sync']
    if (us['discussion_id'], us['note_id']) != (discussion_id, note_id):
        raise ValueError('publication identity changed')
    new_body = note()['body']
    if new_body == original:
        raise ValueError('note not updated')
    unique_marker()
    record('same_note_updated', run_id=run_id, discussion_id=discussion_id, note_id=note_id, generation=us['sent_generation'], body_sha256=hashlib.sha256(new_body.encode()).hexdigest())
    current_mr()
    if note()['body'] != new_body:
        raise ValueError('concurrent note edit')
    edited = new_body + '\n\n[Dedicated test MR: manual-edit conflict acceptance; leave this evidence in place.]'
    record('manual_edit_started', run_id=run_id, note_id=note_id)
    send(gitlab, dp + f'/notes/{note_id}', gh, 'PUT', {'body': edited})
    if note()['body'] != edited:
        raise ValueError('manual edit not observed')
    current_mr()
    record('conflict_review_started', run_id=run_id)
    send(app, review_path, ah, 'PUT', {'status': 'pending', 'reason': 'Dedicated test MR acceptance: preserve manual note edit.'})

    def conflict():
        d = send(app, path, ah)
        s = d['comment_sync']
        if (d['run']['head_sha'], s.get('discussion_id'), s.get('note_id')) != (args.head_sha, discussion_id, note_id):
            raise ValueError('conflict identity changed')
        return d if s['state'] == 'conflict' and s['desired_generation'] > us['sent_generation'] else None

    wait(conflict)
    if note()['body'] != edited:
        raise ValueError('manual edit overwritten')
    unique_marker()
    record('conflict_preserved', run_id=run_id, note_id=note_id, body_sha256=hashlib.sha256(edited.encode()).hexdigest(), webhook_replay_only=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('app-url', 'gitlab-url', 'head-sha', 'output'):
        parser.add_argument('--' + name, required=True)
    for name in ('project-id', 'mr-iid'):
        parser.add_argument('--' + name, required=True, type=int)
    parser.add_argument('--wait-seconds', type=int, default=300)
    parser.add_argument('--allow-test-mr-writes', action='store_true')
    args = parser.parse_args()
    try:
        p = Path(args.output)
        if not p.is_absolute() or not p.parent.is_dir() or p.parent.resolve() != p.parent or p.parent.stat().st_mode & 0o077:
            raise ValueError('private output parent required')
        # Reserve evidence before any network side effects, without overwriting.
        with p.open('xb') as stream:
            os.fchmod(stream.fileno(), 0o600)
            def record(stage, **facts):
                stream.write((json.dumps({'stage': stage, **facts}) + '\n').encode())
                stream.flush()
                os.fsync(stream.fileno())
            try:
                run(args, tuple(os.environ[k] for k in ('AIM_ACCEPT_SESSION', 'AIM_ACCEPT_GITLAB_TOKEN', 'AIM_ACCEPT_WEBHOOK_TOKEN')), record)
            except (OSError, ValueError, KeyError, TypeError, IndexError):
                record('acceptance_failed', side_effects_may_exist=True)
                raise
    except (OSError, ValueError, KeyError, TypeError, IndexError):
        print(json.dumps({'status': 'error', 'error': 'test MR acceptance unavailable; inspect private stage evidence'}))
        return 1
    print(json.dumps({'status': 'ok', 'test_mr_writes': True, 'webhook_replay_only': True}))
    return 0


if __name__ == '__main__':
    sys.exit(main())
