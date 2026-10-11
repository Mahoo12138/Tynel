"""Offline negative tests for source selection and source-tree verification."""
import importlib.util
import pathlib
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("prepare_tsgo", pathlib.Path(__file__).with_name("prepare-tsgo.py"))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class PreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        for name in ["tsc/go.mod", "tsc/go.sum", "tsc/internal/bundled/libs/lib.es5.d.ts"]:
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(name, encoding="utf-8")
        entries = [(p.relative_to(self.root).as_posix(), prepare.digest(p.read_bytes())) for p in self.root.rglob("*") if p.is_file()]
        libs = [e for e in entries if e[0].endswith(".d.ts")]
        self.lock = {"selectedFiles": 3, "selectedTreeSHA256": prepare.tree_digest(entries), "declarationFiles": 1, "declarationSHA256": prepare.tree_digest(libs), "goModSHA256": prepare.digest(b"tsc/go.mod"), "goSumSHA256": prepare.digest(b"tsc/go.sum")}

    def test_selected_scope(self):
        for name in ["LICENSE.txt", "NOTICE.txt", "tsc/internal/checker/checker.go", "tsc/internal/bundled/libs/lib.es5.d.ts", "src/lib/es5.d.ts"]:
            self.assertTrue(prepare.selected(name), name)
        for name in ["tests/cases/test.ts", "tsc/testdata/baselines/test.txt", "tsc/_tools/tool.go", "tsc/_submodules/tests/test.ts"]:
            self.assertFalse(prepare.selected(name), name)

    def test_valid_tree(self):
        self.assertEqual(prepare.verify_tree(self.root, self.lock)["selectedFiles"], 3)

    def test_modified_tree_rejected(self):
        (self.root / "tsc/go.mod").write_text("changed")
        with self.assertRaisesRegex(ValueError, "differs from lock"):
            prepare.verify_tree(self.root, self.lock)

    def test_extra_file_rejected(self):
        (self.root / "unexpected.go").write_text("package unexpected")
        with self.assertRaises(ValueError):
            prepare.verify_tree(self.root, self.lock)

    def test_missing_file_rejected(self):
        (self.root / "tsc/go.sum").unlink()
        with self.assertRaises(ValueError):
            prepare.verify_tree(self.root, self.lock)

    def test_declaration_identity_rejected(self):
        self.lock["declarationSHA256"] = "0" * 64
        with self.assertRaisesRegex(ValueError, "declaration identity"):
            prepare.verify_tree(self.root, self.lock)

    def test_transient_windows_publish_lock_retried(self):
        error = PermissionError("sharing violation")
        error.winerror = 32
        with mock.patch.object(pathlib.Path, "rename", side_effect=[error, None]) as rename, mock.patch.object(prepare.time, "sleep") as sleep:
            prepare.publish_tree(self.root / "stage", self.root / "destination")
        self.assertEqual(rename.call_count, 2)
        sleep.assert_called_once()

    def test_persistent_publish_lock_is_failure(self):
        error = PermissionError("locked")
        error.winerror = 5
        with mock.patch.object(pathlib.Path, "rename", side_effect=error) as rename, mock.patch.object(prepare.time, "sleep"):
            with self.assertRaises(PermissionError):
                prepare.publish_tree(self.root / "stage", self.root / "destination")
        self.assertEqual(rename.call_count, 8)

    def test_existing_destination_not_replaced(self):
        with mock.patch.object(pathlib.Path, "rename") as rename:
            with self.assertRaises(ValueError):
                prepare.publish_tree(self.root / "stage", self.root)
        rename.assert_not_called()

    def test_resume_reverifies_before_publish(self):
        with mock.patch.object(prepare, "publish_tree") as publish:
            prepare.resume_prepared_tree(self.root, self.root / "new", self.lock)
        publish.assert_called_once_with(self.root, self.root / "new")

    def test_resume_corrupt_tree_never_publishes(self):
        (self.root / "tsc/go.mod").write_text("changed")
        with mock.patch.object(prepare, "publish_tree") as publish:
            with self.assertRaisesRegex(ValueError, "differs from lock"):
                prepare.resume_prepared_tree(self.root, self.root / "new", self.lock)
        publish.assert_not_called()

    def test_resume_existing_destination_rejected(self):
        with mock.patch.object(prepare, "publish_tree") as publish:
            with self.assertRaisesRegex(ValueError, "already exists"):
                prepare.resume_prepared_tree(self.root / "stage", self.root, self.lock)
        publish.assert_not_called()

    def test_resume_missing_stage_rejected(self):
        with mock.patch.object(prepare, "publish_tree") as publish:
            with self.assertRaisesRegex(ValueError, "staging directory"):
                prepare.resume_prepared_tree(self.root / "absent", self.root / "new", self.lock)
        publish.assert_not_called()

    def test_copy_recovery_preserves_source_and_checks_target(self):
        with tempfile.TemporaryDirectory() as outer:
            target = pathlib.Path(outer) / "copy"
            prepare.resume_prepared_tree(self.root, target, self.lock, copy_files=True)
            self.assertTrue((self.root / "tsc/go.mod").is_file())
            self.assertEqual(prepare.verify_tree(target, self.lock)["selectedFiles"], 3)

    def test_copy_recovery_rejects_incomplete_destination(self):
        with tempfile.TemporaryDirectory() as outer:
            target = pathlib.Path(outer) / "copy"
            with mock.patch.object(prepare.shutil, "copytree"):
                with self.assertRaisesRegex(ValueError, "differs from lock"):
                    prepare.resume_prepared_tree(self.root, target, self.lock, copy_files=True)
            self.assertTrue((self.root / "tsc/go.mod").is_file())

    def test_manifest_order_independent(self):
        entries = [("b", "f" * 64), ("a", "e" * 64)]
        self.assertEqual(prepare.tree_digest(entries), prepare.tree_digest(list(reversed(entries))))


if __name__ == "__main__":
    unittest.main(verbosity=2)
