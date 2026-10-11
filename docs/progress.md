# 布尔短路与表达式结果

实验版本：`stage/05-short-circuit`。源码范围和预期行为见根目录 replay.json，实际运行结果见 [阶段验证](replay-validation.md)。

- [笔记 01](notes/01-from-ir-to-native.md)
- [笔记 02](notes/02-tsgo-facade-and-type-facts.md)
- [笔记 03](notes/03-typescript-to-native.md)
- [笔记 04](notes/04-control-flow.md)
- [笔记 05](notes/05-short-circuit.md)

本轮实现与未覆盖边界见 [短路记录](progress-short-circuit.md)，设计依据见 [ADR-017](adr/017-boolean-short-circuit.md)。

基础对象与库身份验证见 [GC/ABI](progress-t09.md)，它与类型查询同属第二阶段。

规范以 [SPEC.md](../SPEC.md) 为准；不将局部实验通过解释为全部里程碑完成。
