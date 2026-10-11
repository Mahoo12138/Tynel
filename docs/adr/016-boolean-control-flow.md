# ADR-016：布尔值与结构化条件分支

状态：采纳。依据 SPEC 第 6、8、9、16 节及 T14/T16/T17 的增量目标。对应 `stage/04-control-flow` 和学习笔记 04；不代表完成整个 M2/M3。

## 问题

顺序数值表达式不足以描述条件执行。直接把两条分支里的调用都降低到外层临时变量，会让未选择的分支产生副作用。与此同时，单纯检查函数体中出现过 return 不能保证每条出口都有结果，分支里的局部变量也不能泄露到后续路径。

## 语言范围

增加原生 `boolean` 参数、显式返回类型、const、true/false、布尔 `!`；number 的 `< <= > >=`；同为 number 或同为 boolean 的 `=== !==`；`if`、`else`、`else if`、词法块与块内早返回。观察接口只接受一个 number 或 boolean，按静态类型选择打印辅助函数。

条件必须为 boolean。不隐式转换数字、对象或其他类型；`!number` 也拒绝。暂不支持 `== !=`、`&& ||`、条件表达式、可变绑定、循环和异常控制流。boolean 的标准前端类型可能表示为 false/true 联合；只接受经过成员与标志核对的这种表示，不解析显示字符串，也不把其他联合类型泛化为 bool。

两侧严格相等要求同一种受支持标量表示。NaN 的有序比较保留为原始比较操作，不把 `<` 改写为 `!>=`；number 的 `===` 对正负零相等、对 NaN 不相等。未实现通用 JS 严格相等算法。

## 表示和降低

NIR 升为 v3，facts 升为 v2，profile 为 `tynel.scalar/2`。旧 NIR v2 与旧 facts 协议直接拒绝；旧阶段的 tag 保留其自己的构建工具与格式。boolean 字面量采用可空的 bool 字段：非空 false 是合法值，空指针是缺失输入。Lower 复制该值，不让输出 NIR 继续依赖输入事实对象。

`if` 保存 Condition、Then、Else；`block` 保存 Body。节点之间不允许出现不属于该操作的多余字段。嵌套语句和表达式共享节点预算并检查深度，内存构造的循环引用同样受限。源码、facts 和 NIR 三层均检查范围，后端不相信外部驱动已经验证过。

每个分支或词法块复制当前的名称环境。const 初始化检查完成后才发布绑定；分支内定义不流出。源代码遮蔽由真实 tsgo Symbol 区分，降低后保留独立声明 ID，而非只比较名称文本。

表达式仍按源码顺序生成临时值，但分支使用独立语句容器。条件在分支前求值一次，分支中的调用留在其容器里。Go bridge 直接创建 `ir.NewIfStmt`，booleans 降低为 `types.TBOOL`，后续 CFG、SSA、机器码由 Go 负责。纯词法块在完成作用域检查后可以展开为 Go 语句列表，不合并其独立符号。

## 返回路径

sequence 返回“所有经过本序列的路径是否 return”。return 为真；块取内部结果；if 只有 Then 和 Else 都为真才为真；缺少 Else 视为可能继续。后续 return 可以补齐此前的非终止路径。全部路径已经返回后再出现语句，按当前保守语言范围拒绝。

当前不利用常量条件证明可达性，所以 `if (true) return 1` 仍要求其他结构路径有返回。这是编译子集限制，不是 TypeScript 标准行为的普遍结论。静态 false 分支、未调用函数也全部检查，不通过测试是否执行到来豁免能力缺口。

## 辅助库与兼容

新增手写的 `tynelPrintBool(bool)`，在 bridge 中核对签名。Go bridge 身份变为 `/3`；固定对象、`probe.gc/1`、辅助探针库布局与 `ABI_v1` 未改变，探针 support ABI 仍为 `/1`。新源码与构建器哈希进入已有新鲜度检查，旧 bridge 不可复用。未引入任意 Go 库绑定，也未改变宿主 SDK。

## 验证要求

用实际 TS 检查嵌套 if、早返回、空块、else 绑定、局部遮蔽、布尔参数和返回值；用副作用输出检查条件仅执行一次、未取分支不执行、比较左右操作顺序。数字矩阵覆盖 NaN、正负零、无穷、相等与不等。

原生输出与 Node 对照，保存 NIR 绕过前端回放；同路径 true/false 输入必须改变结果。负向用例覆盖类型错误、条件 truthiness、越界作用域、缺返回路径、未取分支中的未知操作。另直接向修改版编译器送入错误条件类型与旧版本 NIR，要求 TY4001，而不是只在外层脚本拒绝。具体执行结果见 [阶段验证](../replay-validation.md)。

实现入口：[facts](../../compiler/frontend/facts.go)、[控制流降低](../../compiler/frontend/control.go)、[NIR 校验](../../compiler/nir/control_verify.go)、[Go bridge](../../bridge/go/control.go)、[端到端实验](../../scripts/smoke-control.py)。
