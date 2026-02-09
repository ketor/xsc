# XSC - SSH Session Manager CLI
# 功能规范文档 (Gherkin 格式)

Feature: TUI 模式下的 SSH 会话管理
  作为运维工程师
  我希望通过 TUI 界面管理 SSH 会话
  以便快速连接和管理服务器

  Background:
    Given XSC 已安装配置
    And 会话目录 ~/.xsc/sessions/ 存在

  # ========== 基础导航 ==========
  
  Scenario: 启动 TUI 模式
    When 用户运行 "xsc tui"
    Then 显示 TUI 界面
    And 显示会话树形列表
    And 显示底部状态栏
    And 默认展开所有目录

  Scenario: 使用 Vim 风格导航
    Given TUI 已启动
    When 用户按下 "j"
    Then 光标向下移动一行
    When 用户按下 "k"
    Then 光标向上移动一行
    When 用户按下 "gg"
    Then 光标跳转到第一行
    When 用户按下 "G"
    Then 光标跳转到最后一行
    When 用户按下 "42G"
    Then 光标跳转到第 42 行

  Scenario: 使用命令模式跳转
    Given TUI 已启动
    When 用户按下 ":"
    And 输入 "42"
    And 按下 Enter
    Then 光标跳转到第 42 行

  # ========== 目录操作 ==========

  Scenario: 展开和折叠目录
    Given TUI 已启动
    And 当前选中目录 "prod/"
    When 用户按下 "o"
    Then 目录 "prod/" 被折叠
    When 用户再次按下 "o"
    Then 目录 "prod/" 被展开

  Scenario: 使用 h/l 导航目录
    Given TUI 已启动
    And 当前选中已展开的目录 "prod/"
    When 用户按下 "h"
    Then 目录 "prod/" 被折叠
    When 用户按下 "l"
    Then 目录 "prod/" 被展开

  Scenario: 展开所有目录
    Given TUI 已启动
    And 部分目录已折叠
    When 用户按下 "E"
    Then 所有目录被展开
    And 显示全部会话节点

  Scenario: 折叠所有目录
    Given TUI 已启动
    And 部分目录已展开
    When 用户按下 "C"
    Then 所有目录被折叠
    And 只显示顶层节点

  # ========== 搜索功能 ==========

  Scenario: 启动搜索模式
    Given TUI 已启动
    When 用户按下 "/"
    Then 显示搜索输入框
    And 底部提示 "Esc:clear Enter:confirm"

  Scenario: 实时搜索过滤
    Given TUI 已启动
    And 存在会话 "web-server", "db-master", "api-gateway"
    When 用户按下 "/"
    And 输入 "web"
    Then 列表只显示包含 "web" 的会话
    And 底部状态栏显示 "Filter: 'web' (1)"

  Scenario: 确认搜索
    Given 正在搜索模式
    And 已输入 "web"
    When 用户按下 "Enter"
    Then 退出搜索输入模式
    And 保留过滤结果
    And 可以继续浏览过滤后的列表

  Scenario: ESC 取消并清空搜索
    Given 存在搜索过滤 "web"
    When 用户按下 "Esc"
    Then 清空搜索关键词
    And 显示全部会话
    And 光标重置到顶部

  Scenario: 普通模式下 ESC 清空搜索
    Given 已确认搜索 "web"
    And 显示过滤结果
    When 用户按下 "Esc"
    Then 直接清空搜索过滤
    And 显示全部会话

  Scenario: Ctrl+c 保留搜索结果
    Given 正在搜索模式
    And 已输入 "web"
    When 用户按下 "Ctrl+c"
    Then 退出搜索输入模式
    But 保留 "web" 过滤结果

  # ========== 会话操作 ==========

  Scenario: 连接到会话
    Given TUI 已启动
    And 选中了会话 "prod/db/master"
    When 用户按下 "Enter"
    Then TUI 暂停
    And 建立 SSH 连接
    And 进入交互式 SSH 会话

  Scenario: SSH 会话结束后返回 TUI
    Given 已连接到 SSH 会话
    When 用户退出 SSH (Ctrl+d 或 exit)
    Then 返回 TUI 界面
    And TUI 恢复显示

  Scenario: 新建会话
    Given TUI 已启动
    When 用户按下 "n"
    Then 打开编辑器创建新会话文件
    And 基于模板填充默认配置

  Scenario: 编辑会话
    Given TUI 已启动
    And 选中了会话 "prod/db/master"
    When 用户按下 "e"
    Then 打开编辑器编辑该会话文件
    And 保存后自动重新加载

  Scenario: 删除会话
    Given TUI 已启动
    And 选中了会话 "prod/db/master"
    When 用户按下 "D"
    Then 显示删除确认提示 "⚠️  Warning: This action cannot be undone!"
    And 显示确认输入框 "Type YES to confirm:"
    When 用户输入 "YES"
    And 按下 Enter
    Then 删除该会话文件
    And 从列表中移除

  Scenario: 取消删除会话
    Given TUI 已启动
    And 选中了会话 "prod/db/master"
    When 用户按下 "D"
    Then 显示删除确认提示
    When 用户按下 "Esc"
    Then 取消删除操作
    And 会话文件未被删除

  Scenario: 重命名会话
    Given TUI 已启动
    And 选中了会话 "prod/db/master"
    When 用户按下 "c"
    Then 显示重命名输入框
    And 输入框预填充当前文件名 "master"
    When 用户输入 "new-master"
    And 按下 Enter
    Then 会话文件被重命名为 "new-master.yaml"
    And 会话列表更新显示 "new-master"

  # ========== 信息显示 ==========

  Scenario: 显示会话详情
    Given TUI 已启动
    And 选中了会话 "prod/db/master"
    Then 右侧详情面板显示：
      | 字段        | 值                 |
      | Name        | master             |
      | Host        | 192.168.1.100      |
      | Port        | 22                 |
      | User        | root               |
      | Auth        | password           |
      | Password    | ********           |

  Scenario: 显示帮助信息
    Given TUI 已启动
    When 用户按下 "?"
    Then 显示帮助界面
    And 列出所有快捷键
    And 显示快捷键分组

  Scenario: 退出帮助
    Given 正在显示帮助
    When 用户按下任意键（除 q/Ctrl+c）
    Then 关闭帮助界面
    And 返回 TUI 主界面

  # ========== 退出程序 ==========

  Scenario: 使用 :q 退出
    Given TUI 已启动
    When 用户按下 ":"
    And 输入 "q"
    And 按下 Enter
    Then 退出 TUI
    And 返回 shell

  Scenario: 使用 Ctrl+c 退出
    Given TUI 已启动
    When 用户按下 "Ctrl+c"
    Then 退出 TUI
    And 返回 shell

  Scenario: 在帮助界面退出
    Given 正在显示帮助
    When 用户按下 "q"
    Then 退出 TUI 程序

  # ========== 错误处理 ==========

  Scenario: 连接失败显示错误
    Given TUI 已启动
    And 选中了无效会话
    When 用户按下 "Enter"
    And SSH 连接失败
    Then 显示错误弹窗（红色边框）
    And 显示错误信息 "Connection failed: ..."
    And 提示 "Press any key to continue"

  Scenario: 关闭错误弹窗
    Given 显示错误弹窗
    When 用户按下任意键
    Then 关闭错误弹窗
    And 返回 TUI 主界面

