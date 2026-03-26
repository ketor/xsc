# Changelog

All notable changes to this project will be documented in this file.

## [0.2.0.0] - 2026-03-27

### Added
- **xssh**: 新增 `add` 命令 - 通过 CLI 创建会话
- **xssh**: 新增 `show` 命令 - 查看会话详情
- **xssh**: 新增 `edit` 命令 - 修改会话字段（部分更新、原子写入）
- **xssh**: 新增 `delete` 命令 - 删除本地会话
- **xssh**: 新增 `ping` 命令 - SSH 连通性检测（支持单个/批量并发）
- **xssh**: 新增 `list --json` - JSON 格式输出会话列表
- **xssh**: 新增 `exec` 批量模式 - 逗号分隔多路径并发执行
- **xssh**: `exec` 批量模式支持 `--fail-fast`（遇错立即停止）和 `--ignore-errors`（忽略错误）
- **xssh**: `import-*` 命令新增 `--skip-decrypt-errors` 参数
- **xftp**: 新增 `stat` 命令 - 查看远程文件/目录元数据
- **xftp**: 新增 `cp` 命令 - 远程文件复制
- **xftp**: 新增 `mv` 命令 - 远程文件移动/重命名
- **xftp**: 新增 `rename` 命令 - 重命名（mv 的别名）
- **xftp**: 所有命令支持 `--json` 参数输出结构化 JSON
- **internal/cli**: 新增共享 CLI 基础设施包
  - `Printer` - 统一的 JSON/Text 输出处理器
  - 语义化退出码常量（ExitOK, ExitUsage, ExitNotFound 等）
  - `CLIError` - 结构化错误类型
  - `WriteYAML` - 原子写入 YAML 文件
- **认证**: 支持 `XSC_MASTER_PASSWORD` 和 `XSC_PASSWORD` 环境变量

### Changed
- **xssh**: `import-*` 命令进度输出改为 stderr（stdout 保持 JSON 可解析）
- **xssh**: `import-*` 部分失败退出码从 0 改为 9 (ExitPartial)
- **xssh**: 所有命令支持 `--json` 结构化输出
- **xssh**: `edit` 命令禁止编辑非本地会话（SecureCRT/Xshell/MobaXterm）

### Fixed
- **认证**: 修复 `ResolvePassword()` 静默忽略环境变量失败的问题
