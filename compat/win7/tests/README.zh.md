# Win7 原生桌面测试夹具

这些程序只用于隔离的 Win7 测试虚拟机，不进入用户分发包。
测试默认使用 `C:\CSGClawTest`、`agent-win7` 和独立测试配置；场景中的既有房间、文件及服务地址属于这套夹具，换环境时须替换。
真实模型场景需要配置可访问的模型服务，不在源码或结果中保存真实密钥。

## 准备

使用 `build-app.py` 先完成构建，将安装 EXE 和合成预览文件放入测试目录，在虚拟机中安装后执行桌面验收。
使用同一 Win7 Go SDK，在生成的 `.cache/win7/csgclaw` 模块目录编译 `tests/guest`、`tests/webdriver`、`tests/tree` 和 `tests/worker` 的 `main_windows.go`，设置 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`。
`guest` 引用模块中现有的 `golang.org/x/sys/windows`，不另行升级依赖。

启动主机桥接服务，标准输出不会随请求增长而阻塞：

```sh
python3.12 compat/win7/tests/bridge.py /path/to/test-bridge
```

在虚拟机 `C:\CSGClawTest\compat` 内分别启动 `worker.exe task.json` 和 `worker.exe task-pty.json`。
桥接只监听主机 `127.0.0.1:48817`，UTM 用户网络中的虚拟机通过 `10.0.2.2:48817` 访问。
不要将测试命令队列暴露到局域网或互联网。
worker 仅从该桥接下载夹具、执行本地队列并上传结果，网络请求设有超时。
每个队列由命名互斥锁限制为一个执行器，重复启动会直接退出，避免重复发送和互相覆盖验收结果。

任务描述支持 `id`、`downloads`、`command`、`timeout`、`files`；异步程序启动可设置 `spawn: true`。
`downloads` 和 `files` 使用文件名，文件放在桥接的 `artifacts/` 中。
提交与等待结果使用：

```sh
python3.12 compat/win7/tests/run-guest.py /path/to/task.json \
  --bridge /path/to/test-bridge --queue task-pty.json \
  --output /path/to/native-result.json
