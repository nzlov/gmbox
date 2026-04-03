package mail

import "testing"

// TestShouldRetryOAuthSync 确认只有认证或 token 相关错误才会触发自动刷新重试。
func TestShouldRetryOAuthSync(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "oauth 登录失败", err: assertErr("IMAP OAuth 登录失败: imap: connection closed"), want: true},
		{name: "token 过期", err: assertErr("请求微软 token 失败: invalid_grant expired token"), want: true},
		{name: "正文解析失败", err: assertErr("解析邮件正文失败: unknown charset"), want: false},
		{name: "空错误", err: nil, want: false},
	}
	for _, tt := range tests {
		if got := shouldRetryOAuthSync(tt.err); got != tt.want {
			t.Fatalf("%s: shouldRetryOAuthSync() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestSyncResultZeroValue 确认同步统计结构的零值可安全用于失败日志落库。
func TestSyncResultZeroValue(t *testing.T) {
	result := &SyncResult{}
	if result.NewMessages != 0 || result.MailboxCount != 0 {
		t.Fatalf("unexpected zero value result: %+v", result)
	}
}

// assertErr 用最小方式构造错误值，避免测试里重复样板代码。
func assertErr(message string) error {
	return testErr(message)
}

type testErr string

func (e testErr) Error() string {
	return string(e)
}
