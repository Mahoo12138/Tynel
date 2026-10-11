"""Real TS -> detached facts -> NIR -> native regression for the scalar pilot.

Run after build-bridge.ps1 and build-tsgo-probe.ps1 -Test. Node is an oracle,
not a native program dependency. All generated fixtures stay under .tynel.
"""
from __future__ import annotations

import hashlib
import json
import math
import pathlib
import struct
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
WORK = ROOT / '.tynel' / 'ts-smoke'
FRONT = ROOT / '.tynel' / 'tsgo' / 'tynel-tsgo-probe.exe'


def invoke(argv: list[str], timeout: int = 120) -> subprocess.CompletedProcess[str]:
    return subprocess.run(argv, cwd=ROOT, capture_output=True, text=True, encoding='utf-8', errors='strict', timeout=timeout)


def checked(argv: list[str], timeout: int = 120) -> subprocess.CompletedProcess[str]:
    result = invoke(argv, timeout)
    if result.returncode:
        raise RuntimeError(f'{argv[0]} exited {result.returncode}\n{result.stdout}\n{result.stderr}')
    return result


def ps(script: str, *args: str) -> list[str]:
    return ['pwsh', '-NoLogo', '-NoProfile', '-NonInteractive', '-File', str(ROOT / 'scripts' / script), *args]


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def observations(text: str) -> list[str]:
    """Only numeric console lines: normalize documented printer spellings,
    compare finite binary64 values (including zero's sign), not decimal style.
    """
    result = []
    for line in text.splitlines():
        if line in ('NaN', 'nan'):
            result.append('nan')
        elif line in ('Infinity', '+Inf', 'inf'):
            result.append('+inf')
        elif line in ('-Infinity', '-Inf', '-inf'):
            result.append('-inf')
        else:
            number = float(line)
            if not math.isfinite(number):
                raise ValueError(f'Unexpected numeric format: {line!r}')
            result.append(struct.pack('>d', number).hex())
    return result


