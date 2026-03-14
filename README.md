# XSC - SSH Session Manager & SFTP File Manager

基于 Go 和 Bubble Tea 开发的终端 SSH 会话管理工具套件，包含两个独立工具：

- **xssh** — TUI SSH 会话管理器，支持 Vim 风格操作
- **xftp** — TUI SFTP 双面板文件管理器

支持本地 YAML 会话配置，以及直接加载 SecureCRT、Xshell、MobaXterm 会话（含加密密码解密）。

## 特性

### 通用特性
- 🗂️ **文件即会话**：通过 YAML 文件管理 SSH 配置，目录结构即分组层级
- 🖥️ **Gruvbox 配色**：统一的暗色主题 TUI 界面
- 🔍 **实时搜索**：输入即过滤，支持会话名称和文件名模糊匹配
- 🌳 **树形结构**：支持无限层级目录组织，Vim 风格折叠操作
- 🔐 **多种认证**：支持密码、密钥、SSH Agent、keyboard-interactive
- 📱 **原生 SSH**：使用 Go 原生 SSH 客户端，无需依赖外部 `ssh` 或 `sshpass`
- 🔗 **多源集成**：直接加载 SecureCRT / Xshell / MobaXterm 会话，支持加密密码解密
- ⌨️ **命令补全**：命令模式下支持 Tab 自动补全
- 🔒 **灵活的主机密钥验证**：默认跳过验证（便捷优先），可配置启用 TOFU 安全模型

### xftp 专有特性
- 📂 **双面板布局**：左侧本地文件系统，右侧远程 SFTP 文件系统
- 📋 **Yank/Paste 传输**：Vim 风格的 `y` 标记 + `p` 粘贴进行文件传输
- 📊 **传输结果统计**：传输完成后显示目录数、文件数、总数据量
- ⚠️ **覆盖确认**：目标存在同名文件/目录时提示用户确认
- 🔄 **会话内切换**：`:q` 返回会话列表而非退出程序，可快速切换服务器

## 快速开始

### 安装

```bash
# 克隆项目
git clone <repo-url>
cd xsc

# 构建（同时构建 xssh 和 xftp）
make build          # 输出到 ./build/xssh 和 ./build/xftp

# 安装到系统（需要 root 权限）
sudo make install   # 安装到 /usr/local/bin/
```

### 创建第一个会话

```bash
# 创建会话目录
mkdir -p ~/.xsc/sessions/prod

# 创建会话文件
cat > ~/.xsc/sessions/prod/my-server.yaml << 'EOF'
host: "192.168.1.100"
port: 22
user: "root"
auth_type: "password"
password: "your_password"
description: "生产服务器"
EOF
```

### 使用 xssh（SSH 终端）

```bash
xssh                           # 显示帮助
xssh tui                       # 启动 TUI 交互界面
xssh list                      # 列出所有会话
xssh connect prod/my-server    # 直接连接指定会话
xssh connect web               # 模糊匹配连接
xssh import-securecrt          # 将 SecureCRT 会话转换为本地格式
xssh import-xshell             # 将 Xshell 会话转换为本地格式
xssh import-mobaxterm           # 将 MobaXterm 会话转换为本地格式
```

### 使用 xftp（SFTP 文件管理器）

```bash
xftp                           # 显示帮助
xftp tui                       # 启动 TUI 模式（先选择会话再连接）
xftp <session-path>            # 直接连接指定会话并打开 SFTP
xftp connect web-server        # 连接指定会话
```

## 会话配置

会话文件存储在 `~/.xsc/sessions/`，使用 YAML 格式。目录结构直接映射为 TUI 中的树形层级。

### 配置字段

```yaml
host: "192.168.1.100"       # 必填，SSH 主机地址
port: 22                    # 可选，默认 22
user: "root"                # 可选，默认当前系统用户
auth_type: "password"       # 可选，默认 "agent"，可选值：password | key | agent
password: "my_secret"       # auth_type=password 时必填
key_path: "~/.ssh/id_rsa"   # auth_type=key 时必填，支持 ~ 展开
description: "生产数据库"    # 可选，会话描述信息
```

