import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './styles.css'

// createApp 负责挂载前端入口并注入路由。
createApp(App).use(router).mount('#app')
