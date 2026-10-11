# Tynel 实施路线图

以根目录 [SPEC.md](../SPEC.md) 为项目技术规格。这里是阶段摘要；任务 ID、前置依赖和验收见 SPEC 第 18—20 节，实际执行证据见 [开发记录](progress.md)。M0/M1 仍有前置缺口；当前也已连接 M2 的受限数值切片，不能把单个切片完成视为整个阶段退出。

| 阶段 | 目标 | 本轮边界 |
|---|---|---|
| M0 | 语义冻结、锁定两上游、internal/facade 布局 | Go/tsgo commit 已记录，tsgo 源码/声明指纹和独立 facade 已落地；Go 全树来源与完整干净重建未完成 |
| M1 | NIR -> Go 高层 IR -> 原生执行 | 标量闭环和固定对象 GC/ABI 探针已实现；完整阶段仍未退出，见 progress-t09.md |
| M2 | tsgo session、TypeFacts、真实 TS lowering | number/boolean、比较、if/else、局部作用域和返回路径已接通 NIR v3→exe；通用 TypeFacts、短路运算、循环、模块与完整诊断未完成 |
| M3 | number/string/nullish 语义与差分测试 | 无损浮点 IR 先做；完整运算规则后续补齐 |
| M4 | 对象、数组、别名和 GC | 待开发，禁止把类型视图偷偷复制 |
| M5 | 泛型、联合、结构视图、闭包 | 待开发，GC 指针布局优先 |
| M6 | 模块、可达性、能力清单、导出 | 待开发，不输出 unsupported runtime traps |
| M7 | Go 库绑定、ABI 和缓存 | 最小构建身份先做；正式互操作未完成 |
| M8 | 并发、取消与生命周期 | 待开发，不把 goroutine 宣称为 JS Promise |
| M9 | 共享类型的书签 HTTP 纵向示例 | 待开发，以真实客户端故障测试验收 |
| M10 | 可解释性、性能、可复现工具链 | 待开发，先正确再测量优化 |

T01/T04 本轮结果见 [前端基线记录](progress-t01-t04.md)。T14/T16 的数值连接见 [源编译记录](progress-t14-t16.md)。继续完成 Go SDK 全树来源/干净重建、通用 TypeFacts，以及短路表达式；布尔比较与分支的本轮验证见 [阶段记录](progress-control-flow.md)；闭包存活、类型元数据和完整 ABI 组合仍有未覆盖边界。T09 的固定对象 GC 与身份锚点原型见 [独立记录](progress-t09.md)，不能由它推导整个 M0/M1 已完成。