### 认证方式

| 认证方式 | auth_type | 必填字段 | 说明 |
|---------|-----------|---------|------|
| 密码认证 | `password` | `password` | 密码直接存储在配置文件中 |
| 密钥认证 | `key` | `key_path` | 使用 SSH 私钥，支持 ed25519/ecdsa/rsa/dsa |
| SSH Agent | `agent` | 无 | 使用系统 `ssh-agent` |

> 会话文件权限自动设为 0600（仅用户可读写）。建议优先使用 SSH Key 或 SSH Agent。

### 配置示例

**密码认证：**
```yaml
host: "10.0.1.50"
port: 22
user: "admin"
auth_type: "password"
password: "P@ssw0rd"
description: "内网跳板机"
```

**密钥认证：**
```yaml
host: "github-runner.internal"
port: 2222
user: "deploy"
auth_type: "key"
key_path: "~/.ssh/id_ed25519"
description: "CI/CD 部署服务器"
```

**SSH Agent 认证：**
```yaml
host: "bastion.example.com"
user: "ops"
auth_type: "agent"
description: "堡垒机"
```

## xssh TUI 快捷键

### 导航 (Vim 风格)
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
| `n` / `N` | 查找下一个/上一个匹配 |

### 操作
| 按键 | 功能 |
|------|------|
| `Enter` | 连接选中会话 |
| `Space` | 展开/折叠目录 |
| `/` | 进入搜索/过滤模式 |
| `:` | 进入命令模式 |
| `n` | 新建会话（在当前目录下） |
| `e` | 编辑选中会话（打开 `$EDITOR`） |
| `c` | 重命名会话 |
| `D` | 删除选中会话（需输入 YES 确认） |
| `?` | 显示快捷键帮助 |

> 导入的外部会话（SecureCRT / Xshell / MobaXterm）为只读，不支持编辑、删除和重命名。

### 目录折叠 (Vim 风格)
| 按键 | 功能 |
|------|------|
| `o` | 展开/折叠当前目录 |
| `h/←` | 折叠当前目录或跳转到父目录 |
| `l/→` | 展开当前目录 |
| `E` | 展开所有目录 |
| `C` | 折叠所有目录 |

### 搜索模式
按 `/` 进入搜索模式，输入关键词可实时过滤会话列表：

| 按键 | 功能 |
|------|------|
| `Enter` | 确认搜索并退出搜索模式 |
| `Esc` | 取消搜索并清空过滤条件 |
| `Ctrl+c` | 退出搜索模式但保留过滤结果 |
| `Ctrl+u` | 清空当前输入内容 |

已确认搜索后，按 `Esc` 可清空过滤显示全部会话。

### 命令模式 (:)
按 `:` 进入命令模式，支持 Tab 自动补全：

| 命令 | 功能 |
|------|------|
| `:q` / `:quit` | 退出程序 |
| `:noh` / `:nohlsearch` | 清除搜索过滤 |
| `:pw` / `:password` | 切换密码明文/隐藏显示（状态栏显示 `[PW]` 标记） |
| `:<number>` | 跳转到第 n 行 |

### 状态栏
- **`Session: xxx`** — 当前选中的会话名称
- **`Total: N`** — 当前可见节点总数
- **`Filter: 'xxx' (N)`** — 搜索过滤状态
- **`[PW]`** — 密码明文显示已开启

## xftp TUI 快捷键

### 会话选择器
xftp 启动时先进入会话选择器，快捷键与 xssh TUI 相同（导航、搜索、命令模式、`:pw` 切换密码显示等）。选中会话后按 `Enter` 连接并进入文件管理界面。

连接失败时会显示错误信息，按任意键返回会话列表。

