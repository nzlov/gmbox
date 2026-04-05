package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"gorm.io/gorm"

	"gmbox/internal/model"
)

// SyncIMAP 拉取全部可见文件夹中的新邮件并写入本地数据库。
func (s *Service) SyncIMAP(ctx context.Context, account model.MailAccount, state *model.SyncState, fetchBody bool) (*SyncResult, error) {
	password, err := s.ResolveAuthSecret(ctx, &account)
	if err != nil {
		return nil, err
	}
	client, err := s.dialIMAP(account, password)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client != nil {
			_ = client.Logout().Wait()
		}
	}()
	cursors := IMAPCursors(state)
	mailboxes, err := listRemoteMailboxes(client)
	if err != nil {
		var fallbackErr error
		mailboxes, fallbackErr = s.fallbackIMAPMailboxes(account, err)
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		// LIST 解析失败通常意味着当前连接的读状态已经损坏，回退目录前先重连，避免后续命令继续复用坏连接。
		_ = client.Logout().Wait()
		client, err = s.dialIMAP(account, password)
		if err != nil {
			return nil, fmt.Errorf("IMAP 文件夹枚举失败后重连失败: %w", err)
		}
	}
	if err := s.syncMailboxes(account, mailboxes); err != nil {
		return nil, err
	}
	totalNew := 0
	for _, mailbox := range mailboxes {
		count, maxUID, syncErr := s.syncIMAPMailbox(client, account, mailbox.Path, cursors[mailbox.Path], fetchBody)
		if syncErr != nil && shouldRetryIMAPMailboxSelect(account, syncErr) {
			_ = client.Logout().Wait()
			client, err = s.dialIMAP(account, password)
			if err != nil {
				return nil, fmt.Errorf("文件夹 %s 首次选择失败后重连 IMAP 失败: %w", mailbox.Path, err)
			}
			count, maxUID, syncErr = s.syncIMAPMailbox(client, account, mailbox.Path, cursors[mailbox.Path], fetchBody)
		}
		if syncErr != nil {
			return nil, syncErr
		}
		totalNew += count
		cursors[mailbox.Path] = maxUID
	}
	if err := SaveIMAPCursors(state, cursors); err != nil {
		return nil, err
	}
	state.LastMessage = fmt.Sprintf("IMAP 同步完成，新增 %d 封邮件，覆盖 %d 个文件夹", totalNew, len(mailboxes))
	return &SyncResult{NewMessages: totalNew, MailboxCount: len(mailboxes)}, nil
}

