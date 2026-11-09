<script setup lang="ts">
// Inside a material's tile: what it has done, per clay body, so both the
// results and the gaps are visible, and its recipe.
import { computed, onMounted, reactive, ref } from 'vue'
import Choice from '@ui/Choice.vue'
import { say, sayError } from '@ui/messages'
import { PAGE_SIZE, api, type ClayBody, type Level, type MaterialDetail, type MaterialKind, type Piece, type RecipeLine, type Spot, type Suggestions } from '@/api'
import ConfirmButton from '@/components/ConfirmButton.vue'
import OptionalChoice from '@/components/OptionalChoice.vue'
import PieceTile from '@/components/PieceTile.vue'
import { LEVELS, MATERIAL_KINDS, SPOTS, formatDate, positionText } from '@/format'
import { baseForWater, baseTotal, formatGrams, formatMl, formatPercent, scaleRecipe, waterFor } from '@/scale'

const props = defineProps<{ id: number }>()
const emit = defineEmits<{ changed: []; deleted: [] }>()
const id = computed(() => props.id)
const material = ref<MaterialDetail | null>(null)
const pieces = ref<Piece[]>([])
const clayBodies = ref<ClayBody[]>([])
const suggestions = ref<Suggestions>({ ingredients: [], schedules: [], cones: [] })
const view = ref<'results' | 'recipe'>('results')
const versionId = ref<number | null>(null)
const batch = ref<number | ''>('') // nothing to weigh until you say how much
const details = reactive({ name: '', kind: 'own' as MaterialKind, maker: '', notes: '', archived: false })
const editingDetails = ref(false)
const editingRecipe = ref(false)
// An amount is '' while the box is empty.
type DraftLine = Omit<RecipeLine, 'amount'> & { amount: number | '' }
const draftLines = ref<DraftLine[]>([])
const draftWater = ref<number | ''>('')
const draftConsistency = ref('')
const changeNote = ref('')
const schedule = ref('')
const level = ref<Level | null>(null)
const spot = ref<Spot | null>(null)

const hasRecipes = computed(() => material.value?.kind !== 'commercial')
const recipe = computed(() => material.value?.recipes.find((r) => r.id === versionId.value) ?? material.value?.recipes[0])
const latest = computed(() => material.value?.recipes[0])
// An oxide wash is the oxide and its water, and you start from the water.
const isWash = computed(() => material.value?.kind === 'oxide')
const water = computed(() => recipe.value?.water ?? null)
const byWater = computed(() => isWash.value && water.value !== null)
const baseBatch = computed(() => {
  if (!batch.value || !recipe.value) return 0
  return byWater.value ? baseForWater(recipe.value.lines, water.value!, batch.value) : batch.value
})
const waterToAdd = computed(() => (recipe.value && water.value ? waterFor(recipe.value.lines, water.value, baseBatch.value) : 0))
const waterShare = computed(() => (recipe.value && water.value ? (water.value / baseTotal(recipe.value.lines)) * 100 : 0))
const batchLabel = computed(() => (byWater.value ? 'How much water will you use? (ml)' : isWash.value ? 'How much oxide will you use? (grams)' : 'How much base do you want to mix? (grams)'))
const scaled = computed(() => (recipe.value ? scaleRecipe(recipe.value.lines, baseBatch.value) : []))
const scaledBase = computed(() => scaled.value.filter((l) => !l.addition))
const scaledOnTop = computed(() => scaled.value.filter((l) => l.addition))
const totalGrams = computed(() => scaled.value.reduce((sum, l) => sum + l.grams, 0))
const draftBase = computed(() => draftLines.value.filter((l) => !l.addition))
const draftOnTop = computed(() => draftLines.value.filter((l) => l.addition))

// The share of each line as you type it, so what "parts" means is visible.
function draftPercent(line: DraftLine): string {
  if (!line.amount || isWash.value) return ''
  const i = draftLines.value.indexOf(line)
  const numbers = draftLines.value.map((l) => ({ ...l, amount: Number(l.amount) || 0 }))
  return formatPercent(scaleRecipe(numbers, 100)[i]!.percent)
}

