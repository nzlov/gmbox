package mail

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"gorm.io/gorm"

	"gmbox/internal/crypto"
	"gmbox/internal/model"
)

// Service 提供邮箱账户的基础连接验证与后续协议扩展点。
type Service struct {
	db     *gorm.DB
	crypto *crypto.AESService
}

// NewService 创建邮件服务实例。
func NewService(db *gorm.DB, cryptoSvc *crypto.AESService) *Service {
	return &Service{db: db, crypto: cryptoSvc}
}

// AccountInput 描述前端提交的邮箱账户表单。
type AccountInput struct {
	Name             string `json:"name" binding:"required"`
	Email            string `json:"email" binding:"required,email"`
	Username         string `json:"username" binding:"required"`
	Password         string `json:"password"`
	IncomingProtocol string `json:"incoming_protocol" binding:"required,oneof=imap pop3"`
	IMAPHost         string `json:"imap_host"`
	IMAPPort         int    `json:"imap_port"`
	POP3Host         string `json:"pop3_host"`
	POP3Port         int    `json:"pop3_port"`
	SMTPHost         string `json:"smtp_host" binding:"required"`
	SMTPPort         int    `json:"smtp_port" binding:"required"`
	UseTLS           bool   `json:"use_tls"`
	Enabled          bool   `json:"enabled"`
}

// SaveAccount 负责保存邮箱账户并在入库前完成密码加密。
func (s *Service) SaveAccount(existing *model.MailAccount, input AccountInput) error {
	if existing == nil {
		existing = &model.MailAccount{}
	}
	if existing.Model.ID == 0 && strings.TrimSpace(input.Password) == "" {
		return fmt.Errorf("新建邮箱时密码或授权码不能为空")
	}
	if existing.Model.ID > 0 && strings.TrimSpace(input.Password) == "" && strings.TrimSpace(existing.PasswordEncrypted) == "" {
		return fmt.Errorf("当前邮箱缺少可用密码，请重新填写密码或授权码")
	}
	existing.Name = input.Name
	existing.Email = input.Email
	existing.Username = input.Username
	existing.IncomingProtocol = input.IncomingProtocol
	existing.IMAPHost = input.IMAPHost
	existing.IMAPPort = input.IMAPPort
	existing.POP3Host = input.POP3Host
	existing.POP3Port = input.POP3Port
	existing.SMTPHost = input.SMTPHost
	existing.SMTPPort = input.SMTPPort
	existing.UseTLS = input.UseTLS
	existing.Enabled = input.Enabled

	if strings.TrimSpace(input.Password) != "" {
		ciphertext, err := s.crypto.Encrypt(input.Password)
		if err != nil {
			return err
		}
		existing.PasswordEncrypted = ciphertext
	}

	if existing.Model.ID == 0 {
		return s.db.Create(existing).Error
	}
	return s.db.Save(existing).Error
}

// DecryptPassword 为协议拨号恢复邮箱凭证。
func (s *Service) DecryptPassword(account model.MailAccount) (string, error) {
	return s.crypto.Decrypt(account.PasswordEncrypted)
}

// TestConnection 对入站和出站地址做最小连通性验证，避免明显配置错误直接入库。
func (s *Service) TestConnection(ctx context.Context, account model.MailAccount) error {
	if _, err := s.DecryptPassword(account); err != nil {
		return err
	}
	if account.IncomingProtocol == "imap" {
		if err := probe(ctx, account.IMAPHost, account.IMAPPort, account.UseTLS); err != nil {
			return fmt.Errorf("IMAP 连接失败: %w", err)
		}
	}
	if account.IncomingProtocol == "pop3" {
		if err := probe(ctx, account.POP3Host, account.POP3Port, account.UseTLS); err != nil {
			return fmt.Errorf("POP3 连接失败: %w", err)
		}
	}
	if err := probe(ctx, account.SMTPHost, account.SMTPPort, account.UseTLS); err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	return nil
}