// fallbackIMAPMailboxes 在微软 OAuth 的 LIST 响应触发 go-imap 解析异常时，退回到已缓存文件夹和 INBOX，避免整轮同步被目录枚举阻断。
func (s *Service) fallbackIMAPMailboxes(account model.MailAccount, listErr error) ([]model.Mailbox, error) {
	if normalizeAuthType(account.AuthType) != "oauth" || normalizeProvider(account.Provider) != "outlook" {
		return nil, listErr
	}
	fallback := []model.Mailbox{{Name: "INBOX", Path: "INBOX", Role: "inbox"}}
	var cached []model.Mailbox
	if err := s.db.Where("account_id = ?", account.Model.ID).Order("path asc").Find(&cached).Error; err != nil {
		return nil, listErr
	}
	seen := map[string]struct{}{"INBOX": {}}
	for _, mailbox := range cached {
		path := strings.TrimSpace(mailbox.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		fallback = append(fallback, model.Mailbox{Name: displayMailboxName(path), Path: path, Role: mailboxRole(path)})
	}
	return fallback, nil
}

// syncIMAPMailbox 同步单个文件夹，并返回新增数量和最新 UID。
func (s *Service) syncIMAPMailbox(client *imapclient.Client, account model.MailAccount, folder string, lastUID uint32, fetchBody bool) (int, uint32, error) {
	mbox, err := client.Select(folder, nil).Wait()
	if err != nil {
		return 0, lastUID, fmt.Errorf("选择文件夹 %s 失败: %w", folder, err)
	}
	if mbox.NumMessages == 0 {
		return 0, lastUID, nil
	}

	criteria := &imap.SearchCriteria{}
	searchData, err := client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return 0, lastUID, fmt.Errorf("查询文件夹 %s 的 UID 失败: %w", folder, err)
	}
	uids := searchData.AllUIDs()
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })

	newUIDs := make([]imap.UID, 0, len(uids))
	for _, uid := range uids {
		if uint32(uid) > lastUID {
			newUIDs = append(newUIDs, uid)
		}
	}
	if len(newUIDs) == 0 {
		return 0, lastUID, nil
	}

	seqset := imap.UIDSetNum(newUIDs...)
	section := &imap.FetchItemBodySection{}
	fetchCmd := client.Fetch(seqset, &imap.FetchOptions{
		UID:          true,
		Envelope:     true,
		Flags:        true,
		InternalDate: true,
		BodySection:  []*imap.FetchItemBodySection{section},
	})

	var maxUID uint32 = lastUID
	total := 0
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
			return 0, lastUID, fmt.Errorf("抓取文件夹 %s 的邮件失败: %w", folder, collectErr)
		}
		body := buffer.FindBodySection(section)
		if body == nil {
			continue
		}
		parsed, parseErr := parseRawMessage(body)
		if parseErr != nil {
			return 0, lastUID, parseErr
		}
		parsed.enrichFromEnvelope(buffer.Envelope, flagsToStrings(buffer.Flags))
		if parsed.SentAt.IsZero() {
			parsed.SentAt = buffer.InternalDate
		}
		if err := s.upsertMessage(account, folder, uint32(buffer.UID), "", parsed, fetchBody); err != nil {
			return 0, lastUID, err
		}
		total++
		if uint32(buffer.UID) > maxUID {
			maxUID = uint32(buffer.UID)
		}
	}
	if err := fetchCmd.Close(); err != nil {
		return 0, lastUID, fmt.Errorf("抓取文件夹 %s 的邮件失败: %w", folder, err)
	}
	return total, maxUID, nil
}

// shouldRetryIMAPMailboxSelect 仅为微软 OAuth 的已认证未连邮箱会话做一次重连重试，避免瞬时会话异常直接打断整轮同步。
func shouldRetryIMAPMailboxSelect(account model.MailAccount, err error) bool {
	if err == nil {
		return false
	}
	if normalizeAuthType(account.AuthType) != "oauth" || normalizeProvider(account.Provider) != "outlook" {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "authenticated but not connected")
}

// dialIMAP 按账户配置建立 IMAP 连接并完成认证。
func (s *Service) dialIMAP(account model.MailAccount, password string) (*imapclient.Client, error) {
	username := strings.TrimSpace(account.Username)
	if username == "" {
		username = strings.TrimSpace(account.Email)
	}
	if normalizeAuthType(account.AuthType) == "oauth" {
		client, err := s.dialIMAPOAuth(account, username, password)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	client, err := s.openIMAPClient(account)
	if err != nil {
		return nil, err
	}
	s.debugProviderLog("IMAP 密码登录开始", "email", account.Email, "host", account.IMAPHost, "username", username)
	if err := client.Login(username, password).Wait(); err != nil {
		_ = client.Logout().Wait()
		return nil, fmt.Errorf("IMAP 登录失败: %w", err)
	}
	s.debugProviderLog("IMAP 密码登录成功", "email", account.Email, "host", account.IMAPHost)
	return client, nil
}

// openIMAPClient 统一创建 IMAP 连接，避免认证重试时复制拨号逻辑。
func (s *Service) openIMAPClient(account model.MailAccount) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", account.IMAPHost, account.IMAPPort)
	var client *imapclient.Client
	var err error
	options := &imapclient.Options{DebugWriter: s.newIMAPDebugWriter(account)}
	s.debugProviderLog("连接 IMAP", "email", account.Email, "host", account.IMAPHost, "port", account.IMAPPort, "tls", account.UseTLS)
	if account.UseTLS {
		dialer := &net.Dialer{Timeout: 15 * time.Second}
		options.TLSConfig = &tls.Config{ServerName: account.IMAPHost, MinVersion: tls.VersionTLS12}
		options.Dialer = dialer
		client, err = imapclient.DialTLS(addr, options)
	} else {
		client, err = imapclient.DialInsecure(addr, options)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 IMAP 失败: %w", err)
	}
	s.debugProviderLog("连接 IMAP 成功", "email", account.Email, "host", account.IMAPHost, "port", account.IMAPPort)
	return client, nil
}

