<template>
  <q-page class="message-detail-page gmbox-page">
    <div class="detail-shell">
      <div class="detail-topbar gmbox-banner-gap">
        <q-btn flat color="primary" no-caps icon="arrow_back" label="返回列表" @click="router.push('/inbox')" />
      </div>

      <q-card v-if="detail" bordered flat class="detail-card">
        <q-card-section class="detail-header">
          <div class="detail-header-top">
            <div class="detail-subject-wrap">
              <div class="detail-subject">{{ currentMessage.subject || '(无主题)' }}</div>
              <div class="detail-badges gmbox-top-gap-sm">
                <q-badge color="grey-2" text-color="grey-8">{{ currentMessage.folder || '未知文件夹' }}</q-badge>
                <q-badge v-if="currentMessage.has_attachment" color="grey-2" text-color="grey-8">含附件</q-badge>
                <q-badge v-if="!currentMessage.is_read" color="blue-1" text-color="primary">未读</q-badge>
              </div>
            </div>

            <div class="detail-toolbar">
              <q-btn flat round dense color="primary" icon="reply" @click="openReplyDialog">
                <q-tooltip>回复邮件</q-tooltip>
              </q-btn>
              <q-btn flat round dense color="primary" :icon="currentMessage.is_read ? 'mark_email_unread' : 'mark_email_read'" @click="markRead(!currentMessage.is_read)">
                <q-tooltip>{{ currentMessage.is_read ? '标记未读' : '标记已读' }}</q-tooltip>
              </q-btn>
              <q-btn flat round dense color="secondary" :icon="showRemoteImages ? 'hide_image' : 'image'" @click="toggleImages">
                <q-tooltip>{{ showRemoteImages ? '隐藏远程图片' : '显示图片' }}</q-tooltip>
              </q-btn>
              <q-btn flat round dense color="negative" icon="delete" @click="deleteMessage">
                <q-tooltip>删除邮件</q-tooltip>
              </q-btn>
            </div>
          </div>

          <div class="detail-meta gmbox-top-gap-lg">
            <q-avatar size="var(--gmbox-avatar-size)" color="indigo-1" text-color="primary" class="detail-avatar">
              {{ senderInitials }}
            </q-avatar>
            <div class="detail-meta-main">
              <div class="detail-from-row">
                <div class="detail-from-name">{{ formatSender(currentMessage) }}</div>
                <div class="detail-date">{{ formatDate(currentMessage.sent_at) }}</div>
              </div>
              <div class="detail-recipient-line">收件人：{{ formatRecipientLine(currentMessage) }}</div>
              <div class="detail-account-line">接收邮箱：{{ currentMessage.account_email || '未知' }}</div>
            </div>
          </div>

          <div class="detail-primary-actions gmbox-top-gap-lg">
            <q-btn color="primary" unelevated no-caps icon="reply" label="回复" @click="openReplyDialog" />
            <q-btn outline color="primary" no-caps icon="forward" label="转发" @click="openForwardDialog" />
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section class="detail-secondary-actions">
          <div class="detail-move-controls">
            <q-select
              v-model="targetFolder"
              outlined
              dense
              emit-value
              map-options
              :options="mailboxOptions"
              label="移动到文件夹"
              class="detail-folder-select"
            />
            <q-btn color="primary" unelevated no-caps label="移动邮件" @click="moveMessage" />
          </div>
        </q-card-section>

        <q-card-section v-if="message" class="q-pt-none">
          <q-banner rounded :class="isError ? 'gmbox-banner-error' : 'gmbox-banner-success'">
            {{ message }}
          </q-banner>
        </q-card-section>

        <q-card-section v-if="!showRemoteImages && hasRemoteImages" class="q-pt-none">
          <q-banner rounded class="gmbox-banner-info">
            邮件包含远程图片，默认未加载。点击右上角图片按钮后才会显示。
          </q-banner>
        </q-card-section>

        <q-separator />

        <q-card-section class="detail-body-section">
          <div v-if="bodyModeOptions.length > 1" class="detail-body-toolbar gmbox-banner-gap">
            <q-btn-toggle
              v-model="bodyMode"
              unelevated
              no-caps
              toggle-color="primary"
              color="grey-2"
              text-color="grey-8"
              :options="bodyModeOptions"
            />
          </div>
          <div :class="shouldShowHtml ? 'detail-body-wrap detail-body-wrap-html' : 'detail-body-wrap'">
            <article v-if="shouldShowHtml" class="mail-html detail-body-content" v-html="sanitizedHtml"></article>
            <article v-else class="mail-text detail-body-content">{{ safeBody }}</article>
          </div>
        </q-card-section>

        <template v-if="currentAttachments.length > 0">
          <q-separator />
          <q-card-section class="detail-attachments-section">
            <div class="text-subtitle1 text-weight-bold gmbox-banner-gap">附件</div>
            <div class="detail-attachments-grid">
              <button
                v-for="attachment in currentAttachments"
                :key="attachment.id"
                type="button"
                class="detail-attachment-card"
                @click="downloadAttachment(attachment.id)"
              >
                <div class="detail-attachment-icon">
                  <q-icon name="attach_file" color="primary" size="var(--gmbox-icon-md)" />
                </div>
                <div class="detail-attachment-main">
                  <div class="detail-attachment-name">{{ attachment.file_name }}</div>
                  <div class="detail-attachment-meta">{{ attachment.content_type || '未知类型' }}</div>
                </div>
                <div class="detail-attachment-size">{{ formatSize(attachment.size) }}</div>
              </button>
            </div>
          </q-card-section>
        </template>
      </q-card>

      <q-banner v-else-if="message" rounded class="gmbox-banner-error">
        {{ message }}
      </q-banner>
    </div>

    <ComposeDialog v-model="showComposeDialog" :preset="composePreset" />
  </q-page>
