package mail

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"gorm.io/gorm"

	appcfg "gmbox/internal/config"
	"gmbox/internal/crypto"
	"gmbox/internal/model"
)

// Service 提供邮箱账户的基础连接验证与后续协议扩展点。
type Service struct {
	db     *gorm.DB
	crypto *crypto.AESService
	cfg    *appcfg.Config

	// 保存最近几条 IMAP 原始交互，便于把服务端的非标准响应转换成更可读的错误。
	imapDebugMu    sync.Mutex
	imapDebugLines map[string][]string
}

// NewService 创建邮件服务实例。
func NewService(db *gorm.DB, cryptoSvc *crypto.AESService, cfg *appcfg.Config) *Service {
	return &Service{db: db, crypto: cryptoSvc, cfg: cfg}
}

// WithDB 基于当前服务复制一份绑定到指定事务的实例，便于复用现有保存逻辑。
func (s *Service) WithDB(db *gorm.DB) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.db = db
	clone.imapDebugMu = sync.Mutex{}
	clone.imapDebugLines = nil
	return &clone
}

// AccountInput 描述前端提交的邮箱账户表单。
type AccountInput struct {
	Provider         string `json:"provider"`
	ProviderName     string `json:"provider_name"`
	AuthType         string `json:"auth_type"`
	Name             string `json:"name" binding:"required"`
	Email            string `json:"email" binding:"required,email"`
	Username         string `json:"username" binding:"required"`
	Password         string `json:"password"`
	IncomingProtocol string `json:"incoming_protocol" binding:"omitempty,oneof=imap pop3"`
	IMAPHost         string `json:"imap_host"`
	IMAPPort         int    `json:"imap_port"`
	POP3Host         string `json:"pop3_host"`
	POP3Port         int    `json:"pop3_port"`
	SMTPHost         string `json:"smtp_host"`
	SMTPPort         int    `json:"smtp_port"`
	UseTLS           bool   `json:"use_tls"`
	Enabled          bool   `json:"enabled"`
}

// SaveAccount 负责保存邮箱账户并在入库前完成密码加密。
func (s *Service) SaveAccount(existing *model.MailAccount, input AccountInput) error {
	if existing == nil {
		existing = &model.MailAccount{}
	}
	input = ApplyProviderPreset(input)
	provider := normalizeProvider(input.Provider)
	providerName := strings.TrimSpace(input.ProviderName)
	if providerName == "" {
		providerName = providerDisplayName(provider)
	}
	authType := normalizeAuthType(input.AuthType)
	if authType == "password" && existing.Model.ID == 0 && strings.TrimSpace(input.Password) == "" {
		return fmt.Errorf("新建邮箱时密码或授权码不能为空")
	}
	if authType == "password" && existing.Model.ID > 0 && strings.TrimSpace(input.Password) == "" && strings.TrimSpace(existing.PasswordEncrypted) == "" {
		return fmt.Errorf("当前邮箱缺少可用密码，请重新填写密码或授权码")
	}
	if authType == "oauth" && provider != "outlook" {
		return fmt.Errorf("当前仅支持微软 OAuth")
	}
	if authType == "oauth" && input.IncomingProtocol != "imap" {
		return fmt.Errorf("OAuth 邮箱当前仅支持 IMAP 协议")
	}
	if authType == "oauth" && strings.TrimSpace(existing.OAuthAccessToken) == "" {
		return fmt.Errorf("请先完成微软 OAuth 授权，再保存 OAuth 邮箱")
	}
	if authType == "oauth" && !input.UseTLS {
		return fmt.Errorf("OAuth 邮箱必须启用 TLS")
	}
	if input.IncomingProtocol != "imap" && input.IncomingProtocol != "pop3" {
		return fmt.Errorf("收信协议仅支持 IMAP 或 POP3")
	}
	if strings.TrimSpace(input.SMTPHost) == "" || input.SMTPPort <= 0 {
		return fmt.Errorf("SMTP 主机和端口不能为空")
	}
	if input.IncomingProtocol == "imap" && (strings.TrimSpace(input.IMAPHost) == "" || input.IMAPPort <= 0) {
		return fmt.Errorf("IMAP 主机和端口不能为空")
	}
	if input.IncomingProtocol == "pop3" && (strings.TrimSpace(input.POP3Host) == "" || input.POP3Port <= 0) {
		return fmt.Errorf("POP3 主机和端口不能为空")
	}
	existing.Name = input.Name
	existing.Email = input.Email
	existing.Provider = provider
	existing.ProviderName = providerName
	existing.AuthType = authType
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

	if authType == "password" && strings.TrimSpace(input.Password) != "" {
		ciphertext, err := s.crypto.Encrypt(input.Password)
		if err != nil {
			return err
		}
		existing.PasswordEncrypted = ciphertext
	}
	if authType == "oauth" {
		existing.PasswordEncrypted = ""
	}

	if existing.Model.ID == 0 {
		return s.db.Create(existing).Error
	}
	return s.db.Save(existing).Error
}

