# 类型检查器接入

实验版本：`stage/02-tsgo-query`。学习说明见 [笔记 02](notes/02-tsgo-facade-and-type-facts.md)，接口约定见 [ADR-014](adr/014-tsgo-facade-baseline.md)。

前端采用锁定的 TypeScript v7.0.2 提交 `1e4744d68260a7cb91b62b12edc3f6a2187faaf1`，构建使用 Go 1.26.7。准备脚本校验 5,262 个选定源码文件与 108 个声明文件；不包含全部上游测试语料。

`bridge/tsgo/session.go` 在合法上游包树内构建公开 facade，外部 Go module 调用它。单文件会话提供诊断、位置类型查询与关闭操作；只返回自有数据，不把检查器指针交给后端。关闭后拒绝查询，已取得摘要仍然可读。类型显示字符串仅供观察，不是后端类型协议。

本阶段能观察 `answer` 的 number 类型、TS2322 类型错误、声明处与分支内的类型收窄，以及中文和 emoji 的坐标。它尚不输出 NIR，`-emit` 不受支持。后端仍以手写 NIR 为输入，固定对象 GC/ABI 验证随本阶段保存，详见 [基础验证](progress-t09.md)。

```powershell
pwsh -NoProfile -File scripts/replay.ps1 -WithFrontend
```

阶段回归与独立样例的实际结果见 [验证记录](replay-validation.md)。后续工作是从 AST、声明和调用签名生成自有结构化事实，再连接现有 NIR 后端。
