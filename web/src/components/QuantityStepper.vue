<script setup lang="ts">
import { useId } from 'vue'

const props = withDefaults(defineProps<{ min?: number; max?: number; label?: string }>(), { min: 1, max: 20, label: 'Quantity' })
const model = defineModel<number>({ required: true })
const id = useId()
const set = (v: number) => (model.value = Math.min(props.max, Math.max(props.min, v)))
</script>

<template>
  <div class="stepper">
    <span :id="id" class="field-label">{{ label }}</span>
    <div class="row" role="group" :aria-labelledby="id">
      <button type="button" class="btn step" :disabled="model <= min" aria-label="One fewer" @click="set(model - 1)">−</button>
      <output class="qty num" aria-live="polite">{{ model }}</output>
      <button type="button" class="btn step" :disabled="model >= max" aria-label="One more" @click="set(model + 1)">+</button>
    </div>
  </div>
</template>

<style scoped>
.step {
  width: 56px;
  height: 56px;
  font-size: 1.5rem;
  border-radius: 50%;
  padding: 0;
}
.qty {
  min-width: 3ch;
  text-align: center;
  font-size: var(--fs-2xl);
  font-weight: 800;
}
</style>
