---
chapter: 2
experiment_tag: stage/02-tsgo-query
---

# Tynel 学习笔记（二）：查出 answer 的类型

上一篇把手写 NIR 编译成了输出 `42` 的程序。现在让输入往前走一步：直接读取 TypeScript，问检查器最后那个 `answer` 是什么类型。

[examples/frontend/add.ts](../../examples/frontend/add.ts) 写的是这个小程序：

```ts
function add(a: number, b: number): number {
  return a + b;
}
const answer: number = add(40, 2);
answer;
```

`answer` 接住一次函数调用。读取源码以后，工具得找到这个名字对应的声明，并取得检查器里的类型信息。我们复用 TypeScript 的 Go 实现 tsgo 来完成这项工作。

## 从一次查询开始

按[实验指南](../reproducing.md)检出本篇标签，在该目录准备前端与后端。SDK 路径换成本机的 Go 1.26.7 根目录；需要代理时使用进程内的 `-Proxy` 参数。

```powershell
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7 `
  -WithFrontend -Proxy http://127.0.0.1:7890
```

先检查文件：

```powershell
$probe = Join-Path $PWD '.tynel/tsgo/tynel-tsgo-probe.exe'
$check = & $probe examples/frontend/add.ts | ConvertFrom-Json
$check.diagnostics.Count
```

输出 `0`，表示没有发现类型错误。再问最后一个 `answer` 的类型：

```powershell
$text = [IO.File]::ReadAllText((Join-Path $PWD 'examples/frontend/add.ts'))
$index = $text.LastIndexOf('answer;')
$offset = [Text.Encoding]::UTF8.GetByteCount($text.Substring(0, $index))
$result = & $probe -offset $offset examples/frontend/add.ts | ConvertFrom-Json
$result.type.display
```

输出：

```text
number
```

这里的 `offset` 是从零开始的 UTF-8 字节位置。命令先找到目标词，再把它前面的文本换算成字节数，因此前面加入中文注释也能正确定位。

我们已经从自己的工具中取得了检查器的答案。接下来看看程序怎样接上这个查询。

## 用一层小接口接入 tsgo

tsgo 的检查器放在 `internal` 包中，普通外部项目无法直接导入。Tynel 在上游允许访问这些包的位置加入 `tynelapi`，把需要的操作封装起来。外部宿主只调用这个公开接口：

```text
命令行宿主 → tynelapi → tsgo 检查器
```

这层接口叫 facade，在这里可以直接理解为“给检查器做的一个小入口”。源码集中在 [bridge/tsgo/session.go](../../bridge/tsgo/session.go)。构建脚本把它放入固定上游的包树，再与命令行宿主一起编译成刚才运行的 exe。

检查一份文件时，facade 先用输入文本建立程序，再收集诊断。收到位置查询后，它找到该位置的语法节点，请检查器判断类型，最后把结果整理成 JSON。宿主的处理过程在 [main.go](../../experiments/tsgo-host/main.go) 中。

上游内部接口与版本有关，所以这里使用锁文件指定的 TypeScript v7.0.2 和配套声明文件。我们的实验宿主只提供当前这份源码，足够先研究一次检查和查询。

## 答案属于哪一份源码

查询不是只传一个变量名。把 `answer` 的初始化改掉，同一个名字就可能代表另一种类型。因此，facade 把源码文本和检查状态放在一个会话里；一次会话只回答这份输入的问题。

它的使用顺序可以写成下面这样。省略错误处理，只看数据怎样流动：

```go
session, err := tynelapi.New(source)
snapshot, err := session.Snapshot()
fact, err := session.TypeAt(offset)
session.Close()
```

`snapshot` 保存诊断和源码身份，`fact` 保存该位置的类型摘要。它们是复制出来的普通数据，关闭会话后仍可打印或保存。上游的语法树与检查器对象留在接口内部。

这样安排以后，后续编译阶段可以读取一份稳定结果，不必一直持有正在工作的检查器。会话关闭、结果独立性与不同源码之间的隔离，都有[宿主测试](../../experiments/tsgo-host/main_test.go)覆盖。

## 用一个错误检验这条路径

仅仅返回 `number` 还不够，我们还要确认它真的在检查代码。仓库里的错误例子把字符串赋给了数字变量：

```powershell
& $probe examples/frontend/type-error.ts
$LASTEXITCODE
```

诊断中应出现 `TS2322`，退出码为 `1`。这与前面没有诊断、正常退出的例子形成对照：同一个入口，能接受合法程序，也能指出类型不匹配。

还可以读 [narrowing.ts](../../examples/frontend/narrowing.ts)。参数允许 `number | undefined`，经过判空以后，返回位置的 `value` 被判断为 `number`。这说明查询位置很重要：检查器会利用程序走到这里时已知的条件。这个现象称为类型收窄，等到处理相应源程序时再继续展开。

## 从查询结果走向程序结构

现在我们可以回答“这里是什么类型”，还需要回答“这里执行什么”。后端要生成 `add`，不仅需要 `number` 这个答案，还需要函数参数、加法操作和调用关系。

因此下一篇会从检查器里提取这些结构，生成 Tynel 自己的数据，再转成 NIR。当前的 `display` 留给人阅读，机器需要的是明确的类型种类和声明引用。

本篇的产物是一个单文件类型查询工具。配套运行时验证与完整测试结果放在[阶段记录](../replay-validation.md)，术语可以按需查[概念表](../reference/concepts.md)。沿着同一个加法例子，我们已经从“手写程序描述”走到了“能读懂源码里的类型”。
