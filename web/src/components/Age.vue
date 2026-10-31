<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@/composables/useServerClock'
import { formatAge } from '@/utils/duration'
import { parseTs } from '@/utils/time'

const props = defineProps<{ since: string | null; until?: string | null }>()
const now = useNow()
const text = computed(() => {
  const a = parseTs(props.since)
  if (a === null) return '—'
  const b = parseTs(props.until ?? null) ?? now.value
  return formatAge((b - a) / 1000)
})
</script>

<template>
  <span class="num">{{ text }}</span>
</template>
