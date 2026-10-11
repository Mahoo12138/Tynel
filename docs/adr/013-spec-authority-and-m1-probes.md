# ADR-013：规格入口与 M1 GC / ABI 探针

状态：采纳。对应 SPEC 第 5、8、12、16 节及 T09。在标量原型上验证对象存活和辅助库约定。

## 规格与状态分开

用户确认的根目录 `SPEC.md` 是项目技术规格。README、架构摘要、路线图和学习笔记均以它为准。规格中的开放问题和建议通过 ADR 明确选择；已实现/已验证状态写进开发记录，不把设计表改成通过证明。保留旧学习笔记的提交基线与正文。

本轮继续在已有 Windows amd64 探针上实现 T09，不切换宿主平台。SPEC 的 Linux 建议仍是后续平台计划；Go/tsgo 精确基线与干净重建尚未齐备，因此 M0、M1 均不整体标记完成。

## IR 版本和实验边界

NIR schema 升为 2，标量 `shared-core/0.1` 的含义保持不变。v1 输入现在明确拒绝，仓库示例同步为 v2；这不是跨版本自动迁移器。

加入单次定义的局部绑定 `let` / `local`。只有模块显式声明 `features: ["probe.gc/1"]` 时，才能使用 `probe_ref` 与 `probe_*` 操作。未知或重复 feature、错误类型、先使用后定义、非法字段写入等由 verifier 拒绝。

`probe_ref` 仅是测试宿主中固定 `Node { Value float64; Next *Node }` 的可空引用，不是 TypeScript 对象、null/undefined 或结构类型的正式实现。`probe_new` 产生零初始化对象，Next 初始为 nil；探针只读取已经赋值的链。直接解引用 nil 使用 Go 的原生失败行为，不作为未来源语言的错误语义。该门控是功能分类，不是执行不可信 IR 的安全沙箱。

## 哪些步骤由桥生成

类型声明暂时来自手写 `probesupport.Node`，通过类型 witness 的函数签名取得。bridge 检查声明来源、字段顺序和字段类型，然后使用正常的 Go 高层 IR：

- `probe_new` → `ONEW` + 类型节点，再正常类型检查。
- `let` → 函数局部声明和赋值。
- 字段读写 → `DotField` 与赋值节点，Next 始终是真实 GC 可见指针。
- 引用参数、返回与别名 → 原生指针类型的函数签名和局部值。

分配和输入相关字段操作没有交给手写 Go 业务函数，也没有写成生成的 `.go` 文本。手写辅助库只声明固定布局、触发 GC、执行深栈探针、检查身份和提供 ABI 标记。完整对象形状生成、类型元数据生成和闭包捕获仍待实现。

## GC 验证的设计

IR 构造三个函数。`makeNode` 分配并返回对象；`linkNode` 把子对象指针写入父对象；main 保留父对象和别名，经过显式 GC、深栈中的 GC 后读取两个字段，再通过别名修改父对象。

GC/深栈辅助函数不接收对象参数，也不把对象存入全局根；存活必须由生成的调用者引用保持。子对象仅通过父对象字段保留。深栈 helper 使用 128 层递归和每层 1024 字节数组，并在返回时经独立 noinline 函数检查数组，避免变成无用计算。

测试一方面查反汇编中 `runtime.newobject` 和 `runtime.gcWriteBarrier*` 的存在，另一方面在普通模式及 `GOGC=1`、`GODEBUG=gccheckmark=1,clobberfree=1` 下重复运行。每次严格比较 stdout、stderr、退出码，并设置进程超时。

屏障调用存在不等于每次运行都进入了屏障分支；压力测试也不穷尽 GC 的并发交错。这里只形成最小分配/存活/写入证据，不声称已证明完整 GC 正确性、回收及时性或闭包安全。

## 两层 ABI 检查

driver 检查 IR、bridge、support 版本以及支持源文件集合的指纹。bridge 在创建程序节点前检查辅助函数签名与固定对象布局。

生成 main 时调用 `tynelProbeABI_v1`，它调用独立辅助包的 noinline `ABI_v1`。即使程序只计算数字，最终可执行文件也应保留该符号；测试用 `go tool nm` 检查。

负向链接试验先导出同一次编译的包归档，直接调用 Go 链接器完成一个输出 42 的正向对照，再只替换 support 归档。旧包缺少新锚点，并具有不同的 Go 包指纹。链接失败必须具有包指纹或符号不匹配原因，不能把任何非零退出都算成通过。另一条负向试验用源码 overlay 替换成错误布局夹具，要求 bridge 返回 TY4002。

这是版本化锚点与 Go 包指纹联动的原型，不是任意恶意二进制篡改检测；相同公开 API 下的所有行为差异、Go fork 精确源码身份、所有包格式和跨平台 ABI 仍需后续 T39 验证。

## 工具链选择的回归教训

组合脚本必须同时固定 GOROOT 和 go 可执行文件；仅设置其中一个可能把 PATH 中另一个版本的驱动与私有 SDK 拼在一起。首次统一回归实际遇到 Go 1.27.0 驱动 / Go 1.26.7 SDK 的混用，单独运行探针并未暴露它。现已将统一入口的 go/gofmt 和 GC 夹具生成改为明确 SDK 路径，并通过完整重跑验证。这仍不替代 Go/tsgo 精确源码 pin。

## 复现入口

```powershell
pwsh -NoProfile -File scripts/build-bridge.ps1
pwsh -NoProfile -File scripts/smoke-gc.ps1
pwsh -NoProfile -File scripts/smoke-abi.ps1
# 当前完整回归
pwsh -NoProfile -File scripts/test.ps1
```

夹具生成器是 `cmd/tynel-gc-probe` / `internal/probe`，输出 NIR 数据。结果与汇编保存在 `.tynel/gc/`、`.tynel/abi/`，均不纳入 Git。实际执行证据见 [T09 记录](../progress-t09.md)。

参考：SPEC 第 12、16 节；Go 编译器导读 https://go.dev/src/cmd/compile/README ；Go GC 指南 https://go.dev/doc/gc-guide ；实际桥接 API 以隔离 SDK 中的 Go 1.26.7 源码为准。
