<template>
  <AppShell
    active="compose"
    eyebrow="SMTP 发信"
    title="写信"
    subtitle="复用已保存的邮箱配置直接发信，把常用字段、抄送和 HTML 正文开关集中到同一编辑面板。"
    @logout="logout"
  >
    <div class="row q-col-gutter-md">
      <div class="col-12 col-xl-8">
        <q-card bordered>
          <q-card-section>
            <div class="text-subtitle1 text-weight-bold">邮件内容</div>
            <div class="text-body2 text-grey-7 q-mt-xs">发件账户来自邮箱管理页，地址输入支持英文逗号分隔。</div>
          </q-card-section>

          <q-card-section class="q-pt-none">
            <q-form class="column q-gutter-md" @submit.prevent="submit">
              <q-select v-model="form.account_id" outlined dense emit-value map-options :options="accountOptions" label="发件邮箱" />
              <q-input v-model="form.to" outlined dense label="收件人" hint="多个地址用英文逗号分隔" />
              <div class="row q-col-gutter-md">
                <div class="col-12 col-md-6">
                  <q-input v-model="form.cc" outlined dense label="抄送" />
                </div>
                <div class="col-12 col-md-6">
                  <q-input v-model="form.bcc" outlined dense label="密送" />
                </div>
              </div>
              <q-input v-model="form.subject" outlined dense label="主题" />
              <q-toggle v-model="form.is_html" color="primary" label="HTML 正文" />
              <q-input v-model="form.body" outlined autogrow type="textarea" label="正文" />
              <div class="row q-gutter-sm justify-end">
                <q-btn color="primary" unelevated no-caps icon="send" label="发送邮件" type="submit" />
              </div>
            </q-form>
          </q-card-section>
        </q-card>
      </div>

      <div class="col-12 col-xl-4">
        <q-card bordered>
          <q-card-section>
            <div class="text-subtitle1 text-weight-bold">发送说明</div>
            <div class="text-body2 text-grey-7 q-mt-xs">减少发信时遗漏配置的概率。</div>
          </q-card-section>
          <q-list bordered separator>
            <q-item>
              <q-item-section avatar>
                <q-icon name="settings" color="primary" />
              </q-item-section>
              <q-item-section>
                <q-item-label>发件账户来自已保存邮箱</q-item-label>
                <q-item-label caption>无需重复录入 SMTP 主机与端口，直接复用账户配置。</q-item-label>
              </q-item-section>
            </q-item>
            <q-item>
              <q-item-section avatar>
                <q-icon name="verified_user" color="secondary" />
              </q-item-section>
              <q-item-section>
                <q-item-label>优先使用授权码</q-item-label>
                <q-item-label caption>如果服务商要求授权码，应在邮箱管理页保存授权码而不是登录密码。</q-item-label>
              </q-item-section>
            </q-item>
            <q-item>
              <q-item-section avatar>
                <q-icon name="code" color="accent" />
              </q-item-section>
              <q-item-section>
                <q-item-label>HTML 开关独立控制</q-item-label>
                <q-item-label caption>仅在确实需要富文本结构时开启，降低无关样式干扰。</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>

          <q-card-section v-if="message">
            <q-banner rounded :class="isError ? 'bg-red-1 text-negative' : 'bg-green-1 text-positive'">
              {{ message }}
            </q-banner>
          </q-card-section>
        </q-card>
      </div>
    </div>
  </AppShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount } from '@/api'
import AppShell from '@/components/AppShell.vue'

const router = useRouter()
const accounts = ref<MailAccount[]>([])
const message = ref('')
const isError = ref(false)
const form = reactive({
  account_id: 0,
  to: '',
  cc: '',
  bcc: '',
  subject: '',
  body: '',
  is_html: false,
})

const accountOptions = computed(() => [
  { label: '选择发件邮箱', value: 0 },
  ...accounts.value.map((item) => ({ label: `${item.name} / ${item.email}`, value: item.id })),
])

// loadAccounts 让写信页直接复用已有邮箱配置作为发件账户。
async function loadAccounts() {
  try {
    accounts.value = await request<MailAccount[]>('/api/accounts')
    if (!form.account_id && accounts.value.length > 0) {
      form.account_id = accounts.value[0].id
    }
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '加载发件邮箱失败'
  }
}

// splitAddresses 统一处理逗号分隔的地址输入，避免后端收到空元素。
function splitAddresses(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

// submit 提交发信请求，并把错误直接反馈给用户。
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
      }),
    })
    message.value = '发送成功'
    Object.assign(form, { to: '', cc: '', bcc: '', subject: '', body: '', is_html: false })
  } catch (err) {
    isError.value = true
    message.value = err instanceof Error ? err.message : '发送失败'
  }
}

// logout 通过后端清理登录态，确保跳回登录页后状态一致。
async function logout() {
  await request('/api/auth/logout', { method: 'POST' })
  await router.push('/login')
}

onMounted(loadAccounts)
</script>
