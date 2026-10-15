"""Local synthetic HTTP fixture for the real built settings UI; no production APIs."""
import copy, json, time, socket, argparse
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from pathlib import Path
ROOT = str(Path(__file__).resolve().parents[2] / 'web/dist')
BASE = json.loads(Path(__file__).with_suffix('.json').read_text())
STATE = {}

def reset(mode):
    STATE.clear()
    STATE.update(mode=mode, settings=copy.deepcopy(BASE), gets=0, writes=[])
    a = dict(state='pending', code='credential_activation_pending', can_reload=True)
    if mode in ['active', 'save_lost', 'save_pending', 'forbidden', 'unauthorized', 'invalid_proxy']:
        a = dict(state='active', code='', can_reload=True)
    if mode == 'unavailable':
        a = dict(state='unavailable', code='credential_activation_unavailable', can_reload=False)
    if mode == 'malformed':
        a = None
    if mode == 'unsafe':
        STATE['settings']['config_revision'] = 9007199254740992
    if mode != 'unbound':
        STATE['settings']['repository_activation'] = a
reset('pending')

class Handler(SimpleHTTPRequestHandler):

    def __init__(self, *a, **kw):
        super().__init__(*a, directory=ROOT, **kw)

    def log_message(self, *a):
        pass

    def sendjson(self, status, data):
        raw = json.dumps(data, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        try:
            self.wfile.write(raw)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_GET(self):
        if self.path == '/_proof':
            return self.sendjson(200, STATE)
        if self.path == '/api/v1/auth/me':
            return self.sendjson(200, dict(id=1, username='fixture-admin', role='admin', disabled=False))
        if self.path == '/api/v1/settings':
            STATE['gets'] += 1
            if STATE['mode'] == 'get_failure':
                return self.sendjson(503, dict(error='fixture read failure'))
            if STATE['mode'] == 'success_read_failure' and STATE['writes']:
                return self.sendjson(503, dict(error='fixture followup read failure'))
            return self.sendjson(200, STATE['settings'])
        if self.path.startswith('/api/'):
            return self.sendjson(200, dict(items=[]))
        super().do_GET()

    def do_POST(self):
        self.mutation()

    def do_PUT(self):
        self.mutation()

    def mutation(self):
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or '{}')
        if self.path == '/_fixture':
            reset(body['mode'])
            return self.sendjson(200, dict(ok=True))
        if self.path == '/_read_ok':
            STATE['mode'] = 'read_ok'
            return self.sendjson(200, dict(ok=True))
        if self.path not in ['/api/v1/settings', '/api/v1/settings/repository-activation/reload']:
            return self.sendjson(404, {})
        STATE['writes'].append(dict(method=self.command, path=self.path, body=body))
        mode = STATE['mode']
        if mode in ['forbidden', 'unauthorized']:
            return self.sendjson(401 if mode == 'unauthorized' else 403, dict(code='permission_changed', error='permission_changed'))
        if mode == 'invalid_proxy':
            return self.sendjson(400, dict(code='invalid_trusted_proxies', error='invalid_trusted_proxies'))
        if mode == 'save_pending':
            STATE['settings']['repository_activation'] = dict(state='pending', code='credential_activation_pending', can_reload=True)
            if self.command == 'PUT':
                return self.sendjson(503, dict(code='credential_activation_pending', error='credential_activation_pending'))
        if self.command == 'POST':
            if body != {'expected_config_revision': str(STATE['settings']['config_revision'])}:
                return self.sendjson(400, dict(error='bad expected revision'))
        else:
            if 'repository_activation' in body:
                return self.sendjson(400, dict(error='readonly payload'))
            for k, v in body.items():
                if k not in ['has_api_key', 'has_gitlab_token', 'has_webhook_token', 'project_config_sync', 'restart_required']:
                    STATE['settings'][k] = v
            for scope, key in [('gitlab', 'token'), ('openai', 'api_key')]:
                STATE['settings'][scope][key] = ''
        STATE['settings']['config_revision'] += 1
        STATE['settings']['repository_activation'] = dict(state='active', code='', can_reload=True)
        if mode in ['reload_lost', 'save_lost']:
            self.connection.shutdown(socket.SHUT_RDWR)
            self.connection.close()
            return
        if mode == 'timeout':
            time.sleep(25)
        self.sendjson(200, dict(settings=STATE['settings'], restart_required=False))
if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Synthetic settings UI fixture; never uses real credentials or services.')
    parser.add_argument('--port', type=int, default=18766)
    parser.add_argument('--mode', default='pending')
    args = parser.parse_args()
    reset(args.mode)
    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    print(f'fixture http://127.0.0.1:{args.port}/#/settings', flush=True)
    server.serve_forever()
