# 固定对象 GC 与辅助 ABI 验证

这组验证属于 `stage/02-tsgo-query` 的后端基础工作，随第二篇笔记的实现保存。概念说明见 [查询表](reference/concepts.md)，设计见 [ADR-013](adr/013-spec-authority-and-m1-probes.md)。

NIR v2 通过显式 `probe.gc/1` 开放固定对象探针。`Node` 的声明来自手写辅助包，包含 `Value float64` 和 `Next *Node`。分配、字段读写、局部绑定、引用参数和返回由 bridge 构造 Go 高层 IR；这不代表 TypeScript 的通用对象模型已经实现。

探针验证返回的引用、父子对象、别名修改，以及显式 GC 和深栈中的 GC。子程序输出必须依次为 `42`、`7`、`43`，引用身份必须一致。反汇编检查分配与写屏障调用；存在这些指令不等于每次运行都触发了所有并发 GC 交错。

ABI 验证包括：普通数值程序保留辅助库锚点；正确包归档直接链接并运行；替换旧辅助归档被链接器拒绝；错误字段布局被 bridge 以 `TY4002` 拒绝。它验证固定组合，不提供任意 Go 版本、对象布局或恶意二进制的兼容保证。

```powershell
pwsh -NoProfile -File scripts/build-bridge.ps1
pwsh -NoProfile -File scripts/smoke-gc.ps1
pwsh -NoProfile -File scripts/smoke-abi.ps1
```

实际运行次数与结果保存在该阶段的 [验证记录](replay-validation.md)。GC 与 ABI 是辅助验证，不另建学习阶段。
