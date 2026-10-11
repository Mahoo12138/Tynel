# ADR-015：受限数值源码到原生程序的首个连接

状态：采纳，研究切片。日期：2026-10-10。依据 SPEC 第 6—9、12、15—16 节及 T14/T16。在类型查询和固定对象验证的基础上连接数值源码与原生后端。

## 目标与保留的边界

连接已经分别验证过的 tsgo facade 和 Go compiler bridge。源文件经真实 AST、符号和类型查询，变为 Tynel 拥有的数据，再降低到 NIR v2。不得解析 `TypeToString`、重新用正则解析 TS，或者产生输入相关的业务 Go 源文件。

前端宿主与 tsgo 同进程；构建驱动调用前端原生进程，再调用现有 Go fork/链接流程。此轮不把两个上游合成一个编译器进程，也不移除固定手写的 Go main/support 骨架。继续将这两个限制写明，不用“直接编译”掩盖它们。

M0 的完整 SDK 来源、干净机器重建与 M1 的闭包/跨包元数据仍有缺口。本次是在已经运行过的受控组合上验证 T14/T16 的一个子集，不将所有前置里程碑改为完成。

## scalar/1 的接受范围

`tynel.scalar/1` 是本轮明确的小范围契约，不是普通 TypeScript 的全量支持标志。

- 单个不超过 64 KiB 的 UTF-8 `.ts` 快照，固定 ES5 声明环境，无磁盘模块解析。
- 普通顶层函数；参数必须是必填、显式 `number` 标注的标识符；返回类型显式为 `number`。
- 顶层或函数体内的 `const` 数值绑定，可省略局部类型标注。用实际 checker 的 Number/NumberLiteral 分类，不看名称显示文本。
- 数字字面量、括号、数字一元正负、`+ - * /`、局部或参数引用、直接函数调用、return。
- 表达式语句求值后丢弃，不自动打印。输出仅接受绑定到专用声明来源的 `console.log(number)`，且只允许一个参数。

暂不接受 let/var/赋值、if/循环、类型断言、对象/数组、闭包/嵌套函数、全局捕获、函数值、泛型、默认/可选/rest 参数、导入导出、只在声明库中存在的原生操作。即使函数未被调用也检查其函数体；无可达性分析的豁免。

这是有明确原生限制的子集，拒绝 let 不表示 let 是“动态语言能力”。它表示本切片暂时没有定义可变绑定的控制流处理。后续扩大范围需要新实现和证据。

## 结构化事实和符号身份

新增 `compiler/frontend`，包含 Program、Function、Binding、Expression、Statement 及 Span。结构只用自有数据字段；不包含上游 AST/Type/Symbol 指针、会话句柄或显示字符串协议。

`bridge/tsgo/scalar.go` 位于上游合法包树内，通过 checker 查询并投影。函数用顺序分配的 `fN` ID，参数和 const 用 `vN` ID。ID 在一个源码快照内标识声明，不是跨构建永久类型 ID，也不暴露上游地址。

引用按 `GetSymbolAtLocation` 的声明身份匹配，直接调用还核对 `GetResolvedSignature` 的声明和参数数。两个函数里的同名参数得到不同 ID；用户名称只用于说明和来源位置，不直接变成 Go 链接符号。

外部 host 在取得完整事实后关闭会话，再调用 `frontend.Lower`。Lower 单独验证身份、ID、操作种类、参数个数、类型、源范围、定义顺序和节点预算，最后调用 NIR verifier。自有事实能够 JSON round-trip；主回放边界仍是已验证的 NIR 文件。

类型检查、受限 AST 投影与原生约束在此轮有组合实现，还不是 SPEC 中独立、通用的 TypeFacts / NativeChecker / ReprPlanner 子系统。

## 作用域、求值次序和浮点

函数体只能引用当前参数和此前定义的局部 const，顶层绑定只能在顶层顺序使用。捕获顶层 const 也拒绝，避免把需要运行时存储的全局变量错误塞进 main 的局部。

