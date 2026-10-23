# CSGClaw Windows 7 兼容技术报告

验证日期：2026-10-17。
验收基线：当前 Windows 10/11 版的 Go 服务、Web UI 和 Codex 引擎能力。
范围已确认：本轮完成 Go + Codex；DSH/Node 单列适配；Electron 不纳入。

## TLDR

**CSGClaw 兼容 Win7 已验证可行，Go + Codex 完整调用链已实际跑通，已验收功能无需删减。**
采用兼容 Go 工具链和定制 Codex，解决系统兼容、沙箱与终端问题，保留当前 Go 业务依赖、默认工作区沙箱和交互式终端。
取消、超时和子进程回收均已验证。
剩余工作是正式安装包和独立更新渠道；DSH/Node 单列适配，Electron 本轮不涉及，第三方服务按依赖条件验收。

## 结论

**CSGClaw 兼容 Windows 7 SP1 x64 技术上可行，Go + Codex 完整调用链已经在虚拟机内运行成功。**
实际链路为浏览器 → CSGClaw Go 服务 → 定制 Codex app-server → 沙箱内工具/终端/随包 CLI → 页面最终回复。

**已验收能力无需因 Win7 而删减，默认工作区沙箱和交互式终端均已保留。**
Go 业务代码及依赖不需要整体降级，采用兼容工具链重新构建；Codex 基于 `rust-v0.149.1` 独立维护 Win7 适配分支。
当前交付是经过原生验证的开发构建，正式安装包及独立更新渠道仍需接入。
本结论不等于所有第三方线上服务、DSH 引擎及 Electron 均已完成验收。

## 一、主要卡点与解决方案

| 卡点 | 解决方案 | 实测结论 |
| --- | --- | --- |
| 普通 Go 工具链与旧系统不兼容；退回 Go 1.20 又无法构建当前源码及依赖 | 使用 XTLS/go-win7 `patched-1.27.2` 构建服务和 CLI；保留现有 `go.mod`、`go.sum`，仓库无 vendor 目录需要降级 | 服务原生运行，真实模型可在沙箱内调用随包 CLI |
| 普通 Codex Windows 二进制依赖 Win7 缺失的系统能力 | 从 `rust-v0.149.1` 源码构建，使用 Rust Win7 目标重建标准库；对六个可选 API 动态解析并提供兼容处理 | CLI 与 app-server 均在 Win7 原生运行 |
| Win7 不支持嵌套 Job，CSGClaw 的进程管理与 Codex 沙箱发生冲突 | 仅对旧系统的可信 Codex 协作路径允许显式脱离外层 Job；受限命令挂起创建，加入禁止脱离的独立命令 Job 后才恢复 | 保留受限令牌、工作区 ACL 和整棵命令树回收；加入失败即终止 |
| Win7 没有 ConPTY，模型交互终端无法启动 | 集成固定版本 WinPTY，并适配受限令牌、私有桌面及命令 Job；失败时直接报错 | `exec_command(tty=true)`、`write_stdin`、中文、行编辑、窗口调整及 Ctrl+C 通过 |
| 未安装 Windows Terminal，工作区终端入口不可用 | 优先使用 Windows Terminal；缺失时通过系统目录的 `cmd.exe` 创建真实交互控制台，以工作目录传递路径并禁用 AutoRun | 浏览器点击打开终端，目录正确，实际键入命令并获得输出 |
| 取消、超时、后台任务可能残留或误报 | 按 turn 回收交互终端，持有后台 Job 所有权，处理 app-server 退出及重复清理；保留执行中任务的 Stop 状态 | 取消、超时、后台回收、拥有者崩溃及会话恢复通过 |
| Win7 可用浏览器缺少部分新 API/CSS 能力 | Firefox 115 ESR；PDF.js 切换同版本 legacy 引擎与 worker，PPT/HTML 使用 transform 和明确布局尺寸缩放 | 12 类预览及 10 个页面巡检通过 |

现代 Windows 继续使用原有 ConPTY 和嵌套 Job 路径。
模型终端保持默认 `standard` / `workspace-write` 模式，并实际验证工作区可写、工作区外写入被拒绝。
用户主动打开的系统命令窗口沿用现有可信本机入口，与模型沙箱终端分别验收。

## 二、原生验收结果

| 能力 | 验证结果 |
| --- | --- |
| 浏览器对话、流式工具结果、最终回复 | 真实模型调用通过，核对服务端持久化 turn 终态与页面最终内容 |
| 默认工作区沙箱 | 实际受限令牌、工作区写入和目录外拒绝通过 |
| 模型交互终端与系统终端入口 | 两类终端均通过，包含实际输入、输出与退出 |
| 取消、超时、进程回收和继续对话 | 原生 Runtime 五项生命周期用例通过；浏览器 Stop 后父子进程句柄均退出，同一会话可继续使用 |
| 文件编辑、补丁、Git、Go CLI | 真实写入与回读、Codex 内置 apply-patch、Git 修改/status/diff/log 和 CLI 帮助输出通过 |
| Skill、MCP、只读模式 | Skill 加载及开关通过；六组 MCP 工具目录配置下的鉴权调用、热更新、关闭后恢复通过；只读请求不暴露本地执行、写入与补丁工具 |
| 团队、任务、审批、定时恢复、附件及配置接口 | Win7 原生 API 验收通过；涉及外部服务的响应按依赖条件区分 |
| 文件预览 | DOCX、XLSX、PPTX、CSV、Markdown、PNG、WAV、MP4、HTML、Go 源码、Windows-1252 文本、PDF 共 12 类通过 |
| 页面巡检 | 智能体、团队、连接器、资源、本机、模型、用户、任务、新建任务及设置共 10 页通过，未捕获路由或脚本异常 |