type imapOAuthAttempt struct {
	name string
	auth func(*imapclient.Client) error
}

// imapOAuthMechanismOrder 为不同服务商提供更稳妥的 OAuth 机制优先级。
// 微软 IMAP 线上兼容性以 XOAUTH2 更稳定，因此优先尝试 XOAUTH2，避免 OAUTHBEARER 失败挑战把连接读状态打坏。
func imapOAuthMechanismOrder(account model.MailAccount) []string {
	if normalizeProvider(account.Provider) == "outlook" {
		return []string{imapOAuthMechXOAUTH2, imapOAuthMechOAuthBearer}
	}
	return []string{imapOAuthMechOAuthBearer, imapOAuthMechXOAUTH2}
}

// dialIMAPOAuth 根据服务端能力按优先级尝试 OAuth 机制，并在失败时自动重连回退。
func (s *Service) dialIMAPOAuth(account model.MailAccount, username string, token string) (*imapclient.Client, error) {
	probeClient, err := s.openIMAPClient(account)
	if err != nil {
		return nil, err
	}
	attempts := buildIMAPOAuthAttempts(probeClient, account, username, token)
	_ = probeClient.Logout().Wait()

	var failureMessages []string
	for _, attempt := range attempts {
		client, err := s.openIMAPClient(account)
		if err != nil {
			failureMessages = append(failureMessages, fmt.Sprintf("%s 连接失败: %v", attempt.name, err))
			continue
		}
		s.debugProviderLog("IMAP OAuth 认证开始", "email", account.Email, "host", account.IMAPHost, "mechanism", attempt.name, "username", username)
		if err := attempt.auth(client); err != nil {
			s.debugProviderLog("IMAP OAuth 认证失败", "email", account.Email, "host", account.IMAPHost, "mechanism", attempt.name, "err", err)
			failureMessages = append(failureMessages, fmt.Sprintf("%s 失败: %v", attempt.name, err))
			_ = client.Logout().Wait()
			continue
		}
		s.debugProviderLog("IMAP OAuth 认证成功", "email", account.Email, "host", account.IMAPHost, "mechanism", attempt.name)
		return client, nil
	}
	if len(failureMessages) == 0 {
		return nil, fmt.Errorf("IMAP OAuth 登录失败: 服务端未声明可用认证机制")
	}
	return nil, fmt.Errorf("IMAP OAuth 登录失败: %s", strings.Join(failureMessages, "；"))
}

// buildIMAPOAuthAttempts 先尊重服务端能力声明，再补一个兜底顺序避免少量服务端声明不完整。
func buildIMAPOAuthAttempts(client *imapclient.Client, account model.MailAccount, username string, token string) []imapOAuthAttempt {
	orderedNames := selectIMAPOAuthMechanisms(client.Caps(), account)

	attempts := make([]imapOAuthAttempt, 0, len(orderedNames))
	for _, name := range orderedNames {
		switch name {
		case imapOAuthMechOAuthBearer:
			attempts = append(attempts, imapOAuthAttempt{
				name: imapOAuthMechOAuthBearer,
				auth: func(client *imapclient.Client) error {
					return client.Authenticate(newOAuthBearerClient(username, token, account.IMAPHost, account.IMAPPort))
				},
			})
		case imapOAuthMechXOAUTH2:
			attempts = append(attempts, imapOAuthAttempt{
				name: imapOAuthMechXOAUTH2,
				auth: func(client *imapclient.Client) error {
					return client.Authenticate(newXOAUTH2Client(username, token))
				},
			})
		}
	}
	return attempts
}

