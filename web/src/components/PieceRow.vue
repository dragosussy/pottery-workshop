<script setup lang="ts">
// A shelf/history row: thumbnail, #number, form + size/thickness, age, dryness
// label, ETA window, flipped badge. In select mode it's a checkbox.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { Piece } from '@/api/types'
import { useReferenceStore } from '@/stores/reference'
import { pieceSpecs } from '@/utils/format'
import PiecePhoto from './PiecePhoto.vue'
import DrynessChip from './DrynessChip.vue'
import EtaWindow from './EtaWindow.vue'
import Age from './Age.vue'

const props = withDefaults(defineProps<{ piece: Piece; selectable?: boolean; selected?: boolean; reasons?: string[] }>(), {
  selectable: false,
  selected: false,
  reasons: () => [],
})
const emit = defineEmits<{ toggle: [id: number] }>()
const reference = useReferenceStore()
const specs = computed(() => pieceSpecs(props.piece, reference.data))
const reasonText = computed(() => props.reasons.map((r) => reference.label('check_reasons', r)).join(' · '))
</script>

<template>
  <component
    :is="selectable ? 'button' : RouterLink"
    :to="selectable ? undefined : `/pieces/${piece.id}`"
    :type="selectable ? 'button' : undefined"
    class="prow"
    :class="{ 'is-selected': selected }"
    :aria-pressed="selectable ? selected : undefined"
    data-testid="piece-row"
    @click="selectable && emit('toggle', piece.id)"
  >
    <span v-if="selectable" class="box" aria-hidden="true">{{ selected ? '✓' : '' }}</span>
    <PiecePhoto :photo-id="piece.photo_id" :form="piece.form" :size="56" />
    <span class="body">
      <span class="top">
        <strong class="num">#{{ piece.number }}</strong>
        <span class="specs">{{ specs }}</span>
        <span v-if="piece.state === 'flipped'" class="badge">flipped</span>
      </span>
      <span class="mid"><DrynessChip :value="piece.current_dryness" /></span>
      <span class="bottom muted">
        <Age :since="piece.added_at" /> on shelf<template v-if="piece.prediction"> · <EtaWindow :prediction="piece.prediction" /></template>
      </span>
      <span v-if="reasonText" class="reason">{{ reasonText }}</span>
    </span>
  </component>
</template>

<style scoped>
.prow {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  width: 100%;
  min-height: 72px;
  padding: var(--sp-2) var(--sp-3);
  border: 0;
  border-radius: var(--radius);
  background: var(--surface);
  color: inherit;
  text-align: left;
  text-decoration: none;
  font: inherit;
  cursor: pointer;
}
.prow.is-selected {
  outline: 3px solid var(--accent);
}
.box {
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: 2px solid var(--border);
  display: grid;
  place-items: center;
  font-weight: 800;
}
.is-selected .box {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-text);
}
.body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.top {
  display: flex;
  align-items: baseline;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.top .num {
  font-size: var(--fs-lg);
}
.specs {
  color: var(--text-muted);
  font-size: var(--fs-sm);
}
.badge {
  font-size: var(--fs-xs);
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: var(--surface-2);
}
.bottom {
  font-size: var(--fs-sm);
}
.reason {
  font-size: var(--fs-sm);
  color: var(--warn-strong);
  font-weight: 600;
}
</style>
