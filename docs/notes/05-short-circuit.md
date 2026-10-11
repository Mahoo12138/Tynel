---
chapter: 5
experiment_tag: stage/05-short-circuit
---

# Tynel 学习笔记（五）：让右边的函数先等一等

上一回用 `if` 选出要执行的分支。这次把选择写进表达式：`left && check()`。如果 left 已经是假，check 就没有执行的必要，整个表达式可以立即得到 false。

先给函数加上打印，看看这个“跳过”是否真的发生。[short-circuit.ts](../../examples/native/short-circuit.ts) 中的 mark 打印自己的编号，再返回一个布尔值：

```ts
function mark(id: number, value: boolean): boolean {
  console.log(id);
  return value;
}

function both(left: boolean): boolean {
  return mark(1, left) && mark(2, true);
}

function either(left: boolean): boolean {
  return mark(3, left) || mark(4, false);
}

console.log(both(false));
console.log(both(true));
console.log(either(true));
console.log(either(false));
```

## 先看哪些编号出现

按[实验指南](../reproducing.md)检出本篇标签，在项目根目录准备工具链。SDK 路径与代理按本机环境设置：

```powershell
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7 `
  -WithFrontend -Proxy http://127.0.0.1:7890

pwsh -NoProfile -File scripts/compile-ts.ps1 `
  -InputTS examples/native/short-circuit.ts -Output dist/short-circuit.exe
.\dist\short-circuit.exe
```

预期输出十行：

```text
1
false
1
2
true
3
true
3
4
false
```

前两行属于 `both(false)`：只出现编号 1，然后返回 false。接下来的 `both(true)` 才会打印 1、2，并返回 true。

`||` 的选择恰好相反。`either(true)` 得到左侧的 true 后结束；`either(false)` 继续执行编号 4，采用右侧的 false 作为结果。

这种根据左值决定是否继续求右值的行为，就是短路求值。这里两个操作数都是 boolean，因此结果也是 boolean。相关的求值顺序来自 [ECMAScript 的逻辑运算规则](https://tc39.es/ecma262/multipage/ecmascript-language-expressions.html#sec-binary-logical-operators)。

## 原来的表达式展开方式需要停一下

第三篇处理 `sum(mark(1), mark(2))` 时，会先按顺序计算两个实参，再调用 sum。把这套做法直接用到 `&&`，就会提前执行右边：

```text
错误的展开：
  a = mark(1, left)
  b = mark(2, true)
  result = a && b
```

即使 a 为 false，编号 2 也已经打印了。等到最后一行再决定采用哪个结果，已经来不及。

因此，[降低器](../../compiler/frontend/short_circuit.go)在看到 `and` 或 `or` 时，先离开普通的双操作数处理流程。左侧仍写入当前语句列表，右侧则写入自己的列表。将 both 中的 NIR 省略编号和位置后，可以这样读：

```text
leftValue = call mark(1, left)
short_and result
  condition: leftValue
  body:
    rightValue = call mark(2, true)
  value: rightValue
return result
```

`body` 收起右侧需要做的工作，`value` 指定做完以后从哪里取得答案。只有需要右侧时，才会执行这段 body。这也能处理更长的右侧：调用、比较以及里面嵌套的另一个 `&&`，都留在这个范围内。

这里多出来的 result 很重要。`if` 可以只控制某几条语句，而 `&&` 还要把选中的值交给 return、const 或下一个函数调用。

## 把选中的值交回后续代码

[Go bridge](../../bridge/go/short_circuit.go)为这个结果建立一个内部 bool 临时变量。对 `&&`，生成的控制关系相当于：

```text
result = 左侧的值
if result:
    执行右侧准备步骤
    result = 右侧的值
接着使用 result
```

左值先保存一次，既用来判断是否继续，也在跳过右侧时充当最终结果。对于 `||`，只需把判断换成 `if !result`。

```text
              保存左侧到 result
                      │
          &&：左侧是否为 true？
              /               \
            是                 否
            │                  │
      执行右侧并写入 result      保留左值
              \               /
                   使用 result
```

实际代码仍通过上一阶段的 `ir.NewIfStmt` 创建 Go 高层分支。右侧语句列表和最后一次结果写入一起放在分支内。Go 后端随后把它们编译成机器指令。

右侧的临时值只在自己的列表里可见，外层得到的是 result。这样即使右侧没有执行，后面的代码仍有一个已经确定的值可以使用。

## 用括号改变一次执行路径

再沿用 mark，比较下面两条表达式：

```ts
console.log(mark(1, true) || mark(2, false) && mark(3, false));
console.log((mark(1, true) || mark(2, false)) && mark(3, false));
```

第一条中 `&&` 结合得更紧，右侧整体是 `mark(2, false) && mark(3, false)`。编号 1 返回 true，整个右侧都会被跳过。第二条先算括号里的 `||`，得到 true 后还要继续执行外面的 `&&`，因此编号 3 会出现。

可以运行配套的[括号示例](../../examples/native/short-circuit-order.ts)：

```powershell
pwsh -NoProfile -File scripts/compile-ts.ps1 `
  -InputTS examples/native/short-circuit-order.ts -Output dist/short-circuit-order.exe
.\dist\short-circuit-order.exe
```

预期输出：

```text
1
true
1
3
false
```

tsgo 已经把运算的组合方式放进语法树。Tynel 沿树递归，每遇到一个短路表达式就建立自己的右侧列表。于是括号造成的区别自然延续到 NIR，不需要重新扫描源码猜优先级。

## 保存这次执行的中间输入

回到第一个示例，查看它的 NIR：

```powershell
.\.tynel\tsgo\tynel-tsgo-probe.exe -emit nir examples/native/short-circuit.ts
```

找到 `short_and` 和 `short_or`，检查右侧的 call 是否位于各自的 body 内。再使用构建时保存的那份 NIR，单独运行后端：

```powershell
$build = Get-Content -Raw dist/short-circuit.exe.tynel.json | ConvertFrom-Json
pwsh -NoProfile -File scripts/compile-ir.ps1 `
  -InputIR $build.nir -Output dist/short-circuit-replayed.exe
.\dist\short-circuit-replayed.exe
```

输出应与最初的十行一致。这个实验把关键点落到了可以观察的地方：左边打印一次，右边按条件出现，最后返回被选中的值。完整验证范围放在[阶段记录](../replay-validation.md)，输入和检查方法在[专项测试](../../scripts/smoke-short-circuit.py)中。

当前只接受 boolean 的 `&&`、`||`，数字或字符串的隐式真假转换、`??` 和三元表达式留待后续。编译时仍检查两侧的类型和操作能力，即使右侧在某次运行中会被跳过。新节点使用 NIR v4，具体约定见 [ADR-017](../adr/017-boolean-short-circuit.md)。

现在，条件已经能选择一个值并交给后续计算。继续研究 `condition ? a : b` 时，就要把“选左值还是右值”扩展为“选择两段计算之一，并交回同一种类型的结果”。
