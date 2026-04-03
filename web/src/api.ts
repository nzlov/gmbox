export interface MailAccount {
  id: number
  name: string
  email: string
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
}

export interface MessageItem {
  id: number
  account_id: number
  mailbox_id: number
  folder: string
  subject: string
  from_name: string
  from_address: string
  snippet: string
  is_read: boolean
  is_deleted: boolean
  has_attachment: boolean
  sent_at: string
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

// request 封装统一请求入口，确保所有页面都带上 Cookie。
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

  return response.json() as Promise<T>
}