```

每次提交自动生成唯一 ID，避免读到旧的成功记录。
结果为部分写入、历史进度或其他任务时，等待器会继续等待。
任务超时可以终止夹具，因此长流程须设置足够的 `timeout`。
场景必须串行执行，不能同时操纵一个桌面会话。

## 桌面操作

使用 `sources.lock.json` 中与 Electron 匹配的官方 ChromeDriver，在虚拟机内只监听 `127.0.0.1:48822`。
测试应用通过夹具启动，调试端口仅为虚拟机回环地址 `127.0.0.1:48824`。
正式用户入口不附加调试参数。
WebDriver 的 `start` 连接真实应用；`mainWindow` 使用先前保存的主窗口句柄。

`desktop-startup.json` 验证全新配置和原生主题调用。
`desktop-chat-progress.json` 验证最新桌面版本、头像旁的初始进度及停止按钮，以及真实模型工具调用后的回复与输入框恢复。
`desktop-browser.json` 验证真实 BrowserView 和远程页面隔离。
`desktop-previews.json` 验证 12 类实际文件预览。
`desktop-sandbox-api.json` 验证真实受限终端、中文输入和 CLI 回调本应用 API。
`desktop-cancel.json` 创建独立任务会话，使用 `tree` 启动父子进程，检查 Stop、退出句柄、取消状态和同会话恢复。
相关原有终端探针及 Runtime E2E 入口来自 `prepare.py` 应用后的兼容源码，按生成目录内的 Win7 文档构建。

测试完成后清空两个队列为 `{}`，避免重启 worker 后再次执行旧任务。
保留结果 JSON 和日志，避免输出令牌、完整凭据或包含认证信息的 URL。
完整结果范围见上级目录的桌面验收报告。

## 安装器验收

`build-installer.py` 从已校验桌面目录构建真正的 NSIS 安装包，不使用下载器或自解压后直接运行的替代入口。
首次安装通过实际安装向导完成，并核对桌面、开始菜单和控制面板卸载注册。
`tests/installer/main_windows.go` 使用同一 Win7 SDK 构建，提供 `inspect`、`stop <pid>` 和 `assert-uninstalled` 原生检查。
`inspect` 校验已安装文件并创建命名明确的测试哨兵；`stop` 仅允许终止注册安装目录中的指定程序并等待 Go 子进程回收。
`check-running-guards` 验证运行中的程序不会被覆盖安装或卸载，`interrupt-and-retry` 在全新安装写入归属标记后中断安装，再验证同一目录可以重新完成安装。
卸载检查确认程序文件、快捷方式和卸载注册已移除，而配置目录及安装目录内用户自建文件仍保留，随后仅清理测试创建的哨兵。
运行中覆盖安装和卸载应返回非零状态且不破坏现有安装；关闭程序后可再次安装与卸载。
卸载验收前只在隔离虚拟机中运行这些测试，不针对真实用户安装目录执行。

## Manager 配置重启回归

`go/appserver_stop_timeout_windows_test.go` 仅在真实 Win7 上执行，使用实际子进程和超过三秒的退出状态写入验证停止边界。
同一测试在旧版三秒期限下应失败，在修复版等待旧进程和退出状态写入完成后才允许替换实例，并验证调用方取消仍能返回。
使用 Win7 SDK 对生成源码中的 `internal/runtime/codex` 执行 `go test -c`，在虚拟机运行 `-test.run=^TestWin7Stop -test.v`。

`desktop-manager-restart.json` 在已配置且在线的隔离 Manager 上，通过实际桌面界面连续保存推理策略，再确认保存返回运行中状态、无待重建标记且界面在线。
开始前选择可连通的模型服务，推理策略使用“模型默认”或“低”；测试最终恢复为“模型默认”。
模型服务校验返回 504 时应记录为外部服务条件未满足，不能算作配置重启通过，也不要禁用校验来绕过。

## 离线且待重建的自动恢复

上游 `internal/agentengine/pending_runtime_recovery_test.go` 通过完整 Engine 对话入口验证恢复及后续对话复用，不直接调用启动函数冒充对话恢复。
仅对配置完整、期望运行、已离线且需要应用配置的 Codex 运行时尝试恢复；缺模型、手动停止和连接器配置未完成时仍保留实际错误。
恢复复用生命周期互斥和现有启动流程，不删除工作区或会话，也不反复重建正在运行的 Agent。
Manager 未保存运行意图时按内置服务的运行意图处理，显式停止状态仍保留。
前端允许符合条件的消息进入后台恢复流程，避免发送前的离线检查永久挡住恢复。

`tests/recovery` 仅用于 `C:\CSGClawTest\` 下的隔离用户配置，先核对进程映像，再终止该 Manager 的 Codex，并保存原状态备份后注入待重建标记。
该夹具不得用于个人真实配置，也不随安装包发布。
在运行中的桌面 App 注入故障后，执行 `scenarios/desktop-manager-recovery.json`，验证不重启 App 即可发送消息、清除待重建标记并获得真实模型回复。
对照首次无模型的情况应确认明确要求配置模型，不能通过伪造默认模型或绕过连接器检查使状态变绿。

`go/runtime_metadata_writer_windows_test.go` 在原生 Win7 上并发读取和替换运行元数据，验证完整 JSON 和临时文件清理。
停止回归还应覆盖元数据写入晚于原三秒期限时的等待、调用方取消，以及默认工作区沙箱中的取消、超时和进程回收。
## Win7 外层 Job 启动回归

`job/main_windows.go` 是使用外层 Job 启动桌面 App 的测试夹具，必须设置指向 `C:\CSGClawTest\` 独立配置的 `CSGCLAW_DESKTOP_CONFIG`。
将该入口复制到准备后的 `.cache/win7/csgclaw/cmd/win7-job-launcher/main_windows.go`，在准备后的模块中用专用 Go 工具链交叉编译。
它使用本仓库准备后的进程管理实现，不属于用户安装包。
原生进程组回归运行 `TestWin7InheritedJobLifecycle`，覆盖允许显式分离、允许隐式分离和禁止分离三种策略，以及子进程停止和拥有者崩溃回收。
