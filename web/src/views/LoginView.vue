<template>
  <div class="login-shell">
    <div class="login-card">
      <div>
        <p class="eyebrow">gmbox</p>
        <h1>登录管理台</h1>
        <p class="muted">首次启动会把配置中的默认管理员导入数据库，之后只校验数据库密码。</p>
      </div>

      <form class="form-grid" @submit.prevent="submit">
        <input v-model="form.username" placeholder="用户名" autocomplete="username" />
        <input
          v-model="form.password"
          type="password"
          placeholder="密码"
          autocomplete="current-password"
        />
        <button class="primary-btn" :disabled="loading">{{ loading ? '登录中...' : '登录' }}</button>
      </form>

      <p v-if="error" class="error-text">{{ error }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { request } from '@/api'

// form 保存登录页最小表单状态，避免页面逻辑分散。
const form = reactive({ username: '', password: '' })
const loading = ref(false)
const error = ref('')
const router = useRouter()

// submit 负责完成登录请求和页面跳转。
async function submit() {
  loading.value = true
  error.value = ''
  try {
    await request('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(form),
    })
    await router.push('/inbox')
  } catch (err) {
    error.value = err instanceof Error ? err.message : '登录失败'
  } finally {
    loading.value = false
  }
}
</script>
