# XSC - XShell CLI

基于 Go 和 Bubble Tea 开发的 SSH 会话管理工具。

## 特性

- 🗂️ 文件即会话：通过 YAML 文件管理 SSH 配置
- 🖥️ 优雅的 TUI：使用 Bubble Tea 构建的交互式界面
- 🔍 实时搜索：支持会话名称模糊匹配
- 🌳 树形结构：支持无限层级目录组织
- 🔐 多种认证：支持密码、密钥、SSH Agent
- 📱 原生体验：使用 Go 原生 SSH 客户端连接，无需依赖外部工具
- 📜 自动滚屏：列表内容超出屏幕时自动滚动保持光标可见
- 🔗 SecureCRT 集成：支持导入 SecureCRT 会话配置

## 安装

```bash
cd cmd/xsc
go build -o xsc
```

## 使用

### 显示帮助
```bash
xsc
# 或
xsc help
```

### TUI 模式
```bash
xsc tui
```

### CLI 模式
```bash
# 列出所有会话
xsc list

# 连接到指定会话
xsc connect prod/db/master

# 模糊匹配连接
xsc connect web-server

# 导入 SecureCRT 会话
xsc import-securecrt
```

### TUI 快捷键

#### 导航 (Vim 风格)
| 按键 | 功能 |
|------|------|
| `↑/k` | 向上移动 |
| `↓/j` | 向下移动 |
| `PgUp/Ctrl+b` | 向上整页翻页 |
| `PgDn/Ctrl+f` | 向下整页翻页 |
| `Ctrl+u` | 向上半页翻页 |
| `Ctrl+d` | 向下半页翻页 |
| `gg` / `Home/g` | 跳转到第一行 |
| `G` / `End` | 跳转到最后行 |
| `nG` / `:n` | 跳转到第 n 行（如 `42G` 或 `:42`） |
| `0` | 跳转到第一行 |
| `$` | 跳转到最后行 |
| `^` | 跳转到第一个会话文件 |
| `n` | 查找下一个匹配 |
| `N` | 查找上一个匹配 |
| `:q` / `:quit` | 退出程序 |
| `Ctrl+c` | 退出程序 |

> **注意**: 当列表内容超出屏幕时，会自动滚屏保持光标可见

#### 操作
| 按键 | 功能 |
|------|------|
| `Enter` | 连接选中会话 |
| `Space` | 展开/折叠目录 |
| `/` | 进入搜索/过滤模式 |
| `:` | 进入命令模式 |
| `n` | 新建会话 |
| `e` | 编辑选中会话 |
| `c` | 重命名会话 |
| `D` | 删除选中会话（需输入 YES 确认） |
| `?` | 显示快捷键帮助（任意键退出） |

#### 目录折叠 (Vim 风格)
| 按键 | 功能 |
|------|------|
| `o` | 展开/折叠当前目录 |
| `h/←` | 折叠当前目录或跳转到父目录 |
| `l/→` | 展开当前目录 |
| `E` | 展开所有目录 |
| `C` | 折叠所有目录 |

#### 搜索模式
在搜索模式下输入关键词可**实时过滤**会话列表：

**搜索时按键：**
| 按键 | 功能 |
|------|------|
| `Enter` | 确认搜索并退出搜索模式 |
| `Esc` | 取消搜索并**清空**过滤条件 |
| `Ctrl+c` | 退出搜索模式但**保留**过滤结果 |
| `Ctrl+u` | 清空当前输入内容（Vim 风格） |

**已确认搜索后（普通模式）：**
| 按键 | 功能 |
|------|------|
| `Esc` | 直接清空当前搜索过滤，显示全部会话 |

**搜索结果管理：**
- 底部状态栏显示 `Filter: '关键词' (匹配数)` 表示当前有过滤
- 使用 `:noh` 命令可清除过滤恢复显示全部会话
- 搜索是实时过滤，输入时即时显示匹配结果

#### 命令模式 (:)
按 `:` 进入命令模式（类似 Vim 的命令行）：

| 命令 | 功能 |
|------|------|
| `:q` / `:quit` | 退出程序 |
| `:noh` / `:nohlsearch` | 清除搜索过滤 |
| `:<number>` | 跳转到第 n 行（如 `:42` 跳转到第 42 行） |

