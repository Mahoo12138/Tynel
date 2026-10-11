# Stage 04：布尔比较与条件控制流

本阶段接在 `stage/03-ts-native` 之后，增加布尔比较与条件选择。源码范围、文章和验证入口由根目录 replay.json 登记，技术规格以 SPEC 为准。

## 实现

引入 NIR v3、facts v2 / `tynel.scalar/2`；增加 boolean、数值有序比较、同类严格相等、布尔取反、if/else、词法块、局部作用域与保守的全路径返回检查。未选分支的调用保留在分支内部，源码、事实与 NIR 均检查边界。Go bridge 直接创建高层 if 与 bool 节点，新增固定 bool 打印函数；运行时对象布局与探针辅助 ABI 不改变。

版本升级时更新的是当前示例与驱动协议，不会让旧阶段的文件悄悄按新格式解释。旧 NIR v2 和旧 facts profile 被拒绝。当前仍不支持 truthiness、逻辑短路、三元表达式、循环、可变绑定、闭包、模块或通用对象。

设计见 [ADR-016](adr/016-boolean-control-flow.md)。[笔记 04](notes/04-control-flow.md)围绕分支程序展开，通过输出观察被选中的路径，并检查返回结果；实验代码见 [branches.ts](../examples/native/branches.ts)。

## 验证过程

最初的结构契约测试只允许 string/int，增加 boolean 后被该检查阻断。现在将 bool 纳入普通数据白名单，仍禁止上游对象、隐藏字段和不可序列化句柄。根模块重新测试通过。

一个空分支夹具写成 `if (false);`，tsgo 返回 TS1313；没有跳过上游诊断，而是改为实际受支持的空块 `if (false) {}`。重新构建并运行前端测试，95 个测试及子测试通过事件。

独立控制流 smoke 已执行通过：13 组原生/Node 程序、8 组错误构建、2 组绕过 driver 的编译器拒绝与 NIR 回放。三份最终示例的完整统一回归结果另见 [阶段验证](replay-validation.md)。

## 实验边界

`stage/04-control-flow` 保存本阶段的源码、锁文件、笔记和验证记录。`scripts/check-stage.py --tags` 用于检查文章、清单与附注标签的对应关系。Go/tsgo 使用锁定版本，宿主安装保持不变；冷机器离线重建与跨平台验证仍在后续范围。
