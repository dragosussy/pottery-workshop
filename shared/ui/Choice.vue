<script setup lang="ts" generic="K extends string">
// A row of chips, one of which is chosen.
defineProps<{ options: Record<K, string>; label: string }>()
const model = defineModel<K>({ required: true })
</script>

<template>
  <div class="field">
    <span class="field-name">{{ label }}</span>
    <div class="chips" role="group" :aria-label="label">
      <button v-for="(text, key) in options" :key="key" type="button" :aria-pressed="model === key" @click="model = key">
        {{ text }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.field-name {
  display: block;
  font-weight: 600;
  margin-bottom: var(--sp-1);
}
</style>
