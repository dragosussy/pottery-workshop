<script setup lang="ts">
// The stack, in order from the clay out. Each layer asks only for what's
// recorded nearly every time; the rest waits under "More". A new layer starts
// as that material was last applied, so most take a tap or two.
import { computed, ref } from 'vue'
import Choice from '@ui/Choice.vue'
import PreciseSlider from '@ui/PreciseSlider.vue'
import { sayError } from '@ui/messages'
import { api, type LayerInput, type Material, type MaterialKind, type Method, type Stage } from '@/api'
import OptionalChoice from '@/components/OptionalChoice.vue'
import { AREAS, EVERYDAY_STAGES, MATERIAL_KINDS, METHODS, STAGES, layerDetail } from '@/format'
import { THICKNESS_GRADIENT, thicknessWords } from '@/thickness'

export type LayerDraft = LayerInput & { key: number }

const props = defineProps<{
  materials: Material[]
  /** Layers added now go on before a re-fire. */
  refire?: boolean
  /** An already-fired piece: leave what isn't remembered blank, not guessed. */
  blank?: boolean
}>()
const emit = defineEmits<{ created: [m: Material] }>()
const layers = defineModel<LayerDraft[]>({ required: true })

const open = ref<number | null>(null)
const picking = ref(false)
const search = ref('')
const newKind = ref<MaterialKind>('own')

const material = (id: number) => props.materials.find((m) => m.id === id)
const term = computed(() => search.value.trim().toLowerCase())
// The server sends them most recently used first.
const matches = computed(() => props.materials.filter((m) => !m.archived && m.name.toLowerCase().includes(term.value)))
const canCreate = computed(() => term.value !== '' && !props.materials.some((m) => m.name.toLowerCase() === term.value))
const stageOptions = (l: LayerDraft) => (props.refire || l.stage === 'refire' ? STAGES : EVERYDAY_STAGES)

const DEFAULT_METHOD: Record<MaterialKind, Method> = { oxide: 'brush', slip: 'brush', commercial: 'brush', own: 'dip' }
const defaultStage = (m: Material): Stage => (m.kind === 'slip' ? 'greenware' : 'bisque')

function addLayer(m: Material) {
  const last = m.last_use
  const key = Math.max(0, ...layers.value.map((l) => l.key)) + 1
  layers.value = [...layers.value, {
    key,
    material_id: m.id,
    recipe_id: m.versions[0]?.id ?? null,
    stage: props.refire ? 'refire' : (last?.stage ?? (props.blank ? null : defaultStage(m))),
    method: last?.method ?? (props.blank ? null : DEFAULT_METHOD[m.kind]),
    thickness: last?.thickness ?? (props.blank ? null : 3),
    coats: last?.coats ?? null,
    area: '',
    notes: '',
  }]
  open.value = key // the layer before it folds away
  picking.value = false
  search.value = ''
}

async function createMaterial() {
  try {
    const m = await api.addMaterial({ name: search.value.trim(), kind: newKind.value, maker: '', notes: '', archived: false })
    emit('created', m)
    addLayer(m)
  } catch (e) {
    sayError(e)
  }
}

function remove(key: number) {
  layers.value = layers.value.filter((l) => l.key !== key)
}

function move(i: number, by: number) {
  const next = [...layers.value]
  const [item] = next.splice(i, 1)
  next.splice(i + by, 0, item!)
  layers.value = next
}

function setMethod(l: LayerDraft, method: Method | null) {
  l.method = method
  if (method !== 'brush' && method !== 'sponge') l.coats = null
}

const hasCoats = (l: LayerDraft) => l.method === 'brush' || l.method === 'sponge'
</script>

