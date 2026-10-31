<script setup lang="ts">
// Room humidity in large type, temperature, a 6 h sparkline and a freshness
// indicator (dot + words). Tapping opens /climate.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '@/api/endpoints'
import { useLiveStore } from '@/stores/live'
import { useNow } from '@/composables/useServerClock'
import { formatAge } from '@/utils/duration'
import { parseTs } from '@/utils/time'
import Sparkline from './Sparkline.vue'

const live = useLiveStore()
const now = useNow()
const spark = ref<(number | null)[]>([])
let timer: ReturnType<typeof setInterval> | null = null

async function loadSpark() {
  try {
    const r = await api.readings({ from: '-6h' })
    spark.value = r.series[0]?.humidity_avg ?? []
  } catch {
    // keep the last sparkline
  }
}

onMounted(() => {
  void loadSpark()
  timer = setInterval(loadSpark, 5 * 60_000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})

const room = computed(() => live.status?.room ?? null)
const state = computed(() => (live.offline ? 'offline' : live.roomState))
const age = computed(() => {
  const t = parseTs(room.value?.read_at)
  return t === null ? null : formatAge((now.value - t) / 1000)
})
const freshness = computed(() => {
  switch (state.value) {
    case 'ok':
      return { dot: 'ok', text: 'live' }
    case 'stale':
      return { dot: 'warn', text: age.value ? `stale · last reading ${age.value} ago` : 'stale' }
    case 'no_data':
      return { dot: 'error', text: 'no sensor data' }
    default:
      return { dot: 'off', text: age.value ? `offline · ${age.value} ago` : 'offline' }
  }
})
const okSensors = computed(() => (live.status?.sensors ?? []).filter((s) => s.state === 'ok').length)
const allSensors = computed(() => live.status?.sensors.length ?? 0)
</script>

<template>
  <RouterLink to="/climate" class="card climate" :class="{ 'is-stale': state !== 'ok' }" data-testid="climate-card">
    <div class="climate-top">
      <div>
        <div class="climate-label">Shelf humidity</div>
        <div class="climate-rh num" data-testid="humidity">
          <template v-if="room && room.humidity_pct !== null">{{ room.humidity_pct.toFixed(1) }}<span class="unit">%</span></template>
          <template v-else>—</template>
        </div>
      </div>
      <div class="climate-side">
        <div class="climate-temp num">{{ room && room.temperature_c !== null ? room.temperature_c.toFixed(1) + '°C' : '—' }}</div>
        <div class="fresh" :class="`dot-${freshness.dot}`" data-testid="freshness">
          <span class="dot" aria-hidden="true"></span>{{ freshness.text }}
        </div>
      </div>
    </div>
    <Sparkline v-if="spark.length" :values="spark" />
    <div v-if="state === 'stale' || state === 'no_data'" class="climate-warn">
      ⚠ {{ okSensors }} of {{ allSensors }} sensors reporting. Check the sensor cable.
    </div>
  </RouterLink>
</template>

<style scoped>
.climate {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  color: inherit;
  text-decoration: none;
}
.climate-top {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: var(--sp-3);
}
.climate-label {
  font-size: var(--fs-sm);
  color: var(--text-muted);
  font-weight: 600;
}
.climate-rh {
  font-size: var(--fs-3xl);
  font-weight: 800;
  line-height: 1;
}
.unit {
  font-size: var(--fs-xl);
  font-weight: 700;
  margin-left: 2px;
}
.climate-side {
  text-align: right;
}
.climate-temp {
  font-size: var(--fs-xl);
  font-weight: 700;
}
.fresh {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--text-faint);
}
.dot-ok .dot {
  background: var(--ok);
}
.dot-warn .dot {
  background: var(--warn);
}
.dot-error .dot {
  background: var(--danger);
}
.is-stale .climate-rh {
  color: var(--text-faint);
}
.climate-warn {
  font-size: var(--fs-sm);
  color: var(--warn-strong);
  font-weight: 600;
}
</style>
