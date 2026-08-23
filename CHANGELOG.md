# Changelog

All notable changes to this project will be documented in this file.

## [2.0.1] - 2026-08-23

### Added
- xssh 与 xftp TUI 顶部菜单栏右侧常驻显示当前构建版本；窄终端下自动截断，不改变菜单命中坐标或布局。

### Fixed
- 修复 xftp TUI 在窄窗口、长状态警告或展开顶部菜单时总高度超过终端，导致顶部内容被终端裁剪；所有全宽栏按样式边框/内边距计算内容宽度，并强制保持单行。
- 修复从会话选择器进入双面板后，面板标题 padding、选择器边框 padding 和长中文路径/文件名使横向宽度超过终端，引发终端自动换行并把上部 UI 挤出视口。
- 增加选择器、双面板和 xssh 在小尺寸终端下不超过终端宽高的回归测试。
- xftp 文件右键菜单补齐 Space 多选/取消、y 标记传输、p 粘贴/传输、D 删除、r 重命名、m 创建目录、R 刷新，并修正中文菜单项的鼠标命中宽度。
- 参考 tsh-go dashboard：xssh/xftp 顶部菜单改为带边框的 ANSI 安全悬浮弹窗，按矩形区域覆盖内容并保留弹窗左右的底层画面；展开/关闭不改变内容高度、面板尺寸、滚动位置或状态栏位置。

## [2.0.0] - 2026-08-23

### Added
- xssh 和 xftp 顶部下拉菜单栏，支持鼠标点击、悬停切换、菜单项点击和 F10/方向键导航。
- xssh 目录右键菜单、详情面板独立滚轮；xftp 菜单覆盖文件、视图和传输操作。
- 会话路径穿越、符号链接逃逸、模糊匹配歧义和原子传输回归测试。

### Changed
- SSH 非交互连接统一使用 context，覆盖 TCP、ProxyJump 和 SSH 握手超时。
- 单认证和多认证统一默认密钥回退、ProxyJump、环路检测及主机密钥验证。
- xssh exec 使用有界输出缓冲，取消后等待 SSH 执行退出，避免竞争和内存失控。
- xftp 上传、下载和远程复制改为临时文件提交，失败时保留原目标。
- xssh/xftp 子命令返回退出码；仅 main 调用 `os.Exit`，确保 defer 清理资源。
- 会话和全局配置启用严格 YAML 字段检查；全局配置改为原子保存。

### Fixed
- 修复 `xssh edit` 模糊匹配后写入错误路径并产生重复会话。
- 修复 key 认证未配置 `key_path` 时未使用默认 SSH 密钥。
- 修复 `strict_host_key: true` 在 known_hosts 异常时静默关闭验证。
- 修复 ping/exec 超时未覆盖 SSH 握手以及取消后残留连接。
- 修复外部会话加载错误被静默忽略；可选外部源不可用时改为状态栏非阻塞警告，不再遮挡其余可用会话。

### Removed
- 删除 xsc-mcp 可执行程序、MCP 工具实现、依赖、构建目标和活动文档。

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
