package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

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
