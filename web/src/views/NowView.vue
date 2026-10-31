<script setup lang="ts">
// Now (home, plan §4.1): banners, climate card, trim card, shelf summary, actions.
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useLiveStore } from '@/stores/live'
import { useShelfStore } from '@/stores/shelf'
import { useSheet } from '@/composables/useSheet'
import { showToast } from '@/composables/useToast'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'
import ClimateCard from '@/components/ClimateCard.vue'
import TrimCard from '@/components/TrimCard.vue'
import TargetRow from '@/components/TargetRow.vue'
import ShelfSummary from '@/components/ShelfSummary.vue'
import Banner from '@/components/Banner.vue'
import { formatSpanWords } from '@/utils/duration'
import { parseTs } from '@/utils/time'

const live = useLiveStore()
const shelf = useShelfStore()
const sheet = useSheet()
const router = useRouter()
const status = computed(() => live.status)

const pumpTripped = computed(() => !!status.value?.control.safety?.tripped)
const resuming = ref(false)
async function resumePump() {
  resuming.value = true
  try {
    await api.resetPump()
    void live.fetchStatus()
  } catch (e) {
    showToast({ message: errorMessage(e, "Couldn't reach the pump"), tone: 'error' })
  } finally {
    resuming.value = false
  }
}

onMounted(() => void shelf.refresh())

// Back online: "Back on after 9 days · 3 pieces still listed on the shelf".
const backOnline = computed(() => {
  const o = status.value?.last_offline
  if (!o || o.pieces_to_check <= 0) return null
  const from = parseTs(o.from)
  const to = parseTs(o.to)
  const span = from !== null && to !== null ? formatSpanWords((to - from) / 1000) : ''
  return { span, count: o.pieces_to_check }
})
const offlineIds = computed(() =>
  (shelf.shelf?.check_these ?? []).filter((c) => c.reasons.includes('not_checked_since_restart')).map((c) => c.piece_id),
)
function checkOffline() {
  void router.push({ path: '/check', query: offlineIds.value.length ? { ids: offlineIds.value.join(',') } : {} })
}
</script>

<template>
  <main class="page">
    <header class="page-header">
      <h1 class="page-title">Drying shelf</h1>
      <RouterLink to="/settings" class="btn btn-ghost" aria-label="Settings"><span aria-hidden="true" class="gear">⚙</span></RouterLink>
    </header>

    <Banner v-if="backOnline && !live.offline" tone="warn" icon="⏻" data-testid="back-online-banner">
      <strong>Back on after {{ backOnline.span }}</strong> · {{ backOnline.count }} {{ backOnline.count === 1 ? 'piece' : 'pieces' }} still listed on the shelf
      <template #actions>
        <button type="button" class="btn btn-primary" @click="checkOffline">Check them</button>
        <button type="button" class="btn" :disabled="!offlineIds.length" @click="sheet.open('trim', { ids: offlineIds })">Mark trimmed</button>
        <button type="button" class="btn" :disabled="!offlineIds.length" @click="sheet.open('discard', { ids: offlineIds })">Mark discarded</button>
      </template>
    </Banner>

    <Banner v-if="pumpTripped" tone="error" icon="⚠" data-testid="pump-tripped-banner">
      <strong>The pump stopped for safety</strong> · the humidity didn't rise after several runs. Check the water tank and
      the nozzle.
      <template #actions>
        <button type="button" class="btn btn-primary" :disabled="resuming" @click="resumePump">Resume</button>
        <RouterLink to="/settings#pump" class="btn">Pump settings</RouterLink>
      </template>
    </Banner>

    <p v-if="live.clockSynced === false" class="clock-note" data-testid="clock-note">
      ⏱ The controller's clock hasn't synced yet. Recording starts once it has (usually within a minute).
    </p>

    <div :class="{ 'offline-dim': live.offline }">
      <ClimateCard />
    </div>
    <TargetRow />
    <TrimCard />
    <div :class="{ 'offline-dim': live.offline }">
      <ShelfSummary />
    </div>

    <div v-if="status && status.shelf.on_shelf > 0" class="actions">
      <RouterLink to="/check" class="btn btn-primary btn-lg btn-block" data-testid="check-pieces">Check pieces</RouterLink>
      <button type="button" class="btn btn-lg btn-block" @click="sheet.open('add')">Add pieces</button>
    </div>
  </main>
</template>

<style scoped>
.gear {
  font-size: 1.5rem;
}
.clock-note {
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
.actions {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
</style>
