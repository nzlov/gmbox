package mail

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"gmbox/internal/model"
)

// newTestMailService 创建内存数据库，便于验证正文缓存写入策略。
func newTestMailService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Mailbox{}, &model.Message{}, &model.MessageBody{}, &model.Attachment{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	return &Service{db: db}
}

// TestUpsertMessagePreservesFetchedBody 避免关闭抓取正文后把已缓存的完整正文降级回摘要。
func TestUpsertMessagePreservesFetchedBody(t *testing.T) {
	service := newTestMailService(t)
	account := model.MailAccount{Model: model.MailAccount{}.Model}
	account.ID = 1
	parsed := &parsedMessage{
		MessageID:     "msg-1",
		Subject:       "测试主题",
		FromAddress:   "sender@example.com",
		Snippet:       "新的摘要",
		TextBody:      "完整正文",
		HTMLBody:      "<p>完整正文</p>",
		SentAt:        time.Now(),
		HasAttachment: false,
	}
	if err := service.upsertMessage(account, "INBOX", 101, "", parsed, true); err != nil {
		t.Fatalf("首次写入邮件失败: %v", err)
	}
	if err := service.upsertMessage(account, "INBOX", 101, "", &parsedMessage{
		MessageID:   "msg-1",
		Subject:     "测试主题",
		FromAddress: "sender@example.com",
		Snippet:     "被覆盖摘要",
		TextBody:    "不应覆盖的正文",
		SentAt:      parsed.SentAt,
	}, false); err != nil {
		t.Fatalf("关闭正文抓取后的更新失败: %v", err)
	}
	var body model.MessageBody
	if err := service.db.Where("message_id = ?", 1).First(&body).Error; err != nil {
		t.Fatalf("读取正文缓存失败: %v", err)
	}
	if !body.BodyFetched {
		t.Fatalf("body.BodyFetched = false, want true")
	}
	if body.TextBody != "完整正文" {
		t.Fatalf("body.TextBody = %q, want %q", body.TextBody, "完整正文")
	}
	if body.HTMLBody != "<p>完整正文</p>" {
		t.Fatalf("body.HTMLBody = %q, want %q", body.HTMLBody, "<p>完整正文</p>")
	}
}

// TestUpsertMessageStoresSnippetWhenBodyDisabled 确认未启用正文抓取时只缓存摘要，并标记为未抓全量正文。
func TestUpsertMessageStoresSnippetWhenBodyDisabled(t *testing.T) {
	service := newTestMailService(t)
	account := model.MailAccount{Model: model.MailAccount{}.Model}
	account.ID = 2
	if err := service.upsertMessage(account, "INBOX", 202, "", &parsedMessage{
		MessageID:   "msg-2",
		Subject:     "摘要邮件",
		FromAddress: "sender@example.com",
		Snippet:     "只保存摘要",
		TextBody:    "完整正文",
		HTMLBody:    "<p>完整正文</p>",
		SentAt:      time.Now(),
	}, false); err != nil {
		t.Fatalf("写入摘要邮件失败: %v", err)
	}
	var body model.MessageBody
	if err := service.db.Where("message_id = ?", 1).First(&body).Error; err != nil {
		t.Fatalf("读取摘要正文失败: %v", err)
	}
	if body.BodyFetched {
		t.Fatalf("body.BodyFetched = true, want false")
	}
	if body.TextBody != "只保存摘要" {
		t.Fatalf("body.TextBody = %q, want %q", body.TextBody, "只保存摘要")
	}
	if body.HTMLBody != "" {
		t.Fatalf("body.HTMLBody = %q, want empty", body.HTMLBody)
	}
}

// TestShouldFetchMessageBodyKeepsLegacyFullBody 确保历史已缓存完整正文的数据不会因为缺少标记而被强制回源。
func TestShouldFetchMessageBodyKeepsLegacyFullBody(t *testing.T) {
	message := &model.Message{Snippet: "摘要"}
	if !shouldFetchMessageBody(gorm.ErrRecordNotFound, nil, message) {
		t.Fatalf("缺少正文记录时应触发回源")
	}
	if shouldFetchMessageBody(nil, &model.MessageBody{TextBody: "完整正文", BodyFetched: false}, message) {
		t.Fatalf("历史完整正文不应被误判为需要回源")
	}
	if shouldFetchMessageBody(nil, &model.MessageBody{HTMLBody: "<p>完整正文</p>", BodyFetched: false}, message) {
		t.Fatalf("带 HTML 的历史正文不应被误判为需要回源")
	}
	if !shouldFetchMessageBody(nil, &model.MessageBody{TextBody: "摘要", BodyFetched: false}, message) {
		t.Fatalf("仅有摘要缓存时应触发回源")
	}
}

