package mail

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	gomail "github.com/emersion/go-message/mail"
	"gmbox/internal/model"
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// parsedMessage 保存协议层解析后的统一邮件结构，便于复用入库逻辑。
type parsedMessage struct {
	MessageID     string
	Subject       string
	FromName      string
	FromAddress   string
	ToAddresses   string
	Snippet       string
	TextBody      string
	HTMLBody      string
	HasAttachment bool
	SentAt        time.Time
	IsRead        bool
}

// parseRawMessage 统一解析 IMAP/POP3 获取到的原始 RFC822 内容。
func parseRawMessage(raw []byte) (*parsedMessage, error) {
	reader, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解析邮件正文失败: %w", err)
	}

	parsed := &parsedMessage{}
	if subject, err := reader.Header.Subject(); err == nil {
		parsed.Subject = subject
	}
	if msgID, err := reader.Header.MessageID(); err == nil {
		parsed.MessageID = msgID
	}
	if sentAt, err := reader.Header.Date(); err == nil {
		parsed.SentAt = sentAt
	}
	if fromList, err := reader.Header.AddressList("From"); err == nil && len(fromList) > 0 {
		parsed.FromName = fromList[0].Name
		parsed.FromAddress = fromList[0].Address
	}
	if toList, err := reader.Header.AddressList("To"); err == nil && len(toList) > 0 {
		parsed.ToAddresses = joinMailAddresses(toList)
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("遍历邮件分段失败: %w", err)
		}

		switch header := part.Header.(type) {
		case *gomail.InlineHeader:
			contentType, _, _ := header.ContentType()
			body, readErr := io.ReadAll(part.Body)
			if readErr != nil {
				return nil, fmt.Errorf("读取邮件分段失败: %w", readErr)
			}
			if strings.HasPrefix(contentType, "text/plain") {
				parsed.TextBody += string(body)
			}
			if strings.HasPrefix(contentType, "text/html") {
				parsed.HTMLBody += string(body)
			}
		case *gomail.AttachmentHeader:
			parsed.HasAttachment = true
		}
	}

	parsed.Snippet = buildSnippet(parsed.TextBody, parsed.HTMLBody)
	return parsed, nil
}

// enrichFromEnvelope 用 IMAP ENVELOPE 补齐部分服务端未出现在正文头里的元信息。
func (p *parsedMessage) enrichFromEnvelope(envelope *imap.Envelope, flags []string) {
	if envelope == nil {
		p.IsRead = hasSeenFlag(flags)
		return
	}
	if p.Subject == "" {
		p.Subject = envelope.Subject
	}
	if p.MessageID == "" {
		p.MessageID = envelope.MessageId
	}
	if p.SentAt.IsZero() {
		p.SentAt = envelope.Date
	}
	if p.FromAddress == "" && len(envelope.From) > 0 {
		p.FromName = envelope.From[0].PersonalName
		p.FromAddress = envelope.From[0].Address()
	}
	if p.ToAddresses == "" && len(envelope.To) > 0 {
		p.ToAddresses = joinIMAPAddresses(envelope.To)
	}
	p.IsRead = hasSeenFlag(flags)
}

// applyToMessage 将解析结果映射到数据库模型，避免协议层直接拼装 ORM 细节。
func (p *parsedMessage) applyToMessage(target *model.Message, accountID uint, folder string) {
	target.AccountID = accountID
	target.Folder = folder
	target.MessageID = p.MessageID
	target.Subject = p.Subject
	target.FromName = p.FromName
	target.FromAddress = p.FromAddress
	target.ToAddresses = p.ToAddresses
	target.Snippet = p.Snippet
	target.IsRead = p.IsRead
	target.HasAttachment = p.HasAttachment
	target.SentAt = p.SentAt
}

// joinMailAddresses 统一格式化标准库地址列表，减少前端解析复杂度。
func joinMailAddresses(addrs []*gomail.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		parts = append(parts, addr.String())
	}
	return strings.Join(parts, ", ")
}

// joinIMAPAddresses 统一格式化 IMAP 地址列表。
func joinIMAPAddresses(addrs []*imap.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr == nil {
			continue
		}
		label := addr.Address()
		if strings.TrimSpace(addr.PersonalName) != "" {
			label = fmt.Sprintf("%s <%s>", addr.PersonalName, addr.Address())
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}

// buildSnippet 优先从纯文本生成摘要，避免 HTML 标签直接泄漏到列表页。
func buildSnippet(textBody string, htmlBody string) string {
	source := strings.TrimSpace(textBody)
	if source == "" {
		source = strings.TrimSpace(htmlTagPattern.ReplaceAllString(htmlBody, " "))
	}
	source = strings.Join(strings.Fields(source), " ")
	runes := []rune(source)
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return source
}

// hasSeenFlag 将 IMAP flags 转换为统一已读状态。
func hasSeenFlag(flags []string) bool {
	for _, flag := range flags {
		if strings.EqualFold(flag, imap.SeenFlag) {
			return true
		}
	}
	return false
}
