#!/usr/bin/env python3
"""Check one-to-one chapter/stage metadata and a root-first annotated tag chain."""
from __future__ import annotations
import argparse
import json
import pathlib
import re
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]


def git(root: pathlib.Path, *args: str) -> str:
    return subprocess.check_output(['git', '-C', str(root), *args], text=True, encoding='utf-8').strip()


def frontmatter(text: str) -> tuple[int, str]:
    header = re.match(r'\A---\n(.*?)\n---\n', text, re.S)
    if not header:
        raise ValueError('missing note frontmatter')
    chapter = re.findall(r'^chapter: ([1-9][0-9]*)$', header[1], re.M)
    tag = re.findall(r'^experiment_tag: (\S+)$', header[1], re.M)
    if len(chapter) != 1 or len(tag) != 1:
        raise ValueError('missing or duplicate chapter/tag')
    return int(chapter[0]), tag[0]


def validate(root: pathlib.Path, tags: bool = False) -> dict:
    index = json.loads((root / 'docs/stages.json').read_text(encoding='utf-8'))
    stages = index['stages']
    if index.get('schemaVersion') != 3 or not stages or 'baseCommit' in index:
        raise ValueError('invalid root-first stage index')
    names = [s['tag'] for s in stages]
    if len(set(names)) != len(names):
        raise ValueError('duplicate stage tag')
    notes, previous = {}, None
    for number, stage in enumerate(stages, 1):
        if not re.fullmatch(r'stage/' + f'{number:02d}' + r'-[a-z0-9-]+', stage['tag']) or stage.get('parentTag') != previous:
            raise ValueError('nonlinear stage index')
        previous = stage['tag']
        note, chapter = stage.get('note'), stage.get('chapter')
        if not isinstance(chapter, int) or isinstance(chapter, bool) or chapter != number:
            raise ValueError('nonconsecutive chapters')
        if not isinstance(note, str) or not re.fullmatch(r'docs/notes/' + f'{number:02d}' + r'-[a-z0-9-]+\.md', note) or note in notes:
            raise ValueError('each stage requires exactly one chapter path')
        if not isinstance(stage.get('frontend'), bool):
            raise ValueError('frontend flag must be boolean')
        notes[note] = stage
        if frontmatter((root / note).read_text(encoding='utf-8')) != (chapter, stage['tag']):
            raise ValueError('note/tag mismatch')
    actual = {p.relative_to(root).as_posix() for p in (root / 'docs/notes').glob('*.md')}
    if actual != set(notes):
        raise ValueError('orphan note file')
    manifest = json.loads((root / 'replay.json').read_text(encoding='utf-8'))
    current = stages[-1]
    if manifest.get('schemaVersion') != 3 or any(manifest.get(k) != current.get(k) for k in ('tag', 'parentTag', 'chapter', 'note')) or manifest['validation']['frontend'] != current['frontend']:
        raise ValueError('stage manifest mismatch')
    for key in ('script', 'record'):
        if not (root / manifest['validation'][key]).is_file():
            raise ValueError('missing replay entry or validation record')
    documents = [root / 'README.md', *sorted((root / 'docs').rglob('*.md'))]
    count = 0
    for path in documents:
        text = path.read_text(encoding='utf-8')
        prose = re.sub(r'```[^\n]*\n.*?```', '', text, flags=re.S)
        targets = re.findall(r'\]\(([^\s)]+)(?:\s+[^)]*)?\)', prose)
        targets += re.findall(r'^\[(?!\^)[^\]]+\]:\s+(\S+)', prose, re.M)
        for target in targets:
            if target.startswith(('https://', 'http://', '#', 'mailto:')):
                continue
            if not (path.parent / target.split('#', 1)[0]).exists():
                raise ValueError(f'missing link {path.relative_to(root)} -> {target}')
            count += 1
        if path.parent == root / 'docs/notes':
            used = set(re.findall(r'\[\^([^\]]+)\](?!:)', text))
            defined = set(re.findall(r'^\[\^([^\]]+)\]:', text, re.M))
            if not used <= defined:
                raise ValueError(f'missing note footnote: {path.name}')
    resolved = {}
    if tags:
        parent = ''  # The first stage is a root, not a child of an older project.
        for stage in stages:
            tag = stage['tag']
            if git(root, 'cat-file', '-t', 'refs/tags/' + tag) != 'tag':
                raise ValueError('not annotated: ' + tag)
            commit = git(root, 'rev-parse', f'refs/tags/{tag}^{{commit}}')
            if git(root, 'show', '-s', '--format=%P', commit) != parent:
                raise ValueError('nonlinear Git parent: ' + tag)
            saved = json.loads(git(root, 'show', f'{commit}:replay.json'))
            if saved.get('schemaVersion') != 3 or any(saved.get(k) != stage.get(k) for k in ('tag', 'parentTag', 'chapter', 'note')) or saved['validation']['frontend'] != stage['frontend']:
                raise ValueError('tagged manifest mismatch: ' + tag)
            if frontmatter(git(root, 'show', f"{commit}:{stage['note']}")) != (stage['chapter'], tag):
                raise ValueError('tagged note mismatch: ' + tag)
            tree_files = git(root, 'ls-tree', '-r', '--name-only', commit).splitlines()
            present = {p for p in tree_files if p.startswith('docs/notes/') and p.endswith('.md')}
            expected = {s['note'] for s in stages[:stage['chapter']]}
            if present != expected:
                raise ValueError('unexpected chapters in tag: ' + tag)
            resolved[tag] = commit
            parent = commit
    return {'status': 'pass', 'documents': len(documents), 'localLinks': count,
            'chapters': list(range(1, len(stages) + 1)), 'currentStage': manifest['tag'], 'annotatedTags': resolved}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tags', action='store_true')
    args = parser.parse_args()
    print(json.dumps(validate(ROOT, args.tags), ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
