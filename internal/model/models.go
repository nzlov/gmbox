package model

import (
	"time"

	utilsdb "github.com/nzlov/utils/db"
)

// User 保存后台管理员账户。
type User struct {
	utilsdb.Model
	Username       string `gorm:"size:128;uniqueIndex;not null" json:"username"`
	PasswordHash   string `gorm:"size:255;not null" json:"-"`
	SessionVersion uint   `gorm:"not null;default:1" json:"-"`
}

// UserPreference 保存管理员界面偏好，便于跨设备同步主题设置。
type UserPreference struct {
	utilsdb.Model
	UserID         uint   `gorm:"uniqueIndex;not null" json:"user_id"`
	ThemeName      string `gorm:"size:64;not null;default:classic_blue" json:"theme_name"`
	ThemeMode      string `gorm:"size:16;not null;default:light" json:"theme_mode"`
	PrimaryColor   string `gorm:"size:16;not null;default:#2563eb" json:"primary_color"`
	SecondaryColor string `gorm:"size:16;not null;default:#7c3aed" json:"secondary_color"`
	AccentColor    string `gorm:"size:16;not null;default:#06b6d4" json:"accent_color"`
}

// MailAccount 保存外部邮箱账户连接配置。
type MailAccount struct {
	utilsdb.Model
	Name              string     `gorm:"size:128;not null" json:"name"`
	Email             string     `gorm:"size:255;not null;uniqueIndex" json:"email"`
	Provider          string     `gorm:"size:64;not null;default:custom" json:"provider"`
	ProviderName      string     `gorm:"size:128;not null;default:自定义" json:"provider_name"`
	AuthType          string     `gorm:"size:32;not null;default:password" json:"auth_type"`
	Username          string     `gorm:"size:255;not null" json:"username"`
	PasswordEncrypted string     `gorm:"type:text;not null" json:"-"`
	OAuthAccessToken  string     `gorm:"type:text" json:"-"`
	OAuthRefreshToken string     `gorm:"type:text" json:"-"`
	OAuthTokenExpiry  *time.Time `json:"oauth_token_expiry"`
	IncomingProtocol  string     `gorm:"size:16;not null" json:"incoming_protocol"`
	IMAPHost          string     `gorm:"size:255" json:"imap_host"`
	IMAPPort          int        `json:"imap_port"`
	POP3Host          string     `gorm:"size:255" json:"pop3_host"`
	POP3Port          int        `json:"pop3_port"`
	SMTPHost          string     `gorm:"size:255;not null" json:"smtp_host"`
	SMTPPort          int        `json:"smtp_port"`
	UseTLS            bool       `json:"use_tls"`
	Enabled           bool       `json:"enabled"`
}

// Mailbox 保存本地文件夹信息，便于单邮箱视图展示。
type Mailbox struct {
	utilsdb.Model
	AccountID uint   `gorm:"index;not null" json:"account_id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Path      string `gorm:"size:255;not null" json:"path"`
	Role      string `gorm:"size:64" json:"role"`
}

// Message 保存标准化后的邮件摘要。
type Message struct {
	utilsdb.Model
	AccountID     uint      `gorm:"index;not null" json:"account_id"`
	MailboxID     uint      `gorm:"index" json:"mailbox_id"`
	Folder        string    `gorm:"size:255;index" json:"folder"`
	MessageID     string    `gorm:"size:255;index" json:"message_id"`
	UID           uint32    `gorm:"index" json:"uid"`
	POP3UIDL      string    `gorm:"size:255;index" json:"pop3_uidl"`
	Subject       string    `gorm:"size:500" json:"subject"`
	FromName      string    `gorm:"size:255" json:"from_name"`
	FromAddress   string    `gorm:"size:255" json:"from_address"`
	ToAddresses   string    `gorm:"type:text" json:"to_addresses"`
	Snippet       string    `gorm:"type:text" json:"snippet"`
	IsRead        bool      `json:"is_read"`
	IsDeleted     bool      `json:"is_deleted"`
	HasAttachment bool      `json:"has_attachment"`
	SentAt        time.Time `gorm:"index" json:"sent_at"`
}

// MessageBody 保存邮件正文，并记录是否已经抓取过完整正文。
type MessageBody struct {
	utilsdb.Model
	MessageID   uint   `gorm:"uniqueIndex;not null" json:"message_id"`
	TextBody    string `gorm:"type:text" json:"text_body"`
	HTMLBody    string `gorm:"type:text" json:"html_body"`
	BodyFetched bool   `json:"body_fetched"`
}

// Attachment 保存附件元信息。
type Attachment struct {
	utilsdb.Model
	MessageID   uint   `gorm:"index;not null" json:"message_id"`
	FileName    string `gorm:"size:255;not null" json:"file_name"`
	PartID      string `gorm:"size:128" json:"part_id"`
	ContentType string `gorm:"size:255" json:"content_type"`
	Size        int64  `json:"size"`
	StoragePath string `gorm:"size:500" json:"storage_path"`
}

// ContactAggregation 保存成员联系人与主联系人的映射，避免改动原始邮件数据也能完成聚合展示。
type ContactAggregation struct {
	utilsdb.Model
	Address        string `gorm:"size:255;uniqueIndex;not null" json:"address"`
	PrimaryAddress string `gorm:"size:255;index;not null" json:"primary_address"`
}

// ContactBlacklist 保存被管理员拉黑的联系人地址，后续同步时会直接跳过这些发件人。
type ContactBlacklist struct {
	utilsdb.Model
	Address string `gorm:"size:255;uniqueIndex;not null" json:"address"`
}

// SyncState 保存邮箱同步状态和最近错误。
type SyncState struct {
	utilsdb.Model
	AccountID     uint       `gorm:"uniqueIndex;not null" json:"account_id"`
	LastIMAPUID   uint32     `json:"last_imap_uid"`
	IMAPCursorMap string     `gorm:"type:text" json:"imap_cursor_map"`
	LastPOP3UIDL  string     `gorm:"size:255" json:"last_pop3_uidl"`
	LastSyncAt    *time.Time `json:"last_sync_at"`
	LastError     string     `gorm:"type:text" json:"last_error"`
	LastStatus    string     `gorm:"size:64" json:"last_status"`
	LastMessage   string     `gorm:"type:text" json:"last_message"`
	Running       bool       `json:"running"`
	LastDuration  int64      `json:"last_duration"`
	LastMessageAt *time.Time `json:"last_message_at"`
}

// SyncLogDetail 保存单个邮箱的同步结果，避免聚合日志丢失邮箱级排查信息。
type SyncLogDetail struct {
	AccountID    uint   `json:"account_id"`
	AccountName  string `json:"account_name"`
	AccountEmail string `json:"account_email"`
	Success      bool   `json:"success"`
	NewMessages  int    `json:"new_messages"`
	DurationMs   int64  `json:"duration_ms"`
	ErrorMessage string `json:"error_message"`
}

// SyncLog 保存一次同步批次的聚合结果，便于控制日志数量同时保留邮箱明细。
type SyncLog struct {
	utilsdb.Model
	Trigger        string    `gorm:"size:32;not null" json:"trigger"`
	StartedAt      time.Time `gorm:"index" json:"started_at"`
	FinishedAt     time.Time `gorm:"index" json:"finished_at"`
	DurationMs     int64     `json:"duration_ms"`
	AccountCount   int       `json:"account_count"`
	SuccessCount   int       `json:"success_count"`
	SuccessRate    float64   `json:"success_rate"`
	SummaryMessage string    `gorm:"type:text" json:"summary_message"`
	Details        string    `gorm:"type:text" json:"details"`
}
