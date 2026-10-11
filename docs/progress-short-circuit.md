# Stage 05：布尔短路表达式

本阶段接在 `stage/04-control-flow` 后，增加 boolean `&&`、`||`，保持前四篇文章、已有标签与 SPEC 不变。设计见 [ADR-017](adr/017-boolean-short-circuit.md)，学习实验见[笔记 05](notes/05-short-circuit.md)。

## 实现与范围

事实提取访问 tsgo 的逻辑二元 AST，并对左右操作数逐个检查；不以整个表达式被收窄为 boolean 来推断两个操作数都是 boolean。独立 Lower 将右侧调用与算术放入单独容器，产生绑定结果的 `short_and`/`short_or`。NIR 验证器在自己的类型和作用域上验证两个输入、局部准备步骤及结果，限制递归和节点数量。

Go bridge 保存一次左侧结果，按 `&&` / `||` 的条件选择执行右侧，并写回内部 bool 临时值。没有源码生成、动态引擎、源语言赋值或新运行时辅助函数。Go/tsgo 固定上游与 support ABI 未改变；格式为 facts v3 / `tynel.scalar/3`、NIR v4 和 bridge ABI v4，旧格式拒绝。

新增正向与错误输入单元测试覆盖作用域、递归、序列化和独立数据生命周期。原控制流拒绝项中的 &&/|| 替换为仍未支持的位运算及三元式，其他既有检查保留。短路原生测试检查完整输出顺序与 Node 对照、独立布尔树期望、同路径输入变化、错误构建不覆盖成功产物和编译器直接读取非法 NIR。

## 验证入口

```powershell
pwsh -NoProfile -File scripts/replay.ps1 -WithFrontend
python -X utf8 scripts/smoke-short-circuit.py
```

完整 replay 已实际执行并通过，包含 20 组短路源码对照、10 组错误构建、5 组编译器直接拒绝和既有数值/分支/GC/ABI 回归；具体结果见[本阶段验证记录](replay-validation.md)。测试在 Windows amd64、锁定 Go 1.26.7/TypeScript 7.0.2 上运行；不扩展为其他架构、完整 truthiness、空值合并、三元式、逻辑赋值、循环或通用对象的支持声明。
