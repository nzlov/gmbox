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
          <p class="eyebrow">多邮箱管理</p>
          <h1>邮箱账户</h1>
        </div>
        <button class="primary-btn" @click="openCreateModal">添加邮箱</button>
      </header>

      <section class="panel table-panel">
        <div class="panel-head panel-tools">
          <div>
            <h3>邮箱列表</h3>
            <span class="muted">支持多选执行启用、禁用、同步、删除和测试。</span>
          </div>
          <div class="toolbar-actions wrap-actions">
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchUpdateEnabled(true)">启用</button>
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchUpdateEnabled(false)">禁用</button>
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchSync">同步</button>
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchTest">测试</button>
            <button class="ghost-btn danger-btn" :disabled="!hasSelection" @click="batchDelete">删除</button>
          </div>
        </div>

        <p v-if="message" :class="messageClass">{{ message }}</p>
        <p v-if="error" class="error-text">{{ error }}</p>

        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th><input :checked="allSelected" type="checkbox" @change="toggleAll($event)" /></th>
                <th>名称</th>
                <th>邮箱</th>
                <th>协议</th>
                <th>收信配置</th>
                <th>SMTP</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="accounts.length === 0">
                <td colspan="8" class="empty-cell">暂无邮箱，请先添加。</td>
              </tr>
              <tr v-for="item in accounts" :key="item.id">
                <td>
                  <input v-model="selectedIDs" type="checkbox" :value="item.id" />
                </td>
                <td>{{ item.name }}</td>
                <td>{{ item.email }}</td>
                <td>{{ item.incoming_protocol.toUpperCase() }}</td>
                <td>
                  {{ incomingHost(item) }}:{{ incomingPort(item) }}
                </td>
                <td>{{ item.smtp_host }}:{{ item.smtp_port }}</td>
                <td>
                  <span class="status-badge" :class="item.enabled ? 'status-enabled' : 'status-disabled'">
                    {{ item.enabled ? '已启用' : '已禁用' }}
                  </span>
                </td>
                <td>
                  <div class="toolbar-actions wrap-actions">
                    <button class="ghost-btn" @click="openEditModal(item)">编辑</button>
                    <button class="ghost-btn" @click="test(item.id)">测试</button>
                    <button class="ghost-btn" @click="sync(item.id)">同步</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div v-if="showModal" class="modal-mask" @click.self="closeModal">
        <section class="modal-card">
          <div class="panel-head">
            <div>
              <h3>{{ editingID ? '修改邮箱' : '添加邮箱' }}</h3>
              <span class="muted">协议字段互斥显示，避免提交无关配置。</span>
            </div>
            <button class="ghost-btn" @click="closeModal">关闭</button>
          </div>

          <form class="form-grid" @submit.prevent="submit">
            <input v-model="form.name" placeholder="展示名称" />
            <input v-model="form.email" placeholder="邮箱地址" />
            <input v-model="form.username" placeholder="登录用户名" />
            <input v-model="form.password" type="password" :placeholder="editingID ? '留空则保持原密码' : '密码或授权码'" />
            <select v-model="form.incoming_protocol">
              <option value="imap">IMAP</option>
              <option value="pop3">POP3</option>
            </select>
            <label class="switch-row"><span>启用 TLS</span><input v-model="form.use_tls" type="checkbox" /></label>

            <template v-if="form.incoming_protocol === 'imap'">
              <input v-model="form.imap_host" placeholder="IMAP Host" />
              <input v-model.number="form.imap_port" type="number" placeholder="IMAP Port" />
            </template>
            <template v-else>
              <input v-model="form.pop3_host" placeholder="POP3 Host" />
              <input v-model.number="form.pop3_port" type="number" placeholder="POP3 Port" />
            </template>

            <input v-model="form.smtp_host" placeholder="SMTP Host" />
            <input v-model.number="form.smtp_port" type="number" placeholder="SMTP Port" />
            <label class="switch-row"><span>启用账户</span><input v-model="form.enabled" type="checkbox" /></label>
            <button class="primary-btn">{{ editingID ? '保存修改' : '保存邮箱' }}</button>
          </form>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount } from '@/api'

const router = useRouter()
const accounts = ref<MailAccount[]>([])
const error = ref('')
const message = ref('')
const showModal = ref(false)
const editingID = ref<number | null>(null)
const selectedIDs = ref<number[]>([])
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

const messageClass = computed(() => 'success-text')
const hasSelection = computed(() => selectedIDs.value.length > 0)
const allSelected = computed(() => accounts.value.length > 0 && selectedIDs.value.length === accounts.value.length)

// resetForm 统一回到默认表单，避免新增和编辑状态互相污染。
function resetForm() {
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
}

// loadAccounts 刷新当前邮箱列表，保持页面与数据库状态一致。
async function loadAccounts() {
  error.value = ''
  try {
    accounts.value = await request<MailAccount[]>('/api/accounts')
    selectedIDs.value = selectedIDs.value.filter((id) => accounts.value.some((item) => item.id === id))
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载邮箱失败'
  }
}

