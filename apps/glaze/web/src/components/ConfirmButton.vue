<script setup lang="ts">
// A destructive button: the first tap arms it, the second does it.
import { onBeforeUnmount, ref } from 'vue'

defineProps<{ label: string; confirm?: string }>()
const emit = defineEmits<{ confirmed: [] }>()
const armed = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

function tap() {
  if (armed.value) {
    armed.value = false
    clearTimeout(timer)
    emit('confirmed')
    return
  }
  armed.value = true
  timer = setTimeout(() => (armed.value = false), 4000)
}
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <button type="button" class="btn" :class="{ 'btn-danger': armed }" @click="tap">{{ armed ? (confirm ?? 'Tap again to confirm') : label }}</button>
</template>
