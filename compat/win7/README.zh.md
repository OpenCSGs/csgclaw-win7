# CSGClaw Win7 桌面版

## TLDR

CSGClaw 业务源码保留在仓库原有目录，Win7 专用构建、补丁和测试集中在 `compat/win7`。
通过专用入口构建安装包后，用户双击 `Setup.exe` 安装，再从桌面快捷方式打开 CSGClaw。
本轮范围是 Electron 桌面壳、Go 服务和 Codex；DSH/Node 引擎仍单列适配。

## 仓库结构

```text
cli/、cmd/、internal/、web/、desktop/  上游 CSGClaw，保持原路径
compat/win7/                         Win7 兼容层和维护入口
compat/win7/codex/                   已适配的 Codex 完整源码，直接在这里维护
.cache/win7/csgclaw/                 自动生成的兼容源码，禁止手动维护
.cache/win7/codex/                   从仓库内 Codex 源码导出的临时构建目录
dist/win7/                          可交付的安装 EXE、SHA-256 与构建记录
dist/win7/app/                      安装器输入目录，不作为独立分发包
```

当前业务源码同步到主仓库 main 原始提交 `0daf76b1f453fc8765d00cf83163df0544905b49`（2026-10-28）。
桌面版本仍为 `0.9.0`，安装包中的日期和源码 commit 标识本次 main 快照。
HTTP 分站点登录直接继承上游 PR #787，已移除重复的 `http-opencsg-oauth.patch`。
头像等待状态和取消、工作租约、进程清理等通用修复已由主仓库 PR #721、#722 合入，不在 Win7 补丁中重复维护。
离线运行时恢复、重建确认和连接器配置修复已由 PR #739 合入，直接继承上游实现，相关重复补丁和 engine overlay 已移除。
Git 历史保留完整主仓库历史，后续兼容改动单独使用 `win7` scope 提交，仓库根目录与该上游基线保持一致。
全部 Win7 差异集中在 `compat/win7/`，包括定制 Codex 完整源码。
后续维护和上游同步规则见 [源码维护约定](SOURCE-MAINTENANCE.zh.md)。

## 构建桌面 App

构建主机使用 Python 3.12、Node 24、pnpm 11.1.3、make 和 `sources.lock.json` 固定的 NSIS 3.13。
macOS 可通过 `brew install makensis` 安装 NSIS。
服务与 CLI 使用 XTLS/go-win7 `patched-1.27.2` 工具链，不降级业务依赖。
桌面运行时固定为 Electron 22.3.27，Codex 的适配源码完整保存在 `compat/win7/codex`，上游基线为 `rust-v0.149.1`。
具体来源、补丁和运行时校验值见 `sources.lock.json`。

```sh
python3.12 compat/win7/build-app.py \
  --go /path/to/go-win7/bin/go \
  --codex-bundle /path/to/verified/codex-win7-runtime
```

构建使用当前提交中的 Codex 源码，不再克隆上游或依赖其他本地 Codex 仓库。
`--codex-bundle` 需提供通过原生验收的 `codex.exe`、`winpty.dll`、`winpty-agent.exe` 和 `LICENSE.winpty`，脚本会校验其哈希。
需要重建 Codex 时，先运行 `prepare.py --codex`，再进入 `.cache/win7/codex`，按其中 `compat/win7/README.md` 的步骤构建并重复原生验收。
修改 Codex 时直接编辑 `compat/win7/codex` 并提交，临时构建目录不作为维护来源。
`patches/codex.patch` 仅保存首次适配的来源记录，不再重复应用。
更换运行时后须审核 `sources.lock.json`，不能用普通 Windows 最新 Codex 替换。

构建入口会准备隔离源码、安装冻结依赖、执行检查、构建 Web 资源与 Go 后台，再打包真实桌面 App。
服务由桌面程序自动启动和管理，用户不需要运行命令或打开外部浏览器。
Win7 版关闭普通 Windows 更新渠道，并保留原有安装校验逻辑及默认工作区沙箱。
安装包基于上述上游 main 快照构建；本地构建不附带 Windows 代码签名。

