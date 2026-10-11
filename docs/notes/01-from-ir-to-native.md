---
chapter: 1
experiment_tag: stage/01-ir-native
---

# Tynel 学习笔记（一）：把一份加法描述编译成 exe

做 Tynel 的起因很简单：我平时用 TypeScript 写前端、Go 写后端，想让两边共用类型，也想试试用 TypeScript 写能直接运行的原生程序。

先从加法开始。我们要描述一个 `add` 函数，调用 `add(40, 2)`，让生成的程序打印 `42`。这一步先手写程序的结构，把读取 TypeScript 的工作留到后面。这样可以集中解决一个问题：自己的程序描述，怎样进入 Go 编译器？

```text
加法的 NIR 文件 → bridge 转成 Go 内部节点 → Go 后端与链接器 → add.exe
```

## 把加法写成数据

编译器处理程序时，需要在内存里保存“这是一个函数”“这里读取参数”“这里执行加法”这些信息。Tynel 把自己的这份程序结构叫 NIR，中间表示的一种，保存为 JSON 文件。

打开 [examples/add.nir.json](../../examples/add.nir.json)。它描述了两个函数：`add` 接收 `a`、`b`，返回相加结果；`main` 调用 `add(40, 2)` 并打印结果。先看加法这一小段，下面省略了记录文件位置的 `loc` 字段：

```json
{
  "op": "add",
  "type": "number",
  "args": [
    { "op": "param", "type": "number", "name": "a" },
    { "op": "param", "type": "number", "name": "b" }
  ]
}
```

`op` 告诉编译器要做什么，`args` 列出输入。这里的输入是两个参数引用。处理这个节点时，编译器沿着引用找到函数的参数，就知道加法该读取哪两个值。

常量也有自己的节点。文件中 `40` 存为 `"bits": "4044000000000000"`，`2` 存为 `"bits": "4000000000000000"`。这是本项目保存 64 位浮点数的编码；做这次实验只需知道数值与编码的对应关系，稍后会改动其中一个。

## 先得到第一个 42

按[实验指南](../reproducing.md)检出本文标签，在该工作目录执行下面的命令。环境是 Windows amd64、PowerShell 7、Python 3.12、Go 1.26.7；Node 24 用于回归对照。`-SdkSource` 填已有 Go 1.26.7 SDK 的根目录。

```powershell
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7

.\dist\add.exe
```

第一条准备项目内的 SDK，构建修改版编译器并运行阶段回归。第二条运行刚生成的程序：

```text
42
```

准备完成后，单独编译这个输入只需要：

```powershell
pwsh -NoProfile -File scripts/compile-ir.ps1 `
  -InputIR examples/add.nir.json -Output dist/add.exe
```

接下来沿这条命令往里看，弄清楚这个 `42` 是怎样生成的。

## bridge 在哪里接手

程序除了加法，还需要入口和输出功能。项目先提供一个固定的 Go 支持文件，其中有空的 `main`、打印数字的 `tynelPrint`，以及恢复浮点常量的 `tynelF64`。文件在 [experiments/bootstrap/main.go](../../experiments/bootstrap/main.go)。

Go 先处理这个包，建立函数和依赖信息。我们把接入点放在它完成这一步之后：

```go
noder.LoadPackage(flag.Args())
loadTynelIR()
```

第二行是 Tynel 加入的调用。它读取 NIR，将 `add` 加进当前包，再用 NIR 中的内容填充 `main`。接入位置由 [build-bridge.ps1](../../scripts/build-bridge.ps1) 设置，读取入口在 [gc_hook.go](../../bridge/go/gc_hook.go)。

真正的转换在 [bridge/go/load.go](../../bridge/go/load.go) 中。它先为 `add` 建立函数签名：两个 `float64` 参数，一个 `float64` 结果。然后逐个转换函数体里的节点。

遇到上面的加法描述，左右输入分别由 `b.expr` 转换，再组装成 Go 的加法节点。把实际代码中的操作选择固定为加法后，核心动作就是：

```go
ir.NewBinaryExpr(
    pos,
    ir.OADD,
    b.expr(e.Args[0], params),
    b.expr(e.Args[1], params),
)
```

`params` 保存参数名称与 Go 参数节点的对应关系。左边的 `param a` 取出 `a`，右边取出 `b`。外层的 `return` 随后变成 `ir.NewReturnStmt`，将这个加法节点作为返回值。

到这里，Go 后端拿到的已经是它认识的函数与表达式。后面的分析、机器码生成和链接继续沿用 Go 工具链。

还有最后一环：构建命令怎样选中我们的编译器？`compile-ir.ps1` 使用 Go 的 `-toolexec` 选项，把工具调用先交给一个小包装程序。包装程序遇到这个实验包，就启动 `tynel-compile.exe` 并传入 NIR；其他依赖照常构建。对应实现在 [cmd/tynel-toolexec](../../cmd/tynel-toolexec/main.go)。

因此，这次编译中变化的是 NIR 和编译器内存里的函数体。固定的 Go 支持文件一直不变，用户的加法没有被写成另一份 Go 源码。

## 看看 exe 里的加法

用项目内的 Go 工具把 `add` 的机器码显示出来：

```powershell
.\.tynel\sdk\go1.26.7\bin\go.exe tool objdump `
  -s '^main\.add$' dist/add.exe
```

在这个 Windows amd64 实验中，可以看到两条关键指令：

```text
ADDSD X1, X0
RET
```

第一条对寄存器中的两个双精度浮点数做加法，第二条返回调用者。这把前面的 JSON 节点和最终执行的动作接了起来。

输出还会带有 `examples/add.reference.ts` 的位置。它来自手写 NIR 的 `loc` 字段，用于标记来源；本篇的实际编译输入仍是 JSON。

## 改一个输入，再跑一遍

把 `40` 改成 `41`，结果应从 `42` 变成 `43`。我们另存一份输入，保留原例子：

```powershell
New-Item -ItemType Directory -Force .tynel/notes | Out-Null
$ir = Get-Content -Raw examples/add.nir.json | ConvertFrom-Json
$ir.functions[1].body[0].value.args[0].bits = '4044800000000000'
$ir | ConvertTo-Json -Depth 80 | Set-Content -Encoding utf8 .tynel/notes/add-43.nir.json

pwsh -NoProfile -File scripts/compile-ir.ps1 `
  -InputIR .tynel/notes/add-43.nir.json -Output dist/add-43.exe
.\dist\add-43.exe
```

输出：

```text
43
```

这次只修改了输入数据，bridge 和支持文件都没有改动。编译器已经能够根据描述生成不同的程序，而不只是生成一个固定答案。

本阶段留下了可独立运行的 NIR 后端入口，以及构建、反汇编和修改输入这三个观察手段。完整的错误输入、数值编码与缓存测试见[阶段验证记录](../replay-validation.md)，它们不必全部挤进这次加法实验。

下一篇开始读取真实 TypeScript。先让检查器回答：`add(40, 2)` 的结果究竟是什么类型，再让源码自动产生这里手写的程序描述。
