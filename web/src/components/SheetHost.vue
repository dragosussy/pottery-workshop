<script setup lang="ts">
// Renders the sheet named in the route query (?sheet=…&ids=…).
import { computed } from 'vue'
import { useSheet } from '@/composables/useSheet'
import AddPiecesSheet from '@/sheets/AddPiecesSheet.vue'
import FlipSheet from '@/sheets/FlipSheet.vue'
import TrimSheet from '@/sheets/TrimSheet.vue'
import DiscardSheet from '@/sheets/DiscardSheet.vue'
import ObserveSheet from '@/sheets/ObserveSheet.vue'
import EditPieceSheet from '@/sheets/EditPieceSheet.vue'

const sheet = useSheet()
const q = sheet.query
const num = (v: unknown) => (typeof v === 'string' && v !== '' && !Number.isNaN(Number(v)) ? Number(v) : null)
const dryness = computed(() => num(q.value.d))
const obs = computed(() => num(q.value.obs))
const focus = computed(() => (typeof q.value.focus === 'string' ? q.value.focus : null))
const key = computed(() => `${sheet.current.value}:${sheet.ids.value.join(',')}:${obs.value ?? ''}`)
const close = () => sheet.close()
</script>

<template>
  <AddPiecesSheet v-if="sheet.current.value === 'add'" @close="close" @done="close" />
  <FlipSheet v-else-if="sheet.current.value === 'flip'" :key="key" :ids="sheet.ids.value" :initial-dryness="dryness" @close="close" @done="close" />
  <TrimSheet v-else-if="sheet.current.value === 'trim'" :key="key" :ids="sheet.ids.value" :initial-dryness="dryness" @close="close" @done="close" />
  <DiscardSheet v-else-if="sheet.current.value === 'discard'" :key="key" :ids="sheet.ids.value" @close="close" @done="close" />
  <ObserveSheet v-else-if="sheet.current.value === 'observe'" :key="key" :ids="sheet.ids.value" :obs-id="obs" @close="close" @done="close" />
  <EditPieceSheet v-else-if="sheet.current.value === 'edit'" :key="key" :ids="sheet.ids.value" :focus="focus" @close="close" @done="close" />
</template>
