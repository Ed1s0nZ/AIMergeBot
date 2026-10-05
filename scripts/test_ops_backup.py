"""Private fixture-only backup checks. No production files or services used."""
from contextlib import closing
import datetime
import importlib.util
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).with_name("ops-backup.py")
SPEC = importlib.util.spec_from_file_location("ops_backup", SCRIPT)
OPS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(OPS)


class BackupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        self.source.mkdir(mode=0o700)
        self.db = self.source / "pr_agent.db"
        self.config = self.source / "config.yaml"
        self.config.write_text("api_key: synthetic-private-fixture-value\n")
        self.config.chmod(0o600)
        self.binary = self.source / "aimangebot"
        self.binary.write_bytes(b"synthetic binary content, never executed")
        self.binary.chmod(0o700)
        self.connection = sqlite3.connect(self.db)
        self.addCleanup(self.connection.close)
        self.connection.execute("PRAGMA journal_mode=WAL")
        self.connection.execute("CREATE TABLE evidence(id INTEGER PRIMARY KEY, value TEXT)")
        self.connection.execute("INSERT INTO evidence(value) VALUES('uncheckpointed fixture')")
        self.connection.commit()
        self.bundle = self.root / "bundle"

    def deadline(self):
        return time.monotonic() + 10

    def create(self, output=None):
        return OPS.backup(self.db, self.config, output or self.bundle, self.binary, True, self.deadline())

    def test_wal_snapshot_restore_permissions_and_source_preserved(self):
        self.assertTrue(Path(str(self.db) + "-wal").exists())
        before = OPS.digest(self.db, self.deadline())
        manifest = self.create()
        self.assertEqual(set(manifest["files"]), {"pr_agent.db", "config.yaml", "aimangebot"})
        restored = self.root / "restored"
        OPS.restore(self.bundle, restored, self.deadline())
        with closing(sqlite3.connect(restored / "pr_agent.db")) as db:
            self.assertEqual(db.execute("SELECT value FROM evidence").fetchone()[0], "uncheckpointed fixture")
        self.assertEqual(before, OPS.digest(self.db, self.deadline()))
        self.assertEqual((restored / "config.yaml").read_bytes(), self.config.read_bytes())
        for directory in (self.bundle, restored):
            self.assertEqual(directory.stat().st_mode & 0o777, 0o700)
            for name in manifest["files"]:
                self.assertEqual((directory / name).stat().st_mode & 0o777, 0o700 if name == "aimangebot" else 0o600)
        self.assertEqual((self.bundle / "manifest.json").stat().st_mode & 0o777, 0o600)
        self.assertNotIn("synthetic-private", (self.bundle / "manifest.json").read_text())
        self.assertEqual({p.name for p in self.bundle.iterdir()}, set(manifest["files"]) | {"manifest.json"})

    def test_active_lease_and_missing_stopped_assertion_rejected(self):
        self.connection.execute("CREATE TABLE platform_worker_instance(id INTEGER, owner TEXT, lease_until TEXT)")
        until = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(seconds=100)
        self.connection.execute("INSERT INTO platform_worker_instance VALUES(1,'synthetic',?)", (until.isoformat(),))
        self.connection.commit()
        with self.assertRaises(OPS.BackupError):
            self.create()
        self.assertFalse(self.bundle.exists())
        with self.assertRaises(OPS.BackupError):
            OPS.backup(self.db, self.config, self.bundle, None, False, self.deadline())

    def test_existing_targets_never_overwritten(self):
        self.create()
        before = (self.bundle / "manifest.json").read_bytes()
        with self.assertRaises(FileExistsError):
            self.create()
        self.assertEqual(before, (self.bundle / "manifest.json").read_bytes())
        target = self.root / "existing"
        target.mkdir()
        (target / "marker").write_text("keep")
        with self.assertRaises(FileExistsError):
            OPS.restore(self.bundle, target, self.deadline())
        self.assertEqual((target / "marker").read_text(), "keep")

    def test_symlinks_private_permissions_and_nested_output_rejected(self):
        link = self.source / "link.yaml"
        link.symlink_to(self.config)
        with self.assertRaises(OPS.BackupError):
            OPS.backup(self.db, link, self.bundle, None, True, self.deadline())
        self.config.chmod(0o644)
        with self.assertRaises(OPS.BackupError):
            self.create()
        self.config.chmod(0o600)
        with self.assertRaises(OPS.BackupError):
            self.create(self.source / "nested")

    def test_tamper_missing_files_and_manifest_paths_rejected(self):
        self.create()
        config = self.bundle / "config.yaml"
        config.write_text("tampered")
        with self.assertRaises(OPS.BackupError):
            OPS.verify_bundle(self.bundle, self.deadline())
        config.write_bytes(self.config.read_bytes())
        manifest = self.bundle / "manifest.json"
        data = json.loads(manifest.read_text())
        data["files"]["../escape"] = data["files"]["config.yaml"]
        manifest.write_text(json.dumps(data))
        with self.assertRaises(OPS.BackupError):
            OPS.verify_bundle(self.bundle, self.deadline())
        del data["files"]["../escape"]
        manifest.write_text(json.dumps(data))
        config.unlink()
        config.symlink_to(self.config)
        with self.assertRaises(OPS.BackupError):
            OPS.verify_bundle(self.bundle, self.deadline())
        config.unlink()
        with self.assertRaises(OPS.BackupError):
            OPS.verify_bundle(self.bundle, self.deadline())

    def test_corrupt_database_is_rejected_even_with_matching_hash(self):
        self.create()
        path = self.bundle / "pr_agent.db"
        path.write_bytes(b"invalid sqlite database")
        manifest = self.bundle / "manifest.json"
        data = json.loads(manifest.read_text())
        data["files"]["pr_agent.db"] = OPS.digest(path, self.deadline())
        manifest.write_text(json.dumps(data))
        with self.assertRaises(sqlite3.DatabaseError):
            OPS.verify_bundle(self.bundle, self.deadline())
        self.assertFalse((self.root / "restored").exists())

    def test_config_change_failure_cleans_owned_output(self):
        original = OPS.copy_private
        def copy_then_change(source, target, mode, deadline):
            original(source, target, mode, deadline)
            if target.name == "config.yaml":
                self.config.write_text("changed during operation")
        with patch.object(OPS, "copy_private", copy_then_change):
            with self.assertRaises(OPS.BackupError):
                self.create()
        self.assertFalse(self.bundle.exists())

    def test_cli_never_prints_private_config_or_paths(self):
        result = subprocess.run([sys.executable, str(SCRIPT), "backup", "--database", str(self.db), "--config", str(self.config), "--output", str(self.bundle), "--service-stopped"], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(json.loads(result.stdout)["status"], "ok")
        result = subprocess.run([sys.executable, str(SCRIPT), "restore", "--bundle", str(self.bundle), "--output", str(self.source)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn("synthetic-private", result.stdout + result.stderr)
        self.assertNotIn(str(self.source), result.stdout + result.stderr)

    def test_deadline_and_private_bundle_directory_enforced(self):
        with self.assertRaises(OPS.BackupError):
            OPS.backup(self.db, self.config, self.bundle, None, True, time.monotonic() - 1)
        self.assertFalse(self.bundle.exists())
        self.create()
        self.bundle.chmod(0o755)
        with self.assertRaises(OPS.BackupError):
            OPS.verify_bundle(self.bundle, self.deadline())
        self.bundle.chmod(0o700)
        target = self.root / "interrupted-restore"
        with patch.object(OPS, "copy_private", side_effect=OPS.BackupError("fixture copy failure")):
            with self.assertRaises(OPS.BackupError):
                OPS.restore(self.bundle, target, self.deadline())
        self.assertFalse(target.exists())
        OPS.verify_bundle(self.bundle, self.deadline())


if __name__ == "__main__":
    unittest.main()