// selectIMAPOAuthMechanisms 优先使用服务端明确声明的 OAuth 机制，再补充未声明候选兜底，兼容少量能力声明不完整的服务端。
func selectIMAPOAuthMechanisms(caps imap.CapSet, account model.MailAccount) []string {
	preferredOrder := imapOAuthMechanismOrder(account)
	orderedNames := make([]string, 0, 2)
	appendMechanism := func(name string) {
		for _, existing := range orderedNames {
			if existing == name {
				return
			}
		}
		orderedNames = append(orderedNames, name)
	}
	for _, name := range preferredOrder {
		if caps.Has(imap.AuthCap(name)) {
			appendMechanism(name)
		}
	}
	for _, name := range preferredOrder {
		appendMechanism(name)
	}
	return orderedNames
}

// upsertMessage 根据账户和协议唯一标识保存邮件，避免重复落库。
func (s *Service) upsertMessage(account model.MailAccount, folder string, uid uint32, pop3UIDL string, parsed *parsedMessage, fetchBody bool) error {
	mailboxID, err := s.ensureMailbox(account.Model.ID, folder)
	if err != nil {
		return err
	}
	var message model.Message
	query := s.db.Where("account_id = ?", account.Model.ID)
	if uid > 0 {
		query = query.Where("folder = ? AND uid = ?", folder, uid)
	} else {
		query = query.Where("folder = ? AND pop3_uid_l = ?", folder, pop3UIDL)
	}
	err = query.First(&message).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	parsed.applyToMessage(&message, account.Model.ID, folder)
	message.MailboxID = mailboxID
	message.UID = uid
	message.POP3UIDL = pop3UIDL
	if err == gorm.ErrRecordNotFound {
		if err := s.db.Create(&message).Error; err != nil {
			return err
		}
	} else {
		if err := s.db.Save(&message).Error; err != nil {
			return err
		}
	}

	var body model.MessageBody
	bodyErr := s.db.Where("message_id = ?", message.Model.ID).First(&body).Error
	if bodyErr != nil && bodyErr != gorm.ErrRecordNotFound {
		return bodyErr
	}
	body.MessageID = message.Model.ID
	if fetchBody {
		body.TextBody = parsed.TextBody
		body.HTMLBody = parsed.HTMLBody
		body.BodyFetched = true
	} else if bodyErr == gorm.ErrRecordNotFound || !body.BodyFetched {
		body.TextBody = parsed.Snippet
		body.HTMLBody = ""
		body.BodyFetched = false
	}
	if bodyErr == gorm.ErrRecordNotFound {
		if err := s.db.Create(&body).Error; err != nil {
			return err
		}
	} else {
		if err := s.db.Save(&body).Error; err != nil {
			return err
		}
	}
	return s.replaceAttachments(message.Model.ID, parsed.Attachments)
}

// syncMailboxes 将远端文件夹同步到本地表，便于前端直接展示目录树。
func (s *Service) syncMailboxes(account model.MailAccount, mailboxes []model.Mailbox) error {
	for _, mailbox := range mailboxes {
		mailbox.AccountID = account.Model.ID
		if _, err := s.ensureMailbox(account.Model.ID, mailbox.Path); err != nil {
			return err
		}
		_ = s.db.Model(&model.Mailbox{}).Where("account_id = ? AND path = ?", account.Model.ID, mailbox.Path).Updates(map[string]any{
			"name": mailbox.Name,
			"role": mailbox.Role,
		}).Error
	}
	return nil
}

// ensureMailbox 根据账户和路径查找或创建文件夹记录。
func (s *Service) ensureMailbox(accountID uint, path string) (uint, error) {
	var mailbox model.Mailbox
	err := s.db.Where("account_id = ? AND path = ?", accountID, path).First(&mailbox).Error
	if err == nil {
		return mailbox.Model.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}
	mailbox = model.Mailbox{AccountID: accountID, Name: displayMailboxName(path), Path: path, Role: mailboxRole(path)}
	if err := s.db.Create(&mailbox).Error; err != nil {
		return 0, err
	}
	return mailbox.Model.ID, nil
}