// TestFallbackIMAPMailboxesForOutlookOAuth 确保微软 OAuth 在 LIST 失败时仍能回退到 INBOX 和已缓存目录继续同步。
func TestFallbackIMAPMailboxesForOutlookOAuth(t *testing.T) {
	service := newTestMailService(t)
	account := model.MailAccount{
		Provider: "outlook",
		AuthType: "oauth",
	}
	account.ID = 3
	seed := []model.Mailbox{
		{AccountID: account.ID, Name: "Sent Items", Path: "Sent Items", Role: "sent"},
		{AccountID: account.ID, Name: "INBOX", Path: "INBOX", Role: "inbox"},
	}
	for _, mailbox := range seed {
		if err := service.db.Create(&mailbox).Error; err != nil {
			t.Fatalf("写入缓存文件夹失败: %v", err)
		}
	}
	mailboxes, err := service.fallbackIMAPMailboxes(account, errors.New("列目录失败"))
	if err != nil {
		t.Fatalf("回退文件夹失败: %v", err)
	}
	if len(mailboxes) != 2 {
		t.Fatalf("len(mailboxes) = %d, want 2", len(mailboxes))
	}
	if mailboxes[0].Path != "INBOX" {
		t.Fatalf("mailboxes[0].Path = %q, want INBOX", mailboxes[0].Path)
	}
	if mailboxes[1].Path != "Sent Items" {
		t.Fatalf("mailboxes[1].Path = %q, want Sent Items", mailboxes[1].Path)
	}
}

// TestFallbackIMAPMailboxesRejectsNonOutlook 确保普通 IMAP 账户仍保留原始 LIST 错误，避免无意隐藏真实配置问题。
func TestFallbackIMAPMailboxesRejectsNonOutlook(t *testing.T) {
	service := newTestMailService(t)
	originalErr := errors.New("列目录失败")
	_, err := service.fallbackIMAPMailboxes(model.MailAccount{Provider: "gmail", AuthType: "oauth"}, originalErr)
	if !errors.Is(err, originalErr) {
		t.Fatalf("err = %v, want %v", err, originalErr)
	}
}

// TestIMAPOAuthMechanismOrderForOutlook 确保微软 IMAP 优先走更稳定的 XOAUTH2，减少 OAUTHBEARER 挑战把连接读坏的概率。
func TestIMAPOAuthMechanismOrderForOutlook(t *testing.T) {
	order := imapOAuthMechanismOrder(model.MailAccount{Provider: "outlook"})
	if len(order) != 2 {
		t.Fatalf("len(order) = %d, want 2", len(order))
	}
	if order[0] != imapOAuthMechXOAUTH2 {
		t.Fatalf("order[0] = %q, want %q", order[0], imapOAuthMechXOAUTH2)
	}
	if order[1] != imapOAuthMechOAuthBearer {
		t.Fatalf("order[1] = %q, want %q", order[1], imapOAuthMechOAuthBearer)
	}
}

// TestIMAPOAuthMechanismOrderForGenericProvider 保持非微软服务商默认优先 RFC 标准机制，避免无关账号行为回归。
func TestIMAPOAuthMechanismOrderForGenericProvider(t *testing.T) {
	order := imapOAuthMechanismOrder(model.MailAccount{Provider: "gmail"})
	if len(order) != 2 {
		t.Fatalf("len(order) = %d, want 2", len(order))
	}
	if order[0] != imapOAuthMechOAuthBearer {
		t.Fatalf("order[0] = %q, want %q", order[0], imapOAuthMechOAuthBearer)
	}
	if order[1] != imapOAuthMechXOAUTH2 {
		t.Fatalf("order[1] = %q, want %q", order[1], imapOAuthMechXOAUTH2)
	}
}

// TestSelectIMAPOAuthMechanismsPrefersDeclaredXOAUTH2 确保服务端声明 XOAUTH2 时优先尝试它。
func TestSelectIMAPOAuthMechanismsPrefersDeclaredXOAUTH2(t *testing.T) {
	selected := selectIMAPOAuthMechanisms(imap.CapSet{imap.AuthCap(imapOAuthMechXOAUTH2): {}}, model.MailAccount{Provider: "outlook"})
	if len(selected) != 2 {
		t.Fatalf("len(selected) = %d, want 2", len(selected))
	}
	if selected[0] != imapOAuthMechXOAUTH2 {
		t.Fatalf("selected[0] = %q, want %q", selected[0], imapOAuthMechXOAUTH2)
	}
	if selected[1] != imapOAuthMechOAuthBearer {
		t.Fatalf("selected[1] = %q, want %q", selected[1], imapOAuthMechOAuthBearer)
	}
}

