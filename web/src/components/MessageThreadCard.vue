<template>
  <q-card bordered flat class="message-card">
    <q-card-section class="cursor-pointer" @click="toggleExpanded">
      <div class="row items-start q-col-gutter-md no-wrap">
        <div class="col">
          <div :class="message.is_read ? 'text-subtitle2' : 'text-subtitle2 text-weight-bold'">{{ message.subject || '(无主题)' }}</div>
          <div v-if="!hideSender" class="text-caption text-grey-7 q-mt-xs">{{ formatSender(message) }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ message.snippet || '暂无摘要' }}</div>
          <div class="row q-gutter-sm q-mt-sm">
            <q-badge v-if="showFolder" color="grey-3" text-color="dark">{{ message.folder }}</q-badge>
            <q-badge v-if="message.has_attachment" color="grey-3" text-color="dark">含附件</q-badge>
            <q-badge v-if="!message.is_read" color="primary" text-color="white">未读</q-badge>
          </div>
        </div>
        <div class="col-auto text-caption text-grey-6 message-meta">
          <div>{{ formatDate(message.sent_at) }}</div>
          <div class="q-mt-xs">{{ formatAccountEmail(message.account_email) }}</div>
        </div>
      </div>
    </q-card-section>

    <template v-if="expanded">
      <q-separator />

      <q-card-section class="row items-center q-col-gutter-sm">
        <div class="col row q-gutter-xs">
          <q-btn flat round dense color="primary" :icon="message.is_read ? 'mark_email_unread' : 'mark_email_read'" @click="toggleRead"><q-tooltip>{{ message.is_read ? '标记为未读' : '标记为已读' }}</q-tooltip></q-btn>
          <q-btn flat round dense color="negative" icon="delete" @click="deleteMessage"><q-tooltip>删除邮件</q-tooltip></q-btn>
          <q-btn flat round dense color="secondary" :icon="showRemoteImages ? 'hide_image' : 'image'" @click="showRemoteImages = !showRemoteImages"><q-tooltip>{{ showRemoteImages ? '隐藏远程图片' : '显示远程图片' }}</q-tooltip></q-btn>
          <q-btn flat round dense color="secondary" icon="drive_file_move" @click="openMoveDialog"><q-tooltip>移动邮件</q-tooltip></q-btn>
          <q-btn v-if="showReply" flat round dense color="primary" icon="reply" @click="emitReply"><q-tooltip>回复邮件</q-tooltip></q-btn>
        </div>
      </q-card-section>

      <q-card-section v-if="statusMessage" class="q-pt-none">
        <q-banner rounded dense :class="statusError ? 'bg-red-1 text-negative' : 'bg-green-1 text-positive'">
          {{ statusMessage }}
        </q-banner>
      </q-card-section>

      <q-card-section>
        <q-inner-loading :showing="loadingDetail">
          <q-spinner color="primary" size="32px" />
        </q-inner-loading>
        <article v-if="sanitizedHtml" class="mail-html" v-html="sanitizedHtml"></article>
        <article v-else class="mail-text">{{ safeBody }}</article>
      </q-card-section>

      <template v-if="detail?.attachments?.length">
        <q-separator />
        <q-card-section>
          <q-list bordered separator>
            <q-item v-for="attachment in detail.attachments" :key="attachment.id" clickable @click="downloadAttachment(attachment.id, attachment.file_name)">
              <q-item-section avatar>
                <q-icon name="attach_file" color="primary" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ attachment.file_name }}</q-item-label>
                <q-item-label caption>{{ attachment.content_type || '未知类型' }}</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card-section>
      </template>
    </template>
  </q-card>

  <q-dialog v-model="showMoveDialog">
    <q-card class="full-width" style="max-width: 420px">
      <q-card-section class="text-h6">选择移动位置</q-card-section>
      <q-card-section>
        <q-select v-model="targetFolder" outlined emit-value map-options :options="mailboxOptions" label="目标文件夹" />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat no-caps label="取消" v-close-popup />
        <q-btn color="primary" unelevated no-caps label="移动" @click="moveMessage" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import DOMPurify from 'dompurify'
import { computed, onMounted, ref, watch } from 'vue'
import { request, type MailboxItem, type MessageDetailResponse, type MessageItem } from '@/api'
import { extractMailHtml, extractMailText } from '@/utils/mailBody'

const props = withDefaults(defineProps<{
  message: MessageItem
  initialExpanded?: boolean
  collapsible?: boolean
  hideSender?: boolean
  showReply?: boolean
  showFolder?: boolean
}>(), {
  initialExpanded: false,
  collapsible: true,
  hideSender: false,
  showReply: true,
  showFolder: true,
})

const emit = defineEmits<{
  changed: []
  deleted: []
  reply: [{ account_id: number; to: string; subject: string; body: string }]
}>()

const message = ref<MessageItem>({ ...props.message })
const expanded = ref(props.initialExpanded)
const loadingDetail = ref(false)
const detail = ref<MessageDetailResponse | null>(null)
const mailboxes = ref<MailboxItem[]>([])
const showRemoteImages = ref(false)
const showMoveDialog = ref(false)
const targetFolder = ref('')
const statusMessage = ref('')
const statusError = ref(false)

