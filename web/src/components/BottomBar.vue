<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useSheet } from '@/composables/useSheet'
import { chrome } from '@/composables/useChrome'
const sheet = useSheet()
</script>

<template>
  <nav class="bar" aria-label="Main">
    <RouterLink to="/" class="tab" exact-active-class="is-active"><span aria-hidden="true" class="ico">◉</span>Now</RouterLink>
    <RouterLink to="/shelf" class="tab" active-class="is-active"><span aria-hidden="true" class="ico">▤</span>Shelf</RouterLink>
    <RouterLink to="/history" class="tab" active-class="is-active"><span aria-hidden="true" class="ico">↺</span>History</RouterLink>
  </nav>
  <button v-if="!chrome.hideFab" type="button" class="fab" aria-label="Add pieces" data-testid="fab-add" @click="sheet.open('add')"><span aria-hidden="true">+</span></button>
</template>

<style scoped>
.bar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 40;
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  align-items: center;
  height: calc(var(--bottom-bar-h) + env(safe-area-inset-bottom));
  padding-bottom: env(safe-area-inset-bottom);
  background: var(--surface);
  border-top: 1px solid var(--border);
}
.tab {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: var(--touch);
  height: 100%;
  font-size: var(--fs-xs);
  font-weight: 700;
  color: var(--text-muted);
  text-decoration: none;
}
.ico {
  font-size: 1.25rem;
  line-height: 1.2;
}
.tab.is-active {
  color: var(--accent);
}
.tab.is-active .ico {
  transform: scale(1.1);
}
.fab {
  position: fixed;
  z-index: 41;
  right: var(--gutter);
  bottom: calc(var(--bottom-bar-h) + 16px + env(safe-area-inset-bottom));
  width: 64px;
  height: 64px;
  border-radius: 50%;
  border: 0;
  background: var(--accent);
  color: var(--accent-text);
  font-size: 2rem;
  font-weight: 700;
  box-shadow: var(--shadow-lg);
  cursor: pointer;
}
</style>
