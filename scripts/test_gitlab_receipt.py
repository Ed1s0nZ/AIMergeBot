import copy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
from pathlib import Path
import threading
import unittest
import urllib.error

spec = importlib.util.spec_from_file_location("receipt", Path(__file__).with_name("gitlab-acceptance-receipt.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ReceiptTests(unittest.TestCase):
    def setUp(self):
        self.sha = "a" * 40
        self.detail = {"run": {"id": 1, "project_id": 2, "mr_iid": 3, "head_sha": self.sha, "base_sha": "b" * 40, "status": "succeeded"}, "comment_sync": {"state": "sent", "sent_generation": 2, "desired_generation": 2, "discussion_id": "discussion", "note_id": 4}}
        self.mr = {"iid": 3, "project_id": 2, "sha": self.sha}
        self.discussion = {"id": "discussion", "notes": [{"id": 4, "body": "private source evidence", "system": False}]}

    def test_receipt_does_not_copy_body(self):
        proof = module.receipt(self.detail, self.mr, self.discussion, 2, 3, 1, self.sha)
        self.assertNotIn("private source evidence", str(proof))
        self.assertEqual(proof["sent_generation"], 2)

    def test_changed_or_unconverged_refused(self):
        for target, key, value in [("run", "head_sha", "c" * 40), ("run", "status", "running"), ("comment_sync", "sent_generation", 1), ("comment_sync", "state", "conflict")]:
            detail = copy.deepcopy(self.detail)
            detail[target][key] = value
            with self.assertRaises(ValueError):
                module.receipt(detail, self.mr, self.discussion, 2, 3, 1, self.sha)
        self.mr["sha"] = "c" * 40
        with self.assertRaises(ValueError):
            module.receipt(self.detail, self.mr, self.discussion, 2, 3, 1, self.sha)

    def test_endpoints(self):
        for url in ("http://remote.test", "https://user:pass@remote.test", "https://remote.test?token=x", "file:///tmp/private"):
            with self.assertRaises(ValueError):
                module.base_url(url)
        self.assertEqual(module.base_url("http://127.0.0.1:1234/"), "http://127.0.0.1:1234")

    def test_real_http_errors_closed_and_redirect_not_followed(self):
        state = {"status": 200, "body": b'{"ok":true}', "paths": []}

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_GET(self):
                state["paths"].append(self.path)
                self.send_response(state["status"])
                self.send_header("Location", "/redirected")
                self.end_headers()
                self.wfile.write(state["body"])

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        endpoint = f"http://127.0.0.1:{server.server_port}"
        try:
            self.assertEqual(module.get_json(endpoint, "/selected", {}), {"ok": True})
            for status in (302, 503):
                state.update(status=status)
                before = len(state["paths"])
                with self.assertRaises(urllib.error.HTTPError) as caught:
                    module.get_json(endpoint, "/selected", {"PRIVATE-TOKEN": "synthetic-fixture-token"})
                self.assertTrue(caught.exception.closed)
                self.assertEqual(state["paths"][before:], ["/selected"])
            for body in (b"invalid json", b" " * (2 * 1024 * 1024 + 1)):
                state.update(status=200, body=body)
                with self.assertRaises(ValueError):
                    module.get_json(endpoint, "/selected", {})
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)


if __name__ == "__main__":
    unittest.main()
