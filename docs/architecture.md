# Tynel：当前原型的架构边界

本文件是根目录 [SPEC.md](../SPEC.md) 的实现摘要，不替代项目技术规格。

Tynel 是学习型 TypeScript-to-native 编译器。长期主线是 tsgo -> NativeChecker -> Tynel IR -> Go fork 高层 IR -> Go 后端、链接器与运行时。

## 当前切片，不冒充完整编译器

当前先实现 M0/M1 的一部分：手写 JSON IR 进入 fork 编译器。`examples/add.reference.ts` 仅是语义参照，不是编译输入。已增加受 feature 门控的固定对象 GC/ABI 探针，详见 [ADR-013](adr/013-spec-authority-and-m1-probes.md)。tsgo facade 已增加真实 TS→数值事实→NIR，并由脚本连接原生后端；通用对象、泛型、完整 Go 包互操作和完整 ABI 检查尚未实现。

`compiler/nir` 独立于上下游 AST，记录语义类型与源位置；浮点常量用 binary64 位串保存负零、无穷和 NaN payload。验证器对白名单以外的节点直接拒绝，限制大小、深度、函数数量和节点预算。

`bridge/go` 通过 Go 的源文件 overlay 放入合法的 `cmd/compile/internal` 包树。它是一组可复现的编译器 fork 修改，不写入宿主 GOROOT。脚本先检查 Go 版本和接入文件 SHA256，再构建独立 `tynel-compile.exe`。

接缝位于 `noder.LoadPackage` 后、中端优化前。M1 暂保留固定、手写的 `experiments/bootstrap/main.go`，让 Go 建立 main 包、fmt/math 依赖和初始化状态。所有输入相关函数体直接由 Tynel IR 构造，绝不生成业务 `.go` 文本。此 scaffold 是需要明确记录、随后替换的实验限制，不是最终语言前端。

初版禁止新合成函数内联，因为它们还没有自己的 Unified IR 导出数据。`tynel-toolexec` 仅替换 bootstrap 包的编译器，其余包由匹配的宿主工具链编译；工具身份包含 IR 内容和 bridge binary hash，防止缓存把不同输入混为一谈。

## 独立前端接入实验

`bridge/tsgo` 通过 overlay 编译为固定 TypeScript 模块中的公开 `tynelapi` 包，`experiments/tsgo-host` 从外部 module 调用它。宿主与 tsgo 在同一原生进程内运行；只有 facade 导入上游 internal，外部直接导入会被 Go 拒绝。该同进程实验尚未合入 Go compiler fork 的进程。

单文件不可变会话使用上游 ES5 声明、Strict/NoEmit/SkipLibCheck，不自动访问磁盘项目或支持 npm。Display 和 NodeKind 仍是研究显示信息；新增 scalar 投影读取真实类型分类、声明身份与调用签名，返回 `compiler/frontend` 的自有数据。会话关闭后由独立 Lower 验证并生成 NIR。源编译由 `scripts/compile-ts.ps1` 组织，主 CLI 尚未接线。基础会话见 [ADR-014](adr/014-tsgo-facade-baseline.md)，数值切片见 [ADR-015](adr/015-scalar-source-to-native.md)。

## 布尔与结构化控制流

NIR v3 / facts v2 支持 number、boolean 及带独立 Then/Else 的 if。条件只求值一次，分支中的临时值和调用不提升到外层；块与分支复制编译期绑定表，局部声明不向外泄露。sequence 同时计算结构上的全路径返回信息。Go bridge 使用 TBOOL、原始数值比较和 `ir.NewIfStmt`，由 Go 继续生成 CFG/SSA。未引入短路运算、可变绑定或自己的 SSA。

详见 [ADR-016](adr/016-boolean-control-flow.md)；主编译入口仍为脚本，旧协议明确拒绝，历史文章保持其版本范围。

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

IR->原生、输入变更使缓存失效、非法 IR 被编译器拒绝及特殊浮点 smoke 已实际通过，证据见 [开发记录](progress.md)。使用 `scripts/test.ps1` 可重跑当前完整原型回归。固定对象的分配/GC 与辅助 ABI 锚点原型已有独立回归。上游 commit 与独立 tsgo facade 已形成前置探针；数值 TypeFacts/约束与 TS→NIR 已形成受限连接；布尔比较与结构化分支现已接入；下一步研究短路表达式与值的条件选择，并继续补 Go SDK 全树来源、通用前端事实和未覆盖的运行时契约。包括前端的验证使用 `scripts/test.ps1 -WithFrontend`；具体结果见 [本轮进展](progress-t14-t16.md)。不要用正则表达式解析 TS，也不要用源码转译替代接缝。
