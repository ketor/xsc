# Changelog

All notable changes to this project will be documented in this file.

## [1.4.0] - 2026-05-05

### Fixed
- **认证**: 修复 SecureCRT/XShell/MobaXterm 主密码无法从 `~/.xsc/config.yaml` 读取的问题。
  v1.3.0 安全加固提交将 `password` 字段标为 `yaml:"-"`（不持久化到磁盘），但
  没有提供其他读取路径，导致用户在 YAML 里写的 `password:` 被静默丢弃，连接时报
  `master password not set for decryption`。

### Added
- **认证**: 主密码新优先级（高到低）：源特定环境变量 `XSC_SECURECRT_PASSWORD` /
  `XSC_XSHELL_PASSWORD` / `XSC_MOBAXTERM_PASSWORD` → `~/.xsc/config.yaml` 中的
  `password:` 字段 → 通用 `XSC_MASTER_PASSWORD` 兜底 → TTY 交互式提示。
- **认证**: TTY 场景下，启用了导入源但密码仍为空时，`xssh tui/connect/exec/ping/import-*`
  和 `xftp tui/connect/ls/cat/get/put/...` 在执行前会无回显地提示输入主密码。
- **认证**: ProxyJump 跳板机连接支持（来自 v1.3.0 后续提交合并发布）。
- **认证**: 交互式密码输入支持（来自 v1.3.0 后续提交合并发布）。

### Changed
- **认证**: `SecureCRTConfig` / `XShellConfig` / `MobaXtermConfig` 的 `Password`
  字段恢复 `yaml:"password,omitempty"` 标签，但通过自定义 `MarshalYAML` 保证
  `SaveGlobalConfig` **永不写出 password 到磁盘**——读入和持久化解耦。
- **MobaXterm 解析**: INI 文件编码自动检测（GBK 等非 UTF-8 编码）。

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
