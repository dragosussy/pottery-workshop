<script setup lang="ts">
import { ref } from 'vue'
import { photoUrl } from '@/api/endpoints'
import type { FormKey } from '@/api/types'

withDefaults(defineProps<{ photoId: number | null; form?: FormKey; size?: number | 'full'; alt?: string }>(), {
  form: 'other',
  size: 56,
  alt: '',
})
const failed = ref(false)
const ICONS: Record<FormKey, string> = { bowl: '◡', mug: '☕', plate: '◯', vase: '⚱', other: '◇' }
</script>

<template>
  <div class="photo" :class="{ full: size === 'full' }" :style="size === 'full' ? undefined : { width: size + 'px', height: size + 'px' }">
    <img
      v-if="photoId !== null && !failed"
      :src="photoUrl(photoId, size === 'full' ? 'full' : 'thumb')"
      :alt="alt"
      loading="lazy"
      decoding="async"
      @error="failed = true"
    />
    <span v-else class="placeholder" aria-hidden="true">{{ ICONS[form] }}</span>
  </div>
</template>

<style scoped>
.photo {
  flex: none;
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: var(--surface-2);
  display: grid;
  place-items: center;
}
.photo.full {
  width: 100%;
  aspect-ratio: 4 / 3;
  border-radius: var(--radius);
}
img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.placeholder {
  font-size: 1.5rem;
  color: var(--text-faint);
}
.full .placeholder {
  font-size: 3rem;
}
</style>
