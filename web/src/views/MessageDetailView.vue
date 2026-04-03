<template>
  <div class="page-shell">
    <aside class="sidebar">
      <div>
        <div class="brand-pill">G</div>
        <h2>gmbox</h2>
      </div>
      <nav class="nav-links">
        <RouterLink to="/inbox">聚合收件箱</RouterLink>
        <RouterLink to="/accounts">邮箱管理</RouterLink>
        <RouterLink to="/compose">写信</RouterLink>
      </nav>
    </aside>

    <main class="content-shell">
      <header class="topbar">
        <div>
          <p class="eyebrow">邮件详情</p>
          <h1>{{ currentMessage.subject || '(无主题)' }}</h1>
        </div>
        <button class="ghost-btn" @click="router.push('/inbox')">返回列表</button>
      </header>

      <section class="panel" v-if="detail">
        <div class="detail-meta">
          <div>
            <strong>{{ currentMessage.from_name || currentMessage.from_address || '未知发件人' }}</strong>
            <p>{{ currentMessage.from_address || '未知地址' }}</p>
          </div>
          <div class="mail-meta">
            <span>{{ currentMessage.folder || '未知文件夹' }}</span>
            <time>{{ formatDate(currentMessage.sent_at) }}</time>
          </div>
        </div>

        <div class="detail-actions">
          <button class="ghost-btn" @click="markRead(true)">标记已读</button>
          <button class="ghost-btn" @click="markRead(false)">标记未读</button>
          <button class="ghost-btn" @click="deleteMessage">删除</button>
          <button class="ghost-btn" @click="toggleImages">
            {{ showRemoteImages ? '隐藏图片' : '显示图片' }}
          </button>
        </div>

        <div class="move-row">
          <select v-model="targetFolder">
            <option value="">选择目标文件夹</option>
            <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.path">
              {{ mailbox.name }}
            </option>
          </select>
          <button class="primary-btn" @click="moveMessage">移动邮件</button>
        </div>

        <div v-if="message" :class="messageClass">{{ message }}</div>

        <article v-if="sanitizedHtml" class="detail-body detail-html" v-html="sanitizedHtml"></article>
        <article v-else class="detail-body">{{ safeBody }}</article>

        <section v-if="currentAttachments.length > 0" class="attachment-list">
          <h3>附件</h3>
          <button
            v-for="attachment in currentAttachments"
            :key="attachment.id"
            type="button"
            class="attachment-item"
            @click="downloadAttachment(attachment.id)"
          >
            <span>{{ attachment.file_name }}</span>
            <small>{{ formatSize(attachment.size) }}</small>
          </button>
        </section>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import DOMPurify from 'dompurify'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { request, type AttachmentItem, type MailboxItem, type MessageDetailResponse, type MessageItem } from '@/api'

const route = useRoute()
const router = useRouter()
const detail = ref<MessageDetailResponse | null>(null)
const mailboxes = ref<MailboxItem[]>([])
const targetFolder = ref('')
const message = ref('')
const isError = ref(false)
const showRemoteImages = ref(false)

const messageClass = computed(() => (isError.value ? 'error-text' : 'success-text'))
const currentMessage = computed<MessageItem>(() =>
  detail.value?.message ?? {
    id: 0,
    account_id: 0,
    mailbox_id: 0,
    folder: '',
    subject: '',
    from_name: '',
    from_address: '',
    snippet: '',
    is_read: false,
    is_deleted: false,
    has_attachment: false,
    sent_at: '',
  },
)
const currentAttachments = computed<AttachmentItem[]>(() => detail.value?.attachments ?? [])
const sanitizedHtml = computed(() => {
  const html = detail.value?.body?.html_body?.trim()
  if (!html) {
    return ''
  }
  const sanitized = DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      'a',
      'abbr',
      'b',
      'blockquote',
      'br',
      'code',
      'div',
      'em',
      'h1',
      'h2',
      'h3',
      'h4',
      'h5',
      'h6',
      'hr',
      ...(showRemoteImages.value ? ['img'] : []),
      'li',
      'ol',
      'p',
      'pre',
      'span',
      'strong',
      'table',
      'tbody',
      'td',
      'th',
      'thead',
      'tr',
      'u',
      'ul',
    ],
    ALLOWED_ATTR: [
      'alt',
      'class',
      'colspan',
      'href',
      'rowspan',
      ...(showRemoteImages.value ? ['src'] : []),
      'style',
      'target',
      'title',
    ],
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
  const htmlBody = detail.value.body?.html_body?.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  if (htmlBody) {
    return htmlBody
  }
  return detail.value.message.snippet || ''
})

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

// formatDate 统一处理时间显示。
function formatDate(value: string) {
  return value ? new Date(value).toLocaleString('zh-CN') : '刚刚'
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
    if (
      !lowerHref.startsWith('http://') &&
      !lowerHref.startsWith('https://') &&
      !lowerHref.startsWith('mailto:')
    ) {
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
