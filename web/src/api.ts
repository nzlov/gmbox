export interface MailAccount {
  id: number
  name: string
  email: string
  provider: string
  provider_name: string
  auth_type: 'password' | 'oauth'
  username: string
  incoming_protocol: 'imap' | 'pop3'
  imap_host: string
  imap_port: number
  pop3_host: string
  pop3_port: number
  smtp_host: string
  smtp_port: number
  use_tls: boolean
  enabled: boolean
  oauth_token_expiry?: string | null
}

export interface ProviderPreset {
  key: string
  name: string
  incoming_protocol: 'imap' | 'pop3'
  imap_host: string
  imap_port: number
  pop3_host: string
  pop3_port: number
  smtp_host: string
  smtp_port: number
  use_tls: boolean
  supports_oauth: boolean
}

export interface AccountProvidersResponse {
  items: ProviderPreset[]
  microsoft_oauth_enabled: boolean
}

export interface MicrosoftOAuthConfigResponse {
  enabled: boolean
  client_id: string
  tenant_id: string
  redirect_uri: string
  scope: string
  flow: 'pkce' | 'legacy'
}

export interface MicrosoftOAuthExchangeResponse {
  message: string
  account: MailAccount
}

export interface ThemePreference {
  id?: number
  user_id?: number
  theme_name: string
  theme_mode: 'light' | 'dark'
  primary_color: string
  secondary_color: string
  accent_color: string
}

export interface MessageItem {
  id: number
  account_id: number
  account_email: string
  mailbox_id: number
  folder: string
  subject: string
  from_name: string
  from_address: string
  to_addresses: string
  snippet: string
  is_read: boolean
  is_deleted: boolean
  has_attachment: boolean
  sent_at: string
}

export interface MessageListResponse {
  items: MessageItem[]
  total: number
  page: number
  page_size: number
}

export interface MessageBody {
  id: number
  message_id: number
  text_body: string
  html_body: string
}

export interface MailboxItem {
  id: number
  account_id: number
  name: string
  path: string
  role: string
}

export interface AttachmentItem {
  id: number
  message_id: number
  file_name: string
  part_id: string
  content_type: string
  size: number
  storage_path: string
}

export interface MessageDetailResponse {
  message: MessageItem
  body: MessageBody
  attachments: AttachmentItem[]
}

export interface ContactItem {
  address: string
  name: string
  latest_sent_at: string
  total: number
}

export interface ContactListResponse {
  items: ContactItem[]
  total: number
  page: number
  page_size: number
}

export interface SyncLogItem {
  id: number
  account_id: number
  account_name: string
  account_email: string
  trigger: string
  protocol: string
  started_at: string
  finished_at: string
  duration_ms: number
  new_messages: number
  mailbox_count: number
  success: boolean
  retried_oauth: boolean
  summary_message: string
  error_message: string
}

export interface SyncLogListResponse {
  items: SyncLogItem[]
  total: number
  page: number
  page_size: number
}

// request 封装统一请求入口，确保所有页面都带上 Cookie，并归一化后端返回的主键字段。
export async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    const data = await response.json().catch(() => ({ message: '请求失败' }))
    throw new Error(data.message ?? '请求失败')
  }

  const data = await response.json().catch(() => null)
  return normalizePayload(data) as T
}

// normalizePayload 递归把后端返回的 `ID` 映射为前端统一使用的 `id`，避免 URL 拼出 undefined。
function normalizePayload(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => normalizePayload(item))
  }
  if (!value || typeof value !== 'object') {
    return value
  }

  const record = value as Record<string, unknown>
  const normalized: Record<string, unknown> = {}
  for (const [key, item] of Object.entries(record)) {
    normalized[key] = normalizePayload(item)
  }

  if (normalized.id == null && typeof record.ID === 'number') {
    normalized.id = record.ID
  }
  return normalized
}
