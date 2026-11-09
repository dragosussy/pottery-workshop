<script setup lang="ts">
// The four places in the app, always in the same spot.
import { watch } from 'vue'
import { useRoute } from 'vue-router'
import { kilnFiring, refreshKilnStatus } from '@/kilnStatus'

defineProps<{ active: string }>()
const route = useRoute()
watch(() => route.path, refreshKilnStatus, { immediate: true })

const tabs = [
  { key: 'now', to: '/', label: 'Now', icon: '◉' },
  { key: 'kiln', to: '/kiln', label: 'Kiln', icon: '▦' },
  { key: 'pieces', to: '/pieces', label: 'Pieces', icon: '❏' },
  { key: 'glazes', to: '/glazes', label: 'Glazes', icon: '◍' },
]
</script>

<template>
  <nav class="tabs" aria-label="Main">
    <RouterLink v-for="t in tabs" :key="t.key" :to="t.to" class="tab" :class="{ active: t.key === active }" :aria-current="t.key === active ? 'page' : undefined">
      <span class="icon" :class="{ hot: t.key === 'kiln' && kilnFiring }" aria-hidden="true">{{ t.icon }}</span>
      <span>{{ t.key === 'kiln' && kilnFiring ? 'Firing' : t.label }}</span>
    </RouterLink>
  </nav>
</template>

<style scoped>
.tabs {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 5;
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  padding-bottom: env(safe-area-inset-bottom);
  background: var(--surface);
  border-top: 1px solid var(--border);
}
.tab {
  height: var(--tab-bar);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: var(--fs-sm);
  font-weight: 600;
  text-decoration: none;
  touch-action: manipulation;
}
.tab.active {
  color: var(--accent);
}
.icon {
  font-size: var(--fs-lg);
  line-height: 1.2;
}
.icon.hot {
  color: var(--fire);
  animation: glow 1.6s ease-in-out infinite;
}
@keyframes glow {
  50% {
    opacity: 0.45;
  }
}
@media (prefers-reduced-motion: reduce) {
  .icon.hot {
    animation: none;
  }
}
</style>
