<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { useSettingsStore } from '@/stores/settings'
import { serverNow } from '@/composables/useServerClock'
import { thisMorning, type WhenChoice } from '@/utils/when'
import { toLocalInputValue } from '@/utils/time'

const props = withDefaults(defineProps<{ label?: string }>(), { label: 'When' })
const model = defineModel<WhenChoice>({ required: true })
const settings = useSettingsStore()
const id = useId()

type Key = 'now' | 'ago1' | 'ago3' | 'morning' | 'pick'
const key = computed<Key>(() => {
  const m = model.value
  if (m.kind === 'ago') return m.hours === 1 ? 'ago1' : 'ago3'
  return m.kind
})
const morningAvailable = computed(() => thisMorning(serverNow(), settings.tz) !== null)
const pickValue = ref(toLocalInputValue(serverNow(), settings.tz))
const pickMax = computed(() => toLocalInputValue(serverNow(), settings.tz))
watch(pickValue, (v) => {
  if (key.value === 'pick') model.value = { kind: 'pick', local: v }
})

const options = computed(() => [
  { key: 'now' as const, label: 'Now' },
  { key: 'ago1' as const, label: '1h ago' },
  { key: 'ago3' as const, label: '3h ago' },
  { key: 'morning' as const, label: 'This morning', disabled: !morningAvailable.value },
  { key: 'pick' as const, label: 'Pick…' },
])

function choose(k: Key) {
  if (k === 'now') model.value = { kind: 'now' }
  else if (k === 'ago1') model.value = { kind: 'ago', hours: 1 }
  else if (k === 'ago3') model.value = { kind: 'ago', hours: 3 }
  else if (k === 'morning') model.value = { kind: 'morning' }
  else model.value = { kind: 'pick', local: pickValue.value }
}
</script>

<template>
  <div class="when">
    <div :id="id" class="field-label">{{ props.label }}</div>
    <div class="chips" role="radiogroup" :aria-labelledby="id">
      <button
        v-for="o in options"
        :key="o.key"
        type="button"
        role="radio"
        class="chip"
        :class="{ 'is-on': key === o.key }"
        :aria-checked="key === o.key"
        :disabled="o.disabled"
        @click="choose(o.key)"
      >
        <span v-if="key === o.key" aria-hidden="true">✓</span>{{ o.label }}
      </button>
    </div>
    <label v-if="key === 'pick'" class="pick">
      <span class="sr-only">Date and time ({{ settings.tz }})</span>
      <input v-model="pickValue" class="input" type="datetime-local" :max="pickMax" />
      <span class="faint tz">{{ settings.tz }}</span>
    </label>
  </div>
</template>

<style scoped>
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
}
.chip {
  min-height: var(--touch);
  padding: 0 var(--sp-3);
  border-radius: 999px;
  border: 2px solid var(--border);
  background: var(--surface);
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.chip.is-on {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-text);
}
.chip:disabled {
  opacity: 0.45;
}
.pick {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-top: var(--sp-2);
}
.tz {
  font-size: var(--fs-xs);
}
</style>
