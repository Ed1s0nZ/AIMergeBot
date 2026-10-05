import copy
import importlib.util
from pathlib import Path
import unittest

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


if __name__ == "__main__":
    unittest.main()
