<script setup lang="ts">
// Camera tile: thumbnail, progress, ✕ to remove, Retry on failure.
import { ref } from 'vue'
import type { usePhotoUpload } from '@/composables/usePhotoUpload'

const props = defineProps<{ photo: ReturnType<typeof usePhotoUpload> }>()
const input = ref<HTMLInputElement | null>(null)

function onChange(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) props.photo.pick(f)
  ;(e.target as HTMLInputElement).value = ''
}
</script>

<template>
  <div class="picker" data-testid="photo-picker">
    <input ref="input" class="sr-only" type="file" accept="image/*" capture="environment" tabindex="-1" aria-hidden="true" @change="onChange" />
    <button
      v-if="photo.state.value === 'empty'"
      type="button"
      class="tile tile-empty"
      @click="input?.click()"
    >
      <span class="cam" aria-hidden="true">📷</span>
      <span>Add a photo <span class="faint">(optional)</span></span>
    </button>
    <div v-else class="tile tile-filled">
      <img v-if="photo.previewUrl.value" :src="photo.previewUrl.value" alt="Photo preview" class="preview" />
      <div class="status" role="status">
        <template v-if="photo.state.value === 'resizing'">Preparing…</template>
        <template v-else-if="photo.state.value === 'uploading'">
          Uploading {{ Math.round(photo.progress.value * 100) }}%
          <span class="bar"><span :style="{ width: photo.progress.value * 100 + '%' }"></span></span>
        </template>
        <template v-else-if="photo.state.value === 'done'">✓ Photo ready</template>
        <template v-else>
          <span class="err">⚠ {{ photo.error.value }}</span>
          <button type="button" class="btn" @click="photo.retry()">Retry</button>
        </template>
      </div>
      <div class="tile-actions">
        <button type="button" class="btn btn-ghost" @click="input?.click()">Retake</button>
        <button type="button" class="btn btn-ghost" aria-label="Remove photo" @click="photo.clear()">✕</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tile {
  width: 100%;
  border-radius: var(--radius);
  border: 2px dashed var(--border);
  background: var(--surface);
  min-height: 72px;
}
.tile-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-3);
  font-weight: 600;
  cursor: pointer;
}
.cam {
  font-size: 1.6rem;
}
.tile-filled {
  display: grid;
  grid-template-columns: 72px 1fr auto;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-2);
  border-style: solid;
}
.preview {
  width: 72px;
  height: 72px;
  object-fit: cover;
  border-radius: var(--radius-sm);
}
.status {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-weight: 600;
}
.bar {
  height: 6px;
  border-radius: 3px;
  background: var(--surface-3);
  overflow: hidden;
}
.bar span {
  display: block;
  height: 100%;
  background: var(--accent);
}
.err {
  color: var(--danger);
}
.tile-actions {
  display: flex;
  flex-direction: column;
}
</style>