</template>

<script setup lang="ts">
import DOMPurify from 'dompurify'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { request, type AttachmentItem, type MailboxItem, type MessageDetailResponse, type MessageItem } from '@/api'
import ComposeDialog from '@/components/ComposeDialog.vue'
import { extractMailHtml, extractMailText } from '@/utils/mailBody'

const route = useRoute()
const router = useRouter()
const detail = ref<MessageDetailResponse | null>(null)
const mailboxes = ref<MailboxItem[]>([])
const targetFolder = ref('')
const message = ref('')
const isError = ref(false)
const showRemoteImages = ref(false)
const bodyMode = ref<'html' | 'text'>('html')
const showComposeDialog = ref(false)
const composePreset = ref<{
  title?: string
  account_id?: number
  to?: string
  subject?: string
  body?: string
  is_html?: boolean
  notice?: string
  attachment_ids?: number[]
  attachments?: AttachmentItem[]
} | null>(null)

const currentMessage = computed<MessageItem>(() =>
  detail.value?.message ?? {
    id: 0,
    account_id: 0,
    account_email: '',
    mailbox_id: 0,
    folder: '',
    subject: '',
    from_name: '',
    from_address: '',
    to_addresses: '',
    snippet: '',
    is_read: false,
    is_deleted: false,
    has_attachment: false,
    sent_at: '',
  },
)
const currentAttachments = computed<AttachmentItem[]>(() => detail.value?.attachments ?? [])
const senderInitials = computed(() => buildInitials(currentMessage.value.from_name || currentMessage.value.from_address || '?'))
const hasRemoteImages = computed(() => /<img\b/i.test(detail.value?.body?.html_body ?? ''))
const hasHtmlBody = computed(() => Boolean(extractMailHtml(detail.value?.body?.html_body ?? '')))
const bodyModeOptions = computed(() => {
  if (!hasHtmlBody.value) {
    return [{ label: '纯文本', value: 'text' as const }]
  }
  return [
    { label: 'HTML', value: 'html' as const },
    { label: '纯文本', value: 'text' as const },
  ]
})
const shouldShowHtml = computed(() => hasHtmlBody.value && bodyMode.value === 'html' && Boolean(sanitizedHtml.value))
const sanitizedHtml = computed(() => {
  const html = extractMailHtml(detail.value?.body?.html_body ?? '')
  if (!html) {
    return ''
  }
  const sanitized = DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      'a', 'abbr', 'b', 'blockquote', 'br', 'code', 'div', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr',
      ...(showRemoteImages.value ? ['img'] : []), 'li', 'ol', 'p', 'pre', 'span', 'strong', 'table', 'tbody',
      'td', 'th', 'thead', 'tr', 'u', 'ul',
    ],
    ALLOWED_ATTR: ['alt', 'class', 'colspan', 'href', 'rowspan', ...(showRemoteImages.value ? ['src'] : []), 'style', 'target', 'title'],
    ALLOW_DATA_ATTR: false,
    FORBID_TAGS: ['form', 'iframe', 'input', 'script', 'style'],
    FORBID_ATTR: ['onerror', 'onload', 'onclick', 'onmouseover'],
  })
  return hardenSanitizedHtml(sanitized)
})
const safeBody = computed(() => {
  if (!detail.value) {
    return ''
  }
  const textBody = detail.value.body?.text_body?.trim()
  if (textBody) {
    return textBody
  }
  const htmlBody = extractMailText(detail.value.body?.html_body ?? '')
  if (htmlBody) {
    return htmlBody
  }
  return detail.value.message.snippet || ''
})
const mailboxOptions = computed(() => [
  { label: '选择目标文件夹', value: '' },
  ...mailboxes.value.map((mailbox) => ({ label: mailbox.name, value: mailbox.path })),
])

