"""Loopback-only synthetic API fixture for the built repository read-only UI."""
import argparse
import json
import threading
import time
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[2] / 'web' / 'dist'
LOCK = threading.Lock()
STATE = {'mode': 'unready', 'gets': [], 'writes': []}
BINDING = {'revision': 2, 'provider': 'github', 'api_origin': 'https://api.example',
           'remote_id': 7, 'full_name': 'org/fixture-repository', 'integration_id': 4}
PROJECTS = [
    {'id': 1, 'name': '默认 GitLab 项目', 'enabled': True, 'repository_execution_available': True},
    {'id': 2, 'name': '已有 GitHub 绑定', 'enabled': True, 'repository': BINDING,
     'repository_execution_available': False},
]


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(ROOT), **kwargs)

    def log_message(self, *_):
        pass

    def sendjson(self, status, value):
        raw = json.dumps(value, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        try:
            self.wfile.write(raw)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_GET(self):
        path = urlsplit(self.path).path
        if path == '/_proof':
            with LOCK:
                proof = json.loads(json.dumps(STATE))
            return self.sendjson(200, proof)
        with LOCK:
            mode = STATE['mode']
            if path.startswith('/api/'):
                STATE['gets'].append(path)
        if path == '/api/v1/auth/me':
            return self.sendjson(200, {'id': 1, 'username': 'fixture-admin', 'role': 'admin', 'disabled': False})
        if path == '/api/v1/projects':
            if mode == 'projects_error':
                return self.sendjson(503, {'error': '项目列表暂时不可用（合成夹具）'})
            payload = {'items': [] if mode == 'empty' else PROJECTS}
            if mode != 'missing':
                payload['repository_management'] = {'available': False, 'reason': 'native_audit_not_ready'}
            return self.sendjson(200, payload)
        if path.endswith('/repository-binding'):
            if mode == 'slow':
                time.sleep(1.5)
            if mode == 'binding_error':
                return self.sendjson(503, {'error': '绑定暂时不可用（合成夹具）'})
            if mode == 'forbidden':
                return self.sendjson(403, {'error': '绑定权限不可用（合成夹具）'})
            return self.sendjson(200, BINDING if '/projects/2/' in path else {'revision': 0})
        if path.startswith('/api/'):
            return self.sendjson(200, {'items': []})
        return super().do_GET()

    def mutation(self):
        size = int(self.headers.get('Content-Length', 0))
        raw = self.rfile.read(min(size, 8192))
        if self.path == '/_fixture':
            mode = json.loads(raw or b'{}').get('mode', 'unready')
            if mode not in {'unready', 'missing', 'projects_error', 'binding_error', 'forbidden', 'slow', 'empty'}:
                return self.sendjson(400, {'error': 'unknown fixture mode'})
            with LOCK:
                STATE.update(mode=mode, gets=[], writes=[])
            return self.sendjson(200, {'ok': True})
        with LOCK:
            STATE['writes'].append({'method': self.command, 'path': self.path})
        return self.sendjson(501, {'code': 'repository_management_unavailable', 'error': 'synthetic read-only API'})

    do_POST = mutation
    do_PUT = mutation
    do_PATCH = mutation
    do_DELETE = mutation


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, default=18777)
    args = parser.parse_args()
    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    print(json.dumps({'url': f'http://127.0.0.1:{server.server_port}/#/projects'}), flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