降低把调用和算术逐步保存到临时绑定，按左到右建立求值顺序。副作用来自受限数字输出时，应保留例如 `sum(mark(1), mark(2))` 的 1、2、3 输出顺序。普通表达式语句保留求值，但不引入输出。

数字字面量从 tsgo 的数值字面量类型取得 jsnum.Number，再保存 binary64 位串，不自行解析 TypeToString 或把字面量当 Go 精确常量。负号降低成与 -1 的乘法，这对 binary64 数值的符号转换是精确的，也保留负零；不对运算的 NaN payload/sign 作承诺。

Go bridge 对数值算术结果增加显式 float64 到 float64 转换，使用锁定后端提供的 Round64F 语义边界，避免只靠临时变量阻止跨语句乘加融合。回归包含会区分独立舍入与 FMA 的输入；其他架构和优化模式仍需后续审计，不因此宣称 M3 全部完成。

## 输出能力与诊断

`New` 保留原有查询环境。`NewScalar` 额外引入固定 `native-print.d.ts`，只声明一个数字输出签名。识别输出时同时检查名称与声明文件/调用签名来源，不把用户定义的同名值当作运行时内建。

实际输出仍使用手写的 `tynelPrint` / Go fmt；例如 Infinity 的文本与 Node 不同。该受限能力用于学习观察，不是完整 JS console 或 Node 兼容层。

源语言错误保留 TS 编号。受限投影使用 TY1201；独立事实/降低约束使用 TY1202；之后由 NIR 验证器给出 TY4001。前端失败时 stdout 不写部分 NIR。常规类型或能力错误不是 panic。

## 驱动、产物和缓存

入口为 `scripts/compile-ts.ps1`。它读取一次输入快照，在独立 `.tynel/ts-build/<id>/` 中保存源码、facts、NIR 和构建记录；先生成临时二进制，成功并核对工具指纹后才发布到 dist/ 或 .tynel/ 中的目标 `.exe`。失败保留旧成功产物，调用方必须检查非零退出，不能把磁盘上的旧文件当作本次成功。

facts/NIR 两次前端进程读取的是同一份私有快照，而不是再次打开用户正在编辑的源文件。此目录不是防恶意并发修改的沙箱。构建记录包含输入、前端、Go bridge、NIR、二进制和辅助 ABI 身份。二进制与旁注不是跨文件事务；读取旁注时应核对其中的二进制哈希。

前端指纹覆盖 root go.mod、锁文件、facade/host、自有 frontend/NIR 及相关构建脚本。后端仍使用既有 NIR 内容与 compiler hash 缓存身份。不以路径或时间戳代替内容。

`cmd/tynel` 的 TS 命令仍未接线，诊断指向本轮脚本；不增加一个只会假装成功的 build 子命令。

## 验证与后续

单元/集成测试包括：符号分离、关闭会话后 lowering、facts/NIR round-trip、Unicode 位置、数值格式、前向函数调用、未调用函数限制、类型断言/错误签名/对象等拒绝、核心包依赖方向。端到端比较 native 与 Node，覆盖求值顺序、负零、NaN、独立舍入、缓存变化、NIR 回放和失败产物保护。

实际执行结果单列于 [本轮记录](../progress-t14-t16.md)，不以本 ADR 的测试设计充当通过证据。后续依次研究比较与布尔类型、结构化分支、变量赋值/作用域，再扩展循环与模块；完整对象与 GC 能力仍沿用 SPEC 的独立验收。

阅读依据：[Go 编译器说明](https://go.dev/src/cmd/compile/README)、[Go 浮点运算规范](https://go.dev/ref/spec#Floating_point_operators)、固定 SDK 的 `ssagen/ssa.go` 中 `TFLOAT64→TFLOAT64` 转换，以及固定 tsgo 的 checker/AST API。
