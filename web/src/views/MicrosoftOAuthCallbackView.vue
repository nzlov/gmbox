<template>
  <div class="oauth-callback column items-center justify-center q-pa-xl text-center">
    <div class="text-h6 text-weight-bold">微软 OAuth 处理中</div>
    <div class="text-body2 text-grey-7 gmbox-top-gap-sm">{{ status }}</div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { request, type MicrosoftOAuthExchangeResponse } from '@/api'
import { clearMicrosoftOAuthSession, loadMicrosoftOAuthSession } from '@/utils/oauth'

type OAuthMessage = {
  type: 'microsoft-oauth'
  success: boolean
  message: string
}

const route = useRoute()
const status = ref('正在校验授权结果，请稍候...')

// notifyOpener 把授权结果回传给父窗口，让列表页自己决定如何更新界面状态。
function notifyOpener(payload: OAuthMessage) {
  if (window.opener && window.location.origin) {
    window.opener.postMessage(payload, window.location.origin)
  }
}

// closeWindow 延迟关闭授权窗口，给用户保留一瞬间可见的反馈文字。
function closeWindow() {
  window.setTimeout(() => {
    window.close()
  }, 200)
}

onMounted(async () => {
  const state = typeof route.query.state === 'string' ? route.query.state : ''
  const code = typeof route.query.code === 'string' ? route.query.code : ''
  const queryError = typeof route.query.error_description === 'string'
    ? route.query.error_description
    : typeof route.query.error === 'string'
      ? route.query.error
      : ''

  if (!state) {
    status.value = '授权失败：缺少 state'
    notifyOpener({ type: 'microsoft-oauth', success: false, message: status.value })
    closeWindow()
    return
  }

  const session = loadMicrosoftOAuthSession(state)
  clearMicrosoftOAuthSession(state)
  if (!session) {
    status.value = '授权失败：本地授权会话不存在或已失效，请重试'
    notifyOpener({ type: 'microsoft-oauth', success: false, message: status.value })
    closeWindow()
    return
  }

  if (Date.now() - session.createdAt > 10 * 60 * 1000) {
    status.value = '授权失败：授权会话已过期，请重新发起授权'
    notifyOpener({ type: 'microsoft-oauth', success: false, message: status.value })
    closeWindow()
    return
  }

  if (queryError) {
    status.value = `授权失败：${queryError}`
    notifyOpener({ type: 'microsoft-oauth', success: false, message: status.value })
    closeWindow()
    return
  }

  if (!code) {
    status.value = '授权失败：微软未返回授权码'
    notifyOpener({ type: 'microsoft-oauth', success: false, message: status.value })
    closeWindow()
    return
  }

  try {
    const response = await request<MicrosoftOAuthExchangeResponse>('/api/accounts/oauth/microsoft/exchange', {
      method: 'POST',
      body: JSON.stringify({
        code,
        state,
        code_verifier: session.codeVerifier,
      }),
    })
    status.value = response.message
    notifyOpener({ type: 'microsoft-oauth', success: true, message: response.message })
  } catch (err) {
    status.value = err instanceof Error ? err.message : '微软 OAuth 换码失败'
    notifyOpener({ type: 'microsoft-oauth', success: false, message: status.value })
  }

  closeWindow()
})
</script>

<style scoped>
.oauth-callback {
  min-height: 100vh;
}
</style>
