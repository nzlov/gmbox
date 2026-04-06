package mail

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"gmbox/internal/model"
)

// SyncPOP3 拉取收件箱全部 UIDL 并对未入库的新邮件执行增量下载。
func (s *Service) SyncPOP3(ctx context.Context, account model.MailAccount, state *model.SyncState, fetchBody bool) (*SyncResult, error) {
	blockedContacts, err := s.loadBlockedContactSet()
	if err != nil {
		return nil, err
	}
	password, err := s.DecryptPassword(account)
	if err != nil {
		return nil, err
	}
	client, err := dialPOP3(ctx, account)
	if err != nil {
		return nil, err
	}
	defer client.close()

	if err := client.auth(account.Username, password); err != nil {
		return nil, err
	}
	entries, err := client.uidlAll()
	if err != nil {
		return nil, err
	}
	newCount := 0
	for _, entry := range entries {
		var count int64
		if err := s.db.Model(&model.Message{}).Where("account_id = ? AND pop3_uid_l = ?", account.Model.ID, entry.uidl).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			continue
		}
		raw, err := client.retr(entry.number)
		if err != nil {
			return nil, err
		}
		parsed, err := parseRawMessage(raw)
		if err != nil {
			return nil, err
		}
		stored, err := s.upsertMessage(account, "INBOX", 0, entry.uidl, parsed, fetchBody, blockedContacts)
		if err != nil {
			return nil, err
		}
		state.LastPOP3UIDL = entry.uidl
		if stored {
			newCount++
		}
	}
	state.LastMessage = fmt.Sprintf("POP3 同步完成，新增 %d 封邮件", newCount)
	return &SyncResult{NewMessages: newCount, MailboxCount: 1}, nil
}

type pop3Entry struct {
	number int
	uidl   string
}

type pop3Client struct {
	conn net.Conn
	tp   *textproto.Conn
}

// dialPOP3 建立 POP3 连接并验证欢迎消息，减少后续命令级错误噪声。
func dialPOP3(ctx context.Context, account model.MailAccount) (*pop3Client, error) {
	addr := fmt.Sprintf("%s:%d", account.POP3Host, account.POP3Port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if account.UseTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: account.POP3Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 POP3 失败: %w", err)
	}
	client := &pop3Client{conn: conn, tp: textproto.NewConn(conn)}
	line, err := client.tp.ReadLine()
	if err != nil {
		client.close()
		return nil, fmt.Errorf("读取 POP3 欢迎消息失败: %w", err)
	}
	if !strings.HasPrefix(line, "+OK") {
		client.close()
		return nil, fmt.Errorf("POP3 欢迎消息异常: %s", line)
	}
	return client, nil
}

// auth 依次执行 USER/PASS 完成 POP3 认证。
func (c *pop3Client) auth(username string, password string) error {
	if _, err := c.cmdSingle("USER %s", username); err != nil {
		return fmt.Errorf("POP3 USER 失败: %w", err)
	}
	if _, err := c.cmdSingle("PASS %s", password); err != nil {
		return fmt.Errorf("POP3 PASS 失败: %w", err)
	}
	return nil
}

// uidlAll 读取服务器当前可见的全部 UIDL 列表。
func (c *pop3Client) uidlAll() ([]pop3Entry, error) {
	lines, err := c.cmdMulti("UIDL")
	if err != nil {
		return nil, fmt.Errorf("POP3 UIDL 失败: %w", err)
	}
	result := make([]pop3Entry, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		number, convErr := strconv.Atoi(fields[0])
		if convErr != nil {
			continue
		}
		result = append(result, pop3Entry{number: number, uidl: fields[1]})
	}
	return result, nil
}

// retr 拉取指定编号邮件的完整 RFC822 内容。
func (c *pop3Client) retr(number int) ([]byte, error) {
	lines, err := c.cmdMulti("RETR %d", number)
	if err != nil {
		return nil, fmt.Errorf("POP3 RETR 失败: %w", err)
	}
	buffer := bytes.NewBuffer(nil)
	writer := bufio.NewWriter(buffer)
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return nil, err
		}
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return io.ReadAll(buffer)
}

// cmdSingle 发送单行 POP3 命令并读取 +OK 响应。
func (c *pop3Client) cmdSingle(format string, args ...any) (string, error) {
	if err := c.tp.PrintfLine(format, args...); err != nil {
		return "", err
	}
	line, err := c.tp.ReadLine()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(line, "+OK") {
		return "", fmt.Errorf("%s", strings.TrimSpace(line))
	}
	return line, nil
}

// cmdMulti 读取多行 POP3 响应，并处理点转义。
func (c *pop3Client) cmdMulti(format string, args ...any) ([]string, error) {
	if _, err := c.cmdSingle(format, args...); err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for {
		line, err := c.tp.ReadLine()
		if err != nil {
			return nil, err
		}
		if line == "." {
			break
		}
		if strings.HasPrefix(line, "..") {
			line = line[1:]
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// close 退出 POP3 会话并关闭底层连接。
func (c *pop3Client) close() {
	if c == nil {
		return
	}
	_ = c.tp.PrintfLine("QUIT")
	_ = c.tp.Close()
	_ = c.conn.Close()
}
