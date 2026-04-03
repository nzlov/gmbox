<template>
  <div class="page-shell">
    <aside class="sidebar">
      <div>
        <div class="brand-pill">G</div>
        <h2>gmbox</h2>
      </div>
      <nav class="nav-links">
        <RouterLink to="/inbox">聚合信息</RouterLink>
        <RouterLink to="/compose">写信</RouterLink>
        <RouterLink to="/accounts">邮箱管理</RouterLink>
      </nav>
      <button class="ghost-btn sidebar-logout" @click="logout">退出登录</button>
    </aside>

    <main class="content-shell">
      <header class="topbar">
        <div>
          <p class="eyebrow">SMTP 发信</p>
          <h1>写信</h1>
        </div>
      </header>

      <section class="panel account-layout">
        <form class="form-grid" @submit.prevent="submit">
          <select v-model.number="form.account_id">
            <option :value="0">选择发件邮箱</option>
            <option v-for="item in accounts" :key="item.id" :value="item.id">{{ item.name }} / {{ item.email }}</option>
          </select>
          <input v-model="form.to" placeholder="收件人，多个用逗号分隔" />
          <input v-model="form.cc" placeholder="抄送，可选" />
          <input v-model="form.bcc" placeholder="密送，可选" />
          <input v-model="form.subject" placeholder="主题" />
          <label class="switch-row"><span>HTML 正文</span><input v-model="form.is_html" type="checkbox" /></label>
          <textarea v-model="form.body" class="compose-area" placeholder="输入邮件正文"></textarea>
          <button class="primary-btn">发送邮件</button>
        </form>

        <div class="panel compose-tips">
          <h3>使用说明</h3>
          <p>当前版本已打通 SMTP 发信接口，发信账户来自已保存的邮箱配置。</p>
          <p>如果服务端要求授权码，请在邮箱管理页保存授权码而不是登录密码。</p>
          <p v-if="message" :class="messageClass">{{ message }}</p>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount } from '@/api'

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

const messageClass = computed(() => (isError.value ? 'error-text' : 'success-text'))

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