原生 API 共 **92 项不同测试、95 次执行、零跳过**，另有 **2 项系统终端入口回归测试**。
回收验证持有实际进程句柄并检查退出，避免仅根据 PID 或日志判断成功。
真实模型通过主机限定中继访问，模型密钥保留在主机。
部分模型、OAuth、连接器及外部账户测试使用确定性本地服务，不能据此宣称所有线上第三方服务已经登录验收。

工程验证中，Go 全量测试、相关 race 检查及构建通过，Web 检查 **1,626 项通过**。
Rust 全仓首次执行 15,406 项，其中 15,397 通过、9 失败，另有 45 项跳过；通过项含 2 项重试后通过。
针对测试环境、夹具和资源竞争完成修复后，原失败用例及受影响模块复核通过，相关静态检查和格式检查通过。
**未再次执行整仓零失败运行**，完整记录保留首次失败与后续复核的独立结果。

## 三、验证环境与维护方案

| 组件 | 已验证组合 |
| --- | --- |
| 虚拟机 | UTM 5.0.7，Windows 7 Ultimate SP1 x64，6.1.7601 |
| 系统更新 | KB3063858、KB4490628、KB4474419 v3；此为实测组合，未证明是最小补丁集合 |
| Go | XTLS/go-win7 `patched-1.27.2`，现有业务依赖 |
| Codex / Rust | 上游 `rust-v0.149.1` 加适配补丁；Rust 1.95.0，`x86_64-win7-windows-msvc` |
| 终端 | 定制 WinPTY 0.4.3；DLL 仅从 Codex 可执行文件同目录加载 |
| 浏览器 / Git | Firefox 115.43 ESR；MinGit 2.46.2.windows.1 |

维护方式为独立固定 Go 工具链、Codex 源码补丁及 WinPTY 配套产物，不依赖主机代跑业务进程。
主机在本次验证中提供网络和模型中继，CSGClaw、Codex、终端与工具实际运行在 Win7 内。
默认 PowerShell 2.0 使用其支持的管道命令及 .NET 文件 API，不能直接假设存在新版 PowerShell 参数。

## 四、边界与发布待办

- **DSH/Node：单独适配。** 本轮没有验证其 Win7 原生运行时，不能计入 Go + Codex 的通过项。
- **Electron：独立调研。** 本报告不判断桌面壳的兼容性。
- **外部依赖：按条件验收。** Python、其他 CLI、MCP 服务、第三方账户、证书及网络需分别满足运行条件。
- **Windows 功能基线：保持一致。** BoxLite 当前在所有 Windows 上禁用，不属于 Win7 新增功能损失。
- **生命周期边界：保留说明。** Win7 的挂起创建和加入 Job 不是原子操作，拥有者在两者之间异常退出时可能留下尚未执行用户代码的挂起进程；本次通过的是已启动命令树及后台任务的回收验证。
- **正式分发：尚待接入。** 服务、CLI、Codex、WinPTY DLL/agent 与许可证须成套校验，并建立 Win7 独立更新渠道，避免普通 Windows 二进制覆盖；升级后重复沙箱、终端和生命周期验收。

## 五、交付与复测索引

本次验收对应 CSGClaw 基线 `80d920a1c95bd7449f56766cceea7bc6cb23e8a7` 和 Codex 基线 `ff29a44391deccde0aba0f8390337d7f3c319ea4`，以及交付的两份源代码补丁。
源码仍保留在本地工作区，尚未提交或推送。

本机验收目录为 `/Users/longyun/Virtual Machines/CSGClaw-Win7-Test/results/full-features-20261017`。
其中 `CSGClaw-Win7-dev-verified.zip` 是已验收开发构建，`artifact-manifest.json` 记录配套二进制哈希，`verification.json`、`验证记录.zh.md` 和 `logs/` 保存分层结果与原始证据。
压缩包内报告保留交付时版本，本报告为按最终范围重新整理的版本。
虚拟机验收快照名为 `win7-csgclaw-full-features-20261017`。

CSGClaw 原生回归入口位于 `internal/runtime/codex/win7_*_windows_test.go`、`internal/processgroup/process_windows_test.go` 和 `internal/api/workspace_terminal_windows_test.go`。
Codex 构建、WinPTY 来源、补丁及维护检查见配套仓库 `compat/win7/README.md`。
交叉编译成功不能替代 Win7 原生沙箱、终端和生命周期复测。

## 参考实现来源

- [XTLS Go 兼容工具链](https://github.com/XTLS/go-win7/releases/tag/patched-1.27.2)
- [Rust Win7 构建目标](https://doc.rust-lang.org/rustc/platform-support/win7-windows-msvc.html)
- [Windows Job 文档](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)
