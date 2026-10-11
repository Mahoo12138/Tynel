"""Boolean short-circuit effects, source/NIR replay, and compiler rejections.

Run after building both bridges. Fixtures stay in .tynel/short-circuit-smoke;
Node supplies a differential oracle and is not part of the generated program.
"""
from __future__ import annotations

import copy
import itertools
import json
import os
import pathlib
import subprocess
import sys
import uuid
from importlib.util import module_from_spec, spec_from_file_location

sys.dont_write_bytecode = True
spec = spec_from_file_location('scalar_smoke', pathlib.Path(__file__).with_name('smoke-ts.py'))
scalar = module_from_spec(spec)
spec.loader.exec_module(scalar)
ROOT = scalar.ROOT
WORK = ROOT / '.tynel/short-circuit-smoke'
EXPECTED = '1\nfalse\n1\n2\ntrue\n3\ntrue\n3\n4\nfalse\n'
MARK = 'function mark(id:number,b:boolean):boolean{console.log(id);return b;}'


def nested_matrix() -> tuple[str, str]:
    """Independent boolean evaluator also records which leaves were visited."""
    shapes = [('and', 0, ('or', 1, 2)), ('or', 0, ('and', 1, 2)),
              ('and', ('or', 0, 1), 2), ('or', ('and', 0, 1), 2)]

    def source(tree) -> str:
        if isinstance(tree, int):
            return f'mark({tree + 1},b{tree})'
        op, left, right = tree
        token = '&&' if op == 'and' else '||'
        return f'({source(left)}{token}{source(right)})'

    def visit(tree, values, trace) -> bool:
        if isinstance(tree, int):
            trace.append(str(tree + 1))
            return values[tree]
        op, left, right = tree
        value = visit(left, values, trace)
        if (op == 'and' and not value) or (op == 'or' and value):
            return value
        return visit(right, values, trace)

    text = MARK
    for i, shape in enumerate(shapes):
        text += f'function nested{i}(b0:boolean,b1:boolean,b2:boolean):boolean{{return {source(shape)};}}'
    trace: list[str] = []
    for values in itertools.product((False, True), repeat=3):
        args = ','.join(str(v).lower() for v in values)
        for i, shape in enumerate(shapes):
            text += f'console.log(nested{i}({args}));'
            trace.append(str(visit(shape, values, trace)).lower())
    return text, '\n'.join(trace) + '\n'