// TestShouldRetryIMAPMailboxSelectForOutlookOAuth 确保仅对微软 OAuth 的邮箱会话未连接错误触发一次重连重试。
func TestShouldRetryIMAPMailboxSelectForOutlookOAuth(t *testing.T) {
	err := errors.New("选择文件夹 INBOX 失败: imap: BAD User is authenticated but not connected.")
	if !shouldRetryIMAPMailboxSelect(model.MailAccount{Provider: "outlook", AuthType: "oauth"}, err) {
		t.Fatalf("expected retry for outlook oauth mailbox select error")
	}
	if shouldRetryIMAPMailboxSelect(model.MailAccount{Provider: "gmail", AuthType: "oauth"}, err) {
		t.Fatalf("did not expect retry for non-outlook provider")
	}
	if shouldRetryIMAPMailboxSelect(model.MailAccount{Provider: "outlook", AuthType: "password"}, err) {
		t.Fatalf("did not expect retry for non-oauth auth type")
	}
}

// TestDecorateIMAPOAuthErrorUsesOutlookResponseCode 确保 Outlook 返回非标准带引号错误码时，日志中的响应码会被翻译成人可读错误。
func TestDecorateIMAPOAuthErrorUsesOutlookResponseCode(t *testing.T) {
	service := newTestMailService(t)
	account := model.MailAccount{Provider: "outlook", Email: "test@example.com", IMAPHost: "outlook.office365.com"}
	service.rememberIMAPDebugLine(account, `T2 OK [Error="UserDisabled" AuthResult=27] AUTHENTICATE completed.`)
	err := service.decorateIMAPOAuthError(account, errors.New(`in resp-text: imapwire: expected ']', got "\""`))
	if err == nil {
		t.Fatalf("expected translated error")
	}
	if !strings.Contains(err.Error(), "UserDisabled") {
		t.Fatalf("err = %q, want contains UserDisabled", err.Error())
	}
	if !strings.Contains(err.Error(), "未启用 IMAP") && !strings.Contains(err.Error(), "服务端限制") {
		t.Fatalf("err = %q, want readable outlook hint", err.Error())
	}
}

// TestDecorateIMAPOAuthErrorKeepsUnrelatedErrors 确保普通认证失败仍保留原始错误，避免掩盖非 Outlook 的真实问题。
func TestDecorateIMAPOAuthErrorKeepsUnrelatedErrors(t *testing.T) {
	service := newTestMailService(t)
	original := errors.New("imap: BAD Command Argument Error. 12")
	err := service.decorateIMAPOAuthError(model.MailAccount{Provider: "gmail"}, original)
	if !errors.Is(err, original) {
		t.Fatalf("err = %v, want original error", err)
	}
}

// TestDecorateIMAPOAuthErrorUsesLatestOutlookResponseCode 确保同一账号多次认证失败时优先使用最近一条响应，避免旧错误码串到新重试上。
func TestDecorateIMAPOAuthErrorUsesLatestOutlookResponseCode(t *testing.T) {
	service := newTestMailService(t)
	account := model.MailAccount{Provider: "outlook", Email: "test@example.com", IMAPHost: "outlook.office365.com"}
	service.rememberIMAPDebugLine(account, `T2 OK [Error="OldError" AuthResult=1] AUTHENTICATE completed.`)
	service.rememberIMAPDebugLine(account, `T3 OK [Error="UserDisabled" AuthResult=27] AUTHENTICATE completed.`)
	err := service.decorateIMAPOAuthError(account, errors.New(`in resp-text: imapwire: expected ']', got "`))
	if err == nil {
		t.Fatalf("expected translated error")
	}
	if !strings.Contains(err.Error(), "UserDisabled") {
		t.Fatalf("err = %q, want contains latest code UserDisabled", err.Error())
	}
	if strings.Contains(err.Error(), "OldError") {
		t.Fatalf("err = %q, should not use stale code", err.Error())
	}
}

// TestClearIMAPDebugLinesRemovesCachedLines 确保新一轮认证前可以清空旧交互，避免不同机制重试彼此污染错误上下文。
func TestClearIMAPDebugLinesRemovesCachedLines(t *testing.T) {
	service := newTestMailService(t)
	account := model.MailAccount{Provider: "outlook", Email: "test@example.com", IMAPHost: "outlook.office365.com"}
	service.rememberIMAPDebugLine(account, `T2 OK [Error="UserDisabled" AuthResult=27] AUTHENTICATE completed.`)
	service.clearIMAPDebugLines(account)
	if lines := service.recentIMAPDebugLines(account); len(lines) != 0 {
		t.Fatalf("len(lines) = %d, want 0", len(lines))
	}
}
