# Tag 发布 CI

GitHub Actions 入口为 `.github/workflows/release.yml`，具体构建、依赖锁定、兼容检查和测试保存在 `compat/win7/`。
该入口是根目录与已采用上游版本保持一致规则的明确例外。
它替代本仓库继承的普通 Windows/macOS 发布矩阵，避免同一个 tag 同时发布不适合 Win7 的安装包。

推送 `vX.Y.Z` 或 `vX.Y.Z-beta.N` 等语义化版本 tag，会并行生成以下三个包，并在全部成功后附到对应的 GitHub Release。

| 目标 | 产物 | 构建环境 |
| --- | --- | --- |
| Win7 SP1 x64 | `csgclaw-desktop_<tag>_win7_amd64.exe` | macOS 15 交叉编译，固定 Win7 Go SDK、Electron 22、定制 Codex/WinPTY、NSIS 3.13 |
| 麒麟 V10 amd64 | `csgclaw-desktop_<tag>_kylin-v10_amd64.deb` | Ubuntu 24.04 amd64 |
| 麒麟 V10 arm64 | `csgclaw-desktop_<tag>_kylin-v10_arm64.deb` | Ubuntu 24.04 arm64 |

Release 同时包含 SHA-256 校验表、Win7 构建记录和麒麟 ELF 审计记录。
预发布 tag 生成 prerelease，稳定 tag 生成普通 Release。
手动运行同一 workflow 可以指定版本及单个构建目标，运行结果仅保留为 Actions artifacts，不创建 tag 或 Release。
Win7 仓库统一在 `main` 维护适配、连接修复和发布工作流，版本 tag 从已验证的 `main` 提交创建。
不需要为每个 tag 修改 `sources.lock.json` 的默认版本；CI 通过 `CSGCLAW_WIN7_VERSION` 传递已校验版本，源码来源及校验值保持不变。

## 固定依赖

Win7 使用 `ci-lock.json` 中指定的独立运行时依赖 Release，不从 `latest` 或普通 Windows Codex 包替代。
下载压缩包后先验证 SHA-256，再逐个验证 `sources.lock.json` 中的运行时文件哈希和大小。
Go SDK 同样按平台校验归档哈希及 Win7 源码指纹。
缓存只保存下载归档，使用前重新校验，再生成构建目录。
更新定制运行时必须先完成 Win7 原生验收，再发布新的依赖资产并更新锁定记录。

## 麒麟兼容检查

后端以 `CGO_ENABLED=0` 构建，避免宿主 Ubuntu 的 glibc 版本被链接进 Go 服务。
Codex 保留主仓库完整 Linux 包中的辅助程序、资源与 `bwrap`，不只复制一个可执行文件。
CI 检查包内每个 ELF 的架构、glibc 和 libstdc++ 符号需求，最高分别为 GLIBC 2.31 和 GLIBCXX 3.4.28。
DEB 的 control/data 使用 xz 压缩，兼容旧版 dpkg。
每个架构在固定 Ubuntu 20.04 / glibc 2.31 容器中安装包，运行服务与 Codex 的版本检查，并用 Xvfb 做桌面启动检查。
容器启动参数中的 `--no-sandbox` 仅用于隔离的 smoke 检查，不写入发布包的启动器或生产设置。
这属于 ABI 基线与容器安装验收，不代表麒麟原生硬件或虚拟机已验收。

两个兼容目标均保留本地构建升级保护；麒麟桌面使用系统包管理器更新，Win7 继续使用兼容安装包。
本流水线不上传主仓库 OSS 更新渠道、不构建云端镜像，也不修改 CSGHub PVC 的服务包和升级路径。

源码导出和补丁应用固定 LF 行尾，校验输入在原生 Windows checkout 时也保持固定字节。
测试进程在启动时固定为 Asia/Shanghai。
Win7 使用已验证的 macOS 构建主机执行全量 Go、前端与桌面测试，再以固定 Win7 SDK 交叉编译。
这不会将现代 Windows runner 的测试误称为 Win7 原生验收。