// loadDetail 获取正文和附件列表，供详情页展示和操作。
async function loadDetail() {
  try {
    detail.value = await request<MessageDetailResponse>(`/api/messages/${route.params.id}`)
    const accountID = Number(detail.value?.message?.account_id ?? 0)
    if (Number.isFinite(accountID) && accountID > 0) {
      mailboxes.value = await request<MailboxItem[]>(`/api/mailboxes?account_id=${accountID}`)
    } else {
      mailboxes.value = []
    }
    bodyMode.value = extractMailHtml(detail.value?.body?.html_body ?? '') ? 'html' : 'text'
    showRemoteImages.value = false
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '加载邮件详情失败'
  }
}

// markRead 切换已读状态后刷新详情，避免列表和详情状态不一致。
async function markRead(isRead: boolean) {
  try {
    await request(`/api/messages/${route.params.id}/${isRead ? 'read' : 'unread'}`, { method: 'POST' })
    isError.value = false
    message.value = isRead ? '已标记为已读' : '已标记为未读'
    await loadDetail()
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '操作失败'
  }
}

// deleteMessage 删除成功后返回列表，避免用户停留在已失效详情页。
async function deleteMessage() {
  try {
    await request(`/api/messages/${route.params.id}/delete`, { method: 'POST' })
    await router.push('/inbox')
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '删除失败'
  }
}

// moveMessage 将当前邮件移动到目标文件夹。
async function moveMessage() {
  if (!targetFolder.value) {
    isError.value = true
    message.value = '请先选择目标文件夹'
    return
  }
  try {
    await request(`/api/messages/${route.params.id}/move`, {
      method: 'POST',
      body: JSON.stringify({ folder: targetFolder.value }),
    })
    isError.value = false
    message.value = '移动成功'
    await loadDetail()
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '移动失败'
  }
}

// downloadAttachment 直接拉取后端下载接口并触发浏览器保存。
async function downloadAttachment(id: number) {
  const response = await fetch(`/api/attachments/${id}/download`, { credentials: 'include' })
  if (!response.ok) {
    isError.value = true
    message.value = '下载附件失败'
    return
  }
  const blob = await response.blob()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = currentAttachments.value.find((item) => item.id === id)?.file_name ?? 'attachment'
  link.click()
  URL.revokeObjectURL(url)
}

