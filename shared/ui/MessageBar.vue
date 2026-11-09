<script setup lang="ts">
// The message along the bottom of the screen, with an optional Undo.
defineProps<{ message: { text: string; error: boolean; undo?: unknown } | null }>()
defineEmits<{ dismiss: []; undo: [] }>()
</script>

<template>
  <div v-if="message" class="message" :class="{ error: message.error }" role="status" @click="$emit('dismiss')">
    <span>{{ message.text }}</span>
    <button v-if="message.undo" type="button" class="undo" @click.stop="$emit('undo')">Undo</button>
  </div>
</template>

<style scoped>
.message {
  position: fixed;
  left: var(--sp-4);
  right: var(--sp-4);
  /* --message-offset: an app with a bottom bar keeps messages above it. */
  bottom: calc(var(--sp-4) + env(safe-area-inset-bottom) + var(--message-offset, 0px));
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