// submit 统一承接新增和编辑提交，减少两套表单分叉后的维护成本。
async function submit() {
  error.value = ''
  message.value = ''
  try {
    const payload = {
      ...form,
      imap_host: form.incoming_protocol === 'imap' ? form.imap_host : '',
      imap_port: form.incoming_protocol === 'imap' ? form.imap_port : 0,
      pop3_host: form.incoming_protocol === 'pop3' ? form.pop3_host : '',
      pop3_port: form.incoming_protocol === 'pop3' ? form.pop3_port : 0,
    }
    if (editingID.value) {
      await request<MailAccount>(`/api/accounts/${editingID.value}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      message.value = '邮箱修改成功'
    } else {
      await request<MailAccount>('/api/accounts', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      message.value = '邮箱添加成功'
    }
    closeModal()
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
    message.value = '连接测试成功'
    error.value = ''
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
    message.value = '同步完成'
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : '同步失败'
  }
}

// openCreateModal 为新增场景重置表单并打开弹窗，避免继承上次编辑数据。
function openCreateModal() {
  editingID.value = null
  resetForm()
  error.value = ''
  message.value = ''
  showModal.value = true
}

// openEditModal 把现有邮箱映射到表单，保证批量操作外仍可单条精确修改。
function openEditModal(account: MailAccount) {
  editingID.value = account.id
  Object.assign(form, {
    name: account.name,
    email: account.email,
    username: account.username,
    password: '',
    incoming_protocol: account.incoming_protocol,
    imap_host: account.imap_host,
    imap_port: account.imap_port,
    pop3_host: account.pop3_host,
    pop3_port: account.pop3_port,
    smtp_host: account.smtp_host,
    smtp_port: account.smtp_port,
    use_tls: account.use_tls,
    enabled: account.enabled,
  })
  error.value = ''
  message.value = ''
  showModal.value = true
}

// closeModal 关闭弹窗时同步清理编辑态，避免后续提交误走更新接口。
function closeModal() {
  showModal.value = false
  editingID.value = null
  error.value = ''
  resetForm()
}

// toggleAll 让表格多选支持一键选择当前页全部邮箱。
function toggleAll(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  selectedIDs.value = checked ? accounts.value.map((item) => item.id) : []
}

// incomingHost 统一根据协议显示当前实际收信服务器，避免表格重复判断分支。
function incomingHost(account: MailAccount) {
  return account.incoming_protocol === 'imap' ? account.imap_host : account.pop3_host
}

// incomingPort 统一根据协议显示当前实际收信端口，减少模板里的条件噪音。
function incomingPort(account: MailAccount) {
  return account.incoming_protocol === 'imap' ? account.imap_port : account.pop3_port
}

// buildAccountPayload 基于现有数据回放更新请求，以便批量启停复用单条更新接口。
function buildAccountPayload(account: MailAccount, enabled: boolean) {
  return {
    name: account.name,
    email: account.email,
    username: account.username,
    password: '',
    incoming_protocol: account.incoming_protocol,
    imap_host: account.imap_host,
    imap_port: account.imap_port,
    pop3_host: account.pop3_host,
    pop3_port: account.pop3_port,
    smtp_host: account.smtp_host,
    smtp_port: account.smtp_port,
    use_tls: account.use_tls,
    enabled,
  }
}

// batchUpdateEnabled 复用既有更新接口批量切换启用状态，避免额外扩展后端协议。
async function batchUpdateEnabled(enabled: boolean) {
  message.value = ''
  error.value = ''
  try {
    const selectedAccounts = accounts.value.filter((item) => selectedIDs.value.includes(item.id))
    await Promise.all(
      selectedAccounts.map((item) =>
        request(`/api/accounts/${item.id}`, {
          method: 'PUT',
          body: JSON.stringify(buildAccountPayload(item, enabled)),
        }),
      ),
    )
    message.value = enabled ? '批量启用成功' : '批量禁用成功'
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量更新失败'
  }
}

// batchSync 对选中的邮箱逐一发起同步，保持现有后端接口不变。
async function batchSync() {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(selectedIDs.value.map((id) => request(`/api/accounts/${id}/sync`, { method: 'POST' })))
    message.value = '批量同步完成'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量同步失败'
  }
}

// batchTest 逐一验证所选邮箱连接，避免单条点击重复操作过多。
async function batchTest() {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(selectedIDs.value.map((id) => request(`/api/accounts/${id}/test`, { method: 'POST' })))
    message.value = '批量测试成功'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量测试失败'
  }
}

// batchDelete 复用删除接口处理多选删除，并在完成后刷新列表与勾选状态。
async function batchDelete() {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(selectedIDs.value.map((id) => request(`/api/accounts/${id}`, { method: 'DELETE' })))
    selectedIDs.value = []
    message.value = '批量删除成功'
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量删除失败'
  }
}

// logout 统一清理登录态，保持所有业务页退出行为一致。
async function logout() {
  await request('/api/auth/logout', { method: 'POST' })
  await router.push('/login')
}

onMounted(loadAccounts)
</script>
