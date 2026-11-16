# 兼容构建与 Tag 发布

所有 Win7 专属跟踪改动限定在 `compat/`，根目录 `.github/workflows/` 与当前上游 main 完全一致。
原兼容工作流保存在 `compat/win7/workflows/release.yml`，作为已有构建矩阵和发布步骤的模板。
GitHub 不会从 `compat/` 自动加载工作流，因此推送 tag 不再通过该模板触发 Win7 或麒麟安装包构建。

`release beta win7` 先同步最新上游 main 并完成受影响检查，然后在 Win7 main 的最新提交上递增 beta 编号并推送标签。
该命令以远端标签指向预期提交为完成条件，不手动创建 Release、上传安装包或等待 CI。
旧标签及其安装包保留原始来源和验收记录。

`compat/win7/ci.py` 保留 Win7 x64、麒麟 V10 amd64 和 arm64 三个兼容构建目标。
需要构建安装包时，使用这些入口及固定依赖，不能把上游普通 Windows 产物视为 Win7 产物。

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
