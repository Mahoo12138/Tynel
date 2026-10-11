# stage/05-short-circuit 验证记录

执行日期：2026-10-11。环境：Windows amd64、Go 1.26.7、锁定 TypeScript 7.0.2、PowerShell 7、Python；Node 用作源码行为对照。Go/tsgo 上游提交保持前一阶段的版本。

## 完整回归

```powershell
pwsh -NoLogo -NoProfile -NonInteractive -File scripts/replay.ps1 -WithFrontend
```

本轮最终执行退出码为 0，末行是 `Replay passed using this checkout and a private SDK.`。执行中重新构建了前端与 Go compiler bridge，以及各组原生程序。

| 检查 | 实际结果 |
|---|---|
| Go 格式、PowerShell 语法、根模块测试、vet | 通过；4 个含测试的根模块包 |
| Go 源码比较工具 / 阶段索引测试 / tsgo 源码准备测试 | 6 / 15 / 16 项通过 |
| 前端 facade/host 与内部包边界 | 123 个测试及子测试通过事件，host vet 通过 |
| 固定 tsgo 选定源码与声明校验 | 5,262 个源码文件、108 个声明文件的摘要匹配 |
| 既有 NIR 原生 smoke | 7 项通过 |
| 固定对象 GC | 普通 20/20、压力 20/20 |
| 辅助库 ABI | 4 项通过 |
| 既有数值源码 | 9 组原生/Node 对照、NIR 回放、3 组错误构建通过 |
| 既有布尔分支源码 | 13 组对照、回放、8 组错误构建、2 组编译器直接拒绝通过 |
| 新短路源码 | 20 组对照、回放、10 组错误构建、5 组编译器直接拒绝通过 |
| binary64 主动 fuzz | 五秒、2 个 worker，480,938 次执行通过 |

短路 20 组程序中有一组包含 32 个嵌套路径组合（4 种树形 × 8 种布尔输入）；同时使用独立布尔树求值器与 Node 比较完整的调用轨迹。32 个组合属于上述一组程序，不额外算作 32 份源码。

## 本阶段关键观察

左右调用、左值只求一次、被跳过的递归函数、右侧的多步数值计算、括号与优先级、多个实参顺序、const 结果复用、表达式丢弃和 NaN 比较均有实际原生/Node 对照。同一个输入路径先跳过右侧、再改变输入执行右侧，输出相应变化。

源码负向测试检查两条拒绝路径：tsgo 的类型/名称错误，以及 Tynel 的未实现语义。`false && 42`、`true || 42` 与未执行侧的取余、库调用仍然拒绝。失败构建不输出半份 NIR，并保留先前成功 exe 和构建旁注的内容。

另将非法 NIR 直接交给 fork 编译器，验证旧 v3、非布尔左值、非布尔右值、RHS 中 return、RHS 局部值向外泄露都以 TY4001 拒绝，独立输出目录中未产生对应 exe。

首轮专项测试中，一个返回值声明为 boolean 的数字 `&&` 示例先收到 TS2322，而测试曾错误期待 TY1201。修正期望后，另加入类型检查可通过、由 Tynel 拒绝的数字左值示例。没有跳过 tsgo 诊断或放宽操作数检查；最终完整回归包含两者并通过。

## 笔记实例与产物

`dist/short-circuit.exe` 的实际输出：

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

`dist/short-circuit-order.exe` 输出 `1`、`true`、`1`、`3`、`false` 五行。保存的 NIR 独立回放保持相同短路输出。反汇编中的 `main.f1` 带有 `examples/native/short-circuit.ts` 来源，左侧 mark 调用后才检查布尔结果并决定是否执行右侧 mark。

本机详细结果保存在 `.tynel/short-circuit-smoke/result.json`、`.tynel/short-circuit-smoke/short-circuit.asm.txt`、`.tynel/tsgo/tests.jsonl` 及各 exe 旁的构建记录。源码、笔记和本摘要纳入阶段提交；SDK、缓存和二进制不进入 Git。

上述结果限定于本次锁定环境。123 个事件包含子测试，fuzz 不是穷尽证明；没有验证其他平台、完整 JavaScript 操作数类型或空机器离线安装。
