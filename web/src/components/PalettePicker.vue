<script setup lang="ts">
// Lets the owner try the candidate colour palettes on the real app.
// Temporary: once a palette sticks, this section can go.
import { ref } from 'vue'
import { PALETTES, applyPalette, currentPalette, type PaletteKey } from '@/theme'

const active = ref<PaletteKey>(currentPalette())

function choose(key: PaletteKey) {
  active.value = key
  applyPalette(key)
}
</script>

<template>
  <div class="palettes" role="radiogroup" aria-label="Colour palette">
    <button
      v-for="p in PALETTES"
      :key="p.key"
      type="button"
      role="radio"
      class="palette"
      :data-palette="p.key"
      :aria-checked="active === p.key"
      @click="choose(p.key)"
    >
      <span class="swatches" aria-hidden="true">
        <span class="sw" style="background: var(--accent)" />
        <span class="sw" style="background: var(--dry-0)" />
        <span class="sw" style="background: var(--dry-2)" />
        <span class="sw" style="background: var(--dry-4)" />
      </span>
      <span class="name">{{ p.label }}</span>
      <span class="hint">{{ p.hint }}</span>
    </button>
  </div>
</template>

<style scoped>
.palettes {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--sp-2);
}
/* Each button renders in its own palette (data-palette scopes the tokens),
   so the swatches show that palette whatever is active. */
.palette {
  display: grid;
  gap: var(--sp-1);
  justify-items: start;
  min-height: var(--touch);
  padding: var(--sp-3);
  border: 2px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text);
  text-align: left;
  cursor: pointer;
}
.palette[aria-checked='true'] {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-soft);
}
.swatches {
  display: flex;
  gap: 4px;
}
.sw {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 1px solid rgba(0, 0, 0, 0.15);
}
.name {
  font-weight: 600;
}
.hint {
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
</style>
