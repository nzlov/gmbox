package mail

import "testing"

// TestMicrosoftRefreshTokenFormOmitsEmptyRedirectURI 确保动态回调模式下不会把空 redirect_uri 传给微软 token 端点。
func TestMicrosoftRefreshTokenFormOmitsEmptyRedirectURI(t *testing.T) {
	form := microsoftRefreshTokenForm("refresh-token", "")
	if got := form.Get("redirect_uri"); got != "" {
		t.Fatalf("redirect_uri = %q, want empty", got)
	}
	if got := form.Get("grant_type"); got != "refresh_token" {
		t.Fatalf("grant_type = %q, want refresh_token", got)
	}
	if got := form.Get("refresh_token"); got != "refresh-token" {
		t.Fatalf("refresh_token = %q, want refresh-token", got)
	}
	if got := form.Get("scope"); got != microsoftAuthorizeScope {
		t.Fatalf("scope = %q, want %q", got, microsoftAuthorizeScope)
	}
}

// TestMicrosoftRefreshTokenFormKeepsConfiguredRedirectURI 确保显式配置固定回调地址时，刷新 token 仍沿用同一 redirect_uri。
func TestMicrosoftRefreshTokenFormKeepsConfiguredRedirectURI(t *testing.T) {
	form := microsoftRefreshTokenForm("refresh-token", " https://mail.example.com/oauth/microsoft/callback ")
	if got := form.Get("redirect_uri"); got != "https://mail.example.com/oauth/microsoft/callback" {
		t.Fatalf("redirect_uri = %q, want https://mail.example.com/oauth/microsoft/callback", got)
	}
}
