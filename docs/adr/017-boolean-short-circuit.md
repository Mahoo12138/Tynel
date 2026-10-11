# ADR-017：布尔短路表达式与局部结果绑定

状态：采纳。对应 `stage/05-short-circuit`，扩展 SPEC 第 8 节的结构化 IR 与 T14/T16 的数值/布尔切片。

## 语义范围

本阶段接受两个操作数均为 boolean 的 `&&`、`||`。操作数可以是字面量、在作用域内的引用、比较、调用或嵌套逻辑表达式。结果为 boolean，可以用于 const、参数、return、if、比较、取反、输出与表达式丢弃。

左侧恰好求值一次。`&&` 在左侧为 false 时直接返回左值，`||` 在左侧为 true 时直接返回左值；其他情况求右侧并返回右值。求值顺序从左到右，括号与优先级由 tsgo 的 AST 保留。普通 ECMAScript 还允许非布尔操作数并返回被选中的操作数值；Tynel 这里仅实现其布尔子集。[ECMAScript Binary Logical Operators](https://tc39.es/ecma262/multipage/ecmascript-language-expressions.html#sec-binary-logical-operators)

检查器须检查两个操作数，不能只看整个表达式的类型。比如 `false && 42` 的结果可能已被检查器收窄为 false，仍须拒绝右侧 number。没有 truthiness、`??`、`?:`、`&&=`、`||=`、位运算或任意 JS 值表示。本阶段不引入源语言可变变量。

## 自有事实与 NIR

facts v3 / `tynel.scalar/3` 增加二元表达式 `and` 与 `or`。提取仍访问真实类型/符号信息，关闭会话后由独立 Lower 处理。无论左侧是否常量，右侧仍经过类型与能力检查；只在运行时跳过执行，不把静态未知操作视为可用。

NIR v4 增加 `short_and`、`short_or` 语句：

- `name`：整个表达式的结果绑定，类型固定为 boolean。
- `condition`：左侧表达式，只求值一次。
- `body`：需要求右侧时执行的准备步骤，仅允许 `let` 和嵌套短路语句。
- `value`：在 body 结束后读取的右侧 boolean 结果。

右侧拥有外层绑定表的副本。准备步骤可以依次引用先前的 RHS 临时值；这些局部值不进入外层。结果绑定在整条短路语句校验成功后才进入外层，初始化不能引用尚未建立的结果。现有重复声明、来源范围、深度和总节点预算继续生效。禁止 RHS 中的 return、任意 if/block、独立打印语句与 probe 控制语句；带副作用的函数调用可以通过 `let` 求值。

独立 NIR 验证器检查左右结果类型、绑定可见范围、多余字段、非法操作和递归预算。序列化使用现有字段，因此文件结构虽有复用，语义仍升级版本；v3 输入明确拒绝。`semantics` 的既有标量规则没有改变。

## Go bridge

Lower 先将左侧写入当前语句容器，再将右侧写入独立容器。禁止把右侧调用和算术预计算放到父容器中。嵌套短路也遵守相同规则。

Go bridge 用一个内部 bool 临时变量保存左侧；`short_and` 在该变量为 true 时执行右侧并赋入结果，`short_or` 在其为 false 时执行右侧。使用 `ir.NewIfStmt`，结果合并由 Go 后续 SSA 处理。当前选择显式语句块，避免依赖 Go 逻辑表达式 Init 列表的额外约定，也不为源码开放通用赋值。没有生成业务 Go 源码或调用 JS 引擎。

辅助库未增加函数，support ABI 保持 `tynel.probesupport/1`；NIR 入口约定使 bridge ABI 升为 `tynel.go-probe/4`。Go/tsgo 上游 commit 不变。主 CLI 仍未接线，源文件构建入口仍为 `scripts/compile-ts.ps1`。

## 验收

结构测试验证右侧调用位于独立列表、结果引用连接正确、嵌套局部变量不泄露、false 字段与 round-trip 完整、两侧无论是否被执行都校验。

真实程序比较完整 stdout 与 Node，并以固定期望或独立布尔树求值器检查访问顺序；覆盖常量与参数、短路/非短路路径、嵌套/优先级/括号、多个实参、结果重复读取、丢弃、跳过递归与 NaN 比较。另做同路径输入变化、保存 NIR 回放、编译器直接读取错误 NIR、失败构建保留成功产物。实际结果记录于本阶段验证文件，不由本 ADR 声称测试通过。
