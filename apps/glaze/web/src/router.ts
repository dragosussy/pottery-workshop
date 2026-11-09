import { createRouter, createWebHistory } from 'vue-router'
import FiringView from './views/FiringView.vue'
import GlazesView from './views/GlazesView.vue'
import KilnHistoryView from './views/KilnHistoryView.vue'
import KilnView from './views/KilnView.vue'
import NowView from './views/NowView.vue'
import PieceFormView from './views/PieceFormView.vue'
import PieceView from './views/PieceView.vue'
import PiecesView from './views/PiecesView.vue'
import ResultsView from './views/ResultsView.vue'
import SettingsView from './views/SettingsView.vue'

// `tab` is the tab a screen belongs to. Forms and the result walk have none,
// so they have the whole screen and a thumb can't leave them by accident.
export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', component: NowView, meta: { tab: 'now' } },
    { path: '/settings', component: SettingsView, meta: { tab: 'now' } },
    { path: '/kiln', component: KilnView, meta: { tab: 'kiln' } },
    { path: '/kiln/past', component: KilnHistoryView, meta: { tab: 'kiln' } },
    { path: '/firings/:id(\\d+)', component: FiringView, meta: { tab: 'kiln' } },
    { path: '/firings/:id(\\d+)/results', component: ResultsView },
    { path: '/pieces', component: PiecesView, meta: { tab: 'pieces' } },
    { path: '/pieces/:id(\\d+)', component: PieceView, meta: { tab: 'pieces' } },
    { path: '/pieces/:id(\\d+)/edit', component: PieceFormView },
    { path: '/new', component: PieceFormView },
    { path: '/glazes', component: GlazesView, meta: { tab: 'glazes' } },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
  scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
})