// openReplyDialog 复用写信弹窗生成回复草稿，保证详情页也能直接回信。
function openReplyDialog() {
  composePreset.value = {
    title: '回复邮件',
    account_id: currentMessage.value.account_id,
    to: resolveReplyAddress(currentMessage.value),
    subject: currentMessage.value.subject.startsWith('Re:') ? currentMessage.value.subject : `Re: ${currentMessage.value.subject || '(无主题)'}`,
    body: `\n\n--- 原始邮件 ---\n发件人：${formatSender(currentMessage.value)}\nTo: ${formatAccountEmail(currentMessage.value.account_email).replace(/^To:\s*/, '')}\n时间：${formatDate(currentMessage.value.sent_at)}\n\n${safeBody.value}`,
  }
  showComposeDialog.value = true
}

// openForwardDialog 复用写信弹窗转发当前正文和附件上下文，减少用户手动复制内容。
function openForwardDialog() {
  const forwardContent = buildForwardContent()
  composePreset.value = {
    title: '转发邮件',
    account_id: currentMessage.value.account_id,
    subject: currentMessage.value.subject.startsWith('Fwd:') ? currentMessage.value.subject : `Fwd: ${currentMessage.value.subject || '(无主题)'}`,
    body: forwardContent.body,
    is_html: forwardContent.isHTML,
    notice: currentAttachments.value.length > 0 ? '以下附件会随本次转发一起发送。' : '',
    attachment_ids: currentAttachments.value.map((attachment) => attachment.id),
    attachments: currentAttachments.value,
  }
  showComposeDialog.value = true
}

// buildForwardContent 优先保留 HTML 正文结构，避免转发富文本邮件时丢失表格和链接。
function buildForwardContent() {
  if (hasHtmlBody.value && sanitizedHtml.value) {
    return {
      isHTML: true,
      body: buildForwardHTMLBody(sanitizedHtml.value),
    }
  }
  return {
    isHTML: false,
    body: `\n\n--- 转发邮件 ---\n发件人：${formatSender(currentMessage.value)}\n收件时间：${formatDate(currentMessage.value.sent_at)}\n收件人：${formatRecipientLine(currentMessage.value)}\n\n${safeBody.value}`,
  }
}

// buildForwardHTMLBody 为转发草稿拼接一段结构化头信息，同时保留原始 HTML 正文。
function buildForwardHTMLBody(htmlBody: string) {
  return `<div data-gmbox-forward-meta><p>---</p><p>转发邮件</p><p>发件人：${escapeHtml(formatSender(currentMessage.value))}</p><p>收件时间：${escapeHtml(formatDate(currentMessage.value.sent_at))}</p><p>收件人：${escapeHtml(formatRecipientLine(currentMessage.value))}</p></div>${htmlBody}`
}

// escapeHtml 避免转发头信息里的地址和主题被当作 HTML 片段解析。
function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

// resolveReplyAddress 在已发送邮件场景优先回复原始收件人，避免把邮件回给自己。
function resolveReplyAddress(item: MessageItem) {
  const sender = item.from_address?.trim().toLowerCase()
  const accountEmail = item.account_email?.trim().toLowerCase()
  if (sender && accountEmail && sender === accountEmail) {
    return extractFirstAddress(item.to_addresses) || item.from_address
  }
  return item.from_address
}

// extractFirstAddress 尽量从 RFC822 风格列表里提取第一个邮箱地址，兼容名称包裹格式。
function extractFirstAddress(value: string) {
  const matched = value.match(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i)
  return matched?.[0] ?? ''
}

// formatDate 统一处理时间显示。
function formatDate(value: string) {
  return value ? new Date(value).toLocaleString('zh-CN') : '刚刚'
}

// formatSender 把发件人名称和邮箱合并展示，方便详情页快速确认来源。
function formatSender(item: MessageItem) {
  const name = item.from_name?.trim()
  const address = item.from_address?.trim()
  if (name && address && name !== address) {
    return `${name} <${address}>`
  }
  return address || name || '未知发件人'
}

// formatAccountEmail 统一输出当前接入账户邮箱，避免详情页和列表页口径不一致。
function formatAccountEmail(value: string) {
  const address = value.trim()
  if (!address) {
    return 'To: 未知'
  }
  return `To: ${address}`
}