### 文件管理 - 导航
| 按键 | 功能 |
|------|------|
| `↑/k` | 向上移动 |
| `↓/j` | 向下移动 |
| `PgUp/Ctrl+b` | 向上整页翻页 |
| `PgDn/Ctrl+f` | 向下整页翻页 |
| `Ctrl+u` | 向上半页翻页 |
| `Ctrl+d` | 向下半页翻页 |
| `Home/g` | 跳转到第一行 |
| `End/G` | 跳转到最后行 |
| `Tab` | 切换本地/远程面板 |
| `Enter` | 进入目录 |
| `Backspace` | 返回上级目录 |

### 文件管理 - 文件操作
| 按键 | 功能 |
|------|------|
| `Space` | 多选/取消选择 |
| `y` | 标记文件到传输缓冲区（yank） |
| `p` | 粘贴/传输到对侧面板（paste） |
| `D` | 删除选中文件（需确认） |
| `r` | 重命名文件 |
| `m` | 创建目录 |

### 文件管理 - 命令与搜索
| 按键 | 功能 |
|------|------|
| `/` | 搜索/过滤当前面板文件 |
| `:` | 进入命令模式 |
| `?` | 显示帮助 |
| `Ctrl+c` | 退出程序 |

### 文件管理 - 命令模式
| 命令 | 功能 |
|------|------|
| `:q` / `:quit` | 返回会话选择器（非退出程序） |
| `:reconnect` | 重新连接远程服务器 |

### 文件传输行为
- **Yank → Paste 流程**：先在源面板用 `y` 标记文件，切换到目标面板用 `p` 粘贴
- **多选传输**：用 `Space` 多选后再 `y` 标记，可批量传输
- **覆盖确认**：目标存在同名文件/目录时提示 `y/n` 确认（仅检查第一层，子目录直接覆盖）
- **传输结果**：传输完成后弹出统计对话框，显示目录数、文件数、总数据量和失败数

## SSH 连接行为

### xssh 连接方式
选中会话按 `Enter` 连接时：
- TUI 进入暂停模式，终端恢复到正常状态
- 使用 Go 原生 SSH 客户端建立连接，**连接超时 10 秒**
- 自动处理终端窗口大小调整（SIGWINCH）
- 按 `Ctrl+d` 退出 SSH 或连接断开后，TUI 自动恢复
- 连接失败时显示红色边框错误弹窗，按任意键关闭

### xftp 连接方式
选中会话按 `Enter` 连接时：
- 异步建立 SFTP 连接，界面显示「正在连接…」
- 连接成功后自动加载远程目录
- **连接失败**时显示错误弹窗，按任意键返回会话选择器
- 使用 `:q` 可断开连接返回会话列表，快速切换不同服务器

### 多认证方式回退

当会话配置了多种认证方式（常见于 SecureCRT 导入的会话）时，按优先级顺序逐一尝试：

```
publickey → password → keyboard-interactive → agent
```

如果使用 publickey 认证且未指定密钥文件，会自动在 `~/.ssh/` 下查找默认密钥：
`id_ed25519` → `id_ecdsa` → `id_ecdsa_sk` → `id_ed25519_sk` → `id_rsa` → `id_dsa`

### 主机密钥验证

xssh/xftp 默认**跳过主机密钥验证**，可直接连接任何主机，无需处理 `known_hosts` 冲突。

可通过配置启用 TOFU（Trust On First Use）安全模型：
- `strict_host_key` 未配置或为 `false`（默认）：跳过所有主机密钥验证
- `strict_host_key: true`：启用 TOFU 模式 — 首次连接自动信任并记录，密钥变更时拒绝连接

## SecureCRT 集成

xssh/xftp 支持直接加载 SecureCRT 的会话配置文件（`.ini` 格式），无需手动转换。启用后，SecureCRT 会话以紫色样式显示在 `[CRT] securecrt/` 目录下。

### 配置方法

找到 SecureCRT 会话目录：

| 平台 | 默认路径 |
|------|---------|
| **macOS** | `~/Library/Application Support/VanDyke/SecureCRT/Config/Sessions` |
| **Linux** | `~/.vandyke/SecureCRT/Config/Sessions` |
| **Windows** | `%APPDATA%\VanDyke\Config\Sessions` |

