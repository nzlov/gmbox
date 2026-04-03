<template>
  <q-layout view="hHh lpR fFf">
    <q-page-container>
      <q-page class="row items-center justify-center q-pa-md">
        <q-card bordered class="full-width" style="max-width: 420px">
          <q-card-section>
            <div class="text-overline text-primary">登录管理台</div>
            <div class="text-h5 text-weight-bold q-mt-sm">欢迎回来</div>
            <div class="text-body2 text-grey-7 q-mt-sm">
              输入管理员账号后进入控制台，继续处理邮箱同步、写信与账户维护。
            </div>

            <q-form class="column q-gutter-md q-mt-lg" @submit.prevent="submit">
              <q-input v-model="form.username" outlined dense label="用户名" autocomplete="username" />
              <q-input v-model="form.password" outlined dense type="password" label="密码" autocomplete="current-password" />
              <q-btn color="primary" unelevated no-caps type="submit" :loading="loading" label="登录" />
            </q-form>

            <q-banner v-if="error" rounded dense class="bg-red-1 text-negative q-mt-md">
              {{ error }}
            </q-banner>

            <q-separator class="q-my-lg" />

            <div class="text-body2 text-grey-7">
              首次启动会导入默认管理员，之后统一以数据库中的管理员密码校验。
            </div>
          </q-card-section>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
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