// formatRecipientLine 优先展示原始收件人，避免详情页只看到接入邮箱看不出真实投递对象。
function formatRecipientLine(item: MessageItem) {
  const recipients = item.to_addresses?.trim()
  if (recipients) {
    return recipients
  }
  return formatAccountEmail(item.account_email).replace(/^To:\s*/, '')
}

// buildInitials 用首字母头像强化发件人识别，接近常见邮箱客户端的阅读体验。
function buildInitials(value: string) {
  const text = value.trim()
  if (!text) {
    return '?'
  }
  const parts = text
    .replace(/<.*?>/g, ' ')
    .split(/\s+/)
    .filter(Boolean)
  const initials = parts.slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('')
  return initials || text.slice(0, 1).toUpperCase()
}

// formatSize 输出更易读的附件大小。
function formatSize(size: number) {
  if (size < 1024) {
    return `${size} B`
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

// toggleImages 让远程图片只在用户显式确认后才渲染，降低追踪像素风险。
function toggleImages() {
  showRemoteImages.value = !showRemoteImages.value
}

// hardenSanitizedHtml 对已经过白名单清洗的 HTML 再补一层链接安全策略。
function hardenSanitizedHtml(html: string) {
  const parser = new DOMParser()
  const doc = parser.parseFromString(html, 'text/html')
  doc.querySelectorAll('a').forEach((anchor) => {
    const href = anchor.getAttribute('href')?.trim() ?? ''
    if (!href) {
      anchor.removeAttribute('href')
      return
    }
    const lowerHref = href.toLowerCase()
    if (!lowerHref.startsWith('http://') && !lowerHref.startsWith('https://') && !lowerHref.startsWith('mailto:')) {
      anchor.removeAttribute('href')
      return
    }
    anchor.setAttribute('rel', 'noopener noreferrer nofollow')
    if (lowerHref.startsWith('http://') || lowerHref.startsWith('https://')) {
      anchor.setAttribute('target', '_blank')
    } else {
      anchor.removeAttribute('target')
    }
  })
  doc.querySelectorAll('img').forEach((image) => {
    const src = image.getAttribute('src')?.trim() ?? ''
    if (!showRemoteImages.value || !src) {
      image.remove()
      return
    }
    const lowerSrc = src.toLowerCase()
    if (!lowerSrc.startsWith('http://') && !lowerSrc.startsWith('https://')) {
      image.removeAttribute('src')
    }
  })
  return doc.body.innerHTML
}

onMounted(loadDetail)
</script>

<style scoped>
.message-detail-page {
  background: #f5f7fb;
}

.detail-shell {
  max-width: var(--gmbox-detail-shell);
  margin: 0 auto;
}

.detail-topbar {
  display: flex;
  align-items: center;
}

.detail-card {
  border-radius: 1.125rem;
  background: #fff;
  box-shadow: 0 0.625rem 1.75rem rgba(15, 23, 42, 0.08);
}

.detail-header {
  padding: var(--gmbox-space-2xl) var(--gmbox-space-xl) var(--gmbox-space-lg);
}

.detail-header-top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--gmbox-space-xl);
}

.detail-subject-wrap {
  min-width: 0;
  flex: 1;
}

.detail-subject {
  font-size: clamp(1.75rem, 1.4rem + 1vw, 2.125rem);
  line-height: 1.2;
  font-weight: 400;
  color: #1d4ed8;
  word-break: break-word;
}

.detail-badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gmbox-space-sm);
}

.detail-toolbar {
  display: flex;
  align-items: center;
  gap: clamp(0.25rem, 0.2rem + 0.15vw, 0.375rem);
}

.detail-meta {
  display: flex;
  align-items: flex-start;
  gap: var(--gmbox-space-md);
}

.detail-primary-actions {
  display: flex;
  align-items: center;
  gap: var(--gmbox-space-md);
}

.detail-avatar {
  flex: none;
  font-size: var(--gmbox-font-body);
  font-weight: 600;
}

.detail-meta-main {
  min-width: 0;
  flex: 1;
}

.detail-from-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--gmbox-space-md);
}

.detail-from-name {
  min-width: 0;
  font-size: var(--gmbox-font-body);
  font-weight: 600;
  color: #0f172a;
  word-break: break-word;
}

