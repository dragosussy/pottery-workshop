<script setup lang="ts">
import { reactive, ref, watchEffect } from 'vue'
import { api, apiUrl } from '@/api'
import { ago } from '@/format'
import { live, say, sayError, setNow } from '@/live'

// Offset drafts by sensor address, filled from the server once and then left
// alone while the potter types.
const drafts = reactive<Record<string, number | string>>({})
const busy = ref('')
watchEffect(() => {
  for (const s of live.now?.sensors ?? []) drafts[s.addr] ??= s.offset
})

async function save(addr: string) {
  const v = Number(drafts[addr])
  if (drafts[addr] === '' || !Number.isFinite(v)) return say('Enter a number, e.g. 1.5 or -2', true)
  busy.value = addr
  try {
    setNow(await api.setOffset(addr, v))
    say(`Offset for ${addr} saved`)
  } catch (e) {
    sayError(e)
  } finally {
    busy.value = ''
  }
}
const fmt = (v: number | null, unit: string) => (v === null ? '–' : `${v.toFixed(1)} ${unit}`)
</script>

<template>
  <main class="page">
    <header class="bar">
      <RouterLink to="/" class="btn btn-ghost">← Shelf</RouterLink>
      <h1>Settings</h1>
    </header>

    <section class="card">
      <h2>Sensor offsets</h2>
      <p class="muted small">
        Salt test: seal the sensor in a jar with a damp paste of table salt for 8 hours. It should read 75 %, so set the offset to 75 − raw.
      </p>
      <p v-if="!live.now" class="muted">Loading…</p>
      <div v-for="s in live.now?.sensors ?? []" :key="s.addr" class="sensor">
        <p>
          <strong>{{ s.addr }}</strong> raw <span class="num">{{ fmt(s.raw_rh, '%') }}</span>, shown as
          <span class="num">{{ fmt(s.rh, '%') }}</span> · <span class="num">{{ fmt(s.temp, '°C') }}</span>
          <span v-if="s.at" class="muted small"> · {{ ago(live.clock - Date.parse(s.at)) }}</span>
        </p>
        <p v-if="s.error" class="bad small">{{ s.error }}</p>
        <form class="row" @submit.prevent="save(s.addr)">
          <label class="row">Offset
            <input v-model="drafts[s.addr]" class="input num offset" type="number" step="0.1" min="-10" max="10" />
            %
          </label>
          <button type="submit" class="btn" :disabled="busy === s.addr">Save</button>
        </form>
      </div>
    </section>

    <section class="card">
      <h2>Backup</h2>
      <p class="muted small">The Pi keeps a daily copy of the database (the last 7). Photos aren't included.</p>
      <a class="btn" :href="apiUrl('/backup')" download>Download a backup</a>
    </section>
  </main>
</template>

<style scoped>
.sensor {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  padding-bottom: var(--sp-3);
  border-bottom: 1px solid var(--surface-3);
}
.offset {
  width: 6em;
}
</style>
