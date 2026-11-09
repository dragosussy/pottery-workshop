<script setup lang="ts">
// One piece in a grid: its photo, what it is, and what went on it.
import { computed } from 'vue'
import { thumbUrl, type Piece } from '@/api'
import { RATINGS, pieceTitle, stackText } from '@/format'

const props = defineProps<{ piece: Piece; caption?: string }>()
// The fired result if there is one, since that's what people look for.
const photo = computed(() => [...props.piece.photos].reverse().find((p) => p.stage === 'after') ?? props.piece.photos.at(-1))
</script>

<template>
  <RouterLink :to="`/pieces/${piece.id}`" class="tile">
    <img v-if="photo" :src="thumbUrl(photo.id)" :alt="pieceTitle(piece)" loading="lazy" />
    <div v-else class="no-photo muted small">No photo</div>
    <div class="text">
      <div class="row title">
        <strong>{{ pieceTitle(piece) }}</strong>
        <span v-if="piece.rating" class="badge" :class="piece.rating">{{ RATINGS[piece.rating] }}</span>
      </div>
      <div v-if="piece.layers.length" class="small muted stack-line">{{ stackText(piece.layers) }}</div>
      <div v-if="caption" class="small muted">{{ caption }}</div>
    </div>
  </RouterLink>
</template>

<style scoped>
.tile {
  display: flex;
  flex-direction: column;
  background: var(--surface);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  overflow: hidden;
  color: inherit;
  text-decoration: none;
}
img {
  width: 100%;
  aspect-ratio: 1;
  object-fit: cover;
}
.no-photo {
  width: 100%;
  min-height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface-2);
}
.text {
  padding: var(--sp-2) var(--sp-3) var(--sp-3);
}
.title {
  justify-content: space-between;
}
.stack-line {
  overflow-wrap: anywhere;
}
</style>
