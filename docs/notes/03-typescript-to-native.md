---
chapter: 3
experiment_tag: stage/03-ts-native
---

# Tynel 学习笔记（三）：让 TypeScript 接替手写 IR

前两篇已经准备好两端：Go 后端能编译 NIR，tsgo 能读取 TypeScript 并判断类型。现在把它们连起来，让同一个加法程序从 `.ts` 文件出发，生成 exe。

输入就是 [examples/native/add.ts](../../examples/native/add.ts)：

```ts
// This source is compiled by tsgo, then lowered to Tynel NIR.
function add(a: number, b: number): number {
  return a + b;
}
const answer: number = add(40, 2);
console.log(answer);
```

## 从源码得到 42

按[实验指南](../reproducing.md)检出本篇标签，在该目录运行准备与回归命令。SDK 路径和代理按本机环境填写。

```powershell
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7 `
  -WithFrontend -Proxy http://127.0.0.1:7890
```

然后单独构建源程序：

```powershell
pwsh -NoProfile -File scripts/compile-ts.ps1 `
  -InputTS examples/native/add.ts -Output dist/ts-add.exe
.\dist\ts-add.exe
```

输出：

```text
42
```

这次改用 `compile-ts.ps1`，输入也是 `.ts`。它先调用前端生成 NIR，再交给已有的 `compile-ir.ps1`。手写 IR 的入口还保留着，两条路径最终使用同一个 Go 后端。

## 把检查器知道的东西带出来

查询到 `answer` 是 `number`，还不足以生成程序。后端得知道它来自 `add(40, 2)`，也得知道 `add` 的函数体怎样计算。

Tynel 从 tsgo 中取出函数、参数、表达式和调用关系，保存为一份自有数据，叫 ScalarFacts。可以把它理解为“检查器已经确认过的程序结构”。用阅读摘要表示，`add` 大致是这样：

```text
函数 f0，源码名 add
  参数 v1：a，number
  参数 v2：b，number
  返回 number
  函数体：返回 ref(v1) + ref(v2)
```

`f0`、`v1`、`v2` 是 Tynel 分配的编号。有了它们，两个函数即使都把参数命名为 `a`，后端也能分清每次引用指向哪个声明。编号根据 tsgo 已解析的符号关系建立，不靠搜索字符串猜测。

可以直接观察这份数据：

```powershell
.\.tynel\tsgo\tynel-tsgo-probe.exe -emit facts examples/native/add.ts
```

对应定义在 [facts.go](../../compiler/frontend/facts.go)，提取过程在 [bridge/tsgo/scalar.go](../../bridge/tsgo/scalar.go)。提取完毕后，宿主关闭 tsgo 会话，再把数据交给自己的降低器。这样后半段编译只需要这些数据，不再依赖检查器内部的对象。

## 将结构落到已有的 NIR 操作

降低器做的事情可以沿着这个例子逐项看：把 `add` 建成 NIR 函数，把参数使用转成引用，把 `a + b` 转成加法，把调用结果赋给局部值，最后生成数字输出。

```text
TypeScript → tsgo → ScalarFacts → Lower → NIR → Go 后端 → exe
```

其中 `Lower` 就是 [compiler/frontend/lower.go](../../compiler/frontend/lower.go)。它在转换时检查参数类型、调用目标和操作是否受支持。转换完成后，还会调用 NIR 验证器。

查看实际生成的 NIR：

```powershell
.\.tynel\tsgo\tynel-tsgo-probe.exe -emit nir examples/native/add.ts
```

可以找到 `param`、`add`、`call` 和 `print_number`。这些正是第一篇交给后端的那类操作，现在由前端自动产生。数字仍用 NIR 规定的位串保存，函数的文件位置则来自实际源码。

这也给了我们一个排错顺序：源码对、facts 错，就看提取层；facts 对、NIR 错，就看 Lower；两者都对，再往后端追。

## 改成两个会打印的参数

加法结果正确后，再检查调用顺序。把下面这个变体保存到项目内的 `.tynel/notes/order.ts`：

```powershell
New-Item -ItemType Directory -Force .tynel/notes | Out-Null
@'
function mark(x: number): number {
  console.log(x);
  return x;
}
function sum(a: number, b: number): number {
  return a + b;
}
console.log(sum(mark(1), mark(2)));
'@ | Set-Content -Encoding utf8 .tynel/notes/order.ts

pwsh -NoProfile -File scripts/compile-ts.ps1 `
  -InputTS .tynel/notes/order.ts -Output dist/order.exe
.\dist\order.exe
```

输出应该依次是：

```text
1
2
3
```

前两行来自 `mark`，最后一行才是求和。这里打印就是可以观察的副作用；只检查最后的 `3`，会漏掉调用次序颠倒的问题。

Lower 会先把两次调用结果存成临时值，再调用 `sum`。省略类型和位置以后，顺序是：

```text
t1 = call mark(1)
t2 = call mark(2)
t3 = call sum(t1, t2)
print t3
```

这样，拆开表达式的过程就把求值顺序一并保留了。原生与 Node 的对应实验在 [smoke-ts.py](../../scripts/smoke-ts.py) 中；数值舍入等专门规则另见 [ADR-015](../adr/015-scalar-source-to-native.md)。

## 留下一份能重跑的中间结果

构建脚本会保存生成的 NIR，并把位置写进 exe 旁的记录文件。固定这份输入，就能绕过前端重跑后端：

```powershell
$build = Get-Content -Raw dist/ts-add.exe.tynel.json | ConvertFrom-Json
pwsh -NoProfile -File scripts/compile-ir.ps1 `
  -InputIR $build.nir -Output dist/replayed.exe
.\dist\replayed.exe
```

这里重新运行的是最初的加法，输出仍为 `42`。它说明前后端的交接物已经可以保存、检查和单独重放。

这个阶段接受普通数值函数、`const`、加减乘除、直接调用、返回和受限数字输出。遇到尚未实现的 `%` 等操作，会在构建时报告错误。失败构建保留旧的成功 exe，所以运行前要确认这次构建成功，不能只看文件是否存在。完整结果见[阶段验证记录](../replay-validation.md)。

我们已经把“源码里的加法”接到了“机器执行的加法”。下一步给它加一个条件：只有 `x > 0` 时才返回 `x + 2`，否则返回 `0`。届时要保存的，就不只是运算顺序，还有程序选择的路径。
