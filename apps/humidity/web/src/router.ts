import { createRouter, createWebHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import PieceView from './views/PieceView.vue'
import AddView from './views/AddView.vue'
import FinishedView from './views/FinishedView.vue'
import SettingsView from './views/SettingsView.vue'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', component: HomeView },
    { path: '/pieces/:id(\\d+)', component: PieceView },
    { path: '/add', component: AddView },
    { path: '/finished', component: FinishedView },
    { path: '/settings', component: SettingsView },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
  scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
})