// DecryptPassword 为协议拨号恢复邮箱凭证。
func (s *Service) DecryptPassword(account model.MailAccount) (string, error) {
	if normalizeAuthType(account.AuthType) != "password" {
		return "", fmt.Errorf("当前邮箱使用 OAuth 认证，不支持读取密码")
	}
	return s.crypto.Decrypt(account.PasswordEncrypted)
}

// ResolveAuthSecret 根据认证方式返回密码或 OAuth access token，供协议层统一复用。
func (s *Service) ResolveAuthSecret(ctx context.Context, account *model.MailAccount) (string, error) {
	if normalizeAuthType(account.AuthType) == "oauth" {
		return s.OAuthAccessToken(ctx, account)
	}
	return s.DecryptPassword(*account)
}

// TestConnection 对入站和出站地址做最小连通性验证，避免明显配置错误直接入库。
func (s *Service) TestConnection(ctx context.Context, account model.MailAccount) error {
	if normalizeAuthType(account.AuthType) == "oauth" {
		if _, err := s.OAuthAccessToken(ctx, &account); err != nil {
			return err
		}
	} else {
		if _, err := s.DecryptPassword(account); err != nil {
			return err
		}
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
	if err := s.probeSMTP(ctx, account); err != nil {
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

// GetMessageDetail 读取邮件详情，并在正文尚未抓取时按需回源补齐。
func (s *Service) GetMessageDetail(ctx context.Context, messageID uint) (*model.Message, *model.MessageBody, []model.Attachment, error) {
	var message model.Message
	if err := s.db.First(&message, messageID).Error; err != nil {
		return nil, nil, nil, err
	}
	var body model.MessageBody
	bodyErr := s.db.Where("message_id = ?", message.Model.ID).First(&body).Error
	if bodyErr != nil && bodyErr != gorm.ErrRecordNotFound {
		return nil, nil, nil, bodyErr
	}
	if shouldFetchMessageBody(bodyErr, &body, &message) {
		if err := s.fetchAndStoreMessageBody(ctx, &message, &body, bodyErr == gorm.ErrRecordNotFound); err != nil {
			return nil, nil, nil, err
		}
	}
	var attachments []model.Attachment
	if err := s.db.Where("message_id = ?", message.Model.ID).Order("id asc").Find(&attachments).Error; err != nil {
		return nil, nil, nil, err
	}
	return &message, &body, attachments, nil
}

// fetchAndStoreMessageBody 在首次查看详情时回源抓取完整正文，并写回本地缓存。
func (s *Service) fetchAndStoreMessageBody(ctx context.Context, message *model.Message, body *model.MessageBody, create bool) error {
	if message == nil || body == nil {
		return fmt.Errorf("邮件详情参数不合法")
	}
	var account model.MailAccount
	if err := s.db.First(&account, message.AccountID).Error; err != nil {
		return err
	}
	parsed, err := s.fetchMessageBodyFromRemote(ctx, account, *message)
	if err != nil {
		return err
	}
	body.MessageID = message.Model.ID
	body.TextBody = parsed.TextBody
	body.HTMLBody = parsed.HTMLBody
	body.BodyFetched = true
	if create {
		return s.db.Create(body).Error
	}
	return s.db.Save(body).Error
}

// shouldFetchMessageBody 根据缓存状态判断是否需要回源抓取正文，并兼容历史没有 BodyFetched 标记的数据。
func shouldFetchMessageBody(bodyErr error, body *model.MessageBody, message *model.Message) bool {
	if bodyErr == gorm.ErrRecordNotFound {
		return true
	}
	if body == nil {
		return true
	}
	if body.BodyFetched {
		return false
	}
	if bodyLooksFetched(body, message) {
		return false
	}
	return true
}

// bodyLooksFetched 用已有正文内容反推历史记录是否已缓存完整正文，避免升级后误触发回源。
func bodyLooksFetched(body *model.MessageBody, message *model.Message) bool {
	if body == nil {
		return false
	}
	if strings.TrimSpace(body.HTMLBody) != "" {
		return true
	}
	textBody := strings.TrimSpace(body.TextBody)
	if textBody == "" {
		return false
	}
	if message == nil {
		return true
	}
	return textBody != strings.TrimSpace(message.Snippet)
}

// fetchMessageBodyFromRemote 按协议回源抓取单封邮件正文，避免列表同步时强制下载全部正文。
func (s *Service) fetchMessageBodyFromRemote(ctx context.Context, account model.MailAccount, message model.Message) (*parsedMessage, error) {
	if account.IncomingProtocol == "imap" {
		return s.fetchIMAPMessageBody(ctx, account, message)
	}
	if account.IncomingProtocol == "pop3" {
		return s.fetchPOP3MessageBody(ctx, account, message)
	}
	return nil, fmt.Errorf("当前邮箱协议不支持抓取邮件正文")
}

// fetchIMAPMessageBody 通过 UID 抓取单封 IMAP 邮件的完整 RFC822 内容。
func (s *Service) fetchIMAPMessageBody(ctx context.Context, account model.MailAccount, message model.Message) (*parsedMessage, error) {
	if message.UID == 0 {
		return nil, fmt.Errorf("当前邮件缺少 IMAP UID，无法抓取正文")
	}
	password, err := s.ResolveAuthSecret(ctx, &account)
	if err != nil {
		return nil, err
	}
	client, err := s.dialIMAP(account, password)
	if err != nil {
		return nil, err
	}
	defer closeIMAPClient(client)
	fetched, err := s.fetchIMAPMessageBodyWithClient(client, message)
	if err != nil && shouldRetryIMAPMailboxSelect(account, err) {
		closeIMAPClient(client)
		client = nil
		client, err = s.dialIMAP(account, password)
		if err != nil {
			return nil, fmt.Errorf("邮件详情首次选择文件夹失败后重连 IMAP 失败: %w", err)
		}
		defer closeIMAPClient(client)
		fetched, err = s.fetchIMAPMessageBodyWithClient(client, message)
	}
	if err != nil {
		return nil, err
	}
	return fetched, nil
}

// fetchIMAPMessageBodyWithClient 复用已建立的 IMAP 连接抓取正文，便于在瞬时断链后重连重试。
func (s *Service) fetchIMAPMessageBodyWithClient(client *imapclient.Client, message model.Message) (*parsedMessage, error) {
	if _, err := client.Select(message.Folder, nil).Wait(); err != nil {
		return nil, fmt.Errorf("选择文件夹 %s 失败: %w", message.Folder, err)
	}
	section := &imap.FetchItemBodySection{}
	fetchCmd := client.Fetch(newUIDSet(message.UID), &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		BodySection:  []*imap.FetchItemBodySection{section},
	})
	var fetched *parsedMessage
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		if msg == nil {
			continue
		}
		buffer, collectErr := msg.Collect()
		if collectErr != nil {
			return nil, fmt.Errorf("抓取邮件正文失败: %w", collectErr)
		}
		raw := buffer.FindBodySection(section)
		if raw == nil {
			continue
		}
		parsed, err := parseRawMessage(raw)
		if err != nil {
			return nil, err
		}
		parsed.enrichFromEnvelope(buffer.Envelope, flagsToStrings(buffer.Flags))
		if parsed.SentAt.IsZero() {
			parsed.SentAt = buffer.InternalDate
		}
		fetched = parsed
	}
	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("抓取邮件正文失败: %w", err)
	}
	if fetched == nil {
		return nil, fmt.Errorf("未找到指定 IMAP 邮件正文")
	}
	return fetched, nil
}

