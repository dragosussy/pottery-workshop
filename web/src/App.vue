<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import { useLiveStore } from '@/stores/live'
import { useReferenceStore } from '@/stores/reference'
import { useSettingsStore } from '@/stores/settings'
import ToastHost from '@/components/ToastHost.vue'
import OfflineBanner from '@/components/OfflineBanner.vue'
import Banner from '@/components/Banner.vue'
import BottomBar from '@/components/BottomBar.vue'
import SheetHost from '@/components/SheetHost.vue'

const live = useLiveStore()
const reference = useReferenceStore()
const settings = useSettingsStore()
const route = useRoute()

const bare = computed(() => route.meta.bare === true)
const showBar = computed(() => !bare.value && route.meta.focus !== true)

onMounted(() => {
  live.connect()
  void reference.load()
  void settings.load()
  live.onChange((scopes) => {
    if (scopes.includes('settings')) void settings.load()
  })
})

function reload() {
  window.location.reload()
}
</script>

<template>
  <div class="app" :class="{ 'is-offline': live.offline }">
    <div v-if="!bare && (live.offline || live.newVersionAvailable)" class="app-banners">
      <OfflineBanner v-if="live.offline" />
      <Banner v-if="live.newVersionAvailable" tone="info" icon="↻">
        The controller was updated.
        <template #actions>
          <button type="button" class="btn btn-primary" @click="reload">Reload</button>
        </template>
      </Banner>
    </div>
    <RouterView />
    <BottomBar v-if="showBar" />
    <SheetHost />
    <ToastHost />
  </div>
</template>

<style scoped>
.app-banners {
  max-width: 640px;
  margin: 0 auto;
  padding: calc(var(--sp-3) + env(safe-area-inset-top)) var(--gutter) 0;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
</style>
