import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '@/views/LoginView.vue'
import InboxView from '@/views/InboxView.vue'
import AccountsView from '@/views/AccountsView.vue'
import ComposeView from '@/views/ComposeView.vue'

// router 统一维护页面切换和登录前置校验。
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginView },
    { path: '/', redirect: '/inbox' },
    { path: '/inbox', component: InboxView, meta: { auth: true } },
    { path: '/accounts', component: AccountsView, meta: { auth: true } },
    { path: '/compose', component: ComposeView, meta: { auth: true } },
  ],
})

// beforeEach 通过后端登录态接口判断当前页面是否允许进入。
router.beforeEach(async (to) => {
  if (!to.meta.auth) {
    return true
  }

  try {
    const response = await fetch('/api/auth/me', { credentials: 'include' })
    if (response.ok) {
      return true
    }
  } catch {
    return '/login'
  }

  return '/login'
})

export default router