编辑 `~/.xsc/config.yaml`：

```yaml
securecrt:
  enabled: true
  session_path: "/home/user/.vandyke/SecureCRT/Config/Sessions"
  password: "your_master_password"    # SecureCRT 主密码（用于解密加密密码）
```

> **关于主密码**：如果你在 SecureCRT 中设置了配置加密密码（Options → Global Options → General → Use config encryption passphrase），需要填入相同的密码。未设置主密码或不需要解密密码时可留空。

### TUI 中的表现

| 特性 | 说明 |
|------|------|
| 显示样式 | 紫色文字，目录带 `[CRT]` 前缀，会话带 🔒 图标 |
| 只读模式 | 不支持编辑、删除、重命名 |
| 密码解密 | 延迟解密 — 仅在光标选中时实时解密，启动不卡顿 |
| 密码显示 | 默认隐藏，`:pw` 切换明文/密文 |
| 多认证方式 | 详情面板显示所有认证方式及优先级 |
| SSH 连接 | 按 Enter 直接连接，支持多认证回退 |

### 支持的认证方式

| 认证方式 | 图标 | 说明 |
|---------|------|------|
| Public Key | 🔐 | 公钥认证，支持指定密钥或自动发现 `~/.ssh/` 默认密钥 |
| Password | 🔑 | 加密密码，连接时自动解密 |
| Keyboard Interactive | ⌨️ | 键盘交互式认证 |
| GSSAPI | 🎫 | Kerberos/GSSAPI 认证 |
| SSH Agent | 🔐 | 本地 SSH Agent |

### 密码加密支持

- **Prefix 02**：SHA256(passphrase) 作为 AES-256-CBC 密钥
- **Prefix 03**：bcrypt_pbkdf 派生 AES-256-CBC 密钥和 IV

解密后通过 SHA256 校验和验证。V1 格式（SecureCRT 7.3.3 之前）暂不支持。

### 转换为本地格式

```bash
xssh import-securecrt
```

转换后保存在 `~/.xsc/sessions/securecrt-converted/YYYYMMDD-HHMMSS/`。

## Xshell 集成

支持直接加载 Xshell `.xsh` 会话文件。启用后以青色样式显示在 `[XSH] xshell/` 目录下。

### 配置方法

Xshell 默认会话路径：

| 平台 | 默认路径 |
|------|---------|
| **Windows** | `%APPDATA%\NetSarang Computer\6\Xshell\Sessions` |
| **Windows (旧版)** | `我的文档\NetSarang Computer\6\Xshell\Sessions` |

> 可将 Windows 上的 Sessions 目录拷贝到 Linux/macOS，xssh 能正确处理 UTF-16LE 编码。

编辑 `~/.xsc/config.yaml`：

```yaml
xshell:
  enabled: true
  session_path: "/home/user/xshell-sessions"
  password: "your_master_password"    # Xshell 主密码
```

### TUI 中的表现

| 特性 | 说明 |
|------|------|
| 显示样式 | 青色文字，目录带 `[XSH]` 前缀，会话带 🔒 图标 |
| 只读模式 | 不支持编辑、删除、重命名 |
| 密码解密 | 延迟解密，Base64 → SHA256 密钥 → RC4 解密 → SHA256 校验 |
| 密码显示 | 默认隐藏，`:pw` 切换 |

### 转换为本地格式

```bash
xssh import-xshell
```

转换后保存在 `~/.xsc/sessions/xshell-converted/YYYYMMDD-HHMMSS/`。

## MobaXterm 集成

支持直接加载 MobaXterm `MobaXterm.ini` 中的 SSH 书签。启用后以橙色样式显示在 `[MXT] mobaxterm/` 目录下。

### 配置方法

MobaXterm 配置文件路径：

| 安装方式 | 路径 |
|---------|------|
| **安装版** | `%APPDATA%\MobaXterm\MobaXterm.ini` |
| **便携版** | 与 `MobaXterm.exe` 同目录 |

