---
chapter: 4
experiment_tag: stage/04-control-flow
---

# Tynel 学习笔记（四）：只执行被选中的分支

加法已经能运行，现在给它加一个条件：输入大于零，返回 `x + 2`；否则返回 `0`。同一段程序应该能对 `40` 和 `-1` 作出不同选择。

仓库的 [branches.ts](../../examples/native/branches.ts) 将条件写成了一个单独的函数：

```ts
function positive(x: number): boolean {
  return x > 0;
}

function choose(x: number): number {
  if (positive(x)) {
    const answer = x + 2;
    return answer;
  } else {
    return 0;
  }
}
```

`positive` 返回 `true` 或 `false`，类型是 `boolean`。`choose` 根据这个答案选择要执行的语句。

## 先运行两条路径

按[实验指南](../reproducing.md)检出本篇标签，在该目录准备工具链。SDK 路径与代理按本机环境设置。

```powershell
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7 `
  -WithFrontend -Proxy http://127.0.0.1:7890

pwsh -NoProfile -File scripts/compile-ts.ps1 `
  -InputTS examples/native/branches.ts -Output dist/branches.exe
.\dist\branches.exe
```

文件末尾分别调用 `choose(40)`、`choose(-1)`、`positive(0)` 和 `!positive(-1)`，输出是：

```text
42
0
false
true
```

两条数值路径都能工作，布尔结果也能打印出来。下一步要检查一个更隐蔽的问题：没有选中的分支，会不会仍然执行？

## 给分支加一个可观察的动作

返回值只能告诉我们最后选了哪个答案。为了看清执行过程，在分支内调用一个会打印参数的 `mark`：

```powershell
New-Item -ItemType Directory -Force .tynel/notes | Out-Null
@'
function mark(x: number): number {
  console.log(x);
  return x;
}
function choose(flag: boolean): number {
  if (flag) {
    return mark(10);
  } else {
    return mark(20);
  }
}
console.log(choose(false));
'@ | Set-Content -Encoding utf8 .tynel/notes/chosen.ts

pwsh -NoProfile -File scripts/compile-ts.ps1 `
  -InputTS .tynel/notes/chosen.ts -Output dist/chosen.exe
.\dist\chosen.exe
```

预期只有两行：

```text
20
20
```

第一行来自 Else 分支里的 `mark(20)`，第二行来自外面的打印。`mark(10)` 没有执行。

上一篇为了保留调用顺序，将调用展开成临时值。现在这个做法仍然有用，但临时值必须留在正确的位置。假如先计算 `mark(10)` 和 `mark(20)`，再选返回值，虽然最后还是返回 `20`，却会多打印一行 `10`。

## 把调用放回各自的路径

因此，NIR 里的 `if` 保存三部分：条件、Then 语句列表、Else 语句列表。降低器遇到分支时，分别填充两个列表，再把它们挂到同一个 `if` 节点上。

```text
              读取 flag
                  │
             选择一个分支
             /          \
          true          false
           │              │
      mark(10)         mark(20)
           │              │
       返回 10          返回 20
```

沿箭头看，一次调用只经过其中一边。这样的图可以用来追踪控制流，也就是执行从哪个位置转到哪个位置。

[frontend/control.go](../../compiler/frontend/control.go) 负责保留这两个列表；[bridge/go/control.go](../../bridge/go/control.go) 再将它们转换为 Go 高层 IR。bridge 中的关键动作是：

```go
node = ir.NewIfStmt(pos, condition, yes, no)
```

`condition` 是转换后的布尔表达式，`yes`、`no` 是各自转换后的语句列表。Go 后端接着根据这些节点生成跳转与机器指令。

在最初的 `branches.ts` 中，先调用 `positive(x)`，再选择分支。反汇编可以看到这个顺序：

```powershell
.\.tynel\sdk\go1.26.7\bin\go.exe tool objdump `
  -s '^main\.f1$' dist/branches.exe
```

下面省略地址，并把跳转目标写成 `Else`：

```text
CALL main.f0(SB)
TESTL AL, AL
JE Else
```

调用得到布尔结果，检查它是否为零，为零就跳到 Else。Then 路径中的加法随后才会执行。这样，源代码、NIR 中的分支列表和机器指令表达的是同一个选择。

## 每条路径都要交回结果

还有一件事要在生成机器码前检查。下面这个函数只处理了 `flag` 为真的情况：

```ts
function incomplete(flag: boolean): number {
  if (flag) {
    return 1;
  }
}
```

传入 `false` 就会走到函数末尾，拿不到声明要求的数字。补上最后的 `return 2`，路径才完整：

```ts
function complete(flag: boolean): number {
  if (flag) {
    return 1;
  }
  return 2;
}
```

返回检查因此沿结构递归：遇到 `return`，当前路径结束；遇到 `if`，检查两个分支；还有能继续执行的路径，就接着检查后面的语句。Tynel 在自己的事实降低层和 NIR 验证层都做了检查，代码见 [control_verify.go](../../compiler/nir/control_verify.go)。

分支中的局部变量也各有范围。Then 里定义的变量不会带到 Else 或分支外面；两个分支可以使用同名变量，引用仍根据各自的声明区分。相应的错误输入与返回缺口一起进入了回归测试。

## 把这次选择保存下来

最初的分支程序同样可以保存 NIR 后回放：

```powershell
$build = Get-Content -Raw dist/branches.exe.tynel.json | ConvertFrom-Json
pwsh -NoProfile -File scripts/compile-ir.ps1 `
  -InputIR $build.nir -Output dist/branches-replayed.exe
.\dist\branches-replayed.exe
```

它应再次输出 `42`、`0`、`false`、`true`。本阶段的[专项实验](../../scripts/smoke-control.py)还检查条件只求值一次、同名变量、特殊浮点比较和错误构建，完整结果见[阶段验证记录](../replay-validation.md)。

当前条件必须是 boolean，可以使用数值比较来产生它；`if (x)` 的数字隐式转换、循环和 `&&`、`||` 暂未实现。NIR 使用 v3，旧实验通过自己的标签运行。

下一步考虑 `ready && check()`：`ready` 为假时，`check()` 也应被跳过，但整个表达式还要提供一个结果。它正好延续这次“只执行被选中路径”的问题。
