# Agent Instructions for XSC

XSC (XShell CLI) 是一个基于 Go 和 Bubble Tea 开发的 SSH 会话管理工具，提供 TUI 界面和 CLI 模式两种使用方式。

## 项目概述

XSC 是一个 SSH 会话管理器，主要功能包括：
- 通过 YAML 文件管理 SSH 配置（文件即会话）
- 提供优雅的 TUI 界面（使用 Bubble Tea）
- 支持无限层级的目录树组织
- 实时搜索和过滤会话
- 支持三种认证方式：密码、密钥、SSH Agent
- 原生 Go SSH 客户端，无需外部依赖
- SecureCRT 会话导入和解密

## 技术栈

- **语言**: Go 1.21+
- **TUI 框架**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) v0.25.0
- **UI 组件**: [Bubbles](https://github.com/charmbracelet/bubbles) v0.18.0
- **样式**: [Lipgloss](https://github.com/charmbracelet/lipgloss) v0.9.1
- **SSH 客户端**: golang.org/x/crypto v0.21.0
- **终端控制**: golang.org/x/term v0.18.0
- **配置格式**: YAML (gopkg.in/yaml.v3)

## 项目结构

```
.
├── cmd/xsc/                    # 应用程序入口
│   └── main.go                 # 主程序：命令解析和分发
├── internal/                   # 私有包（不可被外部导入）
│   ├── session/                # 会话管理
│   │   ├── session.go          # Session 结构体、加载、保存、验证
│   │   └── tree.go             # 会话树结构、节点操作
│   ├── ssh/                    # SSH 连接逻辑
│   │   └── client.go           # SSH 客户端、三种认证方式
│   ├── tui/                    # TUI 界面实现
│   │   └── tui.go              # Bubble Tea 模型、视图、更新逻辑
│   └── securecrt/              # SecureCRT 集成
│       ├── parser.go           # SecureCRT 会话解析、密码解密
│       └── bcrypt_pbkdf.go     # bcrypt_pbkdf 密钥派生实现
├── pkg/config/                 # 公共配置包
│   └── config.go               # 全局配置、路径管理
├── specs/                      # 功能规范
│   ├── README.md               # 规范说明
│   └── xsc.feature             # Gherkin 格式功能规范
├── tests/                      # 测试文档
│   └── README.md               # 测试说明和示例
├── src/                        # 源码说明
│   └── README.md               # 开发原则
├── go.mod                      # Go 模块定义
├── go.sum                      # Go 依赖校验
├── Makefile                    # 构建脚本
└── README.md                   # 用户文档（中文）
```

## 构建命令

```bash
# 构建二进制文件到 build/ 目录
make build

# 运行应用程序（TUI 模式）
make run
# 或
make tui

# 列出所有会话
make list

# 安装到 /usr/local/bin
make install

# 卸载
make uninstall

# 清理构建产物
make clean

# 下载并整理依赖
make deps

# 开发模式（需要安装 air）
make dev
```

## 测试命令

```bash
# 运行所有测试
make test
# 等价于: go test -v ./...

# 运行特定包的测试
go test -v ./internal/session/...
go test -v ./internal/ssh/...
go test -v ./internal/securecrt/...

# 运行特定测试函数
go test -v ./internal/securecrt/... -run TestDecryptPasswordV2Real

# 带覆盖率报告
go test -v -cover ./...
```

## 代码风格命令

```bash
# 格式化所有 Go 代码
make fmt
# 等价于: go fmt ./...

# 静态分析
make vet
# 等价于: go vet ./...

# 完整检查（格式化 + 静态分析 + 测试）
make fmt && make vet && make test
```

## 代码风格指南

### 包组织
- 标准库导入在前
- 第三方包导入在中间
- 本地包导入在后，使用完整模块路径

```go
import (
    "fmt"
    "os"

    "github.com/charmbracelet/bubbletea"
    "golang.org/x/crypto/ssh"

    "github.com/user/xsc/internal/session"
    "github.com/user/xsc/pkg/config"
)
```

### 命名规范
- 导出（Public）: PascalCase (`AuthType`, `LoadSession`)
- 未导出（Private）: camelCase (`connectWithPassword`, `handleWindowResize`)
- 常量: PascalCase 或 camelCase (`AuthTypePassword`)
- 接口名: 使用 -er 后缀 (`Reader`, `Writer`)
- 缩写词: 全大写 (`SSH`, `TUI`, `CRT`)

### 类型定义
- 使用 struct tags 进行 YAML 序列化: `` `yaml:"field_name,omitempty"` ``
- 内部字段使用 `yaml:"-"` 排除序列化
- 使用自定义类型定义枚举: `type AuthType string`

### 错误处理
- 使用 `fmt.Errorf("context: %w", err)` 包装错误
- 使用 `%w` 动词进行错误链传递（而非 `%v`）
- 尽早验证输入并返回描述性错误
- 返回错误而非记录并继续

### 注释规范
- 代码注释使用中文（现有代码风格）
- 导出函数使用 `// FunctionName ...` 格式文档化
- 使用 `// TODO:` 标记未完整实现的功能

## 会话配置格式

会话文件存储在 `~/.xsc/sessions/`，使用 YAML 格式：

```yaml
host: "192.168.1.100"       # 必填：目标主机
port: 22                    # 可选：端口（默认 22）
user: "root"                # 可选：用户名（默认当前用户）
auth_type: "password"       # 可选：认证类型（password/key/agent，默认 agent）
password: "my_secret"       # auth_type=password 时：密码
key_path: "~/.ssh/id_rsa"   # auth_type=key 时：私钥路径
description: "生产数据库"    # 可选：描述
```

### 认证类型
1. **password**: 密码认证，连接时自动发送密码
2. **key**: 密钥认证，使用 SSH 私钥文件
3. **agent**: SSH Agent 认证，使用 ssh-agent

## 全局配置

配置文件路径：`~/.xsc/config.yaml`

```yaml
securecrt:
  enabled: true                               # 启用 SecureCRT 集成
  session_path: "/path/to/securecrt/sessions" # SecureCRT 会话目录
  password: "your_master_password"            # 解密密码的主密码

ssh:
  strict_host_key: false                      # 是否启用严格主机密钥验证
  known_hosts_file: "~/.ssh/known_hosts"      # known_hosts 文件路径
```

## TUI 快捷键

### 导航（Vim 风格）
| 按键 | 功能 |
|------|------|
| `↑/k`, `↓/j` | 上/下移动 |
| `PgUp/Ctrl+b`, `PgDn/Ctrl+f` | 整页翻页 |
| `Ctrl+u`, `Ctrl+d` | 半页翻页 |
| `gg` / `Home/g` | 跳转到第一行 |
| `G` / `End` | 跳转到最后行 |
| `nG` / `:n` | 跳转到第 n 行 |
| `0`, `$` | 跳转到首行/末行 |
| `^` | 跳转到第一个会话文件 |
| `n` / `N` | 查找下一个/上一个匹配 |

### 目录操作
| 按键 | 功能 |
|------|------|
| `Space`, `o` | 展开/折叠当前目录 |
| `h/←` | 折叠目录或跳转到父目录 |
| `l/→` | 展开目录 |
| `E` | 展开所有目录 |
| `C` | 折叠所有目录 |

### 搜索
| 按键 | 功能 |
|------|------|
| `/` | 进入搜索模式 |
| `Enter` | 确认搜索 |
| `Esc`（搜索模式） | 取消搜索并清空过滤 |
| `Ctrl+c`（搜索模式） | 退出搜索但保留过滤 |
| `Esc`（普通模式有过滤） | 清空搜索过滤 |
| `:noh` | 清除搜索高亮 |

### 会话操作
| 按键 | 功能 |
|------|------|
| `Enter` | 连接选中会话 |
| `n` | 新建会话 |
| `e` | 编辑选中会话 |
| `c` | 重命名会话 |
| `D` | 删除选中会话（需输入 YES 确认） |
| `c` | 重命名会话 |

### 命令模式 (:)
| 按键 | 功能 |
|------|------|
| `:q` / `:quit` | 退出程序 |
| `:noh` | 清除搜索高亮 |
| `:pw` | 切换密码明文/密文显示 |

### 退出
| 按键 | 功能 |
|------|------|
| `:q` / `:quit` | Vim 风格退出 |
| `Ctrl+c` | 强制退出 |
| `?` | 显示帮助 |

### TUI 详情面板
- 显示会话的多种认证方式及顺序（SecureCRT 风格）
- 支持密码明文/密文切换显示（`:pw` 命令）
- 认证类型图标：🔑 Password、🔐 Public Key、🤖 SSH Agent、⌨️ Keyboard Interactive、🎫 GSSAPI
- Public Key 显示密钥路径或 `(global)` 表示使用默认密钥

## SecureCRT 集成

### 支持的功能
- 解析 SecureCRT `.ini` 会话文件
- 解密 V2 格式加密密码（prefix 02 和 03）
- 支持多种认证方式按优先级顺序尝试（password、publickey、keyboard-interactive、gssapi）
- 自动发现默认 SSH 密钥：当 SecureCRT 使用全局公钥时，自动查找 `~/.ssh/` 下的默认密钥（id_ed25519、id_ecdsa、id_rsa 等）
- 延迟解密：密码在需要时才解密，提高启动速度
- 导入命令将 SecureCRT 会话转换为本地 YAML 格式

### 密码解密算法
- **Prefix 02**: SHA256(passphrase) 作为 AES-256-CBC 密钥，IV 全零
- **Prefix 03**: bcrypt_pbkdf2 派生密钥（32字节）和 IV（16字节）
- **格式**: LVC (Length-Value-Checksum) 格式，带 SHA256 校验

### 默认会话路径
- macOS: `~/Library/Application Support/VanDyke/SecureCRT/Config/Sessions`
- Windows: `%APPDATA%\VanDyke\Config\Sessions`
- Linux: `~/.vandyke/SecureCRT/Config/Sessions`

## 开发注意事项

### SSH 连接实现
- 使用 `tea.Exec()` 暂停 TUI 并执行 SSH 连接
- SSH 连接结束后自动恢复 TUI
- 支持窗口大小调整（SIGWINCH 信号处理）
- 支持终端 raw 模式切换
- 连接超时：10 秒超时防止卡住（使用 `net.DialTimeout`）
- 多认证方式：按配置顺序尝试多种认证方式（password、key、agent、keyboard-interactive）

### 性能优化
- SSH Agent 密钥缓存：详情面板中缓存 Agent 密钥查询结果
- 延迟密码解密：SecureCRT 密码在需要时才解密
- 虚拟滚动：只渲染可见节点，支持大量会话（>1000）

### 安全考虑
- 会话文件权限：保存时设置为 0600（仅用户可读写）
- 配置文件权限：建议设置为 0600
- 主机密钥验证：可配置严格模式（使用 known_hosts）
- 默认禁用严格主机密钥验证，使用 `ssh.InsecureIgnoreHostKey()`

### 测试策略
- 单元测试位于与源码相同的包中，命名 `*_test.go`
- SecureCRT 解密功能有实际测试用例（需要真实密码）
- 规范文档使用 Gherkin 格式，位于 `specs/xsc.feature`
- 测试应保持与规范同步

## 扩展开发

### 添加新的认证方式
1. 在 `internal/session/session.go` 中添加新的 `AuthType` 常量
2. 在 `Session.Validate()` 中添加验证逻辑
3. 在 `internal/ssh/client.go` 中实现连接逻辑
4. 在 TUI 详情面板中添加显示逻辑

### 添加新的命令
1. 在 `cmd/xsc/main.go` 的 `switch` 语句中添加新命令
2. 实现对应的处理函数
3. 更新 `showHelp()` 显示帮助信息

### 修改 TUI 界面
- 主模型逻辑在 `internal/tui/tui.go`
- 样式定义在文件顶部
- 按键映射在 `KeyMap` 结构体和 `DefaultKeyMap()` 函数
- 视图渲染在 `View()` 方法和各 `renderXxx()` 函数

## 依赖管理

```bash
# 查看依赖
go list -m all

# 更新依赖
go get -u ./...
go mod tidy

# 验证依赖
go mod verify
```
