import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import './styles/tokens.css'
import './styles/base.css'
import { initPalette } from './theme'

initPalette()

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
