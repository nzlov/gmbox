package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
	"gmbox/internal/model"
)

// SendInput 描述前端写信页提交的发信请求。
type SendInput struct {
	AccountID uint     `json:"account_id" binding:"required"`
	To        []string `json:"to" binding:"required,min=1"`
	Cc        []string `json:"cc"`
	Bcc       []string `json:"bcc"`
	Subject   string   `json:"subject"`
	Body      string   `json:"body" binding:"required"`
	IsHTML    bool     `json:"is_html"`
}

// Send 使用指定邮箱账户通过 SMTP 发送邮件。
func (s *Service) Send(ctx context.Context, input SendInput) error {
	var account model.MailAccount
	if err := s.db.First(&account, input.AccountID).Error; err != nil {
		return fmt.Errorf("读取发件邮箱失败: %w", err)
	}
	password, err := s.ResolveAuthSecret(ctx, &account)
	if err != nil {
		return err
	}
	raw, recipients, err := buildSMTPMessage(account, input)
	if err != nil {
		return err
	}
	return sendSMTPMessage(ctx, account, password, recipients, raw)
}

// buildSMTPMessage 统一构造 MIME 邮件，避免各个入口各自拼接报文。
func buildSMTPMessage(account model.MailAccount, input SendInput) ([]byte, []string, error) {
	var buffer bytes.Buffer
	var header gomail.Header

	from := []*gomail.Address{{Name: account.Name, Address: account.Email}}
	toAddrs, err := parseAddressList(input.To)
	if err != nil {
		return nil, nil, err
	}
	ccAddrs, err := parseAddressList(input.Cc)
	if err != nil {
		return nil, nil, err
	}
	bccAddrs, err := parseAddressList(input.Bcc)
	if err != nil {
		return nil, nil, err
	}

	header.SetDate(time.Now())
	header.SetAddressList("From", from)
	header.SetAddressList("To", toAddrs)
	header.SetAddressList("Cc", ccAddrs)
	header.SetSubject(input.Subject)
	if err := header.GenerateMessageIDWithHostname(smtpHostname(account)); err != nil {
		return nil, nil, fmt.Errorf("生成 Message-ID 失败: %w", err)
	}

	if input.IsHTML {
		header.SetContentType("text/html", map[string]string{"charset": "utf-8"})
	} else {
		header.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	}

	bodyWriter, err := gomail.CreateSingleInlineWriter(&buffer, header)
	if err != nil {
		return nil, nil, fmt.Errorf("创建邮件正文失败: %w", err)
	}
	if _, err := bodyWriter.Write([]byte(input.Body)); err != nil {
		_ = bodyWriter.Close()
		return nil, nil, fmt.Errorf("写入邮件正文失败: %w", err)
	}
	if err := bodyWriter.Close(); err != nil {
		return nil, nil, fmt.Errorf("关闭邮件写入器失败: %w", err)
	}

	recipients := make([]string, 0, len(input.To)+len(input.Cc)+len(input.Bcc))
	recipients = append(recipients, flattenAddresses(toAddrs)...)
	recipients = append(recipients, flattenAddresses(ccAddrs)...)
	recipients = append(recipients, flattenAddresses(bccAddrs)...)
	return buffer.Bytes(), recipients, nil
}

// sendSMTPMessage 兼容 SMTPS 和普通 SMTP/STARTTLS 两种常见服务形态。
func sendSMTPMessage(ctx context.Context, account model.MailAccount, password string, recipients []string, raw []byte) error {
	conn, err := dialSMTP(ctx, account)
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, account.SMTPHost)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Quit()

	if shouldUseStartTLS(account) {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: account.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("启动 STARTTLS 失败: %w", err)
			}
		} else if account.UseTLS {
			return fmt.Errorf("SMTP 服务端不支持 STARTTLS")
		}
	}

	if ok, _ := client.Extension("AUTH"); ok {
		auth := smtp.PlainAuth("", account.Username, password, account.SMTPHost)
		if normalizeAuthType(account.AuthType) == "oauth" {
			auth = newSMTPXOAUTH2Auth(account.Username, password, account.SMTPHost)
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Mail(account.Email); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("设置收件人失败: %w", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开 DATA 失败: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("提交邮件内容失败: %w", err)
	}
	return nil
}

// probeSMTP 以与实际发送一致的方式验证 SMTP 端口可用性，避免 587 + STARTTLS 被误判为 SMTPS。
func probeSMTP(ctx context.Context, account model.MailAccount) error {
	conn, err := dialSMTP(ctx, account)
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, account.SMTPHost)
	if err != nil {
		return err
	}
	defer client.Close()
	if shouldUseStartTLS(account) {
		if ok, _ := client.Extension("STARTTLS"); ok {
			return client.StartTLS(&tls.Config{ServerName: account.SMTPHost, MinVersion: tls.VersionTLS12})
		}
		if account.UseTLS {
			return fmt.Errorf("SMTP 服务端不支持 STARTTLS")
		}
	}
	return nil
}

// dialSMTP 根据既有配置语义选择隐式 TLS 或明文连接，统一服务于测试与发送。
func dialSMTP(ctx context.Context, account model.MailAccount) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", account.SMTPHost, account.SMTPPort)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if shouldUseImplicitSMTPTLS(account) {
		return tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: account.SMTPHost, MinVersion: tls.VersionTLS12})
	}
	return dialer.DialContext(ctx, "tcp", addr)
}

// shouldUseImplicitSMTPTLS 保持历史 `use_tls=true` 的隐式 TLS 语义，仅对已知 STARTTLS 场景排除。
func shouldUseImplicitSMTPTLS(account model.MailAccount) bool {
	return account.UseTLS && !shouldUseStartTLS(account)
}

// shouldUseStartTLS 仅对已知需要 STARTTLS 的预设启用，避免误伤自定义隐式 TLS 端口。
func shouldUseStartTLS(account model.MailAccount) bool {
	if !account.UseTLS {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(account.Provider), "outlook") && account.SMTPPort == 587
}

// parseAddressList 将前端输入的邮箱字符串转换成标准地址列表。
func parseAddressList(values []string) ([]*gomail.Address, error) {
	result := make([]*gomail.Address, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		addr, err := gomail.ParseAddress(trimmed)
		if err != nil {
			return nil, fmt.Errorf("解析邮箱地址失败: %w", err)
		}
		result = append(result, addr)
	}
	return result, nil
}

// flattenAddresses 提取 SMTP RCPT 所需的纯邮箱地址。
func flattenAddresses(addrs []*gomail.Address) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr.Address)
	}
	return result
}

// smtpHostname 为 Message-ID 和 TLS SNI 提供稳定的域名来源。
func smtpHostname(account model.MailAccount) string {
	host := strings.TrimSpace(account.SMTPHost)
	if host != "" {
		return host
	}
	parts := strings.Split(account.Email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return "localhost"
}
