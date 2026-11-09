<script setup lang="ts">
import { onBeforeUnmount } from 'vue'
import MessageBar from '@ui/MessageBar.vue'
import { bar } from '@ui/messages'
import { startPolling, undoLast } from './live'

const stop = startPolling()
onBeforeUnmount(stop)
</script>

<template>
  <RouterView />
  <MessageBar :message="bar.message" @dismiss="bar.message = null" @undo="undoLast" />
</template>
