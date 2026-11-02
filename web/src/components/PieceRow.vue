<script setup lang="ts">
import type { Piece } from '@/api'
import { drynessWords } from '@/dryness'
import { endedText, estimateText, pieceTitle } from '@/format'
import { live, photoUrl } from '@/live'

defineProps<{ piece: Piece }>()
</script>

<template>
  <div class="piece-row">
    <RouterLink :to="`/pieces/${piece.id}`" class="link">
      <img v-if="piece.photo" :src="photoUrl(piece.id)" alt="" loading="lazy" class="thumb" />
      <span v-else class="thumb num">#{{ piece.number }}</span>
      <span class="text">
        <strong>{{ pieceTitle(piece) }}</strong>
        <span v-if="piece.state === 'flipped'" class="tag small">flipped</span>
        <span class="small">{{ drynessWords(piece.dryness) }} <span class="muted num">{{ piece.dryness.toFixed(1) }}</span></span>
        <span v-if="piece.estimate" class="small" :class="'est-' + piece.estimate.status">{{ estimateText(piece.estimate, live.clock) }}</span>
        <span v-else class="small muted">{{ endedText(piece) }}</span>
      </span>
    </RouterLink>
  </div>
</template>

<style scoped>
.piece-row {
  display: flex;
  gap: var(--sp-2);
  align-items: center;
  border-bottom: 1px solid var(--surface-3);
}
.link {
  flex: 1;
  min-width: 0;
  display: flex;
  gap: var(--sp-3);
  align-items: center;
  padding: var(--sp-2) 0;
  color: inherit;
  text-decoration: none;
}
.thumb {
  width: 56px;
  height: 56px;
  flex: none;
  border-radius: var(--radius-sm);
  object-fit: cover;
  background: var(--surface-2);
  display: grid;
  place-items: center;
  font-weight: 700;
}
.text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.tag {
  align-self: flex-start;
  padding: 0 var(--sp-2);
  border-radius: 999px;
  background: var(--accent-soft);
}
.est-ready {
  color: var(--ok);
  font-weight: 700;
}
.est-past {
  color: var(--warn);
  font-weight: 700;
}
</style>
