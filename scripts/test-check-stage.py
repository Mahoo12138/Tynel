"""Offline contract tests for one chapter per stage and a parentless first commit."""
import importlib.util
import json
import pathlib
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('check_stage', pathlib.Path(__file__).with_name('check-stage.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class StageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.stage = {'tag': 'stage/01-example', 'parentTag': None, 'chapter': 1, 'note': 'docs/notes/01-example.md', 'frontend': False}
        self.index = {'schemaVersion': 3, 'stages': [self.stage]}
        self.manifest = {'schemaVersion': 3, **self.stage, 'validation': {'script': 'scripts/replay.ps1', 'record': 'docs/replay-validation.md', 'frontend': False}}
        self.note = '---\nchapter: 1\nexperiment_tag: stage/01-example\n---\n\n# Example\n'
        for name, text in [('README.md', '# Project\n'), ('docs/notes/01-example.md', self.note), ('docs/replay-validation.md', 'verified fixture'), ('scripts/replay.ps1', '')]:
            self.write(name, text)
        self.save()

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding='utf-8')

    def save(self):
        self.write('docs/stages.json', json.dumps(self.index))
        self.write('replay.json', json.dumps(self.manifest))

    def test_valid_index(self):
        self.assertEqual(check.validate(self.root)['chapters'], [1])

    def test_duplicate_tag(self):
        self.index['stages'].append(dict(self.stage)); self.save()
        with self.assertRaisesRegex(ValueError, 'duplicate stage'):
            check.validate(self.root)

    def test_nonconsecutive_chapters(self):
        self.stage.update(chapter=2); self.save()
        with self.assertRaisesRegex(ValueError, 'nonconsecutive'):
            check.validate(self.root)

    def test_orphan_note(self):
        self.write('docs/notes/02-extra.md', 'unregistered')
        with self.assertRaisesRegex(ValueError, 'orphan'):
            check.validate(self.root)

    def test_frontmatter_target(self):
        self.write('docs/notes/01-example.md', self.note.replace('01-example', '99-wrong'))
        with self.assertRaisesRegex(ValueError, 'note/tag mismatch'):
            check.validate(self.root)

    def test_manifest_frontend(self):
        self.manifest['validation']['frontend'] = True; self.save()
        with self.assertRaisesRegex(ValueError, 'manifest mismatch'):
            check.validate(self.root)

    def test_missing_reference_link(self):
        self.write('README.md', '[source][1]\n\n[1]: missing.go\n')
        with self.assertRaisesRegex(ValueError, 'missing link'):
            check.validate(self.root)

    def test_missing_footnote(self):
        self.write('docs/notes/01-example.md', self.note + 'Text[^missing].\n')
        with self.assertRaisesRegex(ValueError, 'missing note footnote'):
            check.validate(self.root)

    def test_non_linear_parent_index(self):
        self.stage['parentTag'] = 'stage/00-other'; self.save()
        with self.assertRaisesRegex(ValueError, 'nonlinear stage index'):
            check.validate(self.root)

    def test_root_commit_cannot_have_parent(self):
        with mock.patch.object(check, 'git', side_effect=['tag', '2'*40, '3'*40]):
            with self.assertRaisesRegex(ValueError, 'nonlinear Git parent'):
                check.validate(self.root, True)

    def test_real_tag_contract_calls(self):
        results = ['tag', '2'*40, '', json.dumps(self.manifest), self.note, 'README.md\ndocs/notes/01-example.md']
        with mock.patch.object(check, 'git', side_effect=results):
            self.assertEqual(check.validate(self.root, True)['annotatedTags'][self.stage['tag']], '2'*40)

    def test_lightweight_tag_rejected(self):
        with mock.patch.object(check, 'git', return_value='commit'):
            with self.assertRaisesRegex(ValueError, 'not annotated'):
                check.validate(self.root, True)

    def test_standalone_reference_stage_rejected(self):
        self.stage.update(note=None, chapter=None, reference='docs/reference/concepts.md'); self.save()
        with self.assertRaisesRegex(ValueError, 'nonconsecutive'):
            check.validate(self.root)

    def test_prior_history_base_rejected(self):
        self.index['baseCommit'] = '1' * 40; self.save()
        with self.assertRaisesRegex(ValueError, 'invalid root-first'):
            check.validate(self.root)

    def test_extra_chapter_in_tag_rejected(self):
        results = ['tag', '2'*40, '', json.dumps(self.manifest), self.note, 'docs/notes/01-example.md\ndocs/notes/02-extra.md']
        with mock.patch.object(check, 'git', side_effect=results):
            with self.assertRaisesRegex(ValueError, 'unexpected chapters'):
                check.validate(self.root, True)


if __name__ == '__main__':
    unittest.main(verbosity=2)
