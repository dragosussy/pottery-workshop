<script setup lang="ts">
import { useRoute } from 'vue-router'
import MessageBar from '@ui/MessageBar.vue'
import { bar, sayError } from '@ui/messages'
import TabBar from '@/components/TabBar.vue'

const route = useRoute()

async function undo() {
  const run = bar.message?.undo
  bar.message = null
  try {
    await run?.()
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <RouterView />
  <TabBar v-if="route.meta.tab" :active="String(route.meta.tab)" />
  <MessageBar :message="bar.message" @dismiss="bar.message = null" @undo="undo" />
</template>
