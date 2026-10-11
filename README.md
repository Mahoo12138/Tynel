# Tynel

**TypeScript syntax, native execution.**

一个学习与研究型 TypeScript-to-native 编译器项目，目标是复用 tsgo 前端及 Go 编译器中后端与 runtime。不生成业务 Go 源码，不嵌入 JavaScript 引擎。

## 当前状态

当前编译路径是 **受限 TypeScript → 自有结构化事实 → NIR v3 → Go fork → 原生程序**。入口是 `scripts/compile-ts.ps1`，支持 number/boolean 函数、const、算术、比较、布尔取反、if/else、词法块和直接调用，以及单参数标量输出。条件只接受 boolean，严格相等只接受同一种标量；所有分支和返回路径都进行检查。尚不支持隐式 truthiness、&&/||、三元表达式、循环、赋值、模块、泛型或通用对象；主 `tynel build` 尚未接线。固定 `probe.gc/1` GC/ABI 实验保留。当前只是可验证子集，不能视为全部里程碑退出。

根目录 [SPEC.md](SPEC.md) 是整个项目的技术规格。架构、语义和验收以它为准；[开发记录](docs/progress.md) 记录实际实现和验证，[架构说明](docs/architecture.md)、[路线图](docs/roadmap.md) 是摘要，不替代规格。历史学习笔记以各篇注明的提交为基线。

学习路线：[01 把一份加法描述编译成 exe](docs/notes/01-from-ir-to-native.md) → [02 查出 answer 的类型](docs/notes/02-tsgo-facade-and-type-facts.md) → [03 让 TypeScript 接替手写 IR](docs/notes/03-typescript-to-native.md) → [04 只执行被选中的分支](docs/notes/04-control-flow.md)。每篇包含问题、实现思路和运行实例，头部的 `experiment_tag` 对应其代码版本。

术语见 [概念查询表](docs/reference/concepts.md)，环境与检出命令见 [实验指南](docs/reproducing.md)。四篇笔记与四个阶段一一对应：IR → 类型查询 → 源码编译 → 条件控制流。固定对象 GC/ABI 是第二阶段的基础验证，参考表不另设标签。

## 从 TypeScript 构建原生示例

在已准备锁定 SDK/tsgo 源码的环境中：

```powershell
pwsh -NoProfile -File scripts/build-bridge.ps1
pwsh -NoProfile -File scripts/build-tsgo-probe.ps1 -Test
pwsh -NoProfile -File scripts/compile-ts.ps1 -InputTS examples/native/add.ts -Output dist/ts-add.exe
.\dist\ts-add.exe
```

本轮实测输出 `42`。生成的 `.tynel.json` 记录源码、前端、NIR、后端和二进制身份；`-emit facts` / `-emit nir` 可单独观察前端产物。范围与证据见 [ADR-015](docs/adr/015-scalar-source-to-native.md) 和 [T14/T16 记录](docs/progress-t14-t16.md)。失败构建不会替换旧成功二进制，必须检查命令退出码。

分支示例：

```powershell
pwsh -NoProfile -File scripts/compile-ts.ps1 -InputTS examples/native/branches.ts -Output dist/branches.exe
.\dist\branches.exe
```

输出为 `42`、`0`、`false`、`true` 四行。`-WithFrontend` 回归同时运行数值与分支差分。NIR v3 和 facts v2 / `tynel.scalar/2` 会拒绝旧格式；旧文章通过其自己的代码版本复现。范围见 [ADR-016](docs/adr/016-boolean-control-flow.md)，实际结果见 [阶段验证](docs/replay-validation.md)。

## 开发环境

本轮脚本面向 Windows amd64 + PowerShell 7 + Go 1.26.7。Go commit、Git tree、11,470 个 tracked src 文件摘要及接缝 SHA256 锁在 `toolchain.lock.json`。代理拉取与校验入口为 `python -X utf8 scripts/prepare-go-source.py --proxy http://127.0.0.1:7890 --sdk .tynel/sdk/go1.26.7`；SDK 额外文件单列，不声称完成二进制自举重建。构建脚本把已安装 SDK 复制到 `.tynel/sdk/` 后应用源文件 overlay，生成独立的 fork 编译器；不改全局 Go 安装。

```powershell
go test ./... -count=1
go run ./cmd/tynel version
go run ./cmd/tynel check-ir examples/add.nir.json
pwsh -NoProfile -File scripts/build-bridge.ps1
pwsh -NoProfile -File scripts/compile-ir.ps1 -InputIR examples/add.nir.json -Output dist/add.exe
.\dist\add.exe
```

示例预期输出 `42`；请以开发记录中的实际执行证据为准。首次构建会复制一份 SDK 并编译编译器，产物和缓存均不纳入 Git。

## 完整原型回归

```powershell
pwsh -NoProfile -File scripts/test.ps1
```