// fetchPOP3MessageBody 通过 UIDL 定位并抓取单封 POP3 邮件正文。
func (s *Service) fetchPOP3MessageBody(ctx context.Context, account model.MailAccount, message model.Message) (*parsedMessage, error) {
	if strings.TrimSpace(message.POP3UIDL) == "" {
		return nil, fmt.Errorf("当前邮件缺少 POP3 UIDL，无法抓取正文")
	}
	password, err := s.ResolveAuthSecret(ctx, &account)
	if err != nil {
		return nil, err
	}
	client, err := dialPOP3(ctx, account)
	if err != nil {
		return nil, err
	}
	defer client.close()
	if err := client.auth(account.Username, password); err != nil {
		return nil, err
	}
	entries, err := client.uidlAll()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.uidl != message.POP3UIDL {
			continue
		}
		raw, err := client.retr(entry.number)
		if err != nil {
			return nil, err
		}
		return parseRawMessage(raw)
	}
	return nil, fmt.Errorf("未找到指定 POP3 邮件正文")
}

// ListMailboxes 返回指定账户下已同步的文件夹，便于前端渲染目录树。
func (s *Service) ListMailboxes(accountID uint) ([]model.Mailbox, error) {
	var mailboxes []model.Mailbox
	query := s.db.Order("path asc, name asc")
	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	if err := query.Find(&mailboxes).Error; err != nil {
		return nil, err
	}
	if accountID == 0 {
		seen := make(map[string]struct{}, len(mailboxes))
		deduped := make([]model.Mailbox, 0, len(mailboxes))
		for _, mailbox := range mailboxes {
			path := strings.TrimSpace(mailbox.Path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			deduped = append(deduped, mailbox)
		}
		mailboxes = deduped
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
		password, err := s.ResolveAuthSecret(ctx, &account)
		if err != nil {
			return err
		}
		client, err := s.dialIMAP(account, password)
		if err != nil {
			return err
		}
		defer closeIMAPClient(client)
		if _, err := client.Select(message.Folder, nil).Wait(); err != nil {
			return err
		}
		seqset := newUIDSet(message.UID)
		item := &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}
		if !isRead {
			item = &imap.StoreFlags{Op: imap.StoreFlagsDel, Silent: true, Flags: []imap.Flag{imap.FlagSeen}}
		}
		if err := client.Store(seqset, item, nil).Close(); err != nil {
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
	password, err := s.ResolveAuthSecret(ctx, &account)
	if err != nil {
		return err
	}
	client, err := s.dialIMAP(account, password)
	if err != nil {
		return err
	}
	defer closeIMAPClient(client)
	if _, err := client.Select(message.Folder, nil).Wait(); err != nil {
		return err
	}
	seqset := newUIDSet(message.UID)
	if err := client.Store(seqset, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return err
	}
	if _, err := client.Expunge().Collect(); err != nil {
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
	password, err := s.ResolveAuthSecret(ctx, &account)
	if err != nil {
		return err
	}
	client, err := s.dialIMAP(account, password)
	if err != nil {
		return err
	}
	defer closeIMAPClient(client)
	if _, err := client.Select(message.Folder, nil).Wait(); err != nil {
		return err
	}
	if _, err := client.Move(newUIDSet(message.UID), targetFolder).Wait(); err != nil {
		return err
	}
	newUID, err := findUIDInFolder(client, targetFolder, message)
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

// findUIDInFolder 在移动完成后优先按 Message-ID，回退按日期、主题、发件人定位目标文件夹中的新 UID。
func findUIDInFolder(client *imapclient.Client, folder string, message model.Message) (uint32, error) {
	mbox, err := client.Select(folder, nil).Wait()
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(message.MessageID) != "" {
		criteria := &imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Message-Id", Value: message.MessageID}}}
		searchData, err := client.UIDSearch(criteria, nil).Wait()
		if err != nil {
			return 0, err
		}
		uids := searchData.AllUIDs()
		if len(uids) == 1 {
			return uint32(uids[0]), nil
		}
	}
	criteria := &imap.SearchCriteria{}
	if !message.SentAt.IsZero() {
		dayStart := time.Date(message.SentAt.Year(), message.SentAt.Month(), message.SentAt.Day(), 0, 0, 0, 0, message.SentAt.Location())
		criteria.SentSince = dayStart
		criteria.SentBefore = dayStart.Add(24 * time.Hour)
	}
	if strings.TrimSpace(message.Subject) != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "Subject", Value: message.Subject})
	}
	if strings.TrimSpace(message.FromAddress) != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "From", Value: message.FromAddress})
	}
	searchData, err := client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return 0, err
	}
	uids := searchData.AllUIDs()
	if len(uids) == 1 {
		return uint32(uids[0]), nil
	}
	return fallbackFindUIDByEnvelope(client, mbox, message)
}

