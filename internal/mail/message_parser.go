package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	message "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
	"gmbox/internal/model"
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func init() {
	baseReader := message.CharsetReader
	message.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return normalizeMessageCharsetReader(baseReader, charset, input)
	}
	imap.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return normalizeMessageCharsetReader(baseReader, charset, input)
	}
}

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
	Attachments   []parsedAttachment
}

// parsedAttachment 保存附件元数据和内容，便于后续统一落盘。
type parsedAttachment struct {
	FileName    string
	ContentType string
	Data        []byte
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
			body, readErr := io.ReadAll(part.Body)
			if readErr != nil {
				return nil, fmt.Errorf("读取附件失败: %w", readErr)
			}
			filename, _ := header.Filename()
			contentType, _, _ := header.ContentType()
			if strings.TrimSpace(contentType) == "" {
				contentType = mime.TypeByExtension(extensionFromName(filename))
			}
			parsed.Attachments = append(parsed.Attachments, parsedAttachment{
				FileName:    fallbackAttachmentName(filename, len(parsed.Attachments)+1),
				ContentType: contentType,
				Data:        body,
			})
		}
	}

	parsed.Snippet = buildSnippet(parsed.TextBody, parsed.HTMLBody)
	return parsed, nil
}

// normalizeMessageCharsetReader 清洗异常 charset 写法，避免带引号的 gb2312 一类头部直接导致整封邮件解析失败。
func normalizeMessageCharsetReader(baseReader func(string, io.Reader) (io.Reader, error), charset string, input io.Reader) (io.Reader, error) {
	normalized := strings.TrimSpace(charset)
	normalized = strings.Trim(normalized, `"'`)
	if strings.EqualFold(normalized, "gb2312") {
		// 很多中文邮件实际按 GBK/GB18030 发送，但头部仍写成 gb2312，这里统一放宽兼容。
		normalized = "gb18030"
	}
	if normalized == "" {
		normalized = strings.TrimSpace(charset)
	}
	if baseReader == nil {
		return nil, fmt.Errorf("unknown charset: %s", normalized)
	}
	reader, err := baseReader(normalized, input)
	if err == nil {
		return reader, nil
	}
	if normalized == charset {
		return nil, err
	}
	return baseReader(strings.TrimSpace(charset), input)
}

// fallbackAttachmentName 为缺失文件名的附件生成可用名称，避免落盘失败。
func fallbackAttachmentName(name string, index int) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("attachment-%d.bin", index)
}

// extensionFromName 为 MIME 类型推断提供扩展名。
func extensionFromName(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return ""
	}
	return "." + parts[len(parts)-1]
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
