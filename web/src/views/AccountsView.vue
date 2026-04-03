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
          <p class="eyebrow">多邮箱管理</p>
          <h1>邮箱账户</h1>
        </div>
      </header>

      <section class="panel account-layout">
        <form class="form-grid" @submit.prevent="submit">
          <input v-model="form.name" placeholder="展示名称" />
          <input v-model="form.email" placeholder="邮箱地址" />
          <input v-model="form.username" placeholder="登录用户名" />
          <input v-model="form.password" type="password" placeholder="密码或授权码" />
          <select v-model="form.incoming_protocol">
            <option value="imap">IMAP</option>
            <option value="pop3">POP3</option>
          </select>
          <label class="switch-row"><span>启用 TLS</span><input v-model="form.use_tls" type="checkbox" /></label>
          <input v-model="form.imap_host" placeholder="IMAP Host" />
          <input v-model.number="form.imap_port" type="number" placeholder="IMAP Port" />
          <input v-model="form.pop3_host" placeholder="POP3 Host" />
          <input v-model.number="form.pop3_port" type="number" placeholder="POP3 Port" />
          <input v-model="form.smtp_host" placeholder="SMTP Host" />
          <input v-model.number="form.smtp_port" type="number" placeholder="SMTP Port" />
          <label class="switch-row"><span>启用账户</span><input v-model="form.enabled" type="checkbox" /></label>
          <button class="primary-btn">保存邮箱</button>
        </form>

        <div>
          <p v-if="error" class="error-text">{{ error }}</p>
          <div v-for="item in accounts" :key="item.id" class="account-card">
            <div>
              <strong>{{ item.name }}</strong>
              <p>{{ item.email }}</p>
              <small>{{ item.incoming_protocol.toUpperCase() }} / SMTP</small>
            </div>
            <div class="account-actions">
              <button class="ghost-btn" @click="test(item.id)">测试连接</button>
              <button class="ghost-btn" @click="sync(item.id)">立即同步</button>
            </div>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { request, type MailAccount } from '@/api'

const accounts = ref<MailAccount[]>([])
const error = ref('')
const form = reactive({
  name: '',
  email: '',
  username: '',
  password: '',
  incoming_protocol: 'imap',
  imap_host: '',
  imap_port: 993,
  pop3_host: '',
  pop3_port: 995,
  smtp_host: '',
  smtp_port: 465,
  use_tls: true,
  enabled: true,
})

// loadAccounts 刷新当前邮箱列表，保持页面与数据库状态一致。
async function loadAccounts() {
  error.value = ''
  try {
    accounts.value = await request<MailAccount[]>('/api/accounts')
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载邮箱失败'
  }
}

// submit 保存邮箱账户并触发列表刷新，避免用户误以为提交失败。
async function submit() {
  error.value = ''
  try {
    await request<MailAccount>('/api/accounts', {
      method: 'POST',
      body: JSON.stringify(form),
    })
    Object.assign(form, {
      name: '',
      email: '',
      username: '',
      password: '',
      incoming_protocol: 'imap',
      imap_host: '',
      imap_port: 993,
      pop3_host: '',
      pop3_port: 995,
      smtp_host: '',
      smtp_port: 465,
      use_tls: true,
      enabled: true,
    })
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '保存邮箱失败'
  }
}

// test 让用户在保存后即可验证远端服务是否可连通。
async function test(id: number) {
  if (!Number.isFinite(id) || id <= 0) {
    error.value = '邮箱 ID 无效，无法测试连接'
    return
  }
  try {
    await request(`/api/accounts/${id}/test`, { method: 'POST' })
    error.value = '连接测试成功'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '连接测试失败'
  }
}

// sync 允许用户手动触发一轮单邮箱同步，便于验证调度链路。
async function sync(id: number) {
  if (!Number.isFinite(id) || id <= 0) {
    error.value = '邮箱 ID 无效，无法执行同步'
    return
  }
  try {
    await request(`/api/accounts/${id}/sync`, { method: 'POST' })
    error.value = '同步完成'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '同步失败'
  }
}

onMounted(loadAccounts)
</script>
