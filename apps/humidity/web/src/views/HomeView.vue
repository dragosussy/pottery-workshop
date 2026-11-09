<script setup lang="ts">
import { onMounted } from 'vue'
import ClimateCard from '@/components/ClimateCard.vue'
import PieceRow from '@/components/PieceRow.vue'
import { live, refreshShelf } from '@/live'

onMounted(() => {
  live.walk = null // "Next piece →" starts a fresh walk from here
  void refreshShelf()
})
</script>

<template>
  <main class="page">
    <header class="bar">
      <h1>Drying shelf</h1>
      <RouterLink to="/settings" class="btn btn-ghost" aria-label="Settings">⚙︎</RouterLink>
    </header>

    <ClimateCard />

    <section class="card">
      <div class="bar">
        <h2>On the shelf</h2>
        <span v-if="live.now?.learning" class="small muted">estimates are a rough guess for now</span>
      </div>
      <div>
        <PieceRow v-for="p in live.shelf" :key="p.id" :piece="p" />
        <p v-if="live.shelfLoaded && !live.shelf.length" class="muted">Nothing on the shelf.</p>
      </div>
      <RouterLink to="/add" class="btn btn-primary btn-block">Add pieces</RouterLink>
      <RouterLink to="/finished" class="btn btn-ghost btn-block">Finished pieces</RouterLink>
    </section>
  </main>
</template>
