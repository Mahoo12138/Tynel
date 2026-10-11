# Tynel 实验指南

## 文章与版本

每篇学习笔记对应一个阶段、一个提交和一个附注标签。阶段编号与文章编号一致，代码按以下顺序推进：

| 标签 | 学习内容 | 实现范围 |
|---|---|---|
| `stage/01-ir-native` | 笔记 01：把一份加法描述编译成 exe | 手写 NIR v1 → Go 后端 → 原生加法 |
| `stage/02-tsgo-query` | 笔记 02：查出 answer 的类型 | 类型检查与位置查询；包含固定对象 GC/ABI 基础验证，尚无源码 lowering |
| `stage/03-ts-native` | 笔记 03：让 TypeScript 接替手写 IR | 受限数值 TS → 结构化事实 → NIR → 原生程序 |

概念查询表是参考资料，不另设阶段或标签。笔记头部的 `experiment_tag` 指定实验版本；该版本中的 `replay.json` 列出范围、输入和测试入口，`docs/replay-validation.md` 保存实际结果。阶段并不等于整个 SPEC 里程碑已经完成。

## 检出与环境

在已有仓库中检出笔记指定的版本，目标目录应当尚不存在：

```powershell
cd D:\Code\Tynel
git worktree add --detach .tynel/replays/note01 stage/01-ir-native
cd .tynel/replays/note01
git describe --exact-match --tags
Get-Content replay.json
```

独立 worktree 不会切换主开发目录。实验使用 Windows amd64、PowerShell 7、Python 3.12、Go 1.26.7；Node 24 系列只用于对照运行，不是目标 exe 的运行依赖。Git 不包含 SDK、上游源码下载和生成物。

已安装匹配 Go SDK 时，指定其根目录，而不是 `bin`：

```powershell
# 笔记 01
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7

# 笔记 02、03 还需前端；在各自检出的目录执行
pwsh -NoProfile -File scripts/replay.ps1 `
  -SdkSource D:\Code\Tynel\.tynel\sdk\go1.26.7 `
  -WithFrontend -Proxy http://127.0.0.1:7890
```

SDK 已准备时可以省略 `-SdkSource`。代理按需要传入，脚本退出后恢复进程环境。各版本重新构建自己的编译器和示例，不借用其他目录的 exe 或 identity 文件。`-FuzzSeconds 0` 表示显式跳过主动 fuzz；默认运行五秒。上述命令不是空机器的完整离线安装方案。

## 阅读 Go 源码

```powershell
python -X utf8 scripts/prepare-go-source.py `
  --proxy http://127.0.0.1:7890 --sdk .tynel/sdk/go1.26.7
python -X utf8 scripts/prepare-go-source.py `
  --verify-only --sdk .tynel/sdk/go1.26.7
```

源码锁定于 `e3336a22ad3f0a90bd252c95d8b5544e02674205`。脚本检查提交、Git tree、源码摘要与接缝，可比较 SDK 中对应文件。额外文件和已编译工具不由源码比较认证。当前通过私有 SDK 与 overlay 构建修改版编译器，不声称完成整个 SDK 的源码自举。

## 前端源码准备中断

源码已解压但目录发布失败时，先确认原准备进程结束，再核验并恢复：

```powershell
python -X utf8 scripts/prepare-tsgo.py --resume-publish
```

目录仍无法改名、文件却完整可读时，可以显式复制到不存在的目标：

```powershell
python -X utf8 scripts/prepare-tsgo.py --resume-publish --copy-verified-stage
```

两条路径都会校验源码与声明身份，不覆盖已有目标、不改权限或终止占用进程。复制不是原子发布，中断后的半成品仍被后续校验拒绝。恢复后应重跑阶段验证。

## 阶段收尾

撰写或修改笔记前先阅读[写作指南](writing-guide.md)。正文围绕实验推进，完整验证数据保存在阶段记录中。

先完成实现与示例，再执行正负向测试、写入文章和结果、审阅并提交，最后创建 `replay.json` 中登记的附注标签。概念表与辅助验证随相应文章提交，不产生额外节点。源码、锁文件、测试和文章一同保存，SDK 和二进制不提交。

```powershell
python -X utf8 scripts/check-stage.py
python -X utf8 scripts/test-check-stage.py
# 完成阶段提交与附注标签后核对真实关系
python -X utf8 scripts/check-stage.py --tags
```

校验器要求第一阶段没有父提交，后续阶段直接接在前一阶段之后，并且阶段编号、文章编号、标签和保存的文件相符。校验通过不替代运行测试。远程推送与发布单独执行。