// listRemoteMailboxes 列出 IMAP 可见文件夹，并过滤噪声目录。
func listRemoteMailboxes(client *imapclient.Client) ([]model.Mailbox, error) {
	listCmd := client.List("", "*", nil)
	result := make([]model.Mailbox, 0)
	for {
		mailbox := listCmd.Next()
		if mailbox == nil {
			break
		}
		if strings.TrimSpace(mailbox.Mailbox) == "" {
			continue
		}
		if hasMailboxAttribute(mailbox.Attrs, `\\Noselect`) {
			continue
		}
		result = append(result, model.Mailbox{Name: displayMailboxName(mailbox.Mailbox), Path: mailbox.Mailbox, Role: mailboxRole(mailbox.Mailbox)})
	}
	if err := listCmd.Close(); err != nil {
		return nil, fmt.Errorf("列出 IMAP 文件夹失败: %w", err)
	}
	if len(result) == 0 {
		result = append(result, model.Mailbox{Name: "INBOX", Path: "INBOX", Role: "inbox"})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

// replaceAttachments 重新写入当前邮件的附件文件与元数据，避免重复同步后残留旧文件。
func (s *Service) replaceAttachments(messageID uint, attachments []parsedAttachment) error {
	var old []model.Attachment
	if err := s.db.Where("message_id = ?", messageID).Find(&old).Error; err != nil {
		return err
	}
	for _, item := range old {
		if strings.TrimSpace(item.StoragePath) != "" {
			_ = os.Remove(item.StoragePath)
		}
	}
	if err := s.db.Where("message_id = ?", messageID).Delete(&model.Attachment{}).Error; err != nil {
		return err
	}
	if len(attachments) == 0 {
		return nil
	}
	if err := os.MkdirAll(attachmentBaseDir(), 0o755); err != nil {
		return err
	}
	for idx, attachment := range attachments {
		fileName := sanitizeFileName(attachment.FileName)
		storagePath := fmt.Sprintf("%d-%d-%s", messageID, idx+1, fileName)
		fullPath := joinAttachmentPath(storagePath)
		if err := os.WriteFile(fullPath, attachment.Data, 0o644); err != nil {
			return err
		}
		record := model.Attachment{
			MessageID:   messageID,
			FileName:    fileName,
			PartID:      fmt.Sprintf("part-%d", idx+1),
			ContentType: attachment.ContentType,
			Size:        int64(len(attachment.Data)),
			StoragePath: fullPath,
		}
		if err := s.db.Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

// newUIDSet 为单封邮件操作构造 UID 序列集合。
func newUIDSet(uid uint32) imap.UIDSet {
	return imap.UIDSetNum(imap.UID(uid))
}

// hasMailboxAttribute 判断文件夹是否包含指定属性。
func hasMailboxAttribute(attrs []imap.MailboxAttr, target string) bool {
	for _, attr := range attrs {
		if strings.EqualFold(string(attr), target) {
			return true
		}
	}
	return false
}

// flagsToStrings 统一兼容 go-imap v2 的 Flag 类型，减少现有解析逻辑改动范围。
func flagsToStrings(flags []imap.Flag) []string {
	result := make([]string, 0, len(flags))
	for _, flag := range flags {
		result = append(result, string(flag))
	}
	return result
}

// displayMailboxName 生成适合前端展示的文件夹名称。
func displayMailboxName(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '.' })
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

// mailboxRole 推断常见系统文件夹角色，方便前端做高亮。
func mailboxRole(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "inbox"):
		return "inbox"
	case strings.Contains(lower, "sent"):
		return "sent"
	case strings.Contains(lower, "draft"):
		return "draft"
	case strings.Contains(lower, "trash") || strings.Contains(lower, "deleted"):
		return "trash"
	case strings.Contains(lower, "archive"):
		return "archive"
	default:
		return "custom"
	}
}

// sanitizeFileName 去掉路径分隔符，避免附件文件写出目标目录。
func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	clean := replacer.Replace(strings.TrimSpace(name))
	if clean == "" {
		return "attachment.bin"
	}
	return clean
}

// joinAttachmentPath 收敛附件本地存储路径。
func joinAttachmentPath(name string) string {
	return fmt.Sprintf("%s/%s", attachmentBaseDir(), name)
}

// minInt 用于限制通道缓冲大小，避免小批量抓取时申请过大缓冲区。
func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
