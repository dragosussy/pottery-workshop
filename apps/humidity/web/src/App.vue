<script setup lang="ts">
import { onBeforeUnmount } from 'vue'
import { live, startPolling, undoLast } from './live'

const stop = startPolling()
onBeforeUnmount(stop)
</script>

<template>
  <RouterView />
  <div v-if="live.message" class="message" :class="{ error: live.message.error }" role="status" @click="live.message = null">
    <span>{{ live.message.text }}</span>
    <button v-if="live.message.undo" type="button" class="undo" @click.stop="undoLast">Undo</button>
  </div>
</template>

<style scoped>
.message {
  position: fixed;
  left: var(--sp-4);
  right: var(--sp-4);
  bottom: calc(var(--sp-4) + env(safe-area-inset-bottom));
  max-width: 600px;
  margin: 0 auto;
  padding: var(--sp-3) var(--sp-4);
  border-radius: var(--radius);
  background: var(--text);
  color: var(--bg);
  box-shadow: var(--shadow);
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
}
.undo {
  flex: none;
  min-height: 40px;
  padding: 0 var(--sp-4);
  border: 2px solid var(--bg);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--bg);
  font: inherit;
  font-weight: 700;
}
.message.error {
  background: var(--danger-soft);
  color: var(--danger);
  border: 2px solid var(--danger);
}
</style>