const shown = computed(() =>
  pieces.value.filter((p) => (!schedule.value || p.firing?.schedule === schedule.value) && (!level.value || p.level === level.value) && (!spot.value || p.spot === spot.value)),
)
const byClay = computed(() => {
  const groups = clayBodies.value
    .filter((c) => !c.archived || shown.value.some((p) => p.clay_body_id === c.id))
    .map((c) => ({ key: String(c.id), name: c.name, pieces: shown.value.filter((p) => p.clay_body_id === c.id) }))
  const unknown = shown.value.filter((p) => p.clay_body_id === null)
  if (unknown.length) groups.push({ key: 'unknown', name: 'Clay not recorded', pieces: unknown })
  return groups
})
const caption = (p: Piece) => [p.firing?.schedule, positionText(p)].filter(Boolean).join(' · ')

// A material's results come in pages; this page needs all of them to group.
async function allPieces(material: number): Promise<Piece[]> {
  const all: Piece[] = []
  for (;;) {
    const page = await api.pieces({ material, offset: all.length })
    all.push(...page)
    if (page.length < PAGE_SIZE) return all
  }
}

async function load() {
  try {
    ;[material.value, pieces.value, clayBodies.value, suggestions.value] = await Promise.all([api.material(id.value), allPieces(id.value), api.clayBodies(), api.suggestions()])
    versionId.value = material.value.recipes[0]?.id ?? null
    // Something never used has no results to show, so start on its recipe.
    if (!material.value.uses && hasRecipes.value) view.value = 'recipe'
  } catch (e) {
    sayError(e)
  }
}
onMounted(load)

function startDetails() {
  if (!material.value) return
  const { name, kind, maker, notes, archived } = material.value
  Object.assign(details, { name, kind, maker, notes, archived })
  editingDetails.value = true
}

async function saveDetails() {
  try {
    material.value = await api.saveMaterial(id.value, details)
    editingDetails.value = false
    emit('changed')
  } catch (e) {
    sayError(e)
  }
}

async function setArchived(archived: boolean) {
  if (!material.value) return
  const { name, kind, maker, notes } = material.value
  try {
    material.value = await api.saveMaterial(id.value, { name, kind, maker, notes, archived })
    emit('changed')
  } catch (e) {
    sayError(e)
  }
}

async function remove() {
  try {
    await api.deleteMaterial(id.value)
    emit('deleted')
  } catch (e) {
    sayError(e)
  }
}

function startRecipe() {
  draftLines.value = latest.value ? latest.value.lines.map((l) => ({ ...l })) : [{ ingredient: '', amount: '', addition: false }]
  draftWater.value = latest.value?.water ?? ''
  draftConsistency.value = latest.value?.consistency ?? ''
  changeNote.value = ''
  editingRecipe.value = true
}

// The water as you type it, as a share of the dry base.
const draftWaterShare = computed(() => {
  const base = baseTotal(draftLines.value.map((l) => ({ ...l, amount: Number(l.amount) || 0 })))
  return draftWater.value && base > 0 && !isWash.value ? formatPercent((draftWater.value / base) * 100) : ''
})

const addLine = (addition: boolean) => draftLines.value.push({ ingredient: '', amount: '', addition })
const removeLine = (line: DraftLine) => draftLines.value.splice(draftLines.value.indexOf(line), 1)

async function saveRecipe() {
  const before = latest.value?.version
  try {
    const lines = draftLines.value.filter((l) => l.ingredient.trim() || l.amount).map((l) => ({ ...l, amount: Number(l.amount) || 0 }))
    material.value = await api.saveRecipe(id.value, {
      change_note: changeNote.value, water: draftWater.value || null, consistency: draftConsistency.value.trim(), lines,
    })
    versionId.value = material.value.recipes[0]?.id ?? null
    editingRecipe.value = false
    const now = material.value.recipes[0]?.version
    say(now === before ? 'Recipe corrected' : `Saved as version ${now}`)
  } catch (e) {
    sayError(e)
  }
}
</script>

