# 兼容构建与 Tag 发布

Win7 构建、测试和恢复实现位于 `compat/win7/`。
`.github/workflows/release.yml` 是用户授权的唯一 CI 入口例外，负责调度兼容构建，其他根目录文件与上游一致。
GitHub Actions 只加载 `.github/workflows/`，不得再次将该入口移到 `compat/` 或替换为上游普通桌面发布流程。

推送版本 tag 自动构建 Win7 x64、麒麟 V10 amd64 和 arm64 三个包，并在全部成功后发布到该 tag 的 Release。
Release 包含安装包、逐目标 SHA-256 校验表、Win7 构建记录及麒麟 ELF 审计记录。

`release beta` 先发布主仓库 beta tag，再同步 Win7，并在最新 Win7 main 上推送同名 tag。
正常命令以两个远端 tag 校验完成为结束条件，不等待 CI。
用户要求修复发布失败时，须跟进三平台构建与发布并核验资产。

## 已有 tag 的恢复

在 main 上手动运行 Release，指定原 `version`、`publish=true` 和 `target=all`。
工作流使用当前 main 的恢复逻辑，但各构建 checkout 的源码固定为原 tag 的完整 commit。
不存在的 tag 或只构建单个目标时禁止发布，原 tag 不移动，历史验收结果保持原始范围。
不设置 `publish` 时只生成当前工作流提交的预览构建及 Actions artifacts，不创建 Release。

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
