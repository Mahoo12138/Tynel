"""Prepare/verify the exact, selected upstream source tree; never patch it.

Only the compiler subtree and declaration sources are extracted. Upstream's
large conformance corpus is not needed for this facade probe. All preserved
licenses/notices and every selected source byte participate in the tree hash.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import re
import shutil
import tarfile
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
MAX_ARCHIVE_BYTES = 128 << 20
EXCLUDED = {"testdata", "_submodules", "_extension", "_packages", "_tools"}


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def tree_digest(entries: list[tuple[str, str]]) -> str:
    return digest("".join(name + "\0" + sha + "\n" for name, sha in sorted(entries)).encode())


def selected(name: str) -> bool:
    return name in {"AGENTS.md", "LICENSE.txt", "NOTICE.txt"} or (
        name.startswith("tsc/") and not set(name.split("/")).intersection(EXCLUDED)
    ) or name.startswith("src/lib/")


def verify_tree(target: pathlib.Path, lock: dict) -> dict:
    entries = []
    for path in target.rglob("*"):
        if path.is_symlink():
            raise ValueError(f"unexpected symlink: {path}")
        if path.is_file():
            entries.append((path.relative_to(target).as_posix(), digest(path.read_bytes())))
    if len(entries) != lock["selectedFiles"] or tree_digest(entries) != lock["selectedTreeSHA256"]:
        raise ValueError("selected TypeScript source tree differs from lock; do not silently refresh it")
    libs = [item for item in entries if item[0].startswith("tsc/internal/bundled/libs/")]
    if len(libs) != lock["declarationFiles"] or tree_digest(libs) != lock["declarationSHA256"]:
        raise ValueError("bundled declaration identity mismatch")
    for filename, key in [("go.mod", "goModSHA256"), ("go.sum", "goSumSHA256")]:
        if digest((target / "tsc" / filename).read_bytes()) != lock[key]:
            raise ValueError(f"upstream {filename} changed")
    return {"sourceRoot": str(target), "selectedFiles": len(entries), "declarationFiles": len(libs), "selectedTreeSHA256": tree_digest(entries)}


def publish_tree(stage: pathlib.Path, target: pathlib.Path) -> None:
    """Publish verified sources, retrying only bounded Windows sharing errors."""
    for attempt in range(8):
        if target.exists():
            raise ValueError("source destination appeared during preparation; inspect before retry")
        try:
            stage.rename(target)
            return
        except PermissionError as error:
            if getattr(error, "winerror", None) not in (5, 32, 33) or attempt == 7:
                raise
            time.sleep(0.15 * (attempt + 1))


def resume_prepared_tree(stage: pathlib.Path, target: pathlib.Path, lock: dict, *, copy_files: bool = False) -> None:
    """Explicitly resume only a complete checksum-verified extraction."""
    if target.exists() or target.is_symlink():
        raise ValueError("source destination already exists; resume never replaces it")
    if stage.is_symlink() or not stage.is_dir():
        raise ValueError("no regular staging directory to resume")
    verify_tree(stage, lock)
    if copy_files:
        # Explicit non-atomic recovery: readers still verify the complete tree.
        # copytree refuses an existing target; never delete the source or repair
        # a partially copied destination without a separate reviewed action.
        shutil.copytree(stage, target, symlinks=True)
        verify_tree(target, lock)
    else:
        publish_tree(stage, target)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--verify-only", action="store_true", help="never download or extract")
    parser.add_argument("--resume-publish", action="store_true", help="reverify an existing complete extraction and retry its rename; never download")
    parser.add_argument("--copy-verified-stage", action="store_true", help="with --resume-publish, copy instead of rename; reverify target and preserve source")
    args = parser.parse_args()
    if args.verify_only and args.resume_publish:
        raise ValueError("verify-only and resume-publish cannot be combined")
    if args.copy_verified_stage and not args.resume_publish:
        raise ValueError("copy-verified-stage requires resume-publish")
    lock = json.loads((ROOT / "toolchain.lock.json").read_text(encoding="utf-8"))["typescript"]
    sha = lock["commit"]
    if not re.fullmatch(r"[0-9a-f]{40}", sha) or lock["ref"] != "v7.0.2":
        raise ValueError("this preparation recipe requires the reviewed v7.0.2 source layout")
    prefix = "TypeScript-" + sha
    target = ROOT / ".upstream" / "tsgo-v7.0.2" / prefix
    if args.resume_publish:
        resume_prepared_tree(target.with_name(prefix + ".extracting"), target, lock, copy_files=args.copy_verified_stage)
    if not target.exists():
        if args.verify_only:
            raise ValueError("upstream source missing; run prepare-tsgo.py once with network available")
        archive = ROOT / ".upstream" / "downloads" / ("typescript-" + sha + ".tar.gz")
        archive.parent.mkdir(parents=True, exist_ok=True)
        if not archive.exists():
            temporary = archive.with_suffix(".partial")
            total = 0
            try:
                with urllib.request.urlopen("https://codeload.github.com/microsoft/TypeScript/tar.gz/" + sha, timeout=30) as response, temporary.open("wb") as out:
                    while block := response.read(1 << 20):
                        total += len(block)
                        if total > MAX_ARCHIVE_BYTES:
                            raise ValueError("upstream archive exceeds budget")
                        out.write(block)
                if digest(temporary.read_bytes()) != lock["archiveSHA256"]:
                    raise ValueError("downloaded archive hash mismatch")
                temporary.replace(archive)
            finally:
                temporary.unlink(missing_ok=True)
        if archive.stat().st_size > MAX_ARCHIVE_BYTES or digest(archive.read_bytes()) != lock["archiveSHA256"]:
            raise ValueError("upstream archive hash mismatch")
        stage = target.with_name(prefix + ".extracting")
        if stage.exists():
            raise ValueError(f"staging directory at {stage}; inspect, then use --resume-publish for verified complete content")
        stage.mkdir(parents=True)
        with tarfile.open(archive) as tar:
            for member in tar:
                parts = pathlib.PurePosixPath(member.name).parts
                if not parts or parts[0] != prefix or ".." in parts:
                    raise ValueError("unexpected archive path")
                name = "/".join(parts[1:])
                if not member.isfile() or not selected(name):
                    continue
                destination = stage.joinpath(*parts[1:])
                destination.parent.mkdir(parents=True, exist_ok=True)
                if destination.exists() or member.size > (32 << 20):
                    raise ValueError("duplicate or oversized archive entry")
                stream = tar.extractfile(member)
                if stream is None:
                    raise ValueError("missing archive entry")
                with stream:
                    destination.write_bytes(stream.read())
        verify_tree(stage, lock)
        publish_tree(stage, target)
    report = verify_tree(target, lock)
    report.update(status="pass", upstreamCommit=sha, scope="selected compiler/declarations; not the full upstream test corpus")
    print(json.dumps(report, ensure_ascii=True))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, tarfile.TarError) as error:
        raise SystemExit("TY1104: " + str(error))
