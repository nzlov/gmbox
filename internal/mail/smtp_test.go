package mail

import (
	"bytes"
	"io"
	"testing"

	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
	"gmbox/internal/model"
)

// TestBuildSMTPMessageWithAttachments 确保转发附件时会生成 multipart 邮件并保留附件内容。
func TestBuildSMTPMessageWithAttachments(t *testing.T) {
	account := model.MailAccount{
		Name:     "测试邮箱",
		Email:    "sender@example.com",
		SMTPHost: "smtp.example.com",
	}
	input := SendInput{
		To:      []string{"receiver@example.com"},
		Cc:      []string{"copy@example.com"},
		Subject: "附件转发测试",
		Body:    "这是一封带附件的测试邮件。",
	}
	attachments := []sendAttachment{
		{
			FileName:    "note.txt",
			ContentType: "text/plain",
			Content:     []byte("附件内容"),
		},
	}

	raw, recipients, err := buildSMTPMessage(account, input, attachments)
	if err != nil {
		t.Fatalf("buildSMTPMessage returned error: %v", err)
	}
	if len(recipients) != 2 {
		t.Fatalf("len(recipients) = %d, want 2", len(recipients))
	}
	if recipients[0] != "receiver@example.com" || recipients[1] != "copy@example.com" {
		t.Fatalf("recipients = %#v, want [receiver@example.com copy@example.com]", recipients)
	}

	reader, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gomail.CreateReader returned error: %v", err)
	}
	defer reader.Close()

	subject, err := reader.Header.Subject()
	if err != nil {
		t.Fatalf("reader.Header.Subject returned error: %v", err)
	}
	if subject != input.Subject {
		t.Fatalf("subject = %q, want %q", subject, input.Subject)
	}

	partIndex := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reader.NextPart returned error: %v", err)
		}

		body, err := io.ReadAll(part.Body)
		if err != nil {
			t.Fatalf("io.ReadAll(part.Body) returned error: %v", err)
		}

		switch partIndex {
		case 0:
			inlineHeader, ok := part.Header.(*gomail.InlineHeader)
			if !ok {
				t.Fatalf("part.Header type = %T, want *gomail.InlineHeader", part.Header)
			}
			mediaType, _, _ := inlineHeader.ContentType()
			if mediaType != "text/plain" {
				t.Fatalf("inline mediaType = %q, want text/plain", mediaType)
			}
			if string(body) != input.Body {
				t.Fatalf("inline body = %q, want %q", string(body), input.Body)
			}
		case 1:
			attachmentHeader, ok := part.Header.(*gomail.AttachmentHeader)
			if !ok {
				t.Fatalf("part.Header type = %T, want *gomail.AttachmentHeader", part.Header)
			}
			fileName, err := attachmentHeader.Filename()
			if err != nil {
				t.Fatalf("attachmentHeader.Filename returned error: %v", err)
			}
			if fileName != attachments[0].FileName {
				t.Fatalf("attachment filename = %q, want %q", fileName, attachments[0].FileName)
			}
			if string(body) != string(attachments[0].Content) {
				t.Fatalf("attachment body = %q, want %q", string(body), string(attachments[0].Content))
			}
		default:
			t.Fatalf("unexpected extra part at index %d", partIndex)
		}
		partIndex++
	}

	if partIndex != 2 {
		t.Fatalf("part count = %d, want 2", partIndex)
	}
}
