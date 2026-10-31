<script setup lang="ts">
import { computed } from 'vue'
import { useLiveStore } from '@/stores/live'
import { useSettingsStore } from '@/stores/settings'
import { useNow } from '@/composables/useServerClock'
import { formatDayTime } from '@/utils/format'
import Banner from './Banner.vue'

const live = useLiveStore()
const settings = useSettingsStore()
const now = useNow()

const asOf = computed(() => (live.asOf === null ? null : formatDayTime(live.asOf, now.value, settings.tz)))
</script>

<template>
  <Banner tone="warn" icon="⏻" data-testid="offline-banner">
    <strong>The drying controller is offline</strong> (probably switched off)
    <span v-if="asOf"> · showing data from {{ asOf }}</span>
    <template #actions>
      <button type="button" class="btn" @click="live.fetchStatus()">Try again</button>
    </template>
  </Banner>
</template>
