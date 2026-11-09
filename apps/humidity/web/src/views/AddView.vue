<script setup lang="ts">
// Add one or more identical pieces. The last form/size/thickness are
// remembered on the phone; the photo is uploaded as is to each created
// piece (the server shrinks it).
import { onBeforeUnmount, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, type Form, type Size, type Thickness } from '@/api'
import Choice from '@ui/Choice.vue'
import PreciseSlider from '@ui/PreciseSlider.vue'
import { DRYNESS_GRADIENT, TRIM_BAND, drynessWords } from '@/dryness'
import { FORMS, SIZES, THICKNESSES } from '@/format'
import { bumpPhotos, refreshShelf, say, sayError } from '@/live'

const KEY = 'humidity.lastBuild'
const router = useRouter()

function remembered(): { form: Form; size: Size; thickness: Thickness } {
  const fallback = { form: 'bowl' as Form, size: 'medium' as Size, thickness: 'medium' as Thickness }
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? '{}')
    return {
      form: s.form in FORMS ? s.form : fallback.form,
      size: s.size in SIZES ? s.size : fallback.size,
      thickness: s.thickness in THICKNESSES ? s.thickness : fallback.thickness,
    }
  } catch {
    return fallback
  }
}

const f = reactive({ ...remembered(), dryness: 1.0, label: '', quantity: 1 })
const photo = ref<Blob | null>(null)
const preview = ref('')
const busy = ref(false)

function pickPhoto(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  photo.value = file
  URL.revokeObjectURL(preview.value)
  preview.value = URL.createObjectURL(file)
}
onBeforeUnmount(() => URL.revokeObjectURL(preview.value))

async function submit() {
  busy.value = true
  try {
    const created = await api.addPieces({
      form: f.form, size: f.size, thickness: f.thickness, dryness: f.dryness, label: f.label.trim(), quantity: f.quantity,
    })
    try {
      localStorage.setItem(KEY, JSON.stringify({ form: f.form, size: f.size, thickness: f.thickness }))
    } catch {
      // private mode: nothing to remember
    }
    const numbers = created.map((p) => '#' + p.number).join(', ')
    say(`Added ${numbers}. Write the number on the bat.`)
    if (photo.value) {
      try {
        for (const p of created) await api.putPhoto(p.id, photo.value)
      } catch (err) {
        say(`Added ${numbers}, but the photo didn't upload: ${(err as Error).message}`, true)
      }
      bumpPhotos()
    }
    void refreshShelf()
    void router.push('/')
  } catch (err) {
    sayError(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="page">
    <header class="bar">
      <RouterLink to="/" class="btn btn-ghost">← Shelf</RouterLink>
      <h1>Add pieces</h1>
    </header>
    <form class="card" @submit.prevent="submit">
      <label class="photo">
        <img v-if="preview" :src="preview" alt="Photo of the new pieces" />
        <span v-else class="btn btn-block">Add a photo</span>
        <input type="file" accept="image/*" class="hidden-input" @change="pickPhoto" />
      </label>
      <Choice v-model="f.form" label="Form" :options="FORMS" />
      <Choice v-model="f.size" label="Size" :options="SIZES" />
      <Choice v-model="f.thickness" label="Walls" :options="THICKNESSES" />
      <div>
        <strong>Starting dryness</strong>
        <PreciseSlider v-model="f.dryness" :min="0" :max="5" :step="0.1" :band="TRIM_BAND" band-label="✂ trim band" :gradient="DRYNESS_GRADIENT" :words="drynessWords" label="Starting dryness" />
      </div>
      <label class="field">Label (optional)
        <input v-model="f.label" class="input" maxlength="100" placeholder="e.g. blue glaze test" />
      </label>
      <div class="field">
        How many
        <div class="row">
          <button type="button" class="btn" :disabled="f.quantity <= 1" aria-label="One fewer" @click="f.quantity--">−</button>
          <span class="qty num">{{ f.quantity }}</span>
          <button type="button" class="btn" :disabled="f.quantity >= 20" aria-label="One more" @click="f.quantity++">+</button>
        </div>
      </div>
      <button type="submit" class="btn btn-primary btn-block" :disabled="busy">
        {{ busy ? 'Adding…' : f.quantity === 1 ? 'Add piece' : `Add ${f.quantity} pieces` }}
      </button>
    </form>
  </main>
</template>

<style scoped>
.photo {
  cursor: pointer;
}
.photo img {
  border-radius: var(--radius-sm);
  max-height: 240px;
  margin: 0 auto;
}
.hidden-input {
  position: absolute;
  opacity: 0;
  width: 1px;
  height: 1px;
}
.field {
  font-weight: 600;
}
.qty {
  min-width: 2.5em;
  text-align: center;
  font-size: var(--fs-xl);
}
</style>
