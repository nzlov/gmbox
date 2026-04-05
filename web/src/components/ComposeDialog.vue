<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="emit('update:modelValue', $event)">
    <q-card class="full-width" style="max-width: 920px">
      <q-card-section class="row items-start justify-between q-col-gutter-md">
        <div class="col">
          <div class="text-h6 text-weight-bold">{{ dialogTitle }}</div>
          <div class="text-body2 text-grey-7 q-mt-xs">使用已接入邮箱直接发信，支持新写邮件、回复和转发。</div>
        </div>
        <div class="col-auto">
          <q-btn flat round dense icon="close" @click="closeDialog" />
        </div>
      </q-card-section>

      <q-separator />

      <q-card-section>
        <q-banner v-if="message" rounded :class="isError ? 'bg-red-1 text-negative q-mb-md' : 'bg-green-1 text-positive q-mb-md'">
          {{ message }}
        </q-banner>

        <q-banner v-if="presetNotice" rounded class="bg-blue-1 text-primary q-mb-md">
          {{ presetNotice }}
        </q-banner>

        <q-form class="row q-col-gutter-md" @submit.prevent="submit">
          <q-select v-model="form.account_id" class="col-12 col-lg-6" outlined emit-value map-options :options="accountOptions" label="发件邮箱" />
          <q-toggle v-model="form.is_html" class="col-12 col-lg-6" color="primary" label="HTML 正文" />

          <q-input v-model="form.to" class="col-12" outlined label="收件人" hint="多个地址用英文逗号分隔" />
          <q-input v-model="form.cc" class="col-12 col-md-6" outlined label="抄送" />
          <q-input v-model="form.bcc" class="col-12 col-md-6" outlined label="密送" />
          <q-input v-model="form.subject" class="col-12" outlined label="主题" />

          <div v-if="forwardedAttachments.length > 0" class="col-12">
            <div class="text-subtitle2 text-weight-medium q-mb-sm">将随邮件一并转发以下附件</div>
            <q-list bordered separator>
              <q-item v-for="attachment in forwardedAttachments" :key="attachment.id">
                <q-item-section avatar>
                  <q-icon name="attach_file" color="primary" />
                </q-item-section>
                <q-item-section>
                  <q-item-label>{{ attachment.file_name }}</q-item-label>
                  <q-item-label caption>{{ attachment.content_type || '未知类型' }}</q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-item-label caption>{{ formatSize(attachment.size) }}</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </div>

          <q-input v-model="form.body" class="col-12" outlined autogrow type="textarea" label="正文" input-style="min-height: 260px" />

          <div class="col-12 row justify-end q-gutter-sm">
            <q-btn flat no-caps label="取消" @click="closeDialog" />
            <q-btn color="primary" unelevated no-caps type="submit" label="发送邮件" />
          </div>
        </q-form>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { request, type AttachmentItem, type MailAccount } from '@/api'

type ComposePreset = {
  title?: string
  account_id?: number
  to?: string
  cc?: string
  bcc?: string
  subject?: string
  body?: string
  is_html?: boolean
  notice?: string
  attachment_ids?: number[]
  attachments?: AttachmentItem[]
}

const props = defineProps<{
  modelValue: boolean
  preset?: ComposePreset | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  sent: []
}>()

const accounts = ref<MailAccount[]>([])
const message = ref('')
const isError = ref(false)
const loaded = ref(false)
const form = reactive({
  account_id: 0,
  to: '',
  cc: '',
  bcc: '',
  subject: '',
  body: '',
  is_html: false,
  attachment_ids: [] as number[],
})

const dialogTitle = computed(() => props.preset?.title?.trim() || '写信')
const presetNotice = computed(() => props.preset?.notice?.trim() || '')
const forwardedAttachments = computed(() => props.preset?.attachments ?? [])
const accountOptions = computed(() => [
  { label: '选择发件邮箱', value: 0 },
  ...accounts.value.map((item) => ({ label: `${item.name} / ${item.email}`, value: item.id })),
])

// loadAccounts 保证弹窗在任意页面打开时都能拿到最新发件邮箱列表。
async function loadAccounts() {
  accounts.value = await request<MailAccount[]>('/api/accounts')
  loaded.value = true
}

// applyPreset 把外部场景传入的收件人、主题和正文映射到弹窗表单，减少重复输入。
function applyPreset() {
  Object.assign(form, {
    account_id: props.preset?.account_id ?? accounts.value[0]?.id ?? 0,
    to: props.preset?.to ?? '',
    cc: props.preset?.cc ?? '',
    bcc: props.preset?.bcc ?? '',
    subject: props.preset?.subject ?? '',
    body: props.preset?.body ?? '',
    is_html: props.preset?.is_html ?? false,
    attachment_ids: [...(props.preset?.attachment_ids ?? [])],
  })
}

function splitAddresses(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

async function submit() {
  message.value = ''
  isError.value = false
  if (!form.account_id) {
    isError.value = true
    message.value = '请先选择发件邮箱'
    return
  }
  try {
    await request('/api/messages/send', {
      method: 'POST',
      body: JSON.stringify({
        account_id: form.account_id,
        to: splitAddresses(form.to),
        cc: splitAddresses(form.cc),
        bcc: splitAddresses(form.bcc),
        subject: form.subject,
        body: form.body,
        is_html: form.is_html,
        attachment_ids: form.attachment_ids,
      }),
    })
    message.value = '发送成功'
    emit('sent')
    closeDialog()
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '发送失败'
  }
}

function closeDialog() {
  emit('update:modelValue', false)
}

// formatSize 输出更易读的附件体积，方便用户确认转发内容是否正确。
function formatSize(size: number) {
  if (size < 1024) {
    return `${size} B`
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

watch(
  () => props.modelValue,
  async (value) => {
    if (!value) {
      return
    }
    message.value = ''
    isError.value = false
    try {
      if (!loaded.value) {
        await loadAccounts()
      }
      applyPreset()
    } catch (err) {
      isError.value = true
      message.value = err instanceof Error ? err.message : '加载发件邮箱失败'
    }
  },
  { immediate: true },
)

watch(
  () => props.preset,
  () => {
    if (props.modelValue && loaded.value) {
      applyPreset()
    }
  },
  { deep: true },
)
</script>