<template>
  <div v-if="material" class="panel">
      <p v-if="material.maker" class="muted">{{ material.maker }}</p>
      <p v-if="material.notes">{{ material.notes }}</p>

      <form v-if="editingDetails" class="card" @submit.prevent="saveDetails">
        <label class="field">Name <input v-model="details.name" class="input" maxlength="100" /></label>
        <Choice v-model="details.kind" label="Kind" :options="MATERIAL_KINDS" />
        <label class="field">Brand and colour code <input v-model="details.maker" class="input" maxlength="200" /></label>
        <label class="field">Notes <textarea v-model="details.notes" class="input" maxlength="2000"></textarea></label>
        <button type="submit" class="btn btn-primary btn-block">Save</button>
      </form>

      <div v-if="hasRecipes" class="chips" role="group" aria-label="View">
        <button type="button" :aria-pressed="view === 'results'" @click="view = 'results'">Results</button>
        <button type="button" :aria-pressed="view === 'recipe'" @click="view = 'recipe'">Recipe</button>
      </div>

      <template v-if="view === 'results' || !hasRecipes">
        <details class="more">
          <summary>Filter by firing</summary>
          <div class="more-body">
            <label v-if="suggestions.schedules.length" class="field">Schedule
              <select v-model="schedule" class="input">
                <option value="">Any schedule</option>
                <option v-for="s in suggestions.schedules" :key="s" :value="s">{{ s }}</option>
              </select>
            </label>
            <OptionalChoice v-model="level" label="Shelf" :options="LEVELS" />
            <OptionalChoice v-model="spot" label="On the shelf" :options="SPOTS" />
          </div>
        </details>
        <div v-for="g in byClay" :key="g.key" class="rows">
          <strong>{{ g.name }}</strong>
          <div v-if="g.pieces.length" class="tiles">
            <PieceTile v-for="p in g.pieces" :key="p.id" :piece="p" :caption="caption(p)" />
          </div>
          <p v-else class="small muted">{{ schedule || level || spot ? 'Nothing matches.' : 'Not tried on this clay yet.' }}</p>
        </div>
        <p v-if="!byClay.length" class="muted">No results yet.</p>
      </template>

      <section v-if="view === 'recipe' && hasRecipes" class="recipe">
        <template v-if="recipe && !editingRecipe">
          <div v-if="material.recipes.length > 1" class="chips" role="group" aria-label="Version">
            <button v-for="r in material.recipes" :key="r.id" type="button" :aria-pressed="r.id === recipe.id" @click="versionId = r.id">v{{ r.version }}</button>
          </div>
          <p class="small muted">Version {{ recipe.version }}, {{ formatDate(recipe.created_at) }}<template v-if="recipe.change_note">: {{ recipe.change_note }}</template></p>
          <table class="lines">
            <thead><tr><th>Ingredient</th><th class="right">{{ isWash ? 'Parts' : 'Share' }}</th><th v-if="batch" class="right">To weigh</th></tr></thead>
            <tbody>
              <tr v-for="(l, i) in scaledBase" :key="i">
                <td>{{ l.ingredient }}</td>
                <td class="right num">{{ isWash ? l.amount : formatPercent(l.percent) }}</td>
                <td v-if="batch" class="right num">{{ formatGrams(l.grams) }}</td>
              </tr>
            </tbody>
            <tbody v-if="scaledOnTop.length">
              <tr><th colspan="3" class="group">On top of the base</th></tr>
              <tr v-for="(l, i) in scaledOnTop" :key="i">
                <td>{{ l.ingredient }}</td>
                <td class="right num">+ {{ formatPercent(l.percent) }}</td>
                <td v-if="batch" class="right num">{{ formatGrams(l.grams) }}</td>
              </tr>
            </tbody>
            <tfoot v-if="water || (batch && scaledOnTop.length)">
              <tr v-if="batch && scaledOnTop.length"><th>Total dry</th><td></td><td class="right num">{{ formatGrams(totalGrams) }}</td></tr>
              <tr v-if="water">
                <th>{{ isWash ? 'Water' : 'Water to start with' }}</th>
                <td class="right num">{{ isWash ? water : formatPercent(waterShare) }}</td>
                <td v-if="batch" class="right num">{{ formatMl(waterToAdd) }}</td>
              </tr>
            </tfoot>
          </table>
          <p v-if="recipe.consistency"><strong>Aim for:</strong> {{ recipe.consistency }}</p>
          <label class="field">{{ batchLabel }}
            <input v-model.number="batch" class="input num" type="number" inputmode="numeric" min="0" placeholder="e.g. 3000" />
          </label>
        </template>
        <p v-else-if="!editingRecipe" class="muted">No recipe yet.</p>

        <form v-if="editingRecipe" class="lines-form" @submit.prevent="saveRecipe">
          <datalist id="ingredients"><option v-for="s in suggestions.ingredients" :key="s" :value="s"></option></datalist>
          <p class="small muted">{{ isWash ? 'Parts are parts by weight: 10 of oxide and 100 of water means 10 g of oxide in 100 ml of water. Add CMC or a thickener the same way.' : 'Parts are parts by weight, and only the proportions matter: 40 and 60 is the same recipe as 400 and 600.' }}</p>
          <div class="line head small muted"><span>{{ isWash ? 'What goes in' : 'The base' }}</span><span>Parts</span><span>{{ isWash ? '' : 'Share' }}</span><span></span></div>
          <div v-for="l in draftBase" :key="draftLines.indexOf(l)" class="line">
            <input v-model="l.ingredient" class="input" list="ingredients" maxlength="100" :placeholder="isWash ? 'e.g. Copper carbonate, CMC' : 'e.g. Feldspar'" aria-label="Ingredient" />
            <input v-model.number="l.amount" class="input num amount" type="number" inputmode="decimal" step="any" min="0" aria-label="Parts" />
            <span class="small muted num">{{ draftPercent(l) }}</span>
            <button type="button" class="btn btn-small" aria-label="Remove ingredient" @click="removeLine(l)">✕</button>
          </div>
          <button type="button" class="btn" @click="addLine(false)">{{ isWash ? 'Add an ingredient' : 'Add to the base' }}</button>

          <template v-if="!isWash || draftOnTop.length">
          <div class="line head small muted"><span>On top of the base</span><span>Parts</span><span>Share</span><span></span></div>
          <p class="small muted">Oxides and colourants, measured against the base. 2 here with a base of 100 parts means 2 g for every 100 g of base.</p>
          <div v-for="l in draftOnTop" :key="draftLines.indexOf(l)" class="line">
            <input v-model="l.ingredient" class="input" list="ingredients" maxlength="100" placeholder="e.g. Red iron oxide" aria-label="Ingredient" />
            <input v-model.number="l.amount" class="input num amount" type="number" inputmode="decimal" step="any" min="0" aria-label="Parts" />
            <span class="small muted num">{{ draftPercent(l) }}</span>
            <button type="button" class="btn btn-small" aria-label="Remove ingredient" @click="removeLine(l)">✕</button>
          </div>
          <button type="button" class="btn" @click="addLine(true)">Add on top</button>
          </template>

          <div class="line head small muted"><span>Water</span><span>Parts</span><span>{{ isWash ? '' : 'Share' }}</span><span></span></div>
          <div class="line">
            <span class="small">{{ isWash ? 'Water' : 'To start with' }}</span>
            <input v-model.number="draftWater" class="input num amount" type="number" inputmode="decimal" step="any" min="0" aria-label="Water" />
            <span class="small muted num">{{ draftWaterShare }}</span>
            <span></span>
          </div>
          <p class="small muted">1 ml of water weighs 1 g, so it uses the same parts as everything else.</p>
          <label class="field">Consistency to aim for
            <input v-model="draftConsistency" class="input" maxlength="200" placeholder="e.g. thin cream" />
          </label>

          <label class="field">What changed?
            <input v-model="changeNote" class="input" maxlength="500" placeholder="e.g. more silica, it was running" />
          </label>
          <p v-if="latest?.used" class="small muted">Version {{ latest.version }} has been used on pieces, so this saves as version {{ latest.version + 1 }}.</p>
          <button type="submit" class="btn btn-primary btn-block">Save recipe</button>
          <button type="button" class="btn btn-ghost btn-block" @click="editingRecipe = false">Cancel</button>
        </form>
        <button v-else type="button" class="btn btn-block" @click="startRecipe">{{ latest ? 'Change the recipe' : 'Add a recipe' }}</button>
      </section>

      <button v-if="!editingDetails" type="button" class="btn btn-block" @click="startDetails">Edit name and details</button>
      <button v-if="material.uses && !material.archived" type="button" class="btn btn-block" @click="setArchived(true)">Archive</button>
      <button v-if="material.archived" type="button" class="btn btn-block" @click="setArchived(false)">Bring back</button>
      <ConfirmButton v-if="!material.uses" class="btn-block" label="Delete this material" @confirmed="remove" />
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.recipe,
.lines-form {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.lines {
  width: 100%;
  border-collapse: collapse;
}
.lines th,
.lines td {
  padding: var(--sp-2) 0;
  border-bottom: 1px solid var(--border);
  text-align: left;
}
.lines .right {
  text-align: right;
}
.lines .group {
  padding-top: var(--sp-3);
  color: var(--text-muted);
  font-size: var(--fs-sm);
}
.lines tfoot th,
.lines tfoot td {
  border-bottom: 0;
}
.line {
  display: grid;
  grid-template-columns: 1fr 4.5em 3.6em 36px;
  gap: var(--sp-2);
  align-items: center;
}
.line.head {
  font-weight: 600;
}
.amount {
  padding-inline: var(--sp-2);
}
</style>
