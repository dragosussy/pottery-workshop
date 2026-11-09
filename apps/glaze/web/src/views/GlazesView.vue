<script setup lang="ts">
// Every material on one page. Each is a tile that folds open, and which one is
// open lives in the URL, so coming back from a piece finds it still open.
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Choice from '@ui/Choice.vue'
import { sayError } from '@ui/messages'
import { api, type Material, type MaterialKind } from '@/api'
import MaterialTile from '@/components/MaterialTile.vue'
import { MATERIAL_KINDS } from '@/format'

const route = useRoute()
const router = useRouter()
const materials = ref<Material[]>([])
const adding = ref(false)
const f = reactive({ name: '', kind: 'own' as MaterialKind })

const openId = computed(() => Number(route.query.open) || null)
const setOpen = (id: number | null) => router.replace({ query: id ? { open: id } : {} })

async function load() {
  try {
    materials.value = await api.materials()
  } catch (e) {
    sayError(e)
  }
}

function scrollToOpen() {
  void nextTick(() => document.querySelector(`[data-material="${openId.value}"]`)?.scrollIntoView({ block: 'start' }))
}

onMounted(async () => {
  await load()
  if (openId.value) scrollToOpen()
})

const groups = computed(() =>
  (Object.keys(MATERIAL_KINDS) as MaterialKind[])
    .map((kind) => ({
      kind,
      label: MATERIAL_KINDS[kind],
      items: materials.value.filter((m) => m.kind === kind && !m.archived).sort((a, b) => a.name.localeCompare(b.name)),
    }))
    .filter((g) => g.items.length),
)
const archived = computed(() => materials.value.filter((m) => m.archived))
const archivedOpen = computed(() => archived.value.some((m) => m.id === openId.value))

async function create() {
  try {
    const m = await api.addMaterial({ name: f.name, kind: f.kind, maker: '', notes: '', archived: false })
    f.name = ''
    adding.value = false
    await load()
    await setOpen(m.id) // stays on this page, with the new one open
    scrollToOpen()
  } catch (e) {
    sayError(e)
  }
}

async function deleted() {
  await setOpen(null)
  await load()
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <h1>Glazes</h1>
      <button type="button" class="btn btn-ghost" :aria-expanded="adding" @click="adding = !adding">{{ adding ? 'Cancel' : 'Add' }}</button>
    </header>

    <form v-if="adding" class="card" @submit.prevent="create">
      <label class="field">Name
        <input v-model="f.name" class="input" maxlength="100" />
      </label>
      <Choice v-model="f.kind" label="Kind" :options="MATERIAL_KINDS" />
      <button type="submit" class="btn btn-primary btn-block" :disabled="!f.name.trim()">Add</button>
    </form>

    <p v-if="!groups.length && !adding" class="muted">No materials yet. Add your first with the button above.</p>

    <section v-for="g in groups" :key="g.kind" class="rows">
      <h2>{{ g.label }}</h2>
      <MaterialTile
        v-for="m in g.items" :key="m.id" :material="m" :open="m.id === openId"
        @toggle="setOpen(m.id === openId ? null : m.id)" @changed="load" @deleted="deleted"
      />
    </section>

    <details v-if="archived.length" class="more" :open="archivedOpen">
      <summary>Archived</summary>
      <div class="rows">
        <MaterialTile
          v-for="m in archived" :key="m.id" :material="m" :open="m.id === openId"
          @toggle="setOpen(m.id === openId ? null : m.id)" @changed="load" @deleted="deleted"
        />
      </div>
    </details>
  </main>
</template>
