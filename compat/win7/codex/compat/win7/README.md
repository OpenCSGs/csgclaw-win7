# Win7 Codex 维护分支

本分支基于上游 `rust-v0.149.1`，使用 Rust 1.95.0 的 `x86_64-win7-windows-msvc` 目标重新构建标准库。
Go 服务使用 CSGClaw 仓库中的 Win7 兼容 Go 工具链构建。

## 兼容实现

`imports.c` 为六个可选 Windows API 提供动态解析和兼容返回值。
Windows 7 SP1 需要 KB3063858 或包含相同安全加载功能的后续更新。
不存在的 ConPTY 和 WinRT 能力返回不支持，不伪造成功。

CSGClaw 在 Windows 7 上使用允许协作进程显式脱离的外层 Job。
Codex 以挂起状态创建受限命令，并在恢复执行前将它加入禁止脱离的独立命令 Job。
受限令牌和工作区 ACL 保持不变。
加入命令 Job 失败时终止挂起进程。
Windows 8 及以后继续使用原有嵌套 Job 路径。

Windows 7 上，正常结束的命令可以留下后台任务，但 app-server 会保留这些后台 Job 的所有权。
后续保留后台 Job 时会释放已经没有活动进程的 Job 句柄。
app-server 被关闭或终止后，系统关闭其句柄并回收剩余后台任务。

旧系统不支持原子的 JOB_LIST 创建属性。
先挂起、再加入 Job 的方式仍有进程创建与加入之间的窗口，调用方异常终止时可能留下尚未执行用户代码的挂起进程。
当前生命周期测试覆盖已经启动的命令树和后台命令，不证明该极短创建窗口内的崩溃行为。

## WinPTY

源码固定为 MIT 许可的 WinPTY 0.4.3，提交 `3e1ab962d5262dd76159870c6dc0724927ca6a9d`。
`build-winpty.py` 校验源码归档 SHA-256 后应用 `winpty-sandbox.patch`，使用 Windows 7 SDK 目标和静态 CRT 构建。
补丁让可信控制台 agent 使用实际受限令牌挂起启动，严格加入命令 Job 后恢复，用户命令继承相同约束。
指定环境、工作目录和私有桌面同步传递，控制管道仅增加同一登录 SID 的访问权限。
失败时不重试无沙箱进程。

`winpty.dll` 只从当前 Codex 可执行文件同目录加载。
交互式工具能力检查同时支持 ConPTY 和配套 WinPTY，现代 Windows 优先使用 ConPTY。
输出以终端流传递，输入、行编辑、Ctrl+C 和窗口调整使用真实控制台。

## 本机构建

需要 Rust 1.95.0 的 rust-src、cargo-xwin 0.23.1、Homebrew LLVM 和 LLD，以及 cargo-xwin 下载的 Windows SDK。
WinPTY 构建需要 Python 3.12 或更高版本。
首次准备应运行 `rustup component add rust-src --toolchain 1.95.0`。
若 windows_x86_64_msvc 0.48.5 的导入库搜索失败，LIB 需包含 Cargo registry 中该包的 lib 目录。

```sh
compat/win7/build-macos.sh
# 使用开发构建以便快速迭代：
WIN7_PROFILE=dev compat/win7/build-macos.sh
```

构建脚本把兼容对象文件放在系统导入库之前链接。
默认使用 release 配置，开发验证也使用过 dev 配置。
产物目录包含 `codex.exe`、`winpty.dll`、`winpty-agent.exe` 和 `LICENSE.winpty`，分发时必须一起携带。

## 验证

CSGClaw 的 `internal/runtime/codex/win7_sandbox_windows_test.go` 覆盖真实 Runtime 到此 Codex app-server 的调用。
设置 `CSGCLAW_TEST_CODEX_BINARY` 指向构建产物后，在 Win7 原生运行对应 Windows 测试二进制。
测试覆盖工作区写入、目录外写入拒绝、取消、超时、后台进程回收、app-server 崩溃回收，以及取消和超时后的继续对话。
不要以 Mac 上的 Rust 测试代替 Windows 原生生命周期测试。

`windows-sandbox-rs/examples/win7_pty_probe.rs` 是原生控制台探针。
以同一 Win7 工具链构建后，与 DLL 和 agent 放在同目录运行。
探针检查真实控制台、受限令牌、工作区边界、中文输入、行编辑、缩放、Ctrl+C、取消、超时及拥有者崩溃后的进程句柄终态。
CSGClaw 的 `TestWindowsInteractiveCodexToolsE2E` 还通过确定性模型服务驱动真实 `exec_command` 和 `write_stdin`。
该测试需设置 `CSGCLAW_TEST_CONSOLE_PROBE` 指向探针可执行文件。

## 取消归属与维护检查

旧 Windows 在回合中断时按 turn ID 终止该回合创建的交互终端，并在发送中断事件前完成清理。
其他回合持有的后台服务不会被这一操作终止。
CSGClaw 的原生测试和真实浏览器 Stop 操作均检查父子进程句柄退出，并验证同一会话还能继续使用。

macOS 全仓测试可运行 `python3 compat/win7/test-macos.py`。
脚本通过 `just test` 执行，隔离个人配置与桌面进程注入的代理、CLI 路径和 HOME，保留沙箱相关环境变量。
它支持透传 nextest 参数，例如 `--test-threads 4`。
V8 依赖须按仓库 setup-rusty-v8 工作流准备匹配的归档与绑定文件。
测试 fixture 对 release 版本号、macOS 临时目录和本机已安装 Codex 路径作了确定性处理，不改变生产运行行为。

## 当前边界

这里维护的是 CLI/app-server，不是 Codex 桌面应用或 Electron。
Win7 的交互式终端由配套 WinPTY 后端提供，ConPTY 探测仍如实返回不可用。
不通过禁用 `unified_exec` 或放开默认工作区沙箱实现兼容。
升级分发必须固定本分支的独立产物，不能被普通 Windows 最新二进制覆盖。