#### 状态栏说明
TUI 底部状态栏显示当前状态信息：

- **`Session: xxx`** - 当前选中的会话名称（仅当选中会话时显示）
- **`Total: N`** - 当前可见节点总数（会话+目录）
- **`Filter: 'xxx' (N)`** - 搜索过滤状态（仅当有过滤时显示）
- **`Press ? for help, :q or Ctrl+c to quit`** - 操作提示

## 会话配置

会话文件存储在 `~/.xsc/sessions/`，格式如下：

```yaml
host: "192.168.1.100"       # 必填
port: 22                    # 默认 22
user: "root"                # 默认当前用户
auth_type: "password"       # password | key | agent
password: "my_secret"       # auth_type=password 时必填
key_path: "~/.ssh/id_rsa"   # auth_type=key 时必填
description: "生产数据库"    # 可选
```

## 目录结构

```
~/.xsc/
├── config.yaml                    # 全局配置
├── sessions/                      # 本地会话
│   ├── prod/
│   │   └── db/
│   │       ├── master.yaml
│   │       └── slave-01.yaml
│   └── staging/
│       └── web-server.yaml
└── securecrt_sessions/            # SecureCRT 会话（通过配置启用）
```

## SSH 连接行为

选中会话按 `Enter` 连接时：
- TUI 进入**暂停模式**，终端恢复到正常状态
- 使用 Go 原生 SSH 客户端建立连接
- SSH 会话中的所有快捷键都能正常使用
- 按 `Ctrl+d` 退出 SSH 或连接断开后，TUI 自动恢复并可以继续操作
- 使用 Go 原生实现，**无需依赖外部 `ssh` 命令或 `sshpass`**

### 认证方式

支持三种 SSH 认证方式：

1. **密码认证** (`password`): 直接在配置文件中存储密码，连接时自动发送
2. **密钥认证** (`key`): 使用 SSH 私钥文件，支持密码保护的密钥
3. **SSH Agent** (`agent`): 使用系统 SSH Agent（如 `ssh-agent`），无需密码

建议优先使用 **SSH Key** 或 **SSH Agent** 方式以获得更好的安全性。

## SecureCRT 集成

xsc 支持直接导入 SecureCRT 的会话配置。

### 配置方法

编辑 `~/.xsc/config.yaml`：

```yaml
securecrt:
  enabled: true                              # 启用 SecureCRT 集成
  session_path: "/path/to/securecrt/sessions" # SecureCRT 会话目录
  password: "your_master_password"           # 用于解密会话密码的主密码
```

### SecureCRT 会话路径

不同平台的默认路径：
- **macOS**: `~/Library/Application Support/VanDyke/SecureCRT/Config/Sessions`
- **Windows**: `%APPDATA%\VanDyke\Config\Sessions`
- **Linux**: `~/.vandyke/SecureCRT/Config/Sessions`

### 导入 SecureCRT 会话

将 SecureCRT 会话转换为 xsc 格式并保存：

```bash
xsc import-securecrt
```

转换后的会话会保存在 `~/.xsc/sessions/securecrt-converted/YYYYMMDD-HHMMSS/` 目录下，保持原有的目录结构。

### 注意事项

1. SecureCRT 的密码是加密的，需要提供正确的主密码才能解密
2. 启用后，SecureCRT 的会话会显示在 TUI 中作为独立的 `securecrt` 目录
3. 如果不需要密码解密，可以不设置 `password` 字段，此时会使用 agent 认证
4. 使用 `import-securecrt` 命令可以将所有 SecureCRT 会话永久转换为 xsc 本地格式

## 全局配置

xsc 支持全局配置文件 `~/.xsc/config.yaml`：

```yaml
securecrt:
  enabled: true
  session_path: "/Users/david/.xsc/securecrt_sessions"
  password: "your_master_password"
```

- 首次运行时会自动创建 `~/.xsc/` 目录
- 配置文件权限建议设置为 `0600`（仅用户可读写）

## 错误处理

当连接失败时，TUI 会显示错误弹窗（红色边框）：
- 显示详细的错误信息
- 按任意键关闭弹窗继续操作
- 如果是配置错误，可以使用 `e` 键编辑会话
