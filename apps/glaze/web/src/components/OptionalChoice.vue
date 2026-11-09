<script setup lang="ts" generic="K extends string">
// A row of chips where one can be chosen or, tapped again, none: much of what
// gets recorded here isn't always known.
defineProps<{ options: Partial<Record<K, string>>; label: string }>()
const model = defineModel<K | null>({ required: true })
</script>

<template>
  <div class="field-group">
    <span class="section-title">{{ label }}</span>
    <div class="chips" role="group" :aria-label="label">
      <button v-for="(text, key) in options" :key="key" type="button" :aria-pressed="model === key" @click="model = model === key ? null : key">
        {{ text }}
      </button>
    </div>
  </div>
</template>
