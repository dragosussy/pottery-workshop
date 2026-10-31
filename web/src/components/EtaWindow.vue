<script setup lang="ts">
// "in 3–6h", "tonight 20:00–23:00", "Sat morning" in studio time, plus
// confidence and best-before (plan §5.2).
import { computed } from 'vue'
import type { PiecePrediction } from '@/api/types'
import { useNow } from '@/composables/useServerClock'
import { useSettingsStore } from '@/stores/settings'
import { confidenceWord, dayTime, etaText } from '@/utils/format'

const props = withDefaults(defineProps<{ prediction: PiecePrediction | null; detail?: boolean }>(), { detail: false })
const now = useNow()
const settings = useSettingsStore()

const main = computed(() => etaText(props.prediction, now.value, settings.tz))
const extra = computed(() => {
  const p = props.prediction
  if (!props.detail || !p || p.status === 'ended') return ''
  const parts: string[] = []
  if (p.status === 'drying' && p.ready_until) parts.push(`best before ${dayTime(p.ready_until, now.value, settings.tz)}`)
  const c = confidenceWord(p.confidence)
  if (c) parts.push(c)
  return parts.join(' · ')
})
</script>

<template>
  <span class="eta">
    <span>{{ main }}</span>
    <span v-if="extra" class="eta-extra"> · {{ extra }}</span>
  </span>
</template>

<style scoped>
.eta-extra {
  color: var(--text-muted);
}
</style>
