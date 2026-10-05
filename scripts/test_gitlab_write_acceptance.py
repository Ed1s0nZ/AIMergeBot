from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
from pathlib import Path
from types import SimpleNamespace
import threading
import unittest
import urllib.error

spec = importlib.util.spec_from_file_location('write_acceptance', Path(__file__).with_name('gitlab-write-acceptance.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class WriteAcceptanceTests(unittest.TestCase):
    def test_explicit_write_flag_required_before_any_request(self):
        args = SimpleNamespace(app_url='http://127.0.0.1:1', gitlab_url='http://127.0.0.1:1', project_id=2, mr_iid=3, head_sha='a' * 40, wait_seconds=10, allow_test_mr_writes=False)
        def forbidden(*args):
            self.fail('network request before explicit write flag')
        with self.assertRaises(ValueError):
            module.run(args, ('synthetic-session', 'synthetic-token', 'synthetic-webhook'), forbidden, send=forbidden)

    def exercise(self, mode='success'):
        sha = 'a' * 40
        marker = '<!-- AIMergeBot:' + 'b' * 64 + ' -->'
        state = {'body': marker + '\noriginal', 'generation': 1, 'desired': 1, 'sync': 'sent', 'hooks': 0, 'writes': [], 'reads': 0, 'detail_reads': 0}
        prefix = '/api/v4/projects/2/merge_requests/3'
        stages = []

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def respond(self, value, status=200):
                self.send_response(status)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps(value).encode())

            def do_GET(self):
                if self.path == '/readyz':
                    return self.respond({'status': 'ready'})
                if self.path == prefix:
                    state['reads'] += 1
                    changed = mode == 'drift' and state['reads'] >= 2
                    return self.respond({'project_id': 2, 'iid': 3, 'sha': 'c' * 40 if changed else sha})
                discussion = {'id': 'discussion', 'notes': [{'id': 4, 'body': state['body'], 'system': False}]}
                if self.path.startswith(prefix + '/discussions?'):
                    return self.respond([discussion])
                if self.path == prefix + '/discussions/discussion':
                    return self.respond(discussion)
                if self.path == '/api/v1/runs/1':
                    state['detail_reads'] += 1
                    if state['detail_reads'] == 1:
                        return self.respond({'run': {'id': 1, 'project_id': 2, 'mr_iid': 3, 'head_sha': sha, 'status': 'pending'}, 'comment_sync': None})
                    return self.respond({'run': {'id': 1, 'project_id': 2, 'mr_iid': 3, 'base_sha': 'd' * 40, 'head_sha': sha, 'status': 'incomplete' if mode.startswith('incomplete') else 'succeeded', 'result': {'findings': [] if mode == 'empty' else [{'id': 'finding'}]}}, 'comment_sync': None if mode == 'incomplete_no_comment' else {'state': state['sync'], 'sent_generation': state['generation'], 'desired_generation': state['desired'], 'discussion_id': 'discussion', 'note_id': 4}})
                self.respond({}, 404)

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                state['writes'].append((self.command, self.path))
                if self.path != '/webhook' or body['project']['id'] != 2 or body['object_attributes']['iid'] != 3:
                    return self.respond({}, 400)
                state['hooks'] += 1
                creates_new = state['hooks'] == 1 or mode.startswith('incomplete')
                self.respond({'id': state['hooks'] if mode.startswith('incomplete') else 1, 'created': creates_new}, 202)

            def do_PUT(self):
                body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                state['writes'].append((self.command, self.path))
                if mode == 'permission':
                    return self.respond({'private': 'server error must not be printed'}, 403)
                if self.path == '/api/v1/runs/1/findings/finding/review':
                    state['desired'] += 1
                    if body['status'] == 'false_positive':
                        state['generation'] += 1
                        state['body'] = marker + '\nupdated'
                    else:
                        state['sync'] = 'conflict'
                    return self.respond(None, 204)
                if self.path == prefix + '/discussions/discussion/notes/4':
                    state['body'] = body['body']
                    return self.respond({'id': 4})
                self.respond({}, 404)

        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        url = f'http://127.0.0.1:{server.server_port}'
        args = SimpleNamespace(app_url=url, gitlab_url=url, project_id=2, mr_iid=3, head_sha=sha, wait_seconds=10, allow_test_mr_writes=True)
        try:
            error = None
            try:
                module.run(args, ('synthetic-session', 'synthetic-token', 'synthetic-webhook'), lambda stage, **facts: stages.append({'stage': stage, **facts}), pause=lambda seconds: None)
            except (ValueError, urllib.error.HTTPError) as caught:
                error = caught
            return state, stages, error
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)

    def test_real_http_workflow_same_note_update_and_conflict(self):
        state, stages, error = self.exercise()
        self.assertIsNone(error)
        self.assertEqual(state['hooks'], 2)
        self.assertEqual(state['sync'], 'conflict')
        self.assertIn('manual-edit conflict acceptance', state['body'])
        self.assertEqual(len(state['writes']), 5)
        self.assertEqual(stages[-1]['stage'], 'conflict_preserved')
        self.assertNotIn('synthetic-token', str(stages))
        self.assertNotIn('original', str(stages))

    def test_drift_prevents_first_write(self):
        state, stages, error = self.exercise('drift')
        self.assertIsInstance(error, ValueError)
        self.assertEqual(state['writes'], [])
        self.assertEqual(stages[-1]['stage'], 'preflight_complete')

    def test_incomplete_terminal_does_not_trigger_second_audit(self):
        state, stages, error = self.exercise('incomplete')
        self.assertIsInstance(error, ValueError)
        self.assertEqual(state['hooks'], 1)
        self.assertEqual(state['writes'], [('POST', '/webhook')])
        self.assertEqual(stages[-1]['stage'], 'audit_incomplete_stop')
        self.assertTrue(stages[-1]['deduplication_not_verified'])

    def test_incomplete_without_comment_stops_immediately(self):
        state, stages, error = self.exercise('incomplete_no_comment')
        self.assertIsInstance(error, ValueError)
        self.assertEqual(state['detail_reads'], 2)
        self.assertEqual(state['writes'], [('POST', '/webhook')])
        self.assertEqual(stages[-1]['stage'], 'audit_incomplete_stop')
        self.assertTrue(stages[-1]['deduplication_not_verified'])

    def test_zero_findings_preserves_created_run_without_review(self):
        state, stages, error = self.exercise('empty')
        self.assertIsInstance(error, ValueError)
        self.assertEqual(len(state['writes']), 2)
        self.assertEqual(stages[-1]['stage'], 'webhook_deduplicated')

    def test_http_error_closed(self):
        state, stages, error = self.exercise('permission')
        self.assertIsInstance(error, urllib.error.HTTPError)
        self.assertTrue(error.closed)
        self.assertEqual(len(state['writes']), 3)
        self.assertEqual(stages[-1]['stage'], 'review_update_started')


if __name__ == '__main__':
    unittest.main()
