<script setup lang="ts">
// /dev/slider: the muddy-finger test bench for the DrynessSlider (plan §5.1
// "Validation plan"). Shows numbers on purpose; the real screens don't.
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import DrynessSlider from '@/components/DrynessSlider.vue'
import { GESTURE } from '@/composables/usePointerSlider'
import { drynessText } from '@/utils/dryness'

// A: check-in style (pre-filled with the last observed value, untouched).
const LAST = 2.3
const a = ref(LAST)
const aTouched = ref(false)
const changes = ref(0)
watch(a, () => changes.value++)
function resetA() {
  a.value = LAST
  aTouched.value = false
  changes.value = 0
}

// Challenge: set a random target within ±0.1 in under 3 s.
const target = ref<number | null>(null)
const startedAt = ref<number | null>(null)
const results = ref<{ target: number; got: number; ms: number }[]>([])
let idleTimer: ReturnType<typeof setTimeout> | null = null

function newTarget() {
  let t: number
  do t = Math.round(Math.random() * 50) / 10
  while (Math.abs(t - a.value) < 0.8)
  target.value = t
  startedAt.value = performance.now()
}
function check() {
  if (target.value === null || startedAt.value === null) return
  if (Math.abs(a.value - target.value) <= 0.1 + 1e-9) {
    results.value.unshift({ target: target.value, got: a.value, ms: performance.now() - startedAt.value })
    target.value = null
    startedAt.value = null
  }
}
watch(a, () => {
  if (idleTimer) clearTimeout(idleTimer)
  idleTimer = setTimeout(check, 700) // nudges and keys: check once the value settles
})
onBeforeUnmount(() => idleTimer && clearTimeout(idleTimer))

const passRate = computed(() => {
  if (!results.value.length) return ''
  const pass = results.value.filter((r) => r.ms < 3000).length
  return `${pass} of ${results.value.length} under 3 s`
})

// B: piece-detail style with the model estimate marker.
const b = ref(2.0)
const bTouched = ref(false)

const fillerAbove = Array.from({ length: 6 }, (_, i) => i)
const fillerBelow = Array.from({ length: 10 }, (_, i) => i)
</script>

<template>
  <main class="page dev">
    <header class="page-header">
      <RouterLink to="/settings" class="btn btn-ghost" aria-label="Back"><span aria-hidden="true">←</span></RouterLink>
      <h1 class="page-title">Slider test</h1>
      <span style="width: 48px"></span>
    </header>

    <section class="card stack">
      <p><strong>Try this with wet or muddy fingers.</strong> Check that:</p>
      <ol class="checks">
        <li>you can set any value to within ±0.1 in under 3 seconds (use the challenge below);</li>
        <li>scrolling the page over the slider <em>never</em> changes it (watch the change counter);</li>
        <li>the readout is legible at arm's length.</li>
      </ol>
      <p class="muted small">
        Drag sideways to move it. Slide your finger <strong>down, away from the track</strong> while dragging for fine control
        (½× after {{ GESTURE.fineHalfAt }}px, ¼× after {{ GESTURE.fineQuarterAt }}px). Tap the track to jump. Hold − or + to repeat.
      </p>
    </section>

    <p v-for="i in fillerAbove" :key="'a' + i" class="filler">Scroll area {{ i + 1 }}. Swipe up and down across the slider below; its value must not change.</p>

    <section class="card stack" data-testid="slider-a">
      <h2 class="h2">Check-in style</h2>
      <p class="muted small">Starts at the last observed value ({{ LAST.toFixed(1) }}, the ring), untouched.</p>
      <DrynessSlider v-model="a" v-model:touched="aTouched" :last-value="LAST" label="Test slider" @commit="check" />
      <dl class="debug num">
        <dt>value</dt>
        <dd data-testid="dev-value">{{ a.toFixed(1) }}</dd>
        <dt>touched</dt>
        <dd>{{ aTouched ? 'yes' : 'no' }}</dd>
        <dt>changes</dt>
        <dd data-testid="dev-changes">{{ changes }}</dd>
      </dl>
      <div class="row-wrap">
        <button type="button" class="btn" @click="resetA">Reset</button>
        <button type="button" class="btn btn-primary" @click="newTarget">{{ target === null ? 'New challenge' : 'Skip challenge' }}</button>
      </div>
      <div v-if="target !== null" class="challenge" role="status">
        Set it to <strong>{{ drynessText(target) }}</strong> <span class="num">({{ target.toFixed(1) }})</span>
      </div>
      <div v-if="results.length" class="results">
        <p><strong>{{ passRate }}</strong></p>
        <ul>
          <li v-for="(r, i) in results.slice(0, 8)" :key="i" class="num">
            {{ r.ms < 3000 ? '✓' : '✗' }} {{ r.target.toFixed(1) }} → {{ r.got.toFixed(1) }} in {{ (r.ms / 1000).toFixed(1) }} s
          </li>
        </ul>
      </div>
    </section>

    <section class="card stack">
      <h2 class="h2">Piece-detail style</h2>
      <p class="muted small">Last observed 2.0 (ring) and the model's estimate 2.6 (dotted tick).</p>
      <DrynessSlider v-model="b" v-model:touched="bTouched" :last-value="2.0" :estimate="2.6" label="Piece detail test" untouched-hint="" />
      <p class="num muted">value {{ b.toFixed(1) }}</p>
    </section>

    <section class="card stack">
      <h2 class="h2">Disabled</h2>
      <DrynessSlider :model-value="3.4" disabled label="Disabled test" untouched-hint="" />
    </section>

    <section class="card">
      <h2 class="h2">Thresholds</h2>
      <dl class="debug num">
        <dt>capture</dt>
        <dd>horizontal &gt; {{ GESTURE.captureX }}px before vertical &gt; {{ GESTURE.releaseY }}px</dd>
        <dt>fine</dt>
        <dd>½× at {{ GESTURE.fineHalfAt }}px, ¼× at {{ GESTURE.fineQuarterAt }}px below</dd>
        <dt>thumb grab</dt>
        <dd>{{ GESTURE.thumbGrabRadius }}px</dd>
        <dt>nudge repeat</dt>
        <dd>{{ GESTURE.repeatDelay }}ms, then every {{ GESTURE.repeatInterval }}ms</dd>
      </dl>
    </section>

    <p v-for="i in fillerBelow" :key="'b' + i" class="filler">More scroll area {{ i + 1 }}.</p>
  </main>
</template>

<style scoped>
.dev {
  padding-bottom: 120px;
}
.h2 {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.small {
  font-size: var(--fs-sm);
}
.checks {
  margin: 0;
  padding-left: 1.3em;
}
.filler {
  padding: var(--sp-5) var(--sp-3);
  color: var(--text-faint);
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
}
.debug {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 2px var(--sp-3);
  margin: 0;
  font-size: var(--fs-sm);
}
.debug dt {
  color: var(--text-muted);
}
.debug dd {
  margin: 0;
}
.challenge {
  padding: var(--sp-3);
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
  font-size: var(--fs-lg);
}
.results ul {
  margin: var(--sp-1) 0 0;
  padding-left: 1em;
}
</style>
