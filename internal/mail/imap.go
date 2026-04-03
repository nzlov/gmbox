package mail

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	"gorm.io/gorm"

	"gmbox/internal/model"
)

// SyncIMAP 拉取 INBOX 中的新邮件并写入本地数据库。
func (s *Service) SyncIMAP(ctx context.Context, account model.MailAccount, state *model.SyncState, fetchBody bool) error {
	password, err := s.DecryptPassword(account)
	if err != nil {
		return err
	}
	client, err := dialIMAP(account, password)
	if err != nil {
		return err
	}
	defer client.Logout()

	mbox, err := client.Select("INBOX", false)
	if err != nil {
		return fmt.Errorf("选择 INBOX 失败: %w", err)
	}
	if mbox.Messages == 0 {
		return nil
	}

	criteria := &imap.SearchCriteria{}
	uids, err := client.UidSearch(criteria)
	if err != nil {
		return fmt.Errorf("查询 IMAP UID 失败: %w", err)
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })

	newUIDs := make([]uint32, 0, len(uids))
	for _, uid := range uids {
		if uid > state.LastIMAPUID {
			newUIDs = append(newUIDs, uid)
		}
	}
	if len(newUIDs) == 0 {
		return nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(newUIDs...)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, section.FetchItem()}
	messages := make(chan *imap.Message, minInt(10, len(newUIDs)))
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(seqset, items, messages)
	}()

	var maxUID uint32 = state.LastIMAPUID
	for msg := range messages {
		if msg == nil {
			continue
		}
		body := msg.GetBody(section)
		if body == nil {
			continue
		}
		raw, readErr := io.ReadAll(body)
		if readErr != nil {
			return fmt.Errorf("读取 IMAP 正文失败: %w", readErr)
		}
		parsed, parseErr := parseRawMessage(raw)
		if parseErr != nil {
			return parseErr
		}
		parsed.enrichFromEnvelope(msg.Envelope, msg.Flags)
		if parsed.SentAt.IsZero() {
			parsed.SentAt = msg.InternalDate
		}
		if err := s.upsertMessage(account, "INBOX", msg.Uid, "", parsed, fetchBody); err != nil {
			return err
		}
		if msg.Uid > maxUID {
			maxUID = msg.Uid
		}
	}
	if err := <-done; err != nil {
		return fmt.Errorf("抓取 IMAP 邮件失败: %w", err)
	}
	state.LastIMAPUID = maxUID
	state.LastMessage = fmt.Sprintf("IMAP 同步完成，新增 %d 封邮件", len(newUIDs))
	return nil
}

// dialIMAP 按账户配置建立 IMAP 连接并完成认证。
func dialIMAP(account model.MailAccount, password string) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", account.IMAPHost, account.IMAPPort)
	var client *imapclient.Client
	var err error
	if account.UseTLS {
		client, err = imapclient.DialTLS(addr, nil)
	} else {
		client, err = imapclient.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 IMAP 失败: %w", err)
	}
	if err := client.Login(account.Username, password); err != nil {
		_ = client.Logout()
		return nil, fmt.Errorf("IMAP 登录失败: %w", err)
	}
	return client, nil
}

// upsertMessage 根据账户和协议唯一标识保存邮件，避免重复落库。
func (s *Service) upsertMessage(account model.MailAccount, folder string, uid uint32, pop3UIDL string, parsed *parsedMessage, fetchBody bool) error {
	var message model.Message
	query := s.db.Where("account_id = ?", account.Model.ID)
	if uid > 0 {
		query = query.Where("uid = ?", uid)
	} else {
		query = query.Where("pop3_uid_l = ?", pop3UIDL)
	}
	err := query.First(&message).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	parsed.applyToMessage(&message, account.Model.ID, folder)
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
	} else {
		body.TextBody = parsed.Snippet
		body.HTMLBody = ""
	}
	if bodyErr == gorm.ErrRecordNotFound {
		return s.db.Create(&body).Error
	}
	return s.db.Save(&body).Error
}

// minInt 用于限制通道缓冲大小，避免小批量抓取时申请过大缓冲区。
func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
