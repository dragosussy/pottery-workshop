<script setup lang="ts">
// A material in the Glazes list. It folds open in place to everything about it.
import type { Material } from '@/api'
import MaterialPanel from '@/components/MaterialPanel.vue'

defineProps<{ material: Material; open: boolean }>()
defineEmits<{ toggle: []; changed: []; deleted: [] }>()
</script>

<template>
  <article class="tile" :data-material="material.id">
    <button type="button" class="head" :aria-expanded="open" @click="$emit('toggle')">
      <span class="text">
        <strong>{{ material.name }}</strong>
        <span class="small muted">{{ material.uses }} use{{ material.uses === 1 ? '' : 's' }}<template v-if="material.versions[0]"> · recipe v{{ material.versions[0].version }}</template></span>
      </span>
      <span class="chevron" :class="{ open }" aria-hidden="true">›</span>
    </button>
    <div v-if="open" class="body">
      <MaterialPanel :id="material.id" @changed="$emit('changed')" @deleted="$emit('deleted')" />
    </div>
  </article>
</template>

<style scoped>
.tile {
  background: var(--surface);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--sp-3);
  width: 100%;
  min-height: 64px;
  padding: var(--sp-3) var(--sp-4);
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
  touch-action: manipulation;
}
.text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.chevron {
  flex: none;
  color: var(--text-muted);
  font-size: var(--fs-xl);
  line-height: 1;
  transition: transform 0.15s;
}
.chevron.open {
  transform: rotate(90deg);
}
.body {
  padding: 0 var(--sp-4) var(--sp-4);
}
</style>
