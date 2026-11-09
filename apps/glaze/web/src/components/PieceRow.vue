<script setup lang="ts">
// One piece as a compact line, where a grid of photos would take up the page.
import { thumbUrl, type Piece } from '@/api'
import { pieceTitle, stackText } from '@/format'

defineProps<{ piece: Piece }>()
</script>

<template>
  <RouterLink :to="`/pieces/${piece.id}`" class="row-link">
    <img v-if="piece.photos.at(-1)" :src="thumbUrl(piece.photos.at(-1)!.id)" alt="" loading="lazy" />
    <div v-else class="blank"></div>
    <span class="text">
      <strong>{{ pieceTitle(piece) }}</strong>
      <span class="small muted">{{ stackText(piece.layers) || 'Nothing on it yet' }}</span>
    </span>
  </RouterLink>
</template>

<style scoped>
.row-link {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-2);
  background: var(--surface);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow);
  color: inherit;
  text-decoration: none;
}
img,
.blank {
  width: 48px;
  height: 48px;
  flex: none;
  border-radius: 4px;
  object-fit: cover;
  background: var(--surface-2);
}
.text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
</style>
