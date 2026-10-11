# ADR-014：固定 tsgo 基线与独立的同进程 facade 探针

状态：采纳，用于 T01/T04 的阶段性实验。日期：2026-10-10。规格依据：SPEC 第 5、6、15、16 节。本阶段同时包含固定对象 GC/ABI 验证；范围限于研究原型。

## 选择的基线

Go 后端继续使用 `go1.26.7`。已从上游 tag 解析到 commit `e3336a22ad3f0a90bd252c95d8b5544e02674205`，并将该 commit 的 `src/cmd/compile/internal/gc/main.go` 与私有 SDK 接缝文件比较，SHA256 一致。尚未比较整个 SDK 源码树，也没有从这个 commit 自举完整工具链；版本号加单文件哈希不能代替这项工作。

TypeScript 采用 release `v7.0.2`，commit `1e4744d68260a7cb91b62b12edc3f6a2187faaf1`。仓库是 `microsoft/TypeScript`，本版本的 Go module 在 `tsc/`，声明的 module path 仍为 `github.com/microsoft/typescript-go`，要求 Go 1.26。查询到的 main 已要求 Go 1.27，因此本轮不跟随 main，不改动已验证的 Go 后端。

`toolchain.lock.json` 分别记录仓库、tag、commit、module path、下载归档哈希、选择性源码树哈希、go.mod/go.sum 哈希和声明库哈希。仓库地址与 module path 不是同一个字段，不能根据仓库名称猜 import 路径。

## 本轮的同进程接入证明

`bridge/tsgo/session.go` 在本仓库保存，通过 Go source overlay 作为固定上游模块中的 `tynelapi/session.go` 编译。公开包路径为 `github.com/microsoft/typescript-go/tynelapi`，从该包访问同一上游包树内的 `internal/*` 符合 Go 的导入规则。

`experiments/tsgo-host` 是另一个 module，只导入公开 facade。其原生可执行文件在同一进程中调用 tsgo parser/checker，不通过 Node、JS API 或 RPC 传递检查请求。负向测试让这个外部 module 直接导入 `.../internal/ast`，必须得到 `use of internal package` 拒绝。

这证明了“外部 Go 程序 → 公开 facade → tsgo 内部实现”的合法组合。它没有证明 facade 已被链接进 fork 后的 Go compiler 主进程，也没有建立 TS→NIR→原生的完整路径。后续仍需设计两个上游模块与 compiler fork 的统一构建入口。

没有改写上游原始源码；包发现所需的 `tynelapi` 目录为空，新增源码来自 overlay。上游归档中的许可证和 NOTICE 随选取的源码保留。本轮不向上游创建 PR，也不替用户选择 Tynel 的发布许可证。

## 会话和输出契约

探针只处理一份 UTF-8 文本，最多 64 KiB，逻辑路径固定 `/tynel/probe.ts`。使用上游内存测试文件系统 `vfstest` 隔离输入；这是研究夹具宿主，不是未来 SourceManager 或完整模块解析实现。

设置 `strict`、`noEmit`、`skipLibCheck`，使用固定上游附带的 ES5 声明库。标准声明告诉检查器有哪些类型，但不会为 Tynel 提供相同 API 的原生实现。检查器进行真实语法、全局和语义诊断；相对导入无法从单文件快照找到目标时如实报错，不读取工程中其他磁盘文件兜底。

`Session` 的互斥锁串行化查询和 Close；每次取得上游 checker 时使用其 release 回调释放本次独占使用权。Close 丢弃本会话持有的引用，后续调用返回 `TY1101`，不声称清空上游所有全局缓存或立即释放全部内存。

公开值只有 Tynel 自己的 `Snapshot`、`Diagnostic`、`Position` 和 `TypeFact`。诊断数组复制后返回，类型摘要携带源文本 SHA256；没有 AST、Type、Symbol 指针和可调用远程句柄。关闭会话后已有摘要仍然可以序列化。

`TypeFact.Display` 是上游打印的研究用类型描述，`NodeKind` 也是显示信息。它们不是正式的类型 ID、存储布局或可直接驱动后端的稳定协议。下一阶段应加入结构化 TypeFacts、声明来源和显式种类，不能通过解析显示字符串实现类型降低。

位置同时记录零起始 UTF-8 字节偏移，以及一起始行号和 UTF-16 列号。字符边界验证和中文、emoji、CRLF 用例用于捕获偏移单位混淆；这不等于完成文件编码、增量编辑与多文件源映射。

## 来源验证与可复现边界

`prepare-tsgo.py` 固定下载 URL 中的 commit，并验证归档 SHA256。只提取 compiler 子树、声明源以及根目录许可证/说明，排除大型测试语料及非必要工具目录。当前选取 5,262 个文件，其中 bundled declarations 为 108 个文件。

源码树摘要由排序后的 `路径 + NUL + 文件SHA256 + LF` 计算；不会仅靠文件时间。已有树发生改动、多出或缺少文件时，准备工具拒绝，不静默覆盖。首次提取先落到独立暂存目录，验证后再移动到约定路径。归档哈希是版本内容校验，不是独立的发行签名或供应链安全认证。

实际空目录重建遇到 Windows 发布目录的短暂访问/共享错误后，准备工具增加最多八次的有界重试。目标已经存在时不覆盖；校验失败、其他错误或持续占用继续中止。该重试只处理文件系统发布阶段，不改变内容校验或降低检查强度；对应恢复与拒绝用例纳入测试。

构建脚本在构建前后检查选取的上游树和本项目 adapter/host/锁文件，使用 `-mod=readonly` 避免构建时改写依赖约束，记录二进制和输入指纹。Go 依赖保留 host 的 go.mod/go.sum；并未把所有依赖、SDK 和平台链接条件打包成完全离线的发行工具链。

## 验收与后续

独立 facade 测试包括真实类型查询、作用域遮蔽、声明与流收窄的区别、TS2322、语法错误、未解析导入、Unicode 坐标、关闭后拒绝、跨会话隔离、并发查询串行化、公开 DTO 检查和 external internal-import 负向构建。该并发测试未启用 race detector，不算内存竞争的完整证明。

`test.ps1 -WithFrontend` 把前端验证加入已有 NIR、GC、ABI 回归；默认后端回归会明确提示未运行前端探针。实际通过情况见 [T01/T04 进展](../progress-t01-t04.md)。

下一步是稳定 TypeFacts 与 NativeChecker 的最小契约，让真实加法 AST 产生现有 NIR，再用已验证后端处理；同时保留 Go SDK 全树来源、闭包存活和跨包元数据等未完成工作。T01/T04 的这个实验不代表整个 M0/M1 退出。

## 阅读依据

- [固定版本 Go module](https://github.com/microsoft/TypeScript/blob/1e4744d68260a7cb91b62b12edc3f6a2187faaf1/tsc/go.mod)
- [固定版本 compiler host](https://github.com/microsoft/TypeScript/blob/1e4744d68260a7cb91b62b12edc3f6a2187faaf1/tsc/internal/compiler/host.go)
- [固定版本 checker pool](https://github.com/microsoft/TypeScript/blob/1e4744d68260a7cb91b62b12edc3f6a2187faaf1/tsc/internal/compiler/checkerpool.go)
- [Go toolchain selection](https://go.dev/doc/toolchain)
- [Go command / internal directories / overlay](https://pkg.go.dev/cmd/go)