// probe 以统一方式检测远端端口是否可连通。
func probe(ctx context.Context, host string, port int, useTLS bool) error {
	if strings.TrimSpace(host) == "" || port <= 0 {
		return fmt.Errorf("主机或端口未配置")
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	if useTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		return conn.Close()
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// GetMessageDetail 读取邮件详情、正文与附件信息，供前端详情页复用。
func (s *Service) GetMessageDetail(messageID uint) (*model.Message, *model.MessageBody, []model.Attachment, error) {
	var message model.Message
	if err := s.db.First(&message, messageID).Error; err != nil {
		return nil, nil, nil, err
	}
	var body model.MessageBody
	if err := s.db.Where("message_id = ?", message.Model.ID).First(&body).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, nil, nil, err
	}
	var attachments []model.Attachment
	if err := s.db.Where("message_id = ?", message.Model.ID).Order("id asc").Find(&attachments).Error; err != nil {
		return nil, nil, nil, err
	}
	return &message, &body, attachments, nil
}

// ListMailboxes 返回指定账户下已同步的文件夹，便于前端渲染目录树。
func (s *Service) ListMailboxes(accountID uint) ([]model.Mailbox, error) {
	var mailboxes []model.Mailbox
	query := s.db.Order("name asc")
	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	if err := query.Find(&mailboxes).Error; err != nil {
		return nil, err
	}
	return mailboxes, nil
}

// DownloadAttachment 读取附件记录及其本地文件内容，用于下载接口。
func (s *Service) DownloadAttachment(attachmentID uint) (*model.Attachment, []byte, error) {
	var attachment model.Attachment
	if err := s.db.First(&attachment, attachmentID).Error; err != nil {
		return nil, nil, err
	}
	content, err := os.ReadFile(attachment.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return &attachment, content, nil
}

// SetMessageRead 更新邮件已读状态，并同步到 IMAP 远端。
func (s *Service) SetMessageRead(ctx context.Context, messageID uint, isRead bool) error {
	var message model.Message
	if err := s.db.First(&message, messageID).Error; err != nil {
		return err
	}
	var account model.MailAccount
	if err := s.db.First(&account, message.AccountID).Error; err != nil {
		return err
	}
	if account.IncomingProtocol == "imap" && message.UID > 0 {
		password, err := s.DecryptPassword(account)
		if err != nil {
			return err
		}
		client, err := dialIMAP(account, password)
		if err != nil {
			return err
		}
		defer client.Logout()
		if _, err := client.Select(message.Folder, false); err != nil {
			return err
		}
		seqset := newUIDSet(message.UID)
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.SeenFlag}
		if !isRead {
			item = imap.FormatFlagsOp(imap.RemoveFlags, true)
		}
		if err := client.UidStore(seqset, item, flags, nil); err != nil {
			return err
		}
	}
	message.IsRead = isRead
	return s.db.Save(&message).Error
}

// DeleteMessage 删除邮件；对 IMAP 会同步远端删除并从本地标记删除。
func (s *Service) DeleteMessage(ctx context.Context, messageID uint) error {
	var message model.Message
	if err := s.db.First(&message, messageID).Error; err != nil {
		return err
	}
	var account model.MailAccount
	if err := s.db.First(&account, message.AccountID).Error; err != nil {
		return err
	}
	if account.IncomingProtocol != "imap" || message.UID == 0 {
		message.IsDeleted = true
		return s.db.Save(&message).Error
	}
	password, err := s.DecryptPassword(account)
	if err != nil {
		return err
	}
	client, err := dialIMAP(account, password)
	if err != nil {
		return err
	}
	defer client.Logout()
	if _, err := client.Select(message.Folder, false); err != nil {
		return err
	}
	seqset := newUIDSet(message.UID)
	if err := client.UidStore(seqset, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.DeletedFlag}, nil); err != nil {
		return err
	}
	if err := client.Expunge(nil); err != nil {
		return err
	}
	message.IsDeleted = true
	return s.db.Save(&message).Error
}

// MoveMessage 将邮件移动到目标文件夹；POP3 账户不支持该操作。
func (s *Service) MoveMessage(ctx context.Context, messageID uint, targetFolder string) error {
	var message model.Message
	if err := s.db.First(&message, messageID).Error; err != nil {
		return err
	}
	var account model.MailAccount
	if err := s.db.First(&account, message.AccountID).Error; err != nil {
		return err
	}
	if account.IncomingProtocol != "imap" || message.UID == 0 {
		return fmt.Errorf("当前邮箱协议不支持移动邮件")
	}
	password, err := s.DecryptPassword(account)
	if err != nil {
		return err
	}
	client, err := dialIMAP(account, password)
	if err != nil {
		return err
	}
	defer client.Logout()
	if _, err := client.Select(message.Folder, false); err != nil {
		return err
	}
	if err := client.UidMove(newUIDSet(message.UID), targetFolder); err != nil {
		return err
	}
	newUID, err := findUIDInFolder(client, targetFolder, message.MessageID)
	if err != nil {
		return err
	}
	var mailbox model.Mailbox
	_ = s.db.Where("account_id = ? AND path = ?", account.Model.ID, targetFolder).First(&mailbox).Error
	message.Folder = targetFolder
	message.MailboxID = mailbox.Model.ID
	message.UID = newUID
	return s.db.Save(&message).Error
}

// IMAPCursors 返回按文件夹维护的增量游标，避免多文件夹同步互相覆盖。
func IMAPCursors(state *model.SyncState) map[string]uint32 {
	result := map[string]uint32{}
	if state == nil || strings.TrimSpace(state.IMAPCursorMap) == "" {
		if state != nil && state.LastIMAPUID > 0 {
			result["INBOX"] = state.LastIMAPUID
		}
		return result
	}
	_ = json.Unmarshal([]byte(state.IMAPCursorMap), &result)
	if state.LastIMAPUID > 0 && result["INBOX"] < state.LastIMAPUID {
		result["INBOX"] = state.LastIMAPUID
	}
	return result
}

// SaveIMAPCursors 将文件夹游标序列化回同步状态，避免同步进度丢失。
func SaveIMAPCursors(state *model.SyncState, cursors map[string]uint32) error {
	encoded, err := json.Marshal(cursors)
	if err != nil {
		return err
	}
	state.IMAPCursorMap = string(encoded)
	state.LastIMAPUID = cursors["INBOX"]
	return nil
}

// attachmentBaseDir 返回附件落盘目录，统一收敛路径规则。
func attachmentBaseDir() string {
	return filepath.Join("data", "attachments")
}

// findUIDInFolder 在移动完成后按 Message-ID 重新定位目标文件夹中的新 UID，避免后续继续使用旧 UID。
func findUIDInFolder(client *imapclient.Client, folder string, messageID string) (uint32, error) {
	if _, err := client.Select(folder, false); err != nil {
		return 0, err
	}
	if strings.TrimSpace(messageID) == "" {
		return 0, nil
	}
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("Message-Id", messageID)
	uids, err := client.UidSearch(criteria)
	if err != nil {
		return 0, err
	}
	if len(uids) == 0 {
		return 0, nil
	}
	return uids[len(uids)-1], nil
}
