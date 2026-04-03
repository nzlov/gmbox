package mail

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/emersion/go-sasl"
)

const (
	imapOAuthMechOAuthBearer = "OAUTHBEARER"
	imapOAuthMechXOAUTH2     = "XOAUTH2"
)

// xoauth2Client 兼容微软邮箱常用的 XOAUTH2 机制，供 IMAP AUTHENTICATE 复用。
type xoauth2Client struct {
	username string
	token    string
}

// Start 发送 XOAUTH2 初始响应，避免额外的交互轮次。
func (x *xoauth2Client) Start() (string, []byte, error) {
	response := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", x.username, x.token)
	return "XOAUTH2", []byte(response), nil
}

// Next 对失败挑战返回空响应即可结束认证流程。
func (x *xoauth2Client) Next(_ []byte) ([]byte, error) {
	return []byte{}, nil
}

// newXOAUTH2Client 构造 IMAP 所需的 SASL 客户端。
func newXOAUTH2Client(username string, token string) sasl.Client {
	return &xoauth2Client{username: username, token: token}
}

// newOAuthBearerClient 构造标准 OAUTHBEARER 客户端，优先兼容声明 RFC 7628 的服务端。
func newOAuthBearerClient(username string, token string, host string, port int) sasl.Client {
	return sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
		Username: username,
		Token:    token,
		Host:     host,
		Port:     port,
	})
}

// smtpXOAUTH2Auth 适配 net/smtp.Auth 接口，供 SMTP 发信走 OAuth。
type smtpXOAUTH2Auth struct {
	username string
	token    string
	host     string
}

// Start 仅在目标主机匹配时发送 XOAUTH2 首包，避免 token 误发到异常目标。
func (a *smtpXOAUTH2Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, fmt.Errorf("SMTP OAuth 需要 TLS 连接")
	}
	if !strings.EqualFold(server.Name, a.host) {
		return "", nil, fmt.Errorf("SMTP OAuth 主机不匹配")
	}
	response := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.token)
	return "XOAUTH2", []byte(response), nil
}

// Next 遇到服务端挑战时直接终止，保留原始错误给上层处理。
func (a *smtpXOAUTH2Auth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, fmt.Errorf("SMTP OAuth 认证失败")
	}
	return nil, nil
}

// newSMTPXOAUTH2Auth 创建 SMTP OAuth 认证器。
func newSMTPXOAUTH2Auth(username string, token string, host string) smtp.Auth {
	return &smtpXOAUTH2Auth{username: username, token: token, host: host}
}
