package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
	"gmbox/internal/model"
)

// SendInput 描述前端写信页提交的发信请求。
type SendInput struct {
	AccountID     uint     `json:"account_id" binding:"required"`
	To            []string `json:"to" binding:"required,min=1"`
	Cc            []string `json:"cc"`
	Bcc           []string `json:"bcc"`
	Subject       string   `json:"subject"`
	Body          string   `json:"body" binding:"required"`
	IsHTML        bool     `json:"is_html"`
	AttachmentIDs []uint   `json:"attachment_ids"`
}

// sendAttachment 保存待发送附件的元数据和文件内容，便于统一生成 MIME 报文。
type sendAttachment struct {
	FileName    string
	ContentType string
	Content     []byte
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
	attachments, err := s.loadSendAttachments(input.AttachmentIDs)
	if err != nil {
		return err
	}
	raw, recipients, err := buildSMTPMessage(account, input, attachments)
	if err != nil {
		return err
	}
	return s.sendSMTPMessage(ctx, account, password, recipients, raw)
}

// loadSendAttachments 按前端传入顺序加载待转发附件，避免 MIME 中附件顺序错乱。
func (s *Service) loadSendAttachments(ids []uint) ([]sendAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	attachments := make([]sendAttachment, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		attachment, content, err := s.DownloadAttachment(id)
		if err != nil {
			return nil, fmt.Errorf("读取转发附件失败: %w", err)
		}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		attachments = append(attachments, sendAttachment{
			FileName:    attachment.FileName,
			ContentType: contentType,
			Content:     content,
		})
	}
	return attachments, nil
}

// buildSMTPMessage 统一构造 MIME 邮件，避免各个入口各自拼接报文。
func buildSMTPMessage(account model.MailAccount, input SendInput, attachments []sendAttachment) ([]byte, []string, error) {
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

	if len(attachments) == 0 {
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
	} else {
		messageWriter, err := gomail.CreateWriter(&buffer, header)
		if err != nil {
			return nil, nil, fmt.Errorf("创建带附件邮件失败: %w", err)
		}
		if err := writeSMTPInlinePart(messageWriter, input); err != nil {
			_ = messageWriter.Close()
			return nil, nil, err
		}
		for _, attachment := range attachments {
			if err := writeSMTPAttachmentPart(messageWriter, attachment); err != nil {
				_ = messageWriter.Close()
				return nil, nil, err
			}
		}
		if err := messageWriter.Close(); err != nil {
			return nil, nil, fmt.Errorf("关闭带附件邮件失败: %w", err)
		}
	}

	recipients := make([]string, 0, len(input.To)+len(input.Cc)+len(input.Bcc))
	recipients = append(recipients, flattenAddresses(toAddrs)...)
	recipients = append(recipients, flattenAddresses(ccAddrs)...)
	recipients = append(recipients, flattenAddresses(bccAddrs)...)
	return buffer.Bytes(), recipients, nil
}

// writeSMTPInlinePart 在 multipart 邮件中写入正文部分，保持无附件时的正文编码策略一致。
func writeSMTPInlinePart(messageWriter *gomail.Writer, input SendInput) error {
	var inlineHeader gomail.InlineHeader
	if input.IsHTML {
		inlineHeader.SetContentType("text/html", map[string]string{"charset": "utf-8"})
	} else {
		inlineHeader.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	}
	bodyWriter, err := messageWriter.CreateSingleInline(inlineHeader)
	if err != nil {
		return fmt.Errorf("创建邮件正文失败: %w", err)
	}
	if _, err := bodyWriter.Write([]byte(input.Body)); err != nil {
		_ = bodyWriter.Close()
		return fmt.Errorf("写入邮件正文失败: %w", err)
	}
	if err := bodyWriter.Close(); err != nil {
		return fmt.Errorf("关闭邮件正文失败: %w", err)
	}
	return nil
}

// writeSMTPAttachmentPart 逐个写入附件，确保文件名和类型按原邮件元数据透传。
func writeSMTPAttachmentPart(messageWriter *gomail.Writer, attachment sendAttachment) error {
	var attachmentHeader gomail.AttachmentHeader
	attachmentHeader.SetContentType(attachment.ContentType, nil)
	attachmentHeader.SetFilename(attachment.FileName)
	attachmentWriter, err := messageWriter.CreateAttachment(attachmentHeader)
	if err != nil {
		return fmt.Errorf("创建附件 %s 失败: %w", attachment.FileName, err)
	}
	if _, err := io.Copy(attachmentWriter, bytes.NewReader(attachment.Content)); err != nil {
		_ = attachmentWriter.Close()
		return fmt.Errorf("写入附件 %s 失败: %w", attachment.FileName, err)
	}
	if err := attachmentWriter.Close(); err != nil {
		return fmt.Errorf("关闭附件 %s 失败: %w", attachment.FileName, err)
	}
	return nil
}

// sendSMTPMessage 兼容 SMTPS 和普通 SMTP/STARTTLS 两种常见服务形态。
func (s *Service) sendSMTPMessage(ctx context.Context, account model.MailAccount, password string, recipients []string, raw []byte) error {
	s.debugProviderLog("SMTP 发信开始", "email", account.Email, "host", account.SMTPHost, "port", account.SMTPPort, "recipient_count", len(recipients), "oauth", normalizeAuthType(account.AuthType) == "oauth")
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
			s.debugProviderLog("SMTP 启动 STARTTLS", "email", account.Email, "host", account.SMTPHost)
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
		s.debugProviderLog("SMTP 开始认证", "email", account.Email, "host", account.SMTPHost, "username", account.Username, "oauth", normalizeAuthType(account.AuthType) == "oauth")
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
		s.debugProviderLog("SMTP 认证成功", "email", account.Email, "host", account.SMTPHost)
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
	s.debugProviderLog("SMTP 发信完成", "email", account.Email, "host", account.SMTPHost, "recipient_count", len(recipients))
	return nil
}

// probeSMTP 以与实际发送一致的方式验证 SMTP 端口可用性，避免 587 + STARTTLS 被误判为 SMTPS。
func (s *Service) probeSMTP(ctx context.Context, account model.MailAccount) error {
	s.debugProviderLog("SMTP 连通性探测开始", "email", account.Email, "host", account.SMTPHost, "port", account.SMTPPort, "tls", account.UseTLS)
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
			err := client.StartTLS(&tls.Config{ServerName: account.SMTPHost, MinVersion: tls.VersionTLS12})
			if err == nil {
				s.debugProviderLog("SMTP 连通性探测成功", "email", account.Email, "host", account.SMTPHost, "starttls", true)
			}
			return err
		}
		if account.UseTLS {
			return fmt.Errorf("SMTP 服务端不支持 STARTTLS")
		}
	}
	s.debugProviderLog("SMTP 连通性探测成功", "email", account.Email, "host", account.SMTPHost, "starttls", false)
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
