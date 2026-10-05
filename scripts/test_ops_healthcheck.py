from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
from pathlib import Path
import threading
import time
import unittest

spec = importlib.util.spec_from_file_location("healthcheck", Path(__file__).with_name("ops-healthcheck.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class MonitorTests(unittest.TestCase):
    def test_real_http_status_and_response_boundaries(self):
        state = {"status": 200, "body": b'{"status":"ready"}', "calls": 0}
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_GET(self):
                state["calls"] += 1
                self.send_response(state["status"])
                self.send_header("Location", "/redirected")
                self.end_headers()
                self.wfile.write(state["body"])
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        endpoint = f"http://127.0.0.1:{server.server_port}"
        try:
            self.assertTrue(module.check(endpoint))
            for status, body in [(503, b'{"status":"ready"}'), (302, b'{"status":"ready"}'), (200, b'{"status":"ok"}'), (200, b'private malformed'), (200, b' ' * 1024 + b'{"status":"ready"}')]:
                state.update(status=status, body=body)
                before = state["calls"]
                self.assertFalse(module.check(endpoint))
                self.assertEqual(state["calls"], before + 1)
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)
        self.assertFalse(module.check(endpoint, 1))

    def test_overall_deadline_stops_slow_headers_and_body(self):
        for slow_headers in (False, True):
            class Handler(BaseHTTPRequestHandler):
                def log_message(self, *args):
                    pass
                def do_GET(self):
                    try:
                        if slow_headers:
                            self.connection.sendall(b"HTTP/1.0 200 OK\r\nX-Slow: ")
                            for _ in range(20):
                                self.connection.sendall(b"a")
                                time.sleep(0.12)
                            self.connection.sendall(b"\r\n\r\n{\"status\":\"ready\"}")
                        else:
                            self.send_response(200)
                            self.end_headers()
                            for byte in b'{"status":"ready"}':
                                self.wfile.write(bytes([byte]))
                                self.wfile.flush()
                                time.sleep(0.12)
                    except OSError:
                        pass
            server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                start = time.monotonic()
                self.assertFalse(module.check(f"http://127.0.0.1:{server.server_port}", 1))
                self.assertLess(time.monotonic() - start, 1.8)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)

    def test_invalid_transport_no_request(self):
        for endpoint in ("http://remote.test", "https://user:secret@remote.test", "https://remote.test?secret=x", "file:///tmp/data", "https://remote.test/path"):
            self.assertFalse(module.check(endpoint))
        self.assertFalse(module.check("http://127.0.0.1:1", 0))


if __name__ == "__main__":
    unittest.main()
