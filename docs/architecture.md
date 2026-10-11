# Tynel：首轮开发的架构边界

Tynel 是学习型 TypeScript-to-native 编译器。长期主线是 tsgo -> NativeChecker -> Tynel IR -> Go fork 高层 IR -> Go 后端、链接器与运行时。

## 当前切片，不冒充完整编译器

当前先实现 M0/M1 的一部分：手写 JSON IR 进入 fork 编译器。`examples/add.reference.ts` 仅是语义参照，不是编译输入。TypeScript 前端、对象/GC smoke、泛型、Go 包互操作、完整 ABI 检查尚未实现。

`compiler/nir` 独立于上下游 AST，记录语义类型与源位置；浮点常量用 binary64 位串保存负零、无穷和 NaN payload。验证器对白名单以外的节点直接拒绝，限制大小、深度、函数数量和节点预算。

`bridge/go` 通过 Go 的源文件 overlay 放入合法的 `cmd/compile/internal` 包树。它是一组可复现的编译器 fork 修改，不写入宿主 GOROOT。脚本先检查 Go 版本和接入文件 SHA256，再构建独立 `tynel-compile.exe`。

接缝位于 `noder.LoadPackage` 后、中端优化前。M1 暂保留固定、手写的 `experiments/bootstrap/main.go`，让 Go 建立 main 包、fmt/math 依赖和初始化状态。所有输入相关函数体直接由 Tynel IR 构造，绝不生成业务 `.go` 文本。此 scaffold 是需要明确记录、随后替换的实验限制，不是最终语言前端。

初版禁止新合成函数内联，因为它们还没有自己的 Unified IR 导出数据。`tynel-toolexec` 仅替换 bootstrap 包的编译器，其余包由匹配的宿主工具链编译；工具身份包含 IR 内容和 bridge binary hash，防止缓存把不同输入混为一谈。

## 学自 scriptc，但不复制它的 runtime

- 独立 typed IR、源码位置、类型降低边界和验证器。
- 无损浮点序列化、版本拒绝和失败关闭。
- 实现、文档和执行证据分开；测试不同于兼容性宣传。
- 借鉴能力清单与差分测试，不采用 QuickJS、引用计数或运行时 unsupported 陷阱。

参考：
- https://go.dev/src/cmd/compile/README
- https://github.com/vercel-labs/scriptc/blob/9131a3498c877e3a6b8e24a95f1bedf4d031fc5d/packages/compiler/src/ir/serialize.ts
- https://github.com/vercel-labs/scriptc/blob/9131a3498c877e3a6b8e24a95f1bedf4d031fc5d/packages/compiler/src/coverage/report.ts

## 下一阶段

IR->原生、输入变更使缓存失效、非法 IR 被编译器拒绝及特殊浮点 smoke 已实际通过，证据见 [开发记录](progress.md)。使用 `scripts/test.ps1` 可重跑当前完整原型回归。下一步补齐上游 pin、对象分配/GC 与 ABI 身份测试，再接入 tsgo facade 和真实 TS lowering。不要用正则表达式解析 TS，也不要用源码转译替代接缝。
