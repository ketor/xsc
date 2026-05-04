# Changelog

All notable changes to this project will be documented in this file.

## [1.4.2] - 2026-05-05

v1.4.1 修复后续的护栏加固。无 production 行为改动（除 Validate 多接受 4 个 SSH
密钥算法别名）。所有 v1.4.1 用户可平滑升级。

### Added
- **认证**: `Session.Validate()` 现在接受更完整的 SSH 标准/SecureCRT 别名集合。除
  v1.4.1 已支持的 `publickey` 外，新增 `rsa`、`dsa`、`ecdsa`、`ed25519` 也都规范化
  为内部规范的 `key`。SecureCRT 的 `Authentication` 字段把这些算法名当作 publickey
  的同义词使用，用户手写 YAML 时也常这么写。
- **测试**: 新增 import round-trip 端到端防回归测试
  （`TestImportRoundTrip_PreservesFieldsThroughDisk` /
  `TestImportRoundTrip_PublicKeyAuth`）。覆盖
  `buildXSSHSessionFromImport → SaveSession → LoadSession → 字段相等性`
  完整链路，捕获 YAML 序列化层的字段丢失回归（v1.4.1 修复的 ceeb45b 那种类型）。
  反向验证有效性：临时退化 helper 后两个测试均能给出明确失败信息。
- **测试**: 新增 `TestSessionValidateAuthTypeAliases`（表驱动 5 子测试）覆盖每个
  别名分别独立验证。

### Changed
- **重构**: `Session.Validate()` 把硬编码的 `publickey` 别名比较抽出成 `keyAuthAliases`
  map，便于后续添加更多 SSH 标准术语而不修改控制流。

## [1.4.1] - 2026-05-05

### Fixed
修复了 4 个 TUI 显示回归 bug。根因是 commit `ceeb45b`（2026-03-08）"fix: 9 xftp/xssh issues" 中
`convertSessions` 重写时丢字段，以及 `Session.Validate()` 不接受 SSH 协议标准术语 `publickey`。

- **TUI**: YAML session 写 `auth_type: publickey` 时，TUI 不再误显示 `[invalid]`。
  `Session.Validate()` 现在接受 `publickey` 作为 `key` 的别名（这是 SSH 协议和 SecureCRT
  导出器都使用的标准术语），并自动规范化为内部规范的 `key`。
- **TUI**: `auth_type: key` 不再强制要求 `key_path`。当 `key_path` 为空时，连接层
  （`ssh/client.go findDefaultSSHKeys`）自动回退到 `~/.ssh/` 下的默认密钥（id_ed25519/id_rsa
  等），与 OpenSSH 的 `ssh user@host` 默认行为一致。
- **xssh import-***: 修复导入命令丢失多种字段的问题：
  - **AuthMethods 完整列表**：原先 SecureCRT 多种认证方式（password/publickey/keyboard-interactive/gssapi）
    在导入后只剩一种，现在按顺序完整保留。
  - **解密后的密码明文**：原先只在单认证场景写入顶层 `password`，多认证场景下密码丢失。
    现在 `auth_methods` 中的 password 项也带解密后明文，TUI `:pw` 切换可正常显示。
  - **KeyPath**：原先 publickey 认证的密钥路径没保留，现在从 sessionData 透传到 YAML。

### Added
- 5 个新单元测试覆盖回归：
  - `pkg/session`: `TestSessionValidatePublicKeyAuthAccepted`、`TestSessionValidatePublicKeyAuthMissingKeyPathAccepted`
  - `cmd/xssh`: `TestBuildXSSHSessionFromImport_*` 系列（AuthMethods/Password/KeyPath/单认证保留）
  - `internal/tui`: `TestRender_PublicKeySessionNotInvalid`、`TestRender_MultiAuthMethodsAllVisible`、`TestRender_PasswordToggleShowsPlaintext`

### Migration
**已通过旧版本 `xssh import-securecrt` 导入过 SecureCRT 会话的用户，需要重新跑一次导入**
（`xssh import-securecrt`），新版本会写入完整的 YAML 字段。旧的导入产物 YAML 仍可工作，
但 TUI 里只能看到一种认证方式。

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
