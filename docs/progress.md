# 布尔比较与结构化控制流

实验版本：`stage/04-control-flow`。源码范围和预期行为见根目录 replay.json，实际运行结果见 [阶段验证](replay-validation.md)。

- [笔记 01](notes/01-from-ir-to-native.md)
- [笔记 02](notes/02-tsgo-facade-and-type-facts.md)
- [笔记 03](notes/03-typescript-to-native.md)
- [笔记 04](notes/04-control-flow.md)

本轮实现与未覆盖边界见 [控制流记录](progress-control-flow.md)，设计依据见 [ADR-016](adr/016-boolean-control-flow.md)。

基础对象与库身份验证见 [GC/ABI](progress-t09.md)，它与类型查询同属第二阶段。

规范以 [SPEC.md](../SPEC.md) 为准；不将局部实验通过解释为全部里程碑完成。