> 将 `MobaXterm.ini` 拷贝到 Linux/macOS 即可使用。

编辑 `~/.xsc/config.yaml`：

```yaml
mobaxterm:
  enabled: true
  session_path: "/home/user/MobaXterm.ini"
  password: "your_master_password"    # MobaXterm Professional 版主密码
```

> **关于主密码**：仅 Professional 版支持使用主密码加密。MobaXterm 的密码通常保存在 Windows 注册表中（`HKCU\Software\Mobatek\MobaXterm\P`）而非 INI 文件内。免费版加密格式暂不支持。

### TUI 中的表现

| 特性 | 说明 |
|------|------|
| 显示样式 | 橙色文字，目录带 `[MXT]` 前缀，会话带 🔒 图标 |
| 只读模式 | 不支持编辑、删除、重命名 |
| 密码解密 | AES-CFB-8 解密（SHA512 密钥派生） |
| 会话类型 | 仅导入 SSH 类型（type=0），跳过 RDP/VNC/FTP 等 |

### MobaXterm 特殊字符转义

| 转义序列 | 原始字符 |
|---------|---------|
| `__DIEZE__` | `#` |
| `__PTVIRG__` | `;` |
| `__DBLQUO__` | `"` |
| `__PIPE__` | `\|` |
| `__PERCENT__` | `%` |

### 转换为本地格式

```bash
xssh import-mobaxterm
```

转换后保存在 `~/.xsc/sessions/mobaxterm-converted/YYYYMMDD-HHMMSS/`。

## 全局配置

配置文件：`~/.xsc/config.yaml`（首次运行自动创建 `~/.xsc/` 目录）

```yaml
# SecureCRT 集成
securecrt:
  enabled: false
  session_path: ""
  password: ""

# Xshell 集成
xshell:
  enabled: false
  session_path: ""
  password: ""

# MobaXterm 集成
mobaxterm:
  enabled: false
  session_path: ""
  password: ""

# SSH 连接配置
ssh:
  strict_host_key: false            # 主机密钥验证（默认 false，跳过验证）
  known_hosts_file: ""              # 自定义 known_hosts 路径
```

### SSH 主机密钥验证

| 配置值 | 行为 |
|--------|------|
| 未配置 / `false` | 跳过验证（默认），直接连接任何主机 |
| `true` | TOFU 模式：首次连接信任并记录，密钥变更拒绝 |

`known_hosts` 文件查找顺序：
1. 配置中指定的 `known_hosts_file`
2. `~/.ssh/known_hosts`
3. `~/.xsc/known_hosts`

> 配置文件权限建议 `0600`，避免密码泄露。

## 目录结构

```
~/.xsc/
├── config.yaml                    # 全局配置文件
├── known_hosts                    # 主机密钥（TOFU 模式自动写入）
└── sessions/                      # 本地会话目录
    ├── prod/                      # 按环境分组
    │   └── db/
    │       ├── master.yaml
    │       └── slave-01.yaml
    ├── staging/
    │   └── web-server.yaml
    ├── securecrt-converted/       # import-securecrt 转换结果
    │   └── 20240101-120000/
    ├── xshell-converted/          # import-xshell 转换结果
    │   └── 20240101-120000/
    └── mobaxterm-converted/       # import-mobaxterm 转换结果
        └── 20240101-120000/
```

## 开发

```bash
make build          # 构建 xssh + xftp 到 ./build/
make build-xftp     # 仅构建 xftp
make run            # 运行 xssh TUI
make run-xftp       # 运行 xftp TUI
make test           # 运行所有测试：go test -v ./...
make fmt            # 格式化代码：go fmt ./...
make vet            # 静态分析：go vet ./...
make deps           # 下载并整理依赖
make clean          # 清理构建产物
make install        # 安装到 /usr/local/bin（xssh + xftp）
make uninstall      # 卸载
```

运行指定测试：
```bash
go test -v ./internal/securecrt/... -run TestDecryptPasswordV2Real
```

完整质量检查：
```bash
make fmt && make vet && make test
```
