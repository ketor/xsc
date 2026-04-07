// Package mobaxterm 提供解析和解密 MobaXterm 会话文件的功能
package mobaxterm

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Session 表示一个 MobaXterm 会话
type Session struct {
	Name              string // 会话名称
	Hostname          string // 主机地址
	Port              int    // 端口号
	Username          string // 用户名
	Password          string // 解密后的密码（延迟解密时为空）
	EncryptedPassword string // 加密密码（用于延迟解密）
	FilePath          string // MobaXterm.ini 文件路径
	Folder            string // 目录路径
}

// Config 表示 MobaXterm 导入配置
type Config struct {
	SessionPath string // MobaXterm.ini 文件路径
	Password    string // 主密码（用于 AES-CFB-8 解密）
}

// bookmarkEntry 表示一个 Bookmarks section 中的会话条目
type bookmarkEntry struct {
	SubRep   string   // 子目录路径
	Sessions []string // 会话行列表（SessionName=type%host%port%user%...）
}

// LoadSessions 加载 MobaXterm.ini 文件中的所有 SSH 会话
func LoadSessions(config Config) ([]*Session, error) {
	if _, err := os.Stat(config.SessionPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("MobaXterm 配置文件不存在: %s", config.SessionPath)
	}

	// 读取文件
	data, err := os.ReadFile(config.SessionPath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	// 使用 GBK 编码解码（中文 Windows 环境下 MobaXterm 使用 GBK 编码）
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err != nil {
		// 解码失败时回退到原始数据
		decoded = data
	}
	content := string(decoded)

	// 解析所有 Bookmarks sections
	sections := parseBookmarksSections(content)

	var sessions []*Session

	for _, entry := range sections {
		for _, line := range entry.Sessions {
			session, err := parseSessionLine(line, config.SessionPath, entry.SubRep)
			if err != nil {
				// 跳过解析失败的行
				continue
			}

			// 只添加有 hostname 的会话
			if session.Hostname == "" {
				continue
			}

			// 尝试解密密码
			if session.EncryptedPassword != "" && config.Password != "" {
				decrypted, err := decryptModern(session.EncryptedPassword, config.Password)
				if err != nil {
					// 解密失败时保留加密密码，不影响会话导入
					session.Password = ""
				} else {
					session.Password = decrypted
				}
			}

			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// DecryptPassword 解密密码（公开接口，供外部延迟解密调用）
func DecryptPassword(encrypted, masterPassword string) (string, error) {
	if encrypted == "" || masterPassword == "" {
		return "", fmt.Errorf("加密密码或主密码为空")
	}

	return decryptModern(encrypted, masterPassword)
}

// ConvertToXSSHSession 将 MobaXterm 会话转换为 xssh 会话格式
func (s *Session) ConvertToXSSHSession() map[string]interface{} {
	result := map[string]interface{}{
		"host": s.Hostname,
		"port": s.Port,
		"user": s.Username,
	}

	// 有加密密码 → password 认证
	if s.EncryptedPassword != "" {
		result["auth_type"] = "password"
		result["encrypted_password"] = s.EncryptedPassword
	} else {
		// 无密码 → agent 认证（默认）
		result["auth_type"] = "agent"
	}

	// 如果密码已解密，填入
	if s.Password != "" {
		result["password"] = s.Password
	}

	return result
}

// parseBookmarksSections 解析 MobaXterm.ini 中所有 [Bookmarks] 和 [Bookmarks_N] sections
// 返回所有书签条目的切片，每个条目包含 SubRep（目录路径）和会话行列表
func parseBookmarksSections(content string) []bookmarkEntry {
	var entries []bookmarkEntry
	lines := strings.Split(content, "\n")

	inBookmarksSection := false
	var currentEntry *bookmarkEntry

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)

		// 检查 section 头
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := trimmed[1 : len(trimmed)-1]
			// 匹配 [Bookmarks] 或 [Bookmarks_N]
			if sectionName == "Bookmarks" || strings.HasPrefix(sectionName, "Bookmarks_") {
				inBookmarksSection = true
				currentEntry = &bookmarkEntry{}
				entries = append(entries, *currentEntry)
				// 更新引用指向 entries 中最后一个元素
				currentEntry = &entries[len(entries)-1]
			} else {
				inBookmarksSection = false
				currentEntry = nil
			}
			continue
		}

		if !inBookmarksSection || currentEntry == nil {
			continue
		}

		// 跳过空行
		if trimmed == "" {
			continue
		}

		// 检查 SubRep= 行（目录路径）
		if strings.HasPrefix(trimmed, "SubRep=") {
			currentEntry.SubRep = strings.TrimPrefix(trimmed, "SubRep=")
			continue
		}

		// 跳过 ImgNum= 行
		if strings.HasPrefix(trimmed, "ImgNum=") {
			continue
		}

		// 其他行视为会话定义（SessionName=type%host%...）
		if strings.Contains(trimmed, "=") {
			currentEntry.Sessions = append(currentEntry.Sessions, trimmed)
		}
	}

	return entries
}

// parseSessionLine 解析单个会话行
// 格式: SessionName=type%host%port%user%...#FontGroup%...
// SSH 类型 type=0
func parseSessionLine(line, filePath, folder string) (*Session, error) {
	// 分离会话名和值
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return nil, fmt.Errorf("无效的会话行格式")
	}

	sessionName := line[:idx]
	value := line[idx+1:]

	// 反转义会话名
	sessionName = unescapeSpecialChars(sessionName)

	// 处理 MobaXterm 会话值格式
	// 新格式: #ImgNum#Type%Host%Port%User%...#FontConfig%...
	// 旧格式: Type%Host%Port%User%...#FontConfig%...
	mainPart := value

	// 跳过前导 #ImgNum 前缀（如 #109）
	if strings.HasPrefix(mainPart, "#") {
		// 找到第二个 # 的位置，跳过 #ImgNum 前缀
		if secondHash := strings.Index(mainPart[1:], "#"); secondHash >= 0 {
			mainPart = mainPart[secondHash+2:] // 跳过 #ImgNum#，剩余 Type%Host%...#FontConfig%...
		}
	}

	// 取 # 之前的部分（# 后面是字体等显示配置）
	if hashIdx := strings.Index(mainPart, "#"); hashIdx >= 0 {
		mainPart = mainPart[:hashIdx]
	}

	// 按 % 分割字段
	fields := strings.Split(mainPart, "%")
	if len(fields) < 2 {
		return nil, fmt.Errorf("字段数量不足")
	}

	// 检查类型（fields[0]），SSH = 0
	sessionType, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || sessionType != 0 {
		// 不是 SSH 类型，跳过
		return nil, fmt.Errorf("非 SSH 会话类型: %s", fields[0])
	}

	session := &Session{
		Name:     sessionName,
		FilePath: filePath,
		Folder:   folder,
		Port:     22, // 默认端口
	}

	// fields[1] = hostname
	if len(fields) > 1 {
		session.Hostname = unescapeSpecialChars(fields[1])
	}

	// fields[2] = port
	if len(fields) > 2 && fields[2] != "" {
		if port, err := strconv.Atoi(fields[2]); err == nil && port > 0 {
			session.Port = port
		}
	}

	// fields[3] = username
	if len(fields) > 3 {
		session.Username = unescapeSpecialChars(fields[3])
	}

	return session, nil
}

// unescapeSpecialChars 反转义 MobaXterm 的特殊字符
func unescapeSpecialChars(s string) string {
	s = strings.ReplaceAll(s, "__DIEZE__", "#")
	s = strings.ReplaceAll(s, "__PTVIRG__", ";")
	s = strings.ReplaceAll(s, "__DBLQUO__", "\"")
	s = strings.ReplaceAll(s, "__PIPE__", "|")
	s = strings.ReplaceAll(s, "__PERCENT__", "%")
	return s
}