# ========== CLI 命令功能 ==========

Feature: 命令行模式
  作为运维工程师
  我希望通过命令行快速执行操作
  以便在脚本中使用

  Scenario: 列出所有会话
    When 用户运行 "xsc list"
    Then 输出所有会话路径
    And 每行一个会话
    And 格式为 "folder/subfolder/session-name"

  Scenario: 连接到指定会话
    Given 存在会话 "prod/db/master"
    When 用户运行 "xsc connect prod/db/master"
    Then 建立 SSH 连接到该会话
    And 进入交互式终端

  Scenario: 模糊匹配连接
    Given 存在会话 "prod/web-server"
    When 用户运行 "xsc connect web"
    Then 模糊匹配到 "prod/web-server"
    And 建立 SSH 连接

  Scenario: 导入 SecureCRT 会话
    Given 配置了 SecureCRT 路径
    And 存在 SecureCRT 会话
    When 用户运行 "xsc import-securecrt"
    Then 读取所有 SecureCRT 会话
    And 解密密码（使用配置的 master password）
    And 转换为 xsc 格式
    And 保存到 ~/.xsc/sessions/securecrt-converted/YYYYMMDD-HHMMSS/
    And 保持原有目录结构
    And 显示转换统计 "✓ Converted: N | ✗ Errors: M"

# ========== 会话配置 ==========

