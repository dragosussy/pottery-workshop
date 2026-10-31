<script setup lang="ts">
// A banner with an icon and words (never colour alone).
withDefaults(defineProps<{ tone?: 'info' | 'warn' | 'error' | 'ok'; icon?: string }>(), { tone: 'info', icon: '' })
</script>

<template>
  <div class="banner" :class="`tone-${tone}`" role="status">
    <span v-if="icon" class="banner-icon" aria-hidden="true">{{ icon }}</span>
    <div class="banner-body">
      <slot />
      <div v-if="$slots.actions" class="banner-actions">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.banner {
  display: flex;
  gap: var(--sp-3);
  padding: var(--sp-3) var(--sp-4);
  border-radius: var(--radius);
  border: 2px solid var(--border);
  background: var(--surface);
}
.tone-info {
  background: var(--info-soft);
  border-color: transparent;
}
.tone-warn {
  background: var(--warn-soft);
  border-color: var(--warn-border);
  color: var(--text);
}
.tone-error {
  background: var(--danger-soft);
  border-color: var(--danger);
}
.tone-ok {
  background: var(--ok-soft);
  border-color: transparent;
}
.banner-icon {
  font-size: var(--fs-lg);
  line-height: 1.2;
}
.banner-body {
  flex: 1;
  min-width: 0;
}
.banner-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
  margin-top: var(--sp-3);
}
</style>