const safeBody = computed(() => {
  const textBody = detail.value?.body?.text_body?.trim()
  if (textBody) {
    return textBody
  }
  const htmlBody = extractMailText(detail.value?.body?.html_body ?? '')
  if (htmlBody) {
    return htmlBody
  }
  return message.value.snippet || ''
})
const sanitizedHtml = computed(() => {
  const html = extractMailHtml(detail.value?.body?.html_body ?? '')
  if (!html) {
    return ''
  }
  const sanitized = DOMPurify.sanitize(html, {
    ALLOWED_TAGS: ['a', 'abbr', 'b', 'blockquote', 'br', 'code', 'div', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr', ...(showRemoteImages.value ? ['img'] : []), 'li', 'ol', 'p', 'pre', 'span', 'strong', 'table', 'tbody', 'td', 'th', 'thead', 'tr', 'u', 'ul'],
    ALLOWED_ATTR: ['alt', 'class', 'colspan', 'href', 'rowspan', ...(showRemoteImages.value ? ['src'] : []), 'style', 'target', 'title'],
    ALLOW_DATA_ATTR: false,
    FORBID_TAGS: ['form', 'iframe', 'input', 'script', 'style'],
    FORBID_ATTR: ['onerror', 'onload', 'onclick', 'onmouseover'],
  })
  return hardenSanitizedHtml(sanitized)
})
const mailboxOptions = computed(() => [
  { label: '选择目标文件夹', value: '' },
  ...mailboxes.value.map((mailbox) => ({ label: mailbox.name, value: mailbox.path })),
])

// ensureDetail 只在展开时请求正文和附件，避免大列表首屏加载过重。
async function ensureDetail() {
  if (detail.value || loadingDetail.value) {
    return
  }
  loadingDetail.value = true
  statusMessage.value = ''
  try {
    detail.value = await request<MessageDetailResponse>(`/api/messages/${props.message.id}`)
    message.value = { ...detail.value.message }
    mailboxes.value = await request<MailboxItem[]>(`/api/mailboxes?account_id=${message.value.account_id}`)
    statusError.value = false
  } catch (err) {
    detail.value = null
    mailboxes.value = []
    statusError.value = true
    statusMessage.value = err instanceof Error ? err.message : '加载邮件详情失败'
  } finally {
    loadingDetail.value = false
  }
}

function toggleExpanded() {
  if (!props.collapsible) {
    return
  }
  expanded.value = !expanded.value
}

async function toggleRead() {
  statusMessage.value = ''
  try {
    await request(`/api/messages/${message.value.id}/${message.value.is_read ? 'unread' : 'read'}`, { method: 'POST' })
    message.value.is_read = !message.value.is_read
    statusError.value = false
    statusMessage.value = message.value.is_read ? '已标记为已读' : '已标记为未读'
    emit('changed')
  } catch (err) {
    statusError.value = true
    statusMessage.value = err instanceof Error ? err.message : '操作失败'
  }
}

async function deleteMessage() {
  statusMessage.value = ''
  try {
    await request(`/api/messages/${message.value.id}/delete`, { method: 'POST' })
    emit('deleted')
  } catch (err) {
    statusError.value = true
    statusMessage.value = err instanceof Error ? err.message : '删除失败'
  }
}

function openMoveDialog() {
  showMoveDialog.value = true
  targetFolder.value = ''
}

async function moveMessage() {
  if (!targetFolder.value) {
    statusError.value = true
    statusMessage.value = '请先选择目标文件夹'
    return
  }
  try {
    await request(`/api/messages/${message.value.id}/move`, {
      method: 'POST',
      body: JSON.stringify({ folder: targetFolder.value }),
    })
    message.value.folder = targetFolder.value
    showMoveDialog.value = false
    statusError.value = false
    statusMessage.value = '移动成功'
    emit('changed')
  } catch (err) {
    statusError.value = true
    statusMessage.value = err instanceof Error ? err.message : '移动失败'
  }
}

// emitReply 使用当前邮件生成一份基础回复模板，减少用户重复补全主题和引用内容。
function emitReply() {
  emit('reply', {
    account_id: message.value.account_id,
    to: resolveReplyAddress(message.value),
    subject: message.value.subject.startsWith('Re:') ? message.value.subject : `Re: ${message.value.subject || '(无主题)'}`,
    body: `\n\n--- 原始邮件 ---\n发件人：${formatSender(message.value)}\nTo: ${formatAccountEmail(message.value.account_email).replace(/^To:\s*/, '')}\n时间：${formatDate(message.value.sent_at)}\n\n${safeBody.value}`,
  })
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

async function downloadAttachment(id: number, fileName: string) {
  const response = await fetch(`/api/attachments/${id}/download`, { credentials: 'include' })
  if (!response.ok) {
    statusError.value = true
    statusMessage.value = '下载附件失败'
    return
  }
  const blob = await response.blob()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName
  link.click()
  URL.revokeObjectURL(url)
}

// formatSender 把发件人名称和邮箱合并展示，减少列表里信息分散。
function formatSender(item: MessageItem) {
  const name = item.from_name?.trim()
  const address = item.from_address?.trim()
  if (name && address && name !== address) {
    return `${name} <${address}>`
  }
  return address || name || '未知发件人'
}

// formatAccountEmail 统一输出当前接入账户邮箱，避免误用原始收件人列表。
function formatAccountEmail(value: string) {
  const address = value.trim()
  if (!address) {
    return 'To: 未知'
  }
  return `To: ${address}`
}

function formatDate(value: string) {
  return value ? new Date(value).toLocaleString('zh-CN') : '刚刚'
}

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

watch(expanded, (value) => {
  if (value) {
    void ensureDetail()
  }
}, { immediate: true })

watch(
  () => props.message,
  (value) => {
    message.value = { ...value }
  },
  { deep: true },
)

watch(
  () => props.initialExpanded,
  (value) => {
    expanded.value = value
  },
)

onMounted(() => {
  if (props.initialExpanded) {
    void ensureDetail()
  }
})
</script>

<style scoped>
.message-card + .message-card {
  margin-top: 12px;
}

.message-meta {
  max-width: min(280px, 40vw);
  text-align: right;
  word-break: break-all;
}
</style>
