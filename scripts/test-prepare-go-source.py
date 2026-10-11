#!/usr/bin/env python3
import importlib.util
import pathlib
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("prepare_go", pathlib.Path(__file__).with_name("prepare-go-source.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class SourceComparisonTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        (self.root / "src").mkdir()
        (self.root / "src/a.go").write_bytes(b"package a\n")
        (self.root / "src/b.go").write_bytes(b"package b\n")

    def test_deterministic_order(self):
        self.assertEqual(module.source_manifest(self.root, ["src/a.go", "src/b.go"]),
                         module.source_manifest(self.root, ["src/b.go", "src/a.go"]))

    def test_matching_sdk(self):
        _, hashes = module.source_manifest(self.root, ["src/a.go", "src/b.go"])
        result = module.compare_sdk(self.root, hashes)
        self.assertEqual(result["missing"], [])
        self.assertEqual(result["changed"], [])
        self.assertEqual(result["extraFiles"], [])

    def test_modified_file(self):
        _, hashes = module.source_manifest(self.root, ["src/a.go"])
        (self.root / "src/a.go").write_bytes(b"changed")
        self.assertEqual(module.compare_sdk(self.root, hashes)["changed"], ["src/a.go"])

    def test_missing_file(self):
        _, hashes = module.source_manifest(self.root, ["src/a.go"])
        (self.root / "src/a.go").unlink()
        self.assertEqual(module.compare_sdk(self.root, hashes)["missing"], ["src/a.go"])

    def test_extras_are_reported_not_authenticated(self):
        _, hashes = module.source_manifest(self.root, ["src/a.go"])
        self.assertEqual(module.compare_sdk(self.root, hashes)["extraFiles"], ["src/b.go"])

    def test_unsafe_path_rejected(self):
        for path in ("../outside", "/src/a.go", "src/../../outside", "a.go"):
            with self.subTest(path=path), self.assertRaises(ValueError):
                module.source_manifest(self.root, [path])


if __name__ == "__main__":
    unittest.main(verbosity=2)
