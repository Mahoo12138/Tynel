#!/usr/bin/env python3
"""Fetch a locked Go checkout; compare tracked src files without altering the SDK."""
from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]


def git(directory: pathlib.Path, *args: str) -> str:
    result = subprocess.run(["git", "-C", str(directory), *args], check=True,
                            capture_output=True, text=True, encoding="utf-8", timeout=60)
    return result.stdout.strip()


def hash_file(path: pathlib.Path) -> str:
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"expected regular file: {path}")
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_manifest(directory: pathlib.Path, paths: list[str]) -> tuple[str, dict[str, str]]:
    hashes = {}
    for name in sorted(paths):
        p = pathlib.PurePosixPath(name)
        if p.is_absolute() or ".." in p.parts or not name.startswith("src/"):
            raise ValueError(f"invalid tracked source path: {name}")
        hashes[name] = hash_file(directory / name)
    payload = "".join(f"{name}\0{value}\n" for name, value in hashes.items())
    return hashlib.sha256(payload.encode("utf-8")).hexdigest(), hashes


def compare_sdk(sdk: pathlib.Path, hashes: dict[str, str]) -> dict:
    missing, changed = [], []
    for name, expected in hashes.items():
        path = sdk / name
        if not path.is_file():
            missing.append(name)
        elif hash_file(path) != expected:
            changed.append(name)
    extras = sorted(p.relative_to(sdk).as_posix() for p in (sdk / "src").rglob("*")
                    if p.is_file() and p.relative_to(sdk).as_posix() not in hashes)
    return {"missing": missing, "changed": changed, "extraFiles": extras,
            "scope": "tracked src files only; extra/generated files and compiler binaries are not authenticated"}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--proxy", help="per-command HTTP proxy, e.g. http://127.0.0.1:7890")
    parser.add_argument("--verify-only", action="store_true")
    parser.add_argument("--sdk", type=pathlib.Path, help="optional exact SDK root to compare")
    args = parser.parse_args()
    lock = json.loads((ROOT / "toolchain.lock.json").read_text(encoding="utf-8"))["go"]
    commit, version = lock["commit"], lock["version"]
    if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("lock must contain an exact Go commit")
    if not re.fullmatch(r"go[0-9]+\.[0-9]+\.[0-9]+", version) or lock["ref"] != version:
        raise ValueError("unexpected Go release ref")
    if lock["repository"] != "https://github.com/golang/go.git":
        raise ValueError("review the Go repository change before downloading")
    if args.proxy and not args.proxy.startswith(("http://", "https://")):
        raise ValueError("proxy must be an explicit HTTP(S) URL")
    target = ROOT / ".upstream" / f"go-source-{version}"
    if not target.exists():
        if args.verify_only:
            raise ValueError("Go source absent; run without --verify-only to fetch")
        target.parent.mkdir(parents=True, exist_ok=True)
        command = ["git", "-c", "core.autocrlf=false"]
        if args.proxy:
            command += ["-c", "http.proxy=" + args.proxy]
        command += ["clone", "--depth", "1", "--branch", version, lock["repository"], str(target)]
        # An interrupted checkout is left for inspection, never silently reset.
        subprocess.run(command, check=True, timeout=300, stdout=sys.stderr, stderr=sys.stderr)
    if target.is_symlink() or pathlib.Path(git(target, "rev-parse", "--show-toplevel")).resolve() != target.resolve():
        raise ValueError("source is not its own repository root")
    if git(target, "rev-parse", "HEAD") != commit or git(target, "rev-parse", f"refs/tags/{version}^{{commit}}") != commit:
        raise ValueError("Go commit/tag differs from lock; existing checkout preserved")
    if git(target, "status", "--porcelain", "--untracked-files=all"):
        raise ValueError("Go checkout is dirty; no automatic reset is performed")
    tree = git(target, "rev-parse", "HEAD^{tree}")
    paths = git(target, "ls-files", "src").splitlines()
    digest, hashes = source_manifest(target, paths)
    if hash_file(target / "src/cmd/compile/internal/gc/main.go") != lock["gcMainSHA256"]:
        raise ValueError("compiler seam hash differs from lock")
    for key, actual in (("sourceGitTree", tree), ("trackedSrcSHA256", digest), ("trackedSrcFiles", len(paths))):
        if key in lock and lock[key] != actual:
            raise ValueError(f"Go source {key} differs from lock")
    record = {"schemaVersion": 1, "commit": commit, "gitTree": tree,
              "sourceRoot": str(target), "trackedSrcFiles": len(paths), "trackedSrcSHA256": digest,
              "sourceStatus": "verified", "sdkComparison": None}
    if args.sdk:
        sdk = args.sdk.resolve()
        if (sdk / "VERSION").read_bytes() != (target / "VERSION").read_bytes():
            raise ValueError("SDK VERSION differs from pinned source")
        record["sdkComparison"] = compare_sdk(sdk, hashes)
        if record["sdkComparison"]["missing"] or record["sdkComparison"]["changed"]:
            print(json.dumps(record, ensure_ascii=False, indent=2))
            raise ValueError("SDK tracked sources differ; SDK was not changed")
    report = ROOT / ".tynel/go-source/verification.json"
    report.parent.mkdir(parents=True, exist_ok=True)
    report.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(record, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"Go source preparation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
