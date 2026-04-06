package mail

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"gmbox/internal/model"
)

// TestUpsertMessageSkipsBlockedContact 确保黑名单发件人只写隐藏占位，不会把正文内容落到可见邮件列表里。
func TestUpsertMessageSkipsBlockedContact(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.Mailbox{}, &model.Message{}, &model.MessageBody{}, &model.Attachment{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	service := &Service{db: db}
	account := model.MailAccount{}
	account.Model.ID = 1

	stored, err := service.upsertMessage(account, "INBOX", 7, "", &parsedMessage{
		FromAddress: "blocked@example.com",
		Subject:     "测试邮件",
		Snippet:     "不会展示",
		TextBody:    "正文",
		SentAt:      time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC),
	}, false, map[string]struct{}{"blocked@example.com": {}})
	if err != nil {
		t.Fatalf("upsertMessage() error = %v", err)
	}
	if stored {
		t.Fatal("stored = true, want false")
	}

	var message model.Message
	if err := db.First(&message).Error; err != nil {
		t.Fatalf("读取邮件记录失败: %v", err)
	}
	if !message.IsDeleted {
		t.Fatal("message.is_deleted = false, want true")
	}
	if message.Snippet != "" {
		t.Fatalf("message.snippet = %q, want empty", message.Snippet)
	}

	var bodyCount int64
	if err := db.Model(&model.MessageBody{}).Count(&bodyCount).Error; err != nil {
		t.Fatalf("统计正文失败: %v", err)
	}
	if bodyCount != 0 {
		t.Fatalf("bodyCount = %d, want 0", bodyCount)
	}
}
