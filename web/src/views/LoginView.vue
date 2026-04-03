<template>
  <q-layout view="hHh lpR fFf" class="login-layout">
    <q-page-container>
      <q-page class="row items-center justify-center q-pa-md">
        <div class="login-grid">
          <q-card flat class="login-hero-card text-white">
            <q-card-section class="q-pa-xl">
              <div class="text-overline text-blue-1">gmbox</div>
              <div class="text-h3 text-weight-bold q-mt-md">统一邮箱工作台</div>
              <div class="text-subtitle1 text-blue-1 q-mt-md">
                重新整理登录体验，用更清晰的入口承接多邮箱聚合、发信与账户管理。
              </div>

              <div class="row q-col-gutter-md q-mt-lg">
                <div class="col-12 col-sm-6">
                  <q-card flat class="login-metric-card">
                    <q-card-section>
                      <div class="text-caption text-blue-1">管理动作</div>
                      <div class="text-h5 text-weight-bold q-mt-sm">一站完成</div>
                      <div class="text-body2 text-blue-1 q-mt-sm">聚合收件箱、账号配置、SMTP 发信都在同一入口完成。</div>
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12 col-sm-6">
                  <q-card flat class="login-metric-card">
                    <q-card-section>
                      <div class="text-caption text-blue-1">安全边界</div>
                      <div class="text-h5 text-weight-bold q-mt-sm">服务端校验</div>
                      <div class="text-body2 text-blue-1 q-mt-sm">首次启动导入默认管理员，后续均以数据库中的密码为准。</div>
                    </q-card-section>
                  </q-card>
                </div>
              </div>
            </q-card-section>
          </q-card>

          <q-card flat class="login-form-card">
            <q-card-section class="q-pa-xl">
              <div class="text-overline text-primary">登录管理台</div>
              <div class="text-h4 text-weight-bold q-mt-sm">欢迎回来</div>
              <div class="text-body2 text-grey-7 q-mt-sm">
                输入管理员账号后进入控制台，继续处理邮箱同步、写信与账户维护。
              </div>

              <q-form class="column q-gutter-md q-mt-lg" @submit.prevent="submit">
                <q-input
                  v-model="form.username"
                  outlined
                  dense
                  label="用户名"
                  autocomplete="username"
                  prefix="@"
                />
                <q-input
                  v-model="form.password"
                  outlined
                  dense
                  type="password"
                  label="密码"
                  autocomplete="current-password"
                />
                <q-btn color="primary" unelevated no-caps type="submit" :loading="loading" label="登录" />
              </q-form>

              <q-banner v-if="error" rounded dense class="bg-red-1 text-negative q-mt-md">
                {{ error }}
              </q-banner>
            </q-card-section>
          </q-card>
        </div>
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

<style scoped>
.login-layout {
  background:
    radial-gradient(circle at top left, rgba(37, 99, 235, 0.42), transparent 28%),
    radial-gradient(circle at bottom right, rgba(124, 58, 237, 0.34), transparent 24%),
    linear-gradient(135deg, #0f172a 0%, #111c39 45%, #172554 100%);
}

.login-grid {
  width: min(1180px, 100%);
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(340px, 440px);
  gap: 24px;
}

.login-hero-card,
.login-form-card {
  border-radius: 28px;
  overflow: hidden;
}

.login-hero-card {
  background: linear-gradient(155deg, rgba(37, 99, 235, 0.88), rgba(124, 58, 237, 0.78));
  box-shadow: 0 28px 80px rgba(15, 23, 42, 0.32);
}

.login-form-card {
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 24px 70px rgba(15, 23, 42, 0.28);
}

.login-metric-card {
  height: 100%;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.14);
}

@media (max-width: 960px) {
  .login-grid {
    grid-template-columns: 1fr;
  }
}
</style>
