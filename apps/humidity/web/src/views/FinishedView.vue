<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type Piece } from '@/api'
import PieceRow from '@/components/PieceRow.vue'
import { sayError } from '@/live'

const pieces = ref<Piece[] | null>(null)
onMounted(async () => {
  try {
    pieces.value = await api.finished() // newest ended first
  } catch (e) {
    sayError(e)
  }
})
</script>

<template>
  <main class="page">
    <header class="bar">
      <RouterLink to="/" class="btn btn-ghost">← Shelf</RouterLink>
      <h1>Finished</h1>
    </header>
    <section class="card">
      <p v-if="!pieces" class="muted">Loading…</p>
      <p v-else-if="!pieces.length" class="muted">No finished pieces yet.</p>
      <div v-else>
        <PieceRow v-for="p in pieces" :key="p.id" :piece="p" />
      </div>
    </section>
  </main>
</template>
