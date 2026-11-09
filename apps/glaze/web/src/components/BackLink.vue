<script setup lang="ts">
// Every Back goes where you came from, whichever way you got here.
import { useRouter } from 'vue-router'
import { goBack } from '@/nav'

const props = defineProps<{ fallback: string }>()
const router = useRouter()
</script>

<template>
  <button type="button" class="btn btn-ghost" @click="goBack(router, props.fallback)">← Back</button>
</template>