Feature: 会话配置管理
  作为运维工程师
  我希望通过 YAML 文件管理会话
  以便版本控制和共享配置

  Scenario: 加载本地会话
    Given 存在会话文件 "~/.xsc/sessions/prod/db/master.yaml"
    And 文件内容：
      """
      host: "192.168.1.100"
      port: 22
      user: "root"
      auth_type: "password"
      password: "secret123"
      description: "生产数据库主节点"
      """
    When 启动 TUI
    Then 正确显示会话 "master"
    And 显示在目录 "prod/db/" 下

  Scenario: 密码认证会话
    Given 会话配置 auth_type: "password"
    And 提供了 password 字段
    When 连接该会话
    Then 使用密码认证
    And 自动发送密码

  Scenario: 密钥认证会话
    Given 会话配置 auth_type: "key"
    And key_path: "~/.ssh/id_rsa"
    When 连接该会话
    Then 使用密钥认证
    And 尝试加载私钥文件

  Scenario: Agent 认证会话
    Given 会话配置 auth_type: "agent"
    And SSH Agent 已启动
    And Agent 中已加载密钥
    When 连接该会话
    Then 使用 SSH Agent 认证
    And 无需输入密码

  Scenario: 延迟解密 SecureCRT 密码
    Given 从 SecureCRT 导入的会话
    And 密码已加密
    And 配置了 master password
    When 查看会话详情
    Then 实时解密并显示密码
    And 如果解密失败显示错误

# ========== 全局配置 ==========

Feature: 全局配置管理
  作为运维工程师
  我希望配置全局设置
  以便管理 SecureCRT 集成等行为

  Scenario: 加载全局配置
    Given 存在配置文件 "~/.xsc/config.yaml"
    And 文件内容：
      """
      securecrt:
        enabled: true
        session_path: "/path/to/sessions"
        password: "master_password"
      ssh:
        strict_host_key: false
        known_hosts_file: "~/.ssh/known_hosts"
      """
    When 启动 XSC
    Then 加载 SecureCRT 配置
    And 禁用严格主机密钥验证

  Scenario: SecureCRT 集成
    Given 启用了 SecureCRT
    And 配置了 session_path
    When 启动 TUI
    Then 加载 SecureCRT 会话
    And 显示在 "securecrt/" 目录下
    And 保持原有目录结构

  Scenario: 严格主机密钥验证
    Given 配置了 strict_host_key: true
    And 存在 known_hosts 文件
    When 连接新主机
    Then 验证主机密钥
    And 如果密钥不匹配则拒绝连接

  Scenario: 禁用主机密钥验证
    Given 配置了 strict_host_key: false
    When 连接新主机
    Then 自动接受新主机密钥
    And 不显示警告

# ========== TUI 界面元素 ==========

Feature: TUI 界面展示
  作为用户
  我希望界面信息清晰完整
  以便了解当前状态

  Scenario: 显示行号
    Given TUI 已启动
    Then 每行显示行号
    And 行号右对齐
    And 行号颜色为深灰色

  Scenario: 显示会话详情边框
    Given TUI 已启动
    Then 右侧详情面板显示圆角边框
    And 边框颜色为深灰色
    And 标题显示会话文件名

  Scenario: 状态栏信息
    Given TUI 已启动
    Then 底部状态栏显示：
      | 元素              | 示例值                                    |
      | 选中会话          | Session: master                           |
      | 总数/过滤数       | Total: 1166 或 Filter: 'web' (15)        |
      | 操作提示          | Press ? for help, :q or Ctrl+c to quit   |

  Scenario: 搜索状态提示
    Given 正在进行搜索
    Then 搜索框显示提示 "(Esc:clear Enter:confirm)"
    And 底部状态栏显示过滤状态

  Scenario: 实时解密密码显示
    Given 选中了 SecureCRT 会话
    And 密码已加密
    When 光标移动到该会话
    Then 右侧详情面板实时解密密码
    And 显示解密后的密码
    And 如果解密失败显示错误信息

# ========== 性能优化 ==========

Feature: 性能优化
  作为用户
  我希望操作流畅无卡顿
  以便高效工作

  Scenario: 缓存 SSH Agent 密钥
    Given 选中了 Agent 认证会话
    When 查看详情面板
    Then 第一次查询 SSH Agent 密钥
    And 缓存结果供后续使用
    And 避免重复打开 Agent socket

  Scenario: 延迟解密 SecureCRT 密码
    Given 加载了 500+ SecureCRT 会话
    When 启动 TUI
    Then 启动时间 < 1 秒
    And 不立即解密所有密码
    And 只在需要时（光标移动）解密当前会话密码

  Scenario: 高效渲染树形列表
    Given 存在大量会话（>1000）
    When 展开/折叠目录
    Then 只渲染可见节点
    And 滚动时动态计算可见范围
    And 保持界面响应流畅
