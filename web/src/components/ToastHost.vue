<script setup lang="ts">
import { dismissToast, useToasts } from '@/composables/useToast'

const state = useToasts()

async function run(id: number, fn: () => void | Promise<void>) {
  dismissToast(id)
  await fn()
}
</script>

<template>
  <div class="toasts" aria-live="polite" aria-atomic="false">
    <div v-for="t in state.toasts" :key="t.id" class="toast" :class="`tone-${t.tone}`" role="status">
      <div class="toast-text">
        <div v-if="t.big" class="toast-big num">{{ t.big }}</div>
        <div>{{ t.message }}</div>
      </div>
      <button v-if="t.action" type="button" class="btn toast-action" @click="run(t.id, t.action.run)">{{ t.action.label }}</button>
      <button type="button" class="btn btn-ghost toast-close" aria-label="Dismiss" @click="dismissToast(t.id)">
        <span aria-hidden="true">✕</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  left: 0;
  right: 0;
  bottom: calc(var(--bottom-bar-h) + 92px + env(safe-area-inset-bottom));
  z-index: 60;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-2);
  padding: 0 var(--gutter);
  pointer-events: none;
}
.toast {
  pointer-events: auto;
  width: 100%;
  max-width: 560px;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: var(--sp-2) var(--sp-2) var(--sp-2) var(--sp-4);
  border-radius: var(--radius);
  background: var(--text);
  color: var(--bg);
  box-shadow: var(--shadow-lg);
  animation: pop 160ms ease-out;
}
.toast.tone-error {
  background: var(--danger);
  color: #fff;
}
.toast.tone-warn {
  background: var(--warn-soft);
  color: var(--text);
  border: 2px solid var(--warn-border);
}
.toast-text {
  flex: 1;
  min-width: 0;
}
.toast-big {
  font-size: var(--fs-2xl);
  font-weight: 800;
  line-height: 1.1;
}
.toast-action {
  background: transparent;
  color: inherit;
  border-color: currentColor;
}
.toast-close {
  color: inherit;
}
@keyframes pop {
  from {
    transform: translateY(12px);
    opacity: 0;
  }
}
</style>
