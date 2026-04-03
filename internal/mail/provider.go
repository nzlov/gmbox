package mail

import "strings"

// ProviderPreset 描述常见邮箱服务商的默认收发信配置。
type ProviderPreset struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	IncomingProtocol string `json:"incoming_protocol"`
	IMAPHost         string `json:"imap_host"`
	IMAPPort         int    `json:"imap_port"`
	POP3Host         string `json:"pop3_host"`
	POP3Port         int    `json:"pop3_port"`
	SMTPHost         string `json:"smtp_host"`
	SMTPPort         int    `json:"smtp_port"`
	UseTLS           bool   `json:"use_tls"`
	SupportsOAuth    bool   `json:"supports_oauth"`
}

var providerPresets = map[string]ProviderPreset{
	"custom": {
		Key:              "custom",
		Name:             "自定义",
		IncomingProtocol: "imap",
		IMAPPort:         993,
		POP3Port:         995,
		SMTPPort:         465,
		UseTLS:           true,
	},
	"gmail": {
		Key:              "gmail",
		Name:             "Gmail",
		IncomingProtocol: "imap",
		IMAPHost:         "imap.gmail.com",
		IMAPPort:         993,
		POP3Host:         "pop.gmail.com",
		POP3Port:         995,
		SMTPHost:         "smtp.gmail.com",
		SMTPPort:         465,
		UseTLS:           true,
	},
	"qq": {
		Key:              "qq",
		Name:             "QQ 邮箱",
		IncomingProtocol: "imap",
		IMAPHost:         "imap.qq.com",
		IMAPPort:         993,
		POP3Host:         "pop.qq.com",
		POP3Port:         995,
		SMTPHost:         "smtp.qq.com",
		SMTPPort:         465,
		UseTLS:           true,
	},
	"163": {
		Key:              "163",
		Name:             "163 邮箱",
		IncomingProtocol: "imap",
		IMAPHost:         "imap.163.com",
		IMAPPort:         993,
		POP3Host:         "pop.163.com",
		POP3Port:         995,
		SMTPHost:         "smtp.163.com",
		SMTPPort:         465,
		UseTLS:           true,
	},
	"126": {
		Key:              "126",
		Name:             "126 邮箱",
		IncomingProtocol: "imap",
		IMAPHost:         "imap.126.com",
		IMAPPort:         993,
		POP3Host:         "pop.126.com",
		POP3Port:         995,
		SMTPHost:         "smtp.126.com",
		SMTPPort:         465,
		UseTLS:           true,
	},
	"aliyun": {
		Key:              "aliyun",
		Name:             "阿里云邮箱",
		IncomingProtocol: "imap",
		IMAPHost:         "imap.aliyun.com",
		IMAPPort:         993,
		POP3Host:         "pop.aliyun.com",
		POP3Port:         995,
		SMTPHost:         "smtp.aliyun.com",
		SMTPPort:         465,
		UseTLS:           true,
	},
	"outlook": {
		Key:              "outlook",
		Name:             "Outlook / Hotmail",
		IncomingProtocol: "imap",
		IMAPHost:         "outlook.office365.com",
		IMAPPort:         993,
		POP3Host:         "outlook.office365.com",
		POP3Port:         995,
		SMTPHost:         "smtp.office365.com",
		SMTPPort:         587,
		UseTLS:           true,
		SupportsOAuth:    true,
	},
	"yahoo": {
		Key:              "yahoo",
		Name:             "Yahoo Mail",
		IncomingProtocol: "imap",
		IMAPHost:         "imap.mail.yahoo.com",
		IMAPPort:         993,
		POP3Host:         "pop.mail.yahoo.com",
		POP3Port:         995,
		SMTPHost:         "smtp.mail.yahoo.com",
		SMTPPort:         465,
		UseTLS:           true,
	},
}

// ProviderPresets 返回前端可直接消费的服务商配置列表。
func ProviderPresets() []ProviderPreset {
	result := make([]ProviderPreset, 0, len(providerPresets))
	keys := []string{"gmail", "qq", "163", "126", "aliyun", "outlook", "yahoo", "custom"}
	for _, key := range keys {
		result = append(result, providerPresets[key])
	}
	return result
}

// LookupProviderPreset 根据服务商键获取默认配置。
func LookupProviderPreset(provider string) ProviderPreset {
	preset, ok := providerPresets[normalizeProvider(provider)]
	if !ok {
		return providerPresets["custom"]
	}
	return preset
}

// normalizeProvider 统一归一化服务商标识，避免前后端拼写差异。
func normalizeProvider(provider string) string {
	trimmed := strings.ToLower(strings.TrimSpace(provider))
	if trimmed == "" {
		return "custom"
	}
	if trimmed == "microsoft" {
		return "outlook"
	}
	if _, ok := providerPresets[trimmed]; ok {
		return trimmed
	}
	return "custom"
}

// providerDisplayName 为服务商键提供稳定展示名。
func providerDisplayName(provider string) string {
	return LookupProviderPreset(provider).Name
}

// normalizeAuthType 对认证方式做最小兜底，避免空值破坏兼容性。
func normalizeAuthType(authType string) string {
	if strings.EqualFold(strings.TrimSpace(authType), "oauth") {
		return "oauth"
	}
	return "password"
}

// ApplyProviderPreset 用服务商默认值补齐未填写的服务器配置，同时保留用户手动修改结果。
func ApplyProviderPreset(input AccountInput) AccountInput {
	preset := LookupProviderPreset(input.Provider)
	if strings.TrimSpace(input.IncomingProtocol) == "" {
		input.IncomingProtocol = preset.IncomingProtocol
	}
	if strings.TrimSpace(input.IMAPHost) == "" {
		input.IMAPHost = preset.IMAPHost
	}
	if input.IMAPPort <= 0 {
		input.IMAPPort = preset.IMAPPort
	}
	if strings.TrimSpace(input.POP3Host) == "" {
		input.POP3Host = preset.POP3Host
	}
	if input.POP3Port <= 0 {
		input.POP3Port = preset.POP3Port
	}
	if strings.TrimSpace(input.SMTPHost) == "" {
		input.SMTPHost = preset.SMTPHost
	}
	if input.SMTPPort <= 0 {
		input.SMTPPort = preset.SMTPPort
	}
	return input
}
