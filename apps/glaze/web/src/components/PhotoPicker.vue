<script setup lang="ts">
// Takes or chooses a photo. The server shrinks it and turns it upright.
import { onBeforeUnmount, ref, watch } from 'vue'

defineProps<{ label: string }>()
const model = defineModel<Blob | null>({ required: true })
const preview = ref('')

function pick(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (file) model.value = file
}
watch(model, (photo) => {
  URL.revokeObjectURL(preview.value)
  preview.value = photo ? URL.createObjectURL(photo) : ''
})
onBeforeUnmount(() => URL.revokeObjectURL(preview.value))
</script>

<template>
  <label class="photo">
    <img v-if="preview" :src="preview" alt="The chosen photo" />
    <span v-else class="btn btn-block">{{ label }}</span>
    <input type="file" accept="image/*" class="hidden-input" @change="pick" />
  </label>
</template>

<style scoped>
.photo {
  cursor: pointer;
}
.photo img {
  border-radius: var(--radius-sm);
  max-height: 240px;
  margin: 0 auto;
}
.hidden-input {
  position: absolute;
  opacity: 0;
  width: 1px;
  height: 1px;
}
</style>
