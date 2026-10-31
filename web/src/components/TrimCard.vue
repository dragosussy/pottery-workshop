<script setup lang="ts">
// The trim card (plan §4.1): when to come. Greyed out with "as of …" when the
// controller is unreachable (it shows the cached /status).
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useLiveStore } from '@/stores/live'
import { useShelfStore } from '@/stores/shelf'
import { useSettingsStore } from '@/stores/settings'
import { useNow } from '@/composables/useServerClock'
import { useSheet } from '@/composables/useSheet'
import { formatDayTime, numberList } from '@/utils/format'
import { trimCardModel } from '@/utils/trimCard'
import PiecePhoto from './PiecePhoto.vue'

const live = useLiveStore()
const shelf = useShelfStore()
const settings = useSettingsStore()
const sheet = useSheet()
const now = useNow()
const open = ref(false)

onMounted(() => void shelf.ensure())

const model = computed(() => (live.status ? trimCardModel(live.status, shelf.pieces, settings.tz, now.value) : null))
const byId = computed(() => shelf.byId)
const nums = (ids: number[]) => numberList(ids.map((id) => byId.value.get(id)?.number).filter((n): n is number => n !== undefined))
const asOf = computed(() => (live.asOf === null ? '' : formatDayTime(live.asOf, now.value, settings.tz)))
</script>

<template>
  <section class="card trim" :class="{ 'offline-dim': live.offline }" aria-labelledby="trim-h" data-testid="trim-card" :data-kind="model?.kind">
    <h2 id="trim-h" class="sr-only">When to trim</h2>
    <p v-if="!model" class="muted">Loading…</p>
    <template v-else>
      <p v-if="live.offline && asOf" class="asof">as of {{ asOf }}</p>

      <template v-if="model.kind === 'empty'">
        <p class="headline">{{ model.headline }}</p>
        <button type="button" class="btn btn-primary" @click="sheet.open('add')">Add pieces</button>
      </template>

      <template v-else-if="model.kind === 'ready_now'">
        <p class="headline"><span class="ok" aria-hidden="true">✂</span> <strong>{{ model.headline }}</strong><span v-if="model.detail" class="detail"> · {{ model.detail }}</span></p>
        <div class="thumbs">
          <RouterLink v-for="id in model.readyIds" :key="id" :to="`/pieces/${id}`" class="thumb">
            <PiecePhoto :photo-id="byId.get(id)?.photo_id ?? null" :form="byId.get(id)?.form" :size="52" />
            <span class="num">#{{ byId.get(id)?.number }}</span>
          </RouterLink>
        </div>
        <p v-if="model.then" class="muted">{{ model.then }}</p>
      </template>

      <template v-else-if="model.kind === 'next_visit' && model.visit">
        <button type="button" class="visit" :aria-expanded="open" @click="open = !open">
          <span class="headline"><strong>{{ model.headline }}</strong></span>
          <span class="detail">{{ model.detail }} <span aria-hidden="true">{{ open ? '▴' : '▾' }}</span></span>
        </button>
        <dl v-if="open" class="groups">
          <template v-if="model.visit.ready_piece_ids.length">
            <dt>Ready</dt>
            <dd>{{ nums(model.visit.ready_piece_ids) }}</dd>
          </template>
          <template v-if="model.visit.maybe_piece_ids.length">
            <dt>Maybe</dt>
            <dd>{{ nums(model.visit.maybe_piece_ids) }}</dd>
          </template>
          <template v-if="model.visit.not_yet_piece_ids.length">
            <dt>Not yet</dt>
            <dd>{{ nums(model.visit.not_yet_piece_ids) }}</dd>
          </template>
        </dl>
      </template>

      <p v-else class="headline">{{ model.headline }}</p>

      <p v-if="model.confidence" class="conf" data-testid="trim-confidence">{{ model.confidence }}</p>
    </template>
  </section>
</template>

<style scoped>
.trim {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.asof {
  font-size: var(--fs-sm);
  font-weight: 700;
  color: var(--text-muted);
}
.headline {
  font-size: var(--fs-xl);
  line-height: 1.25;
}
.detail {
  font-size: var(--fs-lg);
  color: var(--text-muted);
}
.ok {
  color: var(--ok);
}
.visit {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  min-height: var(--touch);
  padding: 0;
  border: 0;
  background: none;
  text-align: left;
  cursor: pointer;
}
.groups {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--sp-1) var(--sp-3);
  margin: 0;
}
.groups dt {
  font-weight: 700;
}
.groups dd {
  margin: 0;
}
.thumbs {
  display: flex;
  gap: var(--sp-2);
  overflow-x: auto;
}
.thumb {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  color: inherit;
  text-decoration: none;
  font-weight: 700;
}
.conf {
  font-size: var(--fs-sm);
  color: var(--text-muted);
  font-style: italic;
}
</style>