该命令依次检查 Go 格式、PowerShell 语法、根模块单元测试、`go vet`、binary64 模糊测试，重新构建 bridge，执行原有 7 项原生 smoke、固定对象 GC 探针（普通/压力各 20 次）与 4 项 ABI 检查，最后重新生成并运行 `dist/add.exe`。Node 仅用于 `add.reference.ts` 的对照执行，不参与原生产物运行。`-FuzzSeconds 0` 可跳过主动模糊测试；默认运行 5 秒、2 个 worker。

`go test ./...` **不包含**嵌套模块 `bridge/go` 和 `experiments/bootstrap`；bridge 的真实构建与执行由上述回归覆盖。测试入口结束后会恢复调用进程的工作目录及其设置的 Go/Tynel 环境变量。这里只验证当前 Windows amd64 原型，不等于完成全部 M0/M1 或 TS/Node 兼容性认证。

## 真实编译路径与实验限制

```text
examples/add.nir.json
  -> NIR 解码 + 白名单验证
  -> Go fork 内的 tynelbridge
  -> ir.NewFunc / ir.NewBinaryExpr / 原生函数调用
  -> Go 中端 / SSA / 机器码
  -> 匹配的链接器与 Go runtime
  -> dist/add.exe
```

`experiments/bootstrap/main.go` 是固定、手写的包初始化与 fmt/math 支持骨架；它不包含输入相关算法，也不由源程序生成。bridge 直接生成 add 的 IR 并替换 main 的函数体。这个手写 IR 入口用于独立后端实验；上面的 TS 路径已经连接同一个后端。消除骨架、完整包系统和语言兼容仍是后续工作。

## 固定对象实验

```powershell
pwsh -NoProfile -File scripts/build-bridge.ps1
pwsh -NoProfile -File scripts/smoke-gc.ps1
pwsh -NoProfile -File scripts/smoke-abi.ps1
```

实现范围、NIR v1 → v2 的拒绝策略及测试边界见 [ADR-013](docs/adr/013-spec-authority-and-m1-probes.md)；执行证据见 [T09 记录](docs/progress-t09.md)。这不是完整对象模型或 TypeScript 前端。

## 独立 TypeScript 前端探针

候选基线锁为 TypeScript `v7.0.2` 的固定提交，使用 Go 1.26.7。在已准备私有 SDK 的情况下执行：

```powershell
# 首次获取固定源码；已准备时只核对，不跟随 main 更新。
python -X utf8 scripts/prepare-tsgo.py
pwsh -NoProfile -File scripts/build-tsgo-probe.ps1 -Test
.\.tynel\tsgo\tynel-tsgo-probe.exe examples/frontend/add.ts
.\.tynel\tsgo\tynel-tsgo-probe.exe examples/frontend/type-error.ts
# 包含旧后端、GC、ABI 和新前端的统一回归。
pwsh -NoProfile -File scripts/test.ps1 -WithFrontend
```

合法输入返回 JSON 且退出 0；类型错误退出 1；使用错误退出 2。探针使用固定逻辑文件名 `/tynel/probe.ts`、单文件内存快照和上游 ES5 声明；`-offset` 接受零起始 UTF-8 字节偏移。默认查询模式不执行 TS、不生成 NIR，也不提供 npm/Node 运行环境；其类型显示字符串只供研究。新增 `-emit facts` / `-emit nir` 模式通过独立的数值类型/声明身份投影生成 NIR，不解析显示字符串。来源、范围和验证见 [T01/T04 记录](docs/progress-t01-t04.md) 与 [ADR-014](docs/adr/014-tsgo-facade-baseline.md)。

默认 `scripts/test.ps1` 仍只要求后端环境；`-WithFrontend` 明确增加源码锁检查、前端外部消费测试、内部包访问负向验证，以及原有 9 组数值对照/3 组错误构建，新增 13 组分支与布尔对照/8 组错误构建、保存 NIR 回放和 2 组编译器直接拒绝测试。缺源码时会失败，不会悄悄跳过。前端源码、依赖和编译器产物位于 Git 忽略目录；本项目 facade、host 和依赖校验文件正常纳入版本管理范围。

## 布局

- `compiler/frontend`：自有数值事实、声明 ID、独立约束检查与 NIR lowering。
- `compiler/nir`：独立 IR、验证器、无损数字编码与测试。
- `cmd/tynel`：版本查询与 IR 检查/格式化；未实现的 TS 命令明确拒绝。
- `bridge/go`：只在合法 Go 编译器包树中构建的 bridge 源码。
- `cmd/tynel-toolexec`：bootstrap 编译路由和缓存身份。
- `experiments/bootstrap`：手写初始化骨架，不是业务代码生成器。
- `scripts`：隔离构建及 smoke 验证。
- `docs`：设计决策、阶段状态和证据。

上游许可证遵循原有要求；本轮没有为项目擅自选择发布许可证，没有创建远程仓库或 push。

学习笔记的撰写与审阅遵循[写作指南](docs/writing-guide.md)。