def main() -> None:
    WORK.mkdir(parents=True, exist_ok=True)
    source, binary = WORK / 'input.ts', WORK / 'program.exe'
    cases = [
        ('public', (ROOT/'examples/native/short-circuit.ts').read_text(encoding='utf8'), EXPECTED),
        ('grouping-example', (ROOT/'examples/native/short-circuit-order.ts').read_text(encoding='utf8'), '1\ntrue\n1\n3\nfalse\n'),
        ('skip-at-same-path', MARK+'console.log(mark(1,false)&&mark(2,true));', '1\nfalse\n'),
        ('take-at-same-path', MARK+'console.log(mark(1,true)&&mark(2,true));', '1\n2\ntrue\n'),
        ('or-skip', MARK+'console.log(mark(1,true)||mark(2,false));', '1\ntrue\n'),
        ('or-take', MARK+'console.log(mark(1,false)||mark(2,false));', '1\n2\nfalse\n'),
        ('precedence', MARK+'console.log(mark(1,true)||mark(2,false)&&mark(3,false));', '1\ntrue\n'),
        ('parentheses', MARK+'console.log((mark(1,true)||mark(2,false))&&mark(3,false));', '1\n3\nfalse\n'),
        ('nested-right-skipped', MARK+'console.log(mark(1,false)&&(mark(2,false)||mark(3,true)));', '1\nfalse\n'),
        ('nested-right-taken', MARK+'console.log(mark(1,true)&&(mark(2,false)||mark(3,true)));', '1\n2\n3\ntrue\n'),
        ('const-reuse', MARK+'const result=mark(1,false)||mark(2,true);console.log(result);console.log(result);', '1\n2\ntrue\ntrue\n'),
        ('call-argument-order', MARK+'function combine(a:boolean,b:boolean,c:boolean):boolean{console.log(90);return a||b||c;}console.log(combine(mark(1,false)&&mark(2,true),mark(3,false)||mark(4,true),mark(5,true)));', '1\n3\n4\n5\n90\ntrue\n'),
        ('right-preparation', MARK+'function n(id:number,x:number):number{console.log(id);return x;}function check(a:boolean):boolean{return mark(1,a)&&(n(2,40)+n(3,2)>0);}console.log(check(false));console.log(check(true));', '1\nfalse\n1\n2\n3\ntrue\n'),
        ('if-condition', MARK+'function choose(a:boolean):number{if(a&&mark(2,true)){return 42;}else{return 0;}}console.log(choose(false));console.log(choose(true));', '0\n2\n42\n'),
        ('negated-result', MARK+'console.log(!((mark(1,true)&&mark(2,false))||mark(3,true)));', '1\n2\n3\nfalse\n'),
        ('discard', MARK+'mark(1,false)&&mark(2,true);mark(3,true)||mark(4,false);console.log(42);', '1\n3\n42\n'),
        ('skip-recursion', 'function spin():boolean{return spin();}function guarded(a:boolean):boolean{return a&&spin();}console.log(guarded(false));console.log(true||spin());', 'false\ntrue\n'),
        ('scoped-symbols', 'function keep(值:boolean):boolean{return 值||false;}const 值=false;{const 值=true;console.log(keep(值)&&true);}console.log(keep(值)||false);', 'true\nfalse\n'),
        ('nan-comparison', 'function finiteChoice(x:number):boolean{return (x===x)&&(x>0);}console.log(finiteChoice(0/0));console.log(finiteChoice(42));', 'false\ntrue\n'),
    ]
    matrix, expected_matrix = nested_matrix()
    cases.append(('nested-32-path-matrix', matrix, expected_matrix))
    results = []
    for name, text, expected in cases:
        source.write_text(text, encoding='utf8', newline='\n')
        scalar.checked(scalar.ps('compile-ts.ps1','-InputTS',str(source),'-Output',str(binary)))
        native = scalar.checked([str(binary)], 15)
        node = scalar.checked(['node',str(source)], 15)
        if native.stderr or node.stderr or native.stdout != expected or node.stdout != expected:
            raise AssertionError(f'{name}: native={native.stdout!r}, Node={node.stdout!r}, expected={expected!r}')
        record = json.loads(pathlib.Path(str(binary)+'.tynel.json').read_text(encoding='utf-8-sig'))
        if record['profile']!='tynel.scalar/3' or record['irVersion']!=4 or record['sourceSHA256']!=scalar.digest(source) or record['binarySHA256']!=scalar.digest(binary) or record['nirSHA256']!=scalar.digest(ROOT/record['nir']):
            raise AssertionError('Build identity mismatch')
        results.append({'case':name,'stdout':native.stdout,'sourceSHA256':scalar.digest(source),'binarySHA256':scalar.digest(binary)})
        print(f'PASS: short-circuit {name} (native and Node)', flush=True)

    negatives = [
        ('hidden-numeric-right-and','console.log(false&&42);','TY1201'),
        ('hidden-numeric-right-or','console.log(true||42);','TY1201'),
        ('numeric-left-signature','function f(x:number):boolean{return x&&true;}console.log(f(1));','TS2322'),
        ('numeric-left','function f(x:number):number{return x;}console.log(f(0)&&true);','TY1201'),
        ('skipped-remainder','console.log(false&&((1%2)>0));','TY1201'),
        ('skipped-library','console.log(true||(Math.abs(-1)>0));','TY1201'),
        ('unused-function','function f(a:boolean):boolean{return a&&((1%2)>0);}console.log(42);','TY1201'),
        ('logical-assignment','function f(a:boolean,b:boolean):boolean{a&&=b;return a;}console.log(f(true,false));','TY1201'),
        ('nullish','function f(a:boolean):boolean{return a??false;}console.log(f(true));','TY1201'),
        ('skipped-unknown','console.log(false&&missing);','TS2304'),
    ]
    sidecar = pathlib.Path(str(binary)+'.tynel.json')
    for name, text, code in negatives:
        before, before_record = scalar.digest(binary), scalar.digest(sidecar)
        source.write_text(text, encoding='utf8', newline='\n')
        emitted = scalar.invoke([str(scalar.FRONT),'-emit','nir',str(source)],30)
        built = scalar.invoke(scalar.ps('compile-ts.ps1','-InputTS',str(source),'-Output',str(binary)))
        if emitted.returncode==0 or emitted.stdout or code not in emitted.stderr or built.returncode==0 or code not in built.stderr or scalar.digest(binary)!=before or scalar.digest(sidecar)!=before_record:
            raise AssertionError(f'{name}: incorrect rejection or overwritten artifact\n{emitted}\n{built}')
        print(f'PASS: short-circuit rejects {name} ({code})',flush=True)

    public = ROOT/'dist/short-circuit.exe'
    scalar.checked(scalar.ps('compile-ts.ps1','-InputTS','examples/native/short-circuit.ts','-Output',str(public)))
    record = json.loads(pathlib.Path(str(public)+'.tynel.json').read_text(encoding='utf-8-sig'))
    saved = ROOT/record['nir']
    module = json.loads(saved.read_text(encoding='utf8'))
    replay = WORK/'replay.exe'
    scalar.checked(scalar.ps('compile-ir.ps1','-InputIR',str(saved),'-Output',str(replay)))
    if scalar.checked([str(public)],15).stdout!=EXPECTED or scalar.checked([str(replay)],15).stdout!=EXPECTED:
        raise AssertionError('Saved NIR replay changed short-circuit effects')
    print('PASS: short-circuit NIR replay without frontend',flush=True)

    identity = json.loads((ROOT/'.tynel/bridge/identity.json').read_text(encoding='utf-8-sig'))
    env = os.environ.copy()
    env.update(GOROOT=identity['goroot'],GOTOOLCHAIN='local',TYNEL_COMPILER=str(ROOT/'.tynel/bin/tynel-compile.exe'),TYNEL_COMPILER_SHA256=identity['compilerSHA256'])
    sdk = pathlib.Path(identity['goroot'])/'bin/go.exe'
    wrapper = ROOT/'.tynel/bin/tynel-toolexec.exe'
    rejected = WORK/('rejections-'+uuid.uuid4().hex)
    rejected.mkdir()
    # Feed malformed NIR to the compiler directly, independently of CLI checks.
    for name in ('old-version','number-left','number-right','rhs-return','rhs-local-leak'):
        bad = copy.deepcopy(module)
        fn = next(f for f in bad['functions'] if any(s['op']=='short_and' for s in f['body']))
        logical = next(s for s in fn['body'] if s['op']=='short_and')
        number = {'op':'number','type':'number','bits':'0000000000000000','loc':logical['loc']}
        if name=='old-version': bad['irVersion']=3
        elif name=='number-left': logical['condition']=number
        elif name=='number-right': logical['value']=number
        elif name=='rhs-return': logical['body']=[{'op':'return','loc':logical['loc']}]
        else: fn['body'][-1]['value']={'op':'local','type':'boolean','name':logical['body'][0]['name'],'loc':logical['loc']}
        path=rejected/(name+'.nir.json'); path.write_text(json.dumps(bad),encoding='utf8')
        env['TYNEL_IR']=str(path)
        output=rejected/(name+'.exe')
        result=subprocess.run([str(sdk),'build','-p','2','-toolexec','"'+str(wrapper)+'"','-o',str(output),'.'],cwd=ROOT/'experiments/bootstrap',env=env,capture_output=True,text=True,encoding='utf8',timeout=120)
        if result.returncode==0 or 'TY4001' not in result.stderr or output.exists():
            raise AssertionError(f'compiler failed to reject {name}: {result}')
        print(f'PASS: compiler rejects short-circuit {name}',flush=True)
    grouping = ROOT/'dist/short-circuit-order.exe'
    scalar.checked(scalar.ps('compile-ts.ps1','-InputTS','examples/native/short-circuit-order.ts','-Output',str(grouping)))
    if scalar.checked([str(grouping)],15).stdout != '1\ntrue\n1\n3\nfalse\n':
        raise AssertionError('Final grouping example changed')
    asm=scalar.checked([str(sdk),'tool','objdump','-s','^main\\.f1$',str(public)])
    if 'examples/native/short-circuit.ts' not in asm.stdout or 'CALL main.f0' not in asm.stdout or 'RET' not in asm.stdout:
        raise AssertionError('missing source-mapped native short-circuit function')
    (WORK/'short-circuit.asm.txt').write_text(asm.stdout,encoding='utf8')
    report={'schemaVersion':1,'profile':'tynel.scalar/3','irVersion':4,'target':'windows/amd64','positivePrograms':results,'nestedMatrixPaths':32,'negativeBuilds':len(negatives),'compilerRejections':5,'nirReplay':'passed','finalOutput':'dist/short-circuit.exe','stdout':EXPECTED,'nodeVersion':scalar.checked(['node','--version']).stdout.strip()}
    (WORK/'result.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf8')
    print(f'Short-circuit smoke passed: {len(cases)} programs, {len(negatives)} rejected builds, 5 compiler rejections and replay.',flush=True)


if __name__=='__main__':
    try:
        main()
    except (OSError,ValueError,AssertionError,RuntimeError,subprocess.TimeoutExpired) as error:
        print(f'Short-circuit smoke failed: {error}',file=sys.stderr)
        raise SystemExit(1)
