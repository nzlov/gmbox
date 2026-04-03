package mail

import (
	"io"
	"strings"
	"testing"

	message "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

// TestNormalizeMessageCharsetReaderQuotedGB2312 确认异常引号和 gb2312 别名不会再导致正文解析直接失败。
func TestNormalizeMessageCharsetReaderQuotedGB2312(t *testing.T) {
	reader, err := normalizeMessageCharsetReader(message.CharsetReader, `"gb2312"`, strings.NewReader("test"))
	if err != nil {
		t.Fatalf("normalizeMessageCharsetReader returned error: %v", err)
	}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("io.ReadAll(reader) returned error: %v", err)
	}
}

// TestParseRawMessageQuotedCharsetHeader 确认正文头里的异常 charset 仍可被完整解析。
func TestParseRawMessageQuotedCharsetHeader(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"To: receiver@example.com",
		"Subject: charset test",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"\"gb2312\"\"",
		"",
		"hello world",
	}, "\r\n")

	parsed, err := parseRawMessage([]byte(raw))
	if err != nil {
		t.Fatalf("parseRawMessage returned error: %v", err)
	}
	if strings.TrimSpace(parsed.TextBody) != "hello world" {
		t.Fatalf("parsed.TextBody = %q, want %q", parsed.TextBody, "hello world")
	}
}
