<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { sayError } from '@ui/messages'
import { api, backupUrl, type ClayBody } from '@/api'
import BackLink from '@/components/BackLink.vue'
import ConfirmButton from '@/components/ConfirmButton.vue'

const clayBodies = ref<ClayBody[]>([])
const name = ref('')

async function load() {
  try {
    clayBodies.value = await api.clayBodies()
  } catch (e) {
    sayError(e)
  }
}
onMounted(load)

async function add() {
  try {
    await api.addClayBody(name.value)
    name.value = ''
    await load()
  } catch (e) {
    sayError(e)
  }
}

async function save(c: ClayBody) {
  try {
    await api.saveClayBody(c)
  } catch (e) {
    sayError(e)
    await load()
  }
}

async function setArchived(c: ClayBody, archived: boolean) {
  await save({ ...c, archived })
  await load()
}

async function remove(c: ClayBody) {
  try {
    await api.deleteClayBody(c.id)
    await load()
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <BackLink fallback="/" />
      <h1>Settings</h1>
    </header>

    <section class="card">
      <h2>Clay bodies</h2>
      <div v-for="c in clayBodies" :key="c.id" class="clay">
        <input v-model="c.name" class="input" maxlength="100" :aria-label="`Name of ${c.name}`" @change="save(c)" />
        <button v-if="c.archived" type="button" class="btn btn-small" @click="setArchived(c, false)">Bring back</button>
        <button v-else type="button" class="btn btn-small" @click="setArchived(c, true)">Archive</button>
        <ConfirmButton class="btn-small" label="Delete" confirm="Sure?" @confirmed="remove(c)" />
      </div>
      <form class="clay" @submit.prevent="add">
        <input v-model="name" class="input" maxlength="100" placeholder="New clay body" aria-label="New clay body" />
        <button type="submit" class="btn btn-primary" :disabled="!name.trim()">Add</button>
      </form>
    </section>

    <section class="card">
      <h2>Backup</h2>
      <p class="muted small">Everything is copied off the Pi once a day. This downloads the database now, without the photos.</p>
      <a class="btn" :href="backupUrl" download>Download the database</a>
    </section>
  </main>
</template>

<style scoped>
.clay {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
</style>
