<script setup lang="ts">
// One kiln shelf, drawn from above in its real proportions (about 5 wide by 3
// deep) with the front of the kiln at the bottom. Pieces sit wherever they
// were tapped. With `pieces` it's the kiln's map; with only `marked` it shows
// where one piece sat, and a side view of the kiln's height beside it.
import { useTemplateRef } from 'vue'
import { thumbUrl, type Level, type Piece } from '@/api'
import { LOADING_ORDER, pieceTitle } from '@/format'

defineProps<{
  label?: string
  pieces?: Piece[]
  selected?: number | null
  /** A tap puts the selected piece here, even right on top of another. */
  placing?: boolean
  firing?: boolean
  marked?: { x: number; y: number; level: Level | null }
}>()
const emit = defineEmits<{ place: [x: number, y: number]; piece: [p: Piece] }>()
const area = useTemplateRef('area')

function tap(e: MouseEvent) {
  const box = area.value!.getBoundingClientRect()
  const within = (v: number) => Math.min(1, Math.max(0, v))
  emit('place', within((e.clientX - box.left) / box.width), within(1 - (e.clientY - box.top) / box.height))
}

// Kept inside the shelf, so a piece against the wall is still fully tappable.
const at = (p: { x: number | null; y: number | null }) => ({
  left: `clamp(22px, ${(p.x ?? 0) * 100}%, calc(100% - 22px))`,
  bottom: `clamp(22px, ${(p.y ?? 0) * 100}%, calc(100% - 22px))`,
})
const topDown = [...LOADING_ORDER].reverse() // the side view is a picture of the kiln
const photoOf = (p: Piece) => p.photos.at(-1)
const shortName = (p: Piece) => (p.kind === 'tile' ? String(p.number) : 'Pot')
</script>

<template>
  <div class="shelf" :class="{ mini: !pieces, firing }">
    <strong v-if="label">{{ label }}</strong>
    <div class="kiln">
      <div ref="area" class="area" :class="{ placing }" @click="tap">
        <button
          v-for="p in pieces" :key="p.id" type="button" class="marker" :class="{ chosen: p.id === selected }" :style="at(p)"
          :aria-label="pieceTitle(p)" @click.stop="emit('piece', p)"
        >
          <img v-if="photoOf(p)" :src="thumbUrl(photoOf(p)!.id)" alt="" />
          <span class="name num">{{ shortName(p) }}</span>
        </button>
        <div v-if="marked" class="dot" :style="at(marked)"></div>
      </div>
      <div v-if="marked" class="height" role="img" :aria-label="`${marked.level ?? 'Unknown'} of the kiln's height`">
        <div v-for="l in topDown" :key="l" class="level" :class="{ here: marked.level === l }"></div>
      </div>
    </div>
    <span class="front small muted">Front of the kiln</span>
  </div>
</template>

<style scoped>
.shelf {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.mini {
  max-width: 240px;
}
.kiln {
  display: flex;
  align-items: stretch;
  gap: var(--sp-2);
}
/* The faint lines are only a guide to judge distances by. */
.area {
  position: relative;
  flex: 1;
  aspect-ratio: 5 / 3;
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
  background-color: var(--surface);
  background-image:
    linear-gradient(to right, var(--surface-2) 1px, transparent 1px),
    linear-gradient(to bottom, var(--surface-2) 1px, transparent 1px);
  background-size: 20% 100%, 100% 33.34%;
  touch-action: manipulation;
}
.area.placing {
  border-color: var(--accent);
  background-color: var(--accent-soft);
  cursor: crosshair;
}
.firing .area {
  border-color: var(--fire);
  box-shadow: inset 0 0 36px var(--fire-glow);
}
.marker {
  position: absolute;
  width: 44px;
  height: 44px;
  padding: 0;
  overflow: hidden;
  transform: translate(-50%, 50%);
  border: 2px solid var(--surface);
  border-radius: 8px;
  background: var(--surface-3);
  box-shadow: var(--shadow);
  color: var(--text);
  font: inherit;
  cursor: pointer;
}
/* While placing, every tap belongs to the shelf: pieces sit side by side. */
.placing .marker {
  pointer-events: none;
}
.marker.chosen {
  outline: 3px solid var(--accent);
}
.marker img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.name {
  position: absolute;
  left: 1px;
  bottom: 1px;
  padding: 0 3px;
  border-radius: 3px;
  background: var(--surface);
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1.3;
}
.dot {
  position: absolute;
  width: 18px;
  height: 18px;
  transform: translate(-50%, 50%);
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.height {
  flex: none;
  width: 30px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 3px;
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
}
.level {
  flex: 1;
  border-radius: 3px;
  background: var(--surface);
}
.level.here {
  background: var(--accent);
}
.front {
  text-align: center;
}
</style>