def main() -> None:
    WORK.mkdir(parents=True, exist_ok=True)
    source = WORK / 'input.ts'
    binary = WORK / 'program.exe'
    cases = [
        ('add', (ROOT / 'examples/native/add.ts').read_text(encoding='utf-8'), '42\n'),
        ('same-path-cache', 'function add(a:number,b:number):number{return a+b;} console.log(add(41,2));', '43\n'),
        ('discard', (ROOT / 'examples/frontend/add.ts').read_text(encoding='utf-8'), ''),
        ('order', 'function mark(x:number):number{console.log(x);return x;} function sum(a:number,b:number):number{return a+b;} console.log(sum(mark(1),mark(2)));', '1\n2\n3\n'),
        ('float', 'function neg(x:number):number{return -x;} console.log(neg(0)); console.log(1/neg(0)); console.log(0/0); console.log(0.1+0.2); console.log(-(neg(0)));', '-0\n-Inf\nNaN\n0.30000000000000004\n0\n'),
        ('radix', 'console.log(0x2a + 0b10 + 0o10 + 1_000);', '1052\n'),
        ('symbols-unicode', 'function 同名(值:number):number{return 值;} function other(值:number):number{const x=值*2;return x;} const 值=7; console.log(同名(值)+other(值));', '21\n'),
        ('forward-call', 'console.log(first(41)); function first(x:number):number{return second(x);} function second(x:number):number{return x+1;}', '42\n'),
        ('rounding', 'function separate(a:number,b:number,c:number):number{return a*b+c;} console.log(separate(1.0000000000000002,1.0000000000000002,-1.0000000000000004));', '0\n'),
    ]
    results = []
    for name, text, expected in cases:
        source.write_text(text, encoding='utf-8', newline='\n')
        checked(ps('compile-ts.ps1', '-InputTS', str(source), '-Output', str(binary)))
        native = checked([str(binary)], 15)
        node = checked(['node', str(source)], 15)
        if native.stderr or node.stderr or native.stdout != expected or observations(native.stdout) != observations(node.stdout):
            raise AssertionError(f'{name}: native={native.stdout!r}/{native.stderr!r}, node={node.stdout!r}/{node.stderr!r}, expected={expected!r}')
        metadata = json.loads(pathlib.Path(str(binary) + '.tynel.json').read_text(encoding='utf-8-sig'))
        if metadata['sourceSHA256'] != digest(source) or metadata['binarySHA256'] != digest(binary):
            raise AssertionError('Build provenance does not match source/binary')
        if digest(ROOT / metadata['nir']) != metadata['nirSHA256']:
            raise AssertionError('NIR provenance mismatch')
        results.append({'case': name, 'stdout': native.stdout, 'oracle': 'Node numeric observations', 'sourceSHA256': digest(source), 'binarySHA256': digest(binary)})
        print(f'PASS: {name} (native and Node)', flush=True)

    # Replay the saved NIR directly, without invoking the frontend again.
    replay = WORK / 'replay.exe'
    checked(ps('compile-ir.ps1', '-InputIR', str(ROOT / metadata['nir']), '-Output', str(replay)))
    if checked([str(replay)], 15).stdout != '0\n':
        raise AssertionError('NIR replay changed the rounding program')
    print('PASS: saved NIR replay through the backend', flush=True)

    for name, text, code in [
        ('type-error', "const x:number='bad'; console.log(x);", 'TS2322'),
        ('unsupported', 'console.log(42 % 2);', 'TY1201'),
        ('unused-unsupported', 'function unused(x:number):number{return x%2;} console.log(42);', 'TY1201'),
    ]:
        old_binary = digest(binary)
        source.write_text(text, encoding='utf-8', newline='\n')
        emitted = invoke([str(FRONT), '-emit', 'nir', str(source)], 30)
        if emitted.returncode == 0 or emitted.stdout or code not in emitted.stderr:
            raise AssertionError(f'{name}: incorrect source rejection {emitted}')
        built = invoke(ps('compile-ts.ps1', '-InputTS', str(source), '-Output', str(binary)))
        if built.returncode == 0 or code not in built.stderr or digest(binary) != old_binary:
            raise AssertionError(f'{name}: failed build replaced old binary or wrong refusal {built}')
        print(f'PASS: {name} rejects with {code}; no emitted NIR or replacement binary', flush=True)

    # Keep a useful public output from real TS, separate from hand-authored add.nir.json.
    public = ROOT / 'dist' / 'ts-add.exe'
    checked(ps('compile-ts.ps1', '-InputTS', 'examples/native/add.ts', '-Output', str(public)))
    if checked([str(public)], 15).stdout != '42\n':
        raise AssertionError('Final TypeScript example failed')
    sdk = ROOT / '.tynel' / 'sdk' / 'go1.26.7' / 'bin' / 'go.exe'
    asm = checked([str(sdk), 'tool', 'objdump', '-s', '^main\\.f0$', str(public)])
    if 'ADDSD' not in asm.stdout or 'examples/native/add.ts' not in asm.stdout:
        raise AssertionError('Missing generated addition or real TS source location')
    (WORK / 'add.asm.txt').write_text(asm.stdout, encoding='utf-8')
    record = {'schemaVersion': 1, 'profile': 'tynel.scalar/2', 'target': 'windows/amd64', 'positivePrograms': results, 'replay': 'passed', 'negativeBuilds': 3, 'finalOutput': 'dist/ts-add.exe', 'nodeVersion': checked(['node','--version']).stdout.strip(), 'frontendSHA256': digest(FRONT)}
    (WORK / 'result.json').write_text(json.dumps(record, ensure_ascii=False, indent=2), encoding='utf-8')
    print('TypeScript native smoke passed: 9 positive programs, replay, 3 negative builds, final source example.', flush=True)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, AssertionError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f'Scalar smoke failed: {error}', file=sys.stderr)
        raise SystemExit(1)
