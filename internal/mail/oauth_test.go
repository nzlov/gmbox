package mail

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	appcfg "gmbox/internal/config"
	cryptosvc "gmbox/internal/crypto"
	"gmbox/internal/model"
)

// newOAuthTestService 创建带 MailAccount 表和加密服务的测试实例，便于覆盖 OAuth 账户复用逻辑。
func newOAuthTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.MailAccount{}); err != nil {
		t.Fatalf("迁移 MailAccount 表失败: %v", err)
	}
	return &Service{
		db:     db,
		crypto: cryptosvc.NewAESService("oauth-test-secret"),
		cfg:    &appcfg.Config{},
	}
}

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

// TestRestoreSoftDeletedOAuthAccount 确保删除过的邮箱重新 OAuth 授权时复用原记录，避免唯一索引阻止重新创建。
func TestRestoreSoftDeletedOAuthAccount(t *testing.T) {
	service := newOAuthTestService(t)
	account := model.MailAccount{
		Email:             "restored@example.com",
		Username:          "restored@example.com",
		Name:              "旧账户",
		Provider:          "outlook",
		ProviderName:      "Outlook",
		AuthType:          "oauth",
		IncomingProtocol:  "imap",
		IMAPHost:          "outlook.office365.com",
		IMAPPort:          993,
		SMTPHost:          "smtp.office365.com",
		SMTPPort:          587,
		UseTLS:            true,
		Enabled:           true,
		PasswordEncrypted: "",
	}
	if err := service.db.Create(&account).Error; err != nil {
		t.Fatalf("创建历史账户失败: %v", err)
	}
	if err := service.db.Delete(&account).Error; err != nil {
		t.Fatalf("软删历史账户失败: %v", err)
	}

	var restored model.MailAccount
	err := service.db.Unscoped().Where("email = ?", account.Email).First(&restored).Error
	if err != nil {
		t.Fatalf("读取软删账户失败: %v", err)
	}
	if !restored.DeletedAt.Valid {
		t.Fatalf("DeletedAt.Valid = false, want true")
	}

	token := microsoftTokenResponse{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600}
	if err := service.storeOAuthToken(&restored, token); err != nil {
		t.Fatalf("保存 token 失败: %v", err)
	}
	restored.DeletedAt = gorm.DeletedAt{}
	restored.Name = "新账户"
	restored.Enabled = true
	if err := service.db.Save(&restored).Error; err != nil {
		t.Fatalf("恢复软删账户失败: %v", err)
	}

	var all []model.MailAccount
	if err := service.db.Unscoped().Find(&all).Error; err != nil {
		t.Fatalf("查询账户列表失败: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1", len(all))
	}
	if all[0].DeletedAt.Valid {
		t.Fatalf("restored.DeletedAt.Valid = true, want false")
	}
	if all[0].Name != "新账户" {
		t.Fatalf("restored.Name = %q, want 新账户", all[0].Name)
	}
}

// TestRedactFormForLogMasksCodeVerifier 确保 PKCE 的 code_verifier 不会在调试日志中泄漏。
func TestRedactFormForLogMasksCodeVerifier(t *testing.T) {
	form := map[string][]string{
		"code":          {"secret-code"},
		"code_verifier": {"secret-verifier"},
		"scope":         {microsoftAuthorizeScope},
	}
	redacted := redactFormForLog(form)
	if redacted["code"] != "<redacted>" {
		t.Fatalf("code = %q, want <redacted>", redacted["code"])
	}
	if redacted["code_verifier"] != "<redacted>" {
		t.Fatalf("code_verifier = %q, want <redacted>", redacted["code_verifier"])
	}
	if redacted["scope"] != microsoftAuthorizeScope {
		t.Fatalf("scope = %q, want %q", redacted["scope"], microsoftAuthorizeScope)
	}
}