// fallbackFindUIDByEnvelope 抓取目标文件夹最近一批邮件的 ENVELOPE 并本地比对，兜底恢复移动后的新 UID。
func fallbackFindUIDByEnvelope(client *imapclient.Client, mailbox *imap.SelectData, message model.Message) (uint32, error) {
	if mailbox == nil || mailbox.NumMessages == 0 {
		return 0, nil
	}
	from := uint32(1)
	if mailbox.NumMessages > 50 {
		from = mailbox.NumMessages - 49
	}
	seqset := imap.SeqSetNum(from)
	seqset.AddRange(from, mailbox.NumMessages)
	fetchCmd := client.Fetch(seqset, &imap.FetchOptions{UID: true, Envelope: true})
	var matched uint32
	matchedCount := 0
	for {
		fetched := fetchCmd.Next()
		if fetched == nil {
			break
		}
		buffer, collectErr := fetched.Collect()
		if collectErr != nil {
			return 0, collectErr
		}
		if buffer.Envelope == nil {
			continue
		}
		if envelopeMatchesMessage(buffer.Envelope, message) {
			matched = uint32(buffer.UID)
			matchedCount++
		}
	}
	if err := fetchCmd.Close(); err != nil {
		return 0, err
	}
	if matchedCount != 1 {
		return 0, nil
	}
	return matched, nil
}

// envelopeMatchesMessage 使用日期、主题和发件人做近似匹配，用于 MOVE 后 UID 重定位的最后兜底。
func envelopeMatchesMessage(envelope *imap.Envelope, message model.Message) bool {
	if envelope == nil {
		return false
	}
	if normalizeHeaderValue(envelope.Subject) != normalizeHeaderValue(message.Subject) {
		return false
	}
	if !sameMailDay(envelope.Date, message.SentAt) {
		return false
	}
	if len(envelope.From) == 0 {
		return strings.TrimSpace(message.FromAddress) == ""
	}
	return strings.EqualFold(strings.TrimSpace(envelope.From[0].Addr()), strings.TrimSpace(message.FromAddress))
}

// normalizeHeaderValue 统一收敛头字段比较格式，避免大小写和空白差异导致误判。
func normalizeHeaderValue(value string) string {
	return strings.ToLower(strings.TrimSpace(textproto.TrimString(strings.Join(strings.Fields(value), " "))))
}

// sameMailDay 只比较邮件日期所在自然日，避免时区秒级差异让回退匹配失效。
func sameMailDay(left time.Time, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}
