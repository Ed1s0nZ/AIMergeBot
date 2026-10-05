import importlib.util
from pathlib import Path
import plistlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("ops_service", Path(__file__).with_name("ops-service.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ServiceTests(unittest.TestCase):
    def test_launchd_contract(self):
        data = plistlib.loads(module.render("launchd", "/private/build/a b", "/private/data with spaces", "local.fixture"))
        self.assertEqual(data["ProgramArguments"], ["/private/build/a b"])
        self.assertEqual(data["KeepAlive"], {"SuccessfulExit": False})
        self.assertGreater(data["ThrottleInterval"], 30)
        self.assertEqual(data["Umask"], 0o077)
        self.assertNotIn("EnvironmentVariables", data)

    def test_systemd_escaping(self):
        data = module.render("systemd", '/srv/100%/bin"ary', "/srv/data with spaces", "fixture", "audit", "audit").decode()
        self.assertIn('ExecStart="/srv/100%%/bin\\"ary"', data)
        self.assertIn('WorkingDirectory="/srv/data with spaces"', data)
        self.assertIn("RestartSec=35", data)
        self.assertNotIn("Environment=", data)

    def test_rejected_input(self):
        for binary, directory, label in [("relative", "/srv", "ok"), ("/srv/bin", "/srv\ninjected", "ok"), ("/srv/bin", "/srv", "x\n")]:
            with self.assertRaises(ValueError):
                module.render("launchd", binary, directory, label)
        for user in (None, "root", "a b", "a\n"):
            with self.assertRaises(ValueError):
                module.render("systemd", "/srv/bin", "/srv", "ok", user, "audit")

    def test_private_no_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            parent.chmod(0o700)
            output = parent / "service.plist"
            module.write_private(output, b"first")
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                module.write_private(output, b"second")
            self.assertEqual(output.read_bytes(), b"first")
            parent.chmod(0o755)
            with self.assertRaises(ValueError):
                module.write_private(parent / "other", b"data")


if __name__ == "__main__":
    unittest.main()
