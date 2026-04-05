package httpapi

import (
	"net/url"
	"testing"
)

// TestRedactQueryForLog 确保调试日志不会泄漏 OAuth 回调里的授权码和 PKCE 凭证。
func TestRedactQueryForLog(t *testing.T) {
	target, err := url.Parse("https://example.com/oauth/microsoft/callback?code=secret-code&state=secret-state&code_verifier=secret-verifier&plain=value")
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}
	redacted := redactQueryForLog(target)
	values, err := url.ParseQuery(redacted)
	if err != nil {
		t.Fatalf("解析脱敏后的 query 失败: %v", err)
	}
	if values.Get("code") != "<redacted>" {
		t.Fatalf("code = %q, want <redacted>", values.Get("code"))
	}
	if values.Get("state") != "<redacted>" {
		t.Fatalf("state = %q, want <redacted>", values.Get("state"))
	}
	if values.Get("code_verifier") != "<redacted>" {
		t.Fatalf("code_verifier = %q, want <redacted>", values.Get("code_verifier"))
	}
	if values.Get("plain") != "value" {
		t.Fatalf("plain = %q, want value", values.Get("plain"))
	}
}