构建入口仅交付完整安装 EXE、SHA-256 校验文件和构建记录，不再生成运行版 ZIP。
正式构建开始前要求兼容源码已经提交；本机验收可添加 `--allow-dirty`，产物名称会明确带上 `dirty`。
安装包为当前用户安装全部依赖，创建桌面、开始菜单及控制面板卸载入口，不需要管理员权限。
卸载仅删除安装器拥有的程序文件，保留配置、账号、会话、工作区及用户自行添加的文件。
覆盖安装和卸载前必须从托盘退出 CSGClaw；程序仍占用文件时安装器会停止。

已有经过校验的桌面目录时，可单独生成安装包，无需重新编译业务代码：

```sh
python3.12 compat/win7/build-installer.py --go /path/to/pinned-win7-go/bin/go
```

此入口会验证完整分发清单、DLL 和固定运行时的 SHA-256，拒绝缺少 `ffmpeg.dll`、被修改或混入未登记文件的目录。
安装器源码及其构建输入摘要另存为 `Setup.exe.build.json`，程序本身的源码版本仍以包内 `win7-build.json` 为准。
安装包按版本号、构建日期和真实源码 commit 命名，自动识别原安装目录并提供升级或同版本修复。
升级恢复和原生验收规则见 [安装和升级](INSTALLER-UPGRADE.zh.md)。

## 同步后续上游更新

获取选定的 `upstream/main`，保留主仓库原始提交和 SHA，仅将 Win7 提交 rebase 到其后。
Win7 历史保持线性，不创建同步 merge commit。
自行创建的修复、功能和构建提交均使用 `(win7)` scope，例如 `fix(win7): ...` 或 `feat(win7): ...`。
更新 `sources.lock.json` 中的上游基线后，重新运行构建入口。
构建时补丁无法应用会明确失败，不能忽略兼容冲突。
在 `compat/win7` 中调整补丁、配置或适配模块，审核哈希更新，再执行 Win7 原生验收。
不要修改 `.cache/win7` 来交付功能，也不要将原业务目录移动到新的子目录。

`patches/previous-desktop-port.patch` 保存同事此前的 Electron 22 方案，仅用于追溯，不会再次应用。
`REPORT.zh.md` 和 `verification-20261017.json` 记录此前 Go + Codex 验收，不能替代本轮桌面结果。
上游发布工作流保留用于同步；本轮未接入 Win7 正式发布自动化，不应使用上游发布入口生成兼容包。

## 原生验收

测试必须在 Win7 SP1 x64 内执行，交叉编译不能证明原生运行成功。
`tests/guest` 管理独立测试配置、核对安装文件及验证桌面拥有者退出后的 Go 回收。
`tests/webdriver` 通过仅监听虚拟机回环地址的对应 ChromeDriver 操作实际 App，场景位于 `tests/scenarios`。
`tests/tree` 创建可核验的父子进程，用于页面 Stop 和进程回收测试。
`processgroup` 适配 Win7 启动器和 PCA 的外层 Job，仅在外层策略允许时分离子进程，再加入 CSGClaw 自己的回收 Job。
外层策略禁止分离时保留限制并报告具体原因，不使用脱离回收机制的启动方式。
`tests/job` 用外层 Job 启动真实桌面 App，验证这类环境中的重建和聊天。
这些测试夹具不随用户桌面包分发。

首次验收应使用独立配置和用户数据目录，明确区分全新启动、已有配置、窗口关闭到托盘与完整退出。
对话完成须同时验证持久化 turn 终态和实际工具输出，取消须持有父子进程句柄并检查退出。
桌面测试还检查窗口受限尺寸、文件预览、内置浏览器隔离，以及 CLI 返回本应用 API 的调用链。

原生 Runtime 生命周期测试使用生成源码内的 Windows 测试入口。
该项验证包括取消、超时、后台任务、app-server 退出回收，以及后续会话恢复。
第三方登录、MCP 云服务和外部工具按依赖与凭据条件验收，不能用本地夹具宣称全部线上服务通过。

最新 main 同步结果见 [本轮回归记录](UPDATE-MAIN-20261019.zh.md)。
初始桌面验收见 [桌面验收报告](DESKTOP-ACCEPTANCE.zh.md)，复跑方法见 [原生测试说明](tests/README.zh.md)。

准备回复阶段的头像回归结果见 [验收记录](desktop-verification-pending-20261019.json)。
该项在 Win7 原生桌面 App 中单独验证工作状态先于进度消息到达的情况。