.detail-date,
.detail-recipient-line,
.detail-account-line {
  font-size: clamp(0.8125rem, 0.78rem + 0.12vw, 0.875rem);
  color: #64748b;
}

.detail-recipient-line,
.detail-account-line {
  margin-top: var(--gmbox-space-xs);
  word-break: break-word;
}

.detail-secondary-actions {
  padding: clamp(1rem, 0.9rem + 0.3vw, 1.125rem) var(--gmbox-space-xl);
}

.detail-move-controls {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: var(--gmbox-space-md);
}

.detail-folder-select {
  min-width: clamp(12rem, 24vw, 15rem);
}

.detail-body-section,
.detail-attachments-section {
  padding: var(--gmbox-space-2xl) var(--gmbox-space-xl) var(--gmbox-space-xl);
}

.detail-body-wrap,
.detail-attachments-grid {
  max-width: var(--gmbox-body-wrap);
}

.detail-body-wrap-html {
  max-width: var(--gmbox-body-wrap-wide);
  margin: 0 auto;
}

.detail-body-wrap-html .detail-body-content {
  margin: 0 auto;
}

.detail-body-toolbar {
  max-width: var(--gmbox-body-wrap);
}

.detail-attachments-grid {
  display: grid;
  gap: var(--gmbox-space-md);
}

.detail-attachment-card {
  display: flex;
  align-items: center;
  width: 100%;
  padding: clamp(0.875rem, 0.8rem + 0.18vw, 1rem) var(--gmbox-space-md);
  border: 0.0625rem solid #dbe4f0;
  border-radius: 0.875rem;
  background: linear-gradient(180deg, #ffffff 0%, #f8fbff 100%);
  text-align: left;
  cursor: pointer;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.detail-attachment-card:hover {
  border-color: #93c5fd;
  box-shadow: 0 0.625rem 1.5rem rgba(59, 130, 246, 0.12);
  transform: translateY(-0.0625rem);
}

.detail-attachment-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: clamp(2.375rem, 2.2rem + 0.5vw, 2.625rem);
  height: clamp(2.375rem, 2.2rem + 0.5vw, 2.625rem);
  border-radius: 0.75rem;
  background: #eff6ff;
  flex: none;
}

.detail-attachment-main {
  min-width: 0;
  flex: 1;
  margin-left: clamp(0.75rem, 0.65rem + 0.2vw, 0.875rem);
}

.detail-attachment-name {
  color: #0f172a;
  font-size: clamp(0.8125rem, 0.78rem + 0.12vw, 0.875rem);
  font-weight: 600;
  word-break: break-word;
}

.detail-attachment-meta,
.detail-attachment-size {
  color: #64748b;
  font-size: clamp(0.75rem, 0.72rem + 0.1vw, 0.8125rem);
}

.detail-attachment-meta {
  margin-top: var(--gmbox-space-2xs);
}

.detail-attachment-size {
  margin-left: var(--gmbox-space-md);
  white-space: nowrap;
}

.detail-body-content {
  font-size: var(--gmbox-font-body);
}

@media (max-width: 63.9375rem) {
  .detail-header,
  .detail-secondary-actions,
  .detail-body-section,
  .detail-attachments-section {
    padding-left: var(--gmbox-space-lg);
    padding-right: var(--gmbox-space-lg);
  }

  .detail-subject {
    font-size: clamp(1.5rem, 1.3rem + 0.8vw, 1.75rem);
  }

  .detail-header-top,
  .detail-from-row,
  .detail-move-controls {
    flex-direction: column;
    align-items: stretch;
  }

  .detail-primary-actions,
  .detail-attachment-card {
    align-items: stretch;
  }

  .detail-primary-actions,
  .detail-attachment-card {
    flex-direction: column;
  }

  .detail-attachment-main,
  .detail-attachment-size {
    margin-left: 0;
    margin-top: clamp(0.5rem, 0.42rem + 0.2vw, 0.625rem);
  }

  .detail-toolbar {
    justify-content: flex-start;
  }

  .detail-folder-select {
    min-width: 0;
    width: 100%;
  }
}
</style>
