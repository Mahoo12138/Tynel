# Tynel 实施路线图

依据此前确认的 v2 计划，将项目名称统一为 Tynel。这里是可执行状态摘要，不是完整计划文件的替代副本。

| 阶段 | 目标 | 本轮边界 |
|---|---|---|
| M0 | 语义冻结、锁定两上游、internal/facade 布局 | 建立本地项目和 Go 接缝指纹；tsgo pin/facade 尚缺 |
| M1 | NIR -> Go 高层 IR -> 原生执行 | 本轮重点；完成度以 progress.md 实测记录为准 |
| M2 | tsgo session、TypeFacts、真实 TS lowering | 待开发；不使用正则解析冒充 |
| M3 | number/string/nullish 语义与差分测试 | 无损浮点 IR 先做；完整运算规则后续补齐 |
| M4 | 对象、数组、别名和 GC | 待开发，禁止把类型视图偷偷复制 |
| M5 | 泛型、联合、结构视图、闭包 | 待开发，GC 指针布局优先 |
| M6 | 模块、可达性、能力清单、导出 | 待开发，不输出 unsupported runtime traps |
| M7 | Go 库绑定、ABI 和缓存 | 最小构建身份先做；正式互操作未完成 |
| M8 | 并发、取消与生命周期 | 待开发，不把 goroutine 宣称为 JS Promise |
| M9 | 共享类型的书签 HTTP 纵向示例 | 待开发，以真实客户端故障测试验收 |
| M10 | 可解释性、性能、可复现工具链 | 待开发，先正确再测量优化 |

首批继续任务：完成 Go upstream 的精确 commit/source pin；接缝 GC smoke；链接时 ABI 防混用；tsgo 合法包树内 facade。原计划 T09 包含 GC 与身份锚点，不能因为 add 可执行就把整个 M1 标记完成。
