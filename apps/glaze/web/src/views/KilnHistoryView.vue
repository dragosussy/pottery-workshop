<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { sayError } from '@ui/messages'
import { api, type Firing } from '@/api'
import BackLink from '@/components/BackLink.vue'
import { firingLabel } from '@/format'

const firings = ref<Firing[]>([])
const past = computed(() => firings.value.filter((f) => f.state === 'unloaded'))

onMounted(async () => {
  try {
    firings.value = await api.firings()
  } catch (e) {
    sayError(e)
  }
})
</script>

<template>
  <main class="page">
    <header class="bar">
      <BackLink fallback="/kiln" />
      <h1>Past firings</h1>
    </header>
    <p v-if="!past.length" class="muted">None yet.</p>
    <RouterLink v-for="f in past" :key="f.id" :to="`/firings/${f.id}`" class="card link">
      <strong>{{ firingLabel(f) }}</strong>
      <span class="small muted">{{ f.pieces }} piece{{ f.pieces === 1 ? '' : 's' }}<template v-if="f.waiting">, {{ f.waiting }} waiting for a result</template></span>
    </RouterLink>
  </main>
</template>

<style scoped>
.link {
  color: inherit;
  text-decoration: none;
  gap: var(--sp-1);
}
</style>
