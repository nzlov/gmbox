package mail

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"gmbox/internal/model"
)

var (
	oauthJSONSecretPattern = regexp.MustCompile(`"(access_token|refresh_token|id_token|client_secret)"\s*:\s*"[^"]*"`)
	oauthFormSecretPattern = regexp.MustCompile(`(?i)(access_token|refresh_token|client_secret|code)=([^&\s]+)`)
	imapAuthLinePattern    = regexp.MustCompile(`(?i)(AUTHENTICATE\s+(XOAUTH2|OAUTHBEARER)\s+)(\S+)`)
	base64BlobPattern      = regexp.MustCompile(`^[A-Za-z0-9+/=]{40,}$`)
	bearerSecretPattern    = regexp.MustCompile(`(?i)Bearer\s+[^\s\x01]+`)
)

// debugLoggingEnabled 控制是否输出调试和服务商交互日志。
func (s *Service) debugLoggingEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.DebugMode()
}

// debugProviderLog 统一输出服务商交互调试日志，避免普通模式刷屏。
func (s *Service) debugProviderLog(message string, attrs ...any) {
	if !s.debugLoggingEnabled() {
		return
	}
	slog.Debug(message, attrs...)
}

// newIMAPDebugWriter 在 debug 模式下输出脱敏后的 IMAP 原始交互，便于定位服务端兼容性问题。
func (s *Service) newIMAPDebugWriter(account model.MailAccount) io.Writer {
	if !s.debugLoggingEnabled() {
		return nil
	}
	return &imapDebugWriter{service: s, account: account}
}

type imapDebugWriter struct {
	service *Service
	account model.MailAccount
	buffer  bytes.Buffer
}

func (w *imapDebugWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	_, _ = w.buffer.Write(p)
	for {
		line, err := w.buffer.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				remaining := w.buffer.String()
				w.buffer.Reset()
				_, _ = w.buffer.WriteString(remaining)
				break
			}
			return len(p), err
		}
		w.service.debugProviderLog("IMAP 原始交互", "provider", w.account.Provider, "email", w.account.Email, "host", w.account.IMAPHost, "payload", sanitizeIMAPDebugLine(line))
	}
	return len(p), nil
}

// sanitizeIMAPDebugLine 脱敏 IMAP 认证报文中的敏感内容，避免 access token 直接落日志。
func sanitizeIMAPDebugLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return trimmed
	}
	trimmed = imapAuthLinePattern.ReplaceAllString(trimmed, `${1}<redacted>`)
	if base64BlobPattern.MatchString(trimmed) {
		return "<redacted-base64>"
	}
	return sanitizeSensitiveText(trimmed)
}

// sanitizeSensitiveText 统一脱敏 OAuth 请求和响应中的敏感字段。
func sanitizeSensitiveText(value string) string {
	result := oauthJSONSecretPattern.ReplaceAllStringFunc(value, func(matched string) string {
		parts := strings.SplitN(matched, ":", 2)
		if len(parts) != 2 {
			return matched
		}
		return parts[0] + `:"<redacted>"`
	})
	result = oauthFormSecretPattern.ReplaceAllString(result, `${1}=<redacted>`)
	result = bearerSecretPattern.ReplaceAllString(result, `Bearer <redacted>`)
	return result
}

// redactFormForLog 输出脱敏后的表单参数，避免 client_secret 等敏感字段泄漏到日志。
func redactFormForLog(form url.Values) map[string]string {
	result := make(map[string]string, len(form))
	for key, values := range form {
		joined := strings.Join(values, ",")
		if key == "client_secret" || key == "refresh_token" || key == "access_token" || key == "code" || key == "code_verifier" {
			result[key] = "<redacted>"
			continue
		}
		result[key] = joined
	}
	return result
}

// redactJSONBodyForLog 将 OAuth JSON 响应中的敏感字段统一替换，保留其余调试信息。
func redactJSONBodyForLog(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return sanitizeSensitiveText(string(body))
	}
	for _, key := range []string{"access_token", "refresh_token", "id_token", "client_secret"} {
		if _, ok := payload[key]; ok {
			payload[key] = "<redacted>"
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return sanitizeSensitiveText(string(body))
	}
	return string(encoded)
}
