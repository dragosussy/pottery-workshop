import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { resetSheetPush } from './composables/useSheet'

export const routes: RouteRecordRaw[] = [
  { path: '/', name: 'now', component: () => import('./views/NowView.vue') },
  { path: '/shelf', name: 'shelf', component: () => import('./views/ShelfView.vue') },
  { path: '/pieces/:id(\\d+)', name: 'piece', component: () => import('./views/PieceView.vue') },
  { path: '/check', name: 'check', component: () => import('./views/CheckView.vue'), meta: { focus: true } },
  { path: '/history', name: 'history', component: () => import('./views/HistoryView.vue') },
  { path: '/climate', name: 'climate', component: () => import('./views/ClimateView.vue') },
  { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
  { path: '/dev/slider', name: 'dev-slider', component: () => import('./views/DevSliderView.vue'), meta: { bare: true } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from, saved) {
    // Opening or closing a sheet must not scroll the page underneath.
    if (to.path === from.path) return false
    return saved ?? { top: 0 }
  },
})

router.afterEach((to) => {
  if (!to.query.sheet) resetSheetPush()
})
