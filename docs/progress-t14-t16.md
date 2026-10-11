# 数值 TypeScript 原生编译

实验版本：`stage/03-ts-native`。学习说明见 [笔记 03](notes/03-typescript-to-native.md)，语义边界见 [ADR-015](adr/015-scalar-source-to-native.md)。

真实 AST、符号和类型经 `bridge/tsgo/scalar.go` 投影为 `compiler/frontend` 的自有数据，检查会话关闭后继续降低为 NIR v2。Go bridge 直接构造高层 IR，不生成业务 `.go`，不嵌入 JavaScript 引擎。

目前支持单文件 number 函数、const、四则运算、直接调用、返回和受限数字输出。保留声明身份、左到右求值、负零与显式 binary64 舍入边界。裸表达式只求值后丢弃，不自动打印。未调用函数中的未实现操作也拒绝编译。

```powershell
pwsh -NoProfile -File scripts/compile-ts.ps1 -InputTS examples/native/add.ts -Output dist/ts-add.exe
.\dist\ts-add.exe
```

加法示例预期输出 42。构建记录包含源码、前端、NIR、后端和二进制身份；可以固定保存的 NIR 独立回放。失败构建不输出半份 NIR，不覆盖旧成功程序，因此必须检查退出码。

主 `tynel build` 尚未接线，构建由脚本协调前后端进程。手写 Go bootstrap/support 仍然存在。对象、数组、泛型、模块、闭包、if/循环和赋值未纳入源语言支持。

```powershell
pwsh -NoProfile -File scripts/replay.ps1 -WithFrontend
```

原生/Node 对照、错误构建、GC/ABI、源码准备与其他回归的实际执行结果见 [验证记录](replay-validation.md)。
