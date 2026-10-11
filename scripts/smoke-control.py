"""Boolean/branch source differential, saved-NIR replay and fail-closed checks.

Requires the just-built frontend and Go bridge. Node is the scalar oracle only.
All source fixtures and results stay under .tynel/control-smoke.
"""
from __future__ import annotations
import copy
import json
import os
import pathlib
import subprocess
import sys

sys.dont_write_bytecode = True
from importlib.util import spec_from_file_location, module_from_spec
spec = spec_from_file_location('scalar_smoke', pathlib.Path(__file__).with_name('smoke-ts.py'))
scalar = module_from_spec(spec)
spec.loader.exec_module(scalar)
ROOT = scalar.ROOT
WORK = ROOT / '.tynel/control-smoke'
EXPECTED = '42\n0\nfalse\ntrue\n'


def main() -> None:
    WORK.mkdir(parents=True, exist_ok=True)
    source, binary = WORK / 'input.ts', WORK / 'program.exe'
    cases = [
        ('public', (ROOT / 'examples/native/branches.ts').read_text(encoding='utf8'), EXPECTED),
        ('true-at-same-path', 'function f(b:boolean):number{if(b)return 42;else return 0;}console.log(f(true));', '42\n'),
        ('false-at-same-path', 'function f(b:boolean):number{if(b)return 42;else return 0;}console.log(f(false));', '0\n'),
        ('one-condition-one-arm', 'function mark(x:number):number{console.log(x);return x;}function f(x:number):number{if(mark(x)>0){return mark(10);}else{return mark(20);}}console.log(f(1));console.log(f(-1));', '1\n10\n10\n-1\n20\n20\n'),
        ('no-hoist', 'function mark(x:number):number{console.log(x);return x;}function f(b:boolean):number{if(b){const x=mark(3)+mark(4);return x;}return mark(5);}console.log(f(false));', '5\n5\n'),
        ('scopes', 'function f(x:number,b:boolean):number{if(b){const x=10;return x;}else{const x=20;return x;}}const x=7;{const x=9;console.log(x);}console.log(x);console.log(f(0,true));console.log(f(0,false));', '9\n7\n10\n20\n'),
        ('else-if', 'function sign(x:number):number{if(x<0)return -1;else if(x>0)return 1;else return 0;}console.log(sign(-1));console.log(sign(0));console.log(sign(1));', '-1\n0\n1\n'),
        ('booleans', 'function inv(b:boolean):boolean{return !b;}function same(a:boolean,b:boolean):boolean{return a===b;}function different(a:boolean,b:boolean):boolean{return a!==b;}console.log(inv(false));console.log(inv(true));console.log(same(false,false));console.log(different(true,false));', 'true\nfalse\ntrue\ntrue\n'),
        ('comparison-effect-order', 'function mark(x:number):number{console.log(x);return x;}console.log(mark(1)<mark(2));', '1\n2\ntrue\n'),
        ('dangling-else', 'function f(a:boolean,b:boolean):number{if(a)if(b)return 1;else return 2;return 3;}console.log(f(true,true));console.log(f(true,false));console.log(f(false,true));', '1\n2\n3\n'),
        ('empty-and-fallthrough', 'if(true){}else{};if(false){}function f(b:boolean):number{if(b){console.log(1);}console.log(2);return 3;}console.log(f(false));console.log(f(true));', '2\n3\n1\n2\n3\n'),
        ('block-return', 'function f():number{{const x=42;return x;}}console.log(f());', '42\n'),
    ]
    # Ordered comparisons with NaN are not complements. JS Number strict
    # equality also identifies -0 and +0. Parameters avoid literal TS warnings.
    pairs = [('0','-0',0.0,-0.0),('1','2',1.,2.),('2','1',2.,1.),('2','2',2.,2.),
             ('0/0','0',float('nan'),0.),('0','0/0',0.,float('nan')),
             ('0/0','0/0',float('nan'),float('nan')),('1/0','-1/0',float('inf'),-float('inf'))]
    matrix = 'function cmp(a:number,b:number):number{console.log(a<b);console.log(a<=b);console.log(a>b);console.log(a>=b);console.log(a===b);console.log(a!==b);console.log(!(a>=b));return 0;}'
    expected = ''
    for a, b, x, y in pairs:
        matrix += f'cmp({a},{b});'
        expected += ''.join(str(value).lower()+'\n' for value in (x<y,x<=y,x>y,x>=y,x==y,x!=y,not x>=y))
    cases.append(('nan-zero-matrix', matrix, expected))
    results = []
    for name, text, expected in cases:
        source.write_text(text, encoding='utf8', newline='\n')
        scalar.checked(scalar.ps('compile-ts.ps1','-InputTS',str(source),'-Output',str(binary)))
        native = scalar.checked([str(binary)],15)
        oracle = scalar.checked(['node',str(source)],15)
        if native.stderr or oracle.stderr or native.stdout != expected or oracle.stdout != expected:
            raise AssertionError(f'{name}: native={native.stdout!r}, Node={oracle.stdout!r}, expected={expected!r}')
        metadata = json.loads(pathlib.Path(str(binary)+'.tynel.json').read_text(encoding='utf-8-sig'))
        if metadata['profile'] != 'tynel.scalar/2' or metadata['irVersion'] != 3 or metadata['sourceSHA256'] != scalar.digest(source) or metadata['binarySHA256'] != scalar.digest(binary):
            raise AssertionError('source, profile or binary identity mismatch')
        if metadata['nirSHA256'] != scalar.digest(ROOT/metadata['nir']):
            raise AssertionError('NIR identity mismatch')
        results.append({'case':name,'stdout':native.stdout,'sourceSHA256':scalar.digest(source),'binarySHA256':scalar.digest(binary)})
        print(f'PASS: control {name} (native and Node)',flush=True)

    failures = [
        ('numeric-condition','if(1)console.log(42);','TY1201'),
        ('numeric-not','console.log(!0);','TY1202'),
        ('unimplemented-and','function f(a:boolean,b:boolean):boolean{return a&&b;}console.log(f(true,false));','TY1201'),
        ('unimplemented-equality','function f(a:number,b:number):boolean{return a==b;}console.log(f(1,2));','TY1201'),
        ('unused-false-branch','if(false){console.log(1%2);}console.log(42);','TY1201'),
        ('wrong-return','function f():boolean{return 1;}','TS2322'),
        ('scope-escape','if(true){const hidden=42;}console.log(hidden);','TS2304'),
        ('incomplete-return','function f(b:boolean):number{if(b)return 1;}','TS2366'),
    ]
    for name,text,code in failures:
        old=scalar.digest(binary); sidecar=pathlib.Path(str(binary)+'.tynel.json'); old_record=scalar.digest(sidecar)
        source.write_text(text,encoding='utf8',newline='\n')
        emitted=scalar.invoke([str(scalar.FRONT),'-emit','nir',str(source)],30)
        built=scalar.invoke(scalar.ps('compile-ts.ps1','-InputTS',str(source),'-Output',str(binary)))
        if emitted.returncode==0 or emitted.stdout or code not in emitted.stderr or built.returncode==0 or code not in built.stderr or scalar.digest(binary)!=old or scalar.digest(sidecar)!=old_record:
            raise AssertionError(f'{name}: rejection/artifact mismatch\n{emitted}\n{built}')
        print(f'PASS: control rejects {name} ({code}), retains successful artifact',flush=True)

    public=ROOT/'dist/branches.exe'
    scalar.checked(scalar.ps('compile-ts.ps1','-InputTS','examples/native/branches.ts','-Output',str(public)))
    metadata=json.loads(pathlib.Path(str(public)+'.tynel.json').read_text(encoding='utf-8-sig'))
    saved=ROOT/metadata['nir']; module=json.loads(saved.read_text(encoding='utf8'))
    replay=WORK/'replay.exe'
    scalar.checked(scalar.ps('compile-ir.ps1','-InputIR',str(saved),'-Output',str(replay)))
    if scalar.checked([str(public)],15).stdout!=EXPECTED or scalar.checked([str(replay)],15).stdout!=EXPECTED:
        raise AssertionError('saved branch NIR replay mismatch')
    print('PASS: branch NIR replay without the frontend',flush=True)

    # Bypass CLI validation only for this negative test. The compiler must
    # reject malformed control flow itself, not trust a previous driver check.
    identity=json.loads((ROOT/'.tynel/bridge/identity.json').read_text(encoding='utf-8-sig'))
    env=os.environ.copy(); env.update(GOROOT=identity['goroot'],GOTOOLCHAIN='local',TYNEL_COMPILER=str(ROOT/'.tynel/bin/tynel-compile.exe'),TYNEL_COMPILER_SHA256=identity['compilerSHA256'])
    wrapper=ROOT/'.tynel/bin/tynel-toolexec.exe'
    sdk=pathlib.Path(identity['goroot'])/'bin/go.exe'
    for name in ('numeric-condition','old-version'):
        bad=copy.deepcopy(module)
        if name=='old-version': bad['irVersion']=2
        else:
            target=next(s for f in bad['functions'] for s in f['body'] if s['op']=='if')
            target['condition']={'op':'number','type':'number','bits':'3ff0000000000000','loc':target['loc']}
        path=WORK/(name+'.nir.json');path.write_text(json.dumps(bad),encoding='utf8')
        env['TYNEL_IR']=str(path)
        invalid_binary=WORK/(name+'.exe')
        result=subprocess.run([str(sdk),'build','-p','2','-toolexec','"'+str(wrapper)+'"','-o',str(invalid_binary),'.'],cwd=ROOT/'experiments/bootstrap',env=env,capture_output=True,text=True,encoding='utf8',timeout=120)
        expected_message='if condition must be boolean' if name=='numeric-condition' else 'unsupported IR version 2'
        if result.returncode==0 or 'TY4001' not in result.stderr or expected_message not in result.stderr:
            raise AssertionError(f'compiler did not reject {name}: {result}')
        print(f'PASS: compiler-side control rejection {name}',flush=True)
    asm=scalar.checked([str(sdk),'tool','objdump','-s','^main\\.f1$',str(public)])
    if 'examples/native/branches.ts' not in asm.stdout or 'RET' not in asm.stdout:
        raise AssertionError('branch function source/return missing')
    (WORK/'branches.asm.txt').write_text(asm.stdout,encoding='utf8')
    record={'schemaVersion':1,'profile':'tynel.scalar/2','irVersion':3,'target':'windows/amd64','positivePrograms':results,'negativeBuilds':len(failures),'compilerRejections':2,'nirReplay':'passed','finalOutput':'dist/branches.exe','stdout':EXPECTED,'nodeVersion':scalar.checked(['node','--version']).stdout.strip()}
    (WORK/'result.json').write_text(json.dumps(record,ensure_ascii=False,indent=2),encoding='utf8')
    print(f'Control smoke passed: {len(cases)} programs, {len(failures)} rejected builds, 2 compiler rejections and replay.',flush=True)


if __name__=='__main__':
    try:
        main()
    except (OSError,ValueError,AssertionError,RuntimeError,subprocess.TimeoutExpired) as error:
        print(f'Control smoke failed: {error}',file=sys.stderr)
        raise SystemExit(1)