<template>
  <div class="field-group">
    <span class="section-title">What went on, from the clay out</span>
    <ol v-if="layers.length" class="layers">
      <li v-for="(l, i) in layers" :key="l.key" class="layer">
        <button type="button" class="layer-head" :aria-expanded="open === l.key" @click="open = open === l.key ? null : l.key">
          <strong>{{ i + 1 }}. {{ material(l.material_id)?.name }}</strong>
          <span class="small muted">{{ layerDetail(l) || 'Tap to add details' }}</span>
        </button>
        <div v-if="open === l.key" class="layer-body">
          <OptionalChoice :model-value="l.method" label="How" :options="METHODS" @update:model-value="setMethod(l, $event)" />
          <div v-if="hasCoats(l)" class="field-group">
            <span class="section-title">Coats</span>
            <div class="row">
              <button type="button" class="btn" aria-label="One coat fewer" :disabled="!l.coats" @click="l.coats = (l.coats ?? 1) > 1 ? l.coats! - 1 : null">−</button>
              <span class="num coats">{{ l.coats ?? '–' }}</span>
              <button type="button" class="btn" aria-label="One coat more" :disabled="(l.coats ?? 0) >= 20" @click="l.coats = (l.coats ?? 0) + 1">+</button>
            </div>
          </div>
          <div class="field-group">
            <span class="section-title">Thickness</span>
            <template v-if="l.thickness !== null">
              <PreciseSlider v-model="l.thickness" :min="0" :max="5" :step="0.1" :words="thicknessWords" :gradient="THICKNESS_GRADIENT" label="Thickness" />
              <button type="button" class="btn btn-ghost btn-small" @click="l.thickness = null">Not sure</button>
            </template>
            <button v-else type="button" class="btn" @click="l.thickness = 3">Set thickness</button>
          </div>
          <details class="more">
            <summary>More</summary>
            <div class="more-body">
              <label v-if="(material(l.material_id)?.versions.length ?? 0) > 1" class="field">Recipe version
                <select v-model="l.recipe_id" class="input">
                  <option v-for="v in material(l.material_id)?.versions" :key="v.id" :value="v.id">Version {{ v.version }}</option>
                </select>
              </label>
              <OptionalChoice v-model="l.stage" label="Applied on" :options="stageOptions(l)" />
              <label class="field">Where
                <input v-model="l.area" class="input" maxlength="100" placeholder="e.g. left half" />
              </label>
              <div class="chips">
                <button v-for="a in AREAS" :key="a" type="button" :aria-pressed="l.area === a" @click="l.area = l.area === a ? '' : a">{{ a }}</button>
              </div>
              <label class="field">Layer notes
                <input v-model="l.notes" class="input" maxlength="2000" placeholder="e.g. wiped back" />
              </label>
              <div class="row">
                <button type="button" class="btn btn-small" :disabled="i === 0" aria-label="Move earlier" @click="move(i, -1)">↑</button>
                <button type="button" class="btn btn-small" :disabled="i === layers.length - 1" aria-label="Move later" @click="move(i, 1)">↓</button>
                <button type="button" class="btn btn-small" @click="remove(l.key)">Remove this layer</button>
              </div>
            </div>
          </details>
        </div>
      </li>
    </ol>

    <button v-if="!picking" type="button" class="btn btn-block" @click="picking = true">Add a layer</button>
    <div v-else class="picker">
      <span class="section-title">Tap what went on next</span>
      <input v-model="search" class="input" type="search" placeholder="Find or add a material" aria-label="Find or add a material" />
      <div class="options">
        <button v-for="m in matches" :key="m.id" type="button" class="option" @click="addLayer(m)">
          <span class="option-text">
            <strong>{{ m.name }}</strong>
            <span class="small muted">{{ MATERIAL_KINDS[m.kind] }}</span>
          </span>
          <span class="plus" aria-hidden="true">+</span>
        </button>
        <p v-if="!matches.length && !canCreate" class="small muted">No materials yet. Type a name to add one.</p>
      </div>
      <form v-if="canCreate" class="new-material" @submit.prevent="createMaterial">
        <Choice v-model="newKind" :label="`Add “${search.trim()}” as`" :options="MATERIAL_KINDS" />
        <button type="submit" class="btn btn-primary">Add and use it</button>
      </form>
      <button type="button" class="btn btn-ghost btn-small" @click="picking = false">Cancel</button>
    </div>
  </div>
</template>

<style scoped>
.layers {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.layer,
.picker {
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
}
.layer-head {
  display: flex;
  flex-direction: column;
  width: 100%;
  min-height: var(--touch);
  padding: var(--sp-2) var(--sp-3);
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
}
.layer-body,
.new-material,
.picker {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.layer-body {
  padding: 0 var(--sp-3) var(--sp-3);
}
.picker {
  padding: var(--sp-3);
}
.options {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  max-height: 300px;
  overflow-y: auto;
}
.option {
  flex: none;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--sp-2);
  min-height: 56px;
  padding: var(--sp-2) var(--sp-3);
  border: 2px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  text-align: left;
  cursor: pointer;
  touch-action: manipulation;
}
.option:active {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.option-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.plus {
  flex: none;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--accent);
  color: var(--accent-text);
  font-size: var(--fs-lg);
  font-weight: 700;
  line-height: 1;
}
.coats {
  min-width: 2.5em;
  text-align: center;
  font-size: var(--fs-xl);
}
</style>
