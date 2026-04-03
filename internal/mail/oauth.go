package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gmbox/internal/model"
	"gorm.io/gorm"
)

const (
	microsoftAuthorizeScope = "offline_access openid email User.Read https://outlook.office.com/IMAP.AccessAsUser.All https://outlook.office.com/SMTP.Send"
	oauthExpirySkew         = 2 * time.Minute
)

// MicrosoftOAuthScope 返回统一维护的微软授权 scope，避免前后端各自维护时出现偏差。
func MicrosoftOAuthScope() string {
	return microsoftAuthorizeScope
}

type microsoftTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type microsoftProfile struct {
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

// MicrosoftOAuthEnabled 判断当前运行环境是否已配置微软 OAuth 所需密钥。
func (s *Service) MicrosoftOAuthEnabled() bool {
	if s == nil || s.cfg == nil {
		return false
	}
	return strings.TrimSpace(s.cfg.MicrosoftOAuth.ClientID) != "" && strings.TrimSpace(s.cfg.MicrosoftOAuth.ClientSecret) != ""
}

// BuildMicrosoftOAuthURL 生成微软授权跳转地址，供前端发起 OAuth 登录。
func (s *Service) BuildMicrosoftOAuthURL(state string) (string, error) {
	return s.BuildMicrosoftPKCEOAuthURL(state, s.cfg.MicrosoftOAuth.RedirectURL, "")
}

// BuildMicrosoftPKCEOAuthURL 统一组装微软授权地址，兼容传统服务端回跳和前端 PKCE 流程。
func (s *Service) BuildMicrosoftPKCEOAuthURL(state string, redirectURI string, codeChallenge string) (string, error) {
	if !s.MicrosoftOAuthEnabled() {
		return "", fmt.Errorf("微软 OAuth 未配置，请先设置 client_id 和 client_secret")
	}
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = s.cfg.MicrosoftOAuth.RedirectURL
	}
	query := url.Values{}
	query.Set("client_id", s.cfg.MicrosoftOAuth.ClientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", redirectURI)
	query.Set("response_mode", "query")
	query.Set("scope", microsoftAuthorizeScope)
	query.Set("state", state)
	query.Set("prompt", "select_account")
	if strings.TrimSpace(codeChallenge) != "" {
		query.Set("code_challenge", codeChallenge)
		query.Set("code_challenge_method", "S256")
	}
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?%s", s.microsoftTenant(), query.Encode()), nil
}

// CreateOAuthState 生成短期 state，降低第三方回调伪造风险。
func CreateOAuthState() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// UpsertMicrosoftOAuthAccount 完成旧服务端回调流的授权换码、资料获取和邮箱账户落库。
func (s *Service) UpsertMicrosoftOAuthAccount(ctx context.Context, code string, redirectURI string) (*model.MailAccount, error) {
	return s.UpsertMicrosoftOAuthAccountWithPKCE(ctx, code, "", redirectURI)
}

// UpsertMicrosoftOAuthAccountWithPKCE 支持前端 PKCE 回调后带 verifier 的换码流程。
func (s *Service) UpsertMicrosoftOAuthAccountWithPKCE(ctx context.Context, code string, codeVerifier string, redirectURI string) (*model.MailAccount, error) {
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = s.cfg.MicrosoftOAuth.RedirectURL
	}
	form := url.Values{
		"grant_type":   []string{"authorization_code"},
		"code":         []string{code},
		"redirect_uri": []string{redirectURI},
	}
	if strings.TrimSpace(codeVerifier) != "" {
		form.Set("code_verifier", codeVerifier)
	}
	token, err := s.exchangeMicrosoftToken(ctx, form)
	if err != nil {
		return nil, err
	}
	profile, err := s.fetchMicrosoftProfile(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	mailAddress := strings.TrimSpace(profile.Mail)
	if mailAddress == "" {
		mailAddress = strings.TrimSpace(profile.UserPrincipalName)
	}
	if mailAddress == "" {
		return nil, fmt.Errorf("微软账户未返回可用邮箱地址")
	}
	displayName := strings.TrimSpace(profile.DisplayName)
	if displayName == "" {
		displayName = mailAddress
	}
	preset := LookupProviderPreset("outlook")
	var account model.MailAccount
	err = s.db.Where("email = ?", mailAddress).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if account.Model.ID == 0 {
		account = model.MailAccount{
			Name:             displayName,
			Email:            mailAddress,
			Provider:         preset.Key,
			ProviderName:     preset.Name,
			AuthType:         "oauth",
			Username:         mailAddress,
			IncomingProtocol: preset.IncomingProtocol,
			IMAPHost:         preset.IMAPHost,
			IMAPPort:         preset.IMAPPort,
			POP3Host:         preset.POP3Host,
			POP3Port:         preset.POP3Port,
			SMTPHost:         preset.SMTPHost,
			SMTPPort:         preset.SMTPPort,
			UseTLS:           preset.UseTLS,
			Enabled:          true,
		}
	}
	account.Name = displayName
	account.Email = mailAddress
	account.Username = mailAddress
	account.Provider = preset.Key
	account.ProviderName = preset.Name
	account.AuthType = "oauth"
	account.IncomingProtocol = "imap"
	account.IMAPHost = preset.IMAPHost
	account.IMAPPort = preset.IMAPPort
	account.SMTPHost = preset.SMTPHost
	account.SMTPPort = preset.SMTPPort
	account.UseTLS = preset.UseTLS
	account.Enabled = true
	if err := s.storeOAuthToken(&account, *token); err != nil {
		return nil, err
	}
	if account.Model.ID == 0 {
		if err := s.db.Create(&account).Error; err != nil {
			return nil, err
		}
	} else {
		if err := s.db.Save(&account).Error; err != nil {
			return nil, err
		}
	}
	return &account, nil
}

// OAuthAccessToken 返回当前账户可用的 OAuth access token，并在必要时自动刷新。
func (s *Service) OAuthAccessToken(ctx context.Context, account *model.MailAccount) (string, error) {
	if normalizeAuthType(account.AuthType) != "oauth" {
		return "", fmt.Errorf("当前邮箱未启用 OAuth")
	}
	if strings.TrimSpace(account.OAuthAccessToken) == "" {
		return "", fmt.Errorf("当前邮箱缺少 OAuth access token")
	}
	if account.OAuthTokenExpiry != nil && account.OAuthTokenExpiry.After(time.Now().Add(oauthExpirySkew)) {
		return s.crypto.Decrypt(account.OAuthAccessToken)
	}
	refreshToken, err := s.crypto.Decrypt(account.OAuthRefreshToken)
	if err != nil || strings.TrimSpace(refreshToken) == "" {
		return "", fmt.Errorf("OAuth refresh token 不可用，请重新授权")
	}
	token, err := s.exchangeMicrosoftToken(ctx, url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
		"scope":         []string{microsoftAuthorizeScope},
		"redirect_uri":  []string{s.cfg.MicrosoftOAuth.RedirectURL},
	})
	if err != nil {
		return "", err
	}
	if err := s.storeOAuthToken(account, *token); err != nil {
		return "", err
	}
	if err := s.db.Save(account).Error; err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

// storeOAuthToken 统一加密保存 access/refresh token，避免散落多处重复逻辑。
func (s *Service) storeOAuthToken(account *model.MailAccount, token microsoftTokenResponse) error {
	accessToken, err := s.crypto.Encrypt(token.AccessToken)
	if err != nil {
		return err
	}
	account.OAuthAccessToken = accessToken
	if strings.TrimSpace(token.RefreshToken) != "" {
		refreshToken, err := s.crypto.Encrypt(token.RefreshToken)
		if err != nil {
			return err
		}
		account.OAuthRefreshToken = refreshToken
	}
	expiry := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	account.OAuthTokenExpiry = &expiry
	return nil
}

// fetchMicrosoftProfile 读取微软账户基础资料，用于自动填充邮箱名称和地址。
func (s *Service) fetchMicrosoftProfile(ctx context.Context, accessToken string) (*microsoftProfile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/me?$select=displayName,mail,userPrincipalName", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("获取微软用户资料失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("获取微软用户资料失败: %s", strings.TrimSpace(string(body)))
	}
	var profile microsoftProfile
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("解析微软用户资料失败: %w", err)
	}
	return &profile, nil
}

// exchangeMicrosoftToken 统一处理授权码和刷新令牌换取 access token。
func (s *Service) exchangeMicrosoftToken(ctx context.Context, form url.Values) (*microsoftTokenResponse, error) {
	if !s.MicrosoftOAuthEnabled() {
		return nil, fmt.Errorf("微软 OAuth 未配置，请先设置 client_id 和 client_secret")
	}
	form.Set("client_id", s.cfg.MicrosoftOAuth.ClientID)
	form.Set("client_secret", s.cfg.MicrosoftOAuth.ClientSecret)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", s.microsoftTenant()), bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求微软 token 失败: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8192))
	if err != nil {
		return nil, fmt.Errorf("读取微软 token 响应失败: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("请求微软 token 失败: %s", strings.TrimSpace(string(body)))
	}
	var token microsoftTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("解析微软 token 响应失败: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("微软 token 响应缺少 access_token")
	}
	return &token, nil
}

// microsoftTenant 统一返回 OAuth 租户，避免空值导致回调地址不可用。
func (s *Service) microsoftTenant() string {
	tenant := strings.TrimSpace(s.cfg.MicrosoftOAuth.TenantID)
	if tenant == "" {
		return "common"
	}
	return tenant
}
