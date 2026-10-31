<script setup lang="ts" generic="K extends string">
// Single- or multi-select chips. Real buttons, 48px tall, state shown with a
// check mark and aria-pressed/aria-checked (never colour alone).
import { computed, useId } from 'vue'

const props = withDefaults(
  defineProps<{
    options: { key: K; label: string; disabled?: boolean }[]
    multiple?: boolean
    label: string
    /** Hide the visible group label (still read by screen readers). */
    hideLabel?: boolean
    size?: 'md' | 'lg'
  }>(),
  { multiple: false, hideLabel: false, size: 'md' },
)
const gid = useId()
const model = defineModel<K | K[] | null>({ required: true })

const selected = computed(() => new Set<K>(Array.isArray(model.value) ? model.value : model.value !== null ? [model.value] : []))

function toggle(key: K) {
  if (props.multiple) {
    const cur = Array.isArray(model.value) ? [...model.value] : []
    const i = cur.indexOf(key)
    if (i >= 0) cur.splice(i, 1)
    else cur.push(key)
    model.value = props.options.map((o) => o.key).filter((k) => cur.includes(k))
  } else {
    model.value = key
  }
}
</script>

<template>
  <div class="chips-wrap">
    <div :id="gid" class="chips-label" :class="{ 'sr-only': hideLabel }">{{ label }}</div>
    <div class="chips" :role="multiple ? 'group' : 'radiogroup'" :aria-labelledby="gid">
      <button
        v-for="o in options"
        :key="o.key"
        type="button"
        class="chip"
        :class="[{ 'is-on': selected.has(o.key) }, `size-${size}`]"
        :disabled="o.disabled"
        :role="multiple ? undefined : 'radio'"
        :aria-pressed="multiple ? selected.has(o.key) : undefined"
        :aria-checked="multiple ? undefined : selected.has(o.key)"
        @click="toggle(o.key)"
      >
        <span v-if="selected.has(o.key)" class="chip-check" aria-hidden="true">✓</span>{{ o.label }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.chips-label {
  font-weight: 600;
  margin-bottom: var(--sp-2);
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
}
.chip {
  min-height: var(--touch);
  min-width: var(--touch);
  padding: 0 var(--sp-4);
  border-radius: 999px;
  border: 2px solid var(--border);
  background: var(--surface);
  font-weight: 600;
  cursor: pointer;
  touch-action: manipulation;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.chip.size-lg {
  min-height: 56px;
  font-size: var(--fs-lg);
}
.chip.is-on {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-text);
}
.chip:disabled {
  opacity: 0.45;
}
.chip-check {
  font-weight: 800;
}
</style>
