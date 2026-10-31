<script setup lang="ts">
// A follow-up question after a save (contract §3.9). Always skippable; the
// server already recorded that it asked, so skipping needs no call.
import { ref } from 'vue'
import type { FollowUp, FollowUpOption } from '@/api/types'
import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/client'

const props = defineProps<{ followUp: FollowUp }>()
const emit = defineEmits<{ answered: [option: FollowUpOption]; skipped: [] }>()
const busy = ref(false)
const error = ref('')

async function answer(o: FollowUpOption) {
  busy.value = true
  error.value = ''
  try {
    await api.answerFollowUp(props.followUp.observation_id, o.key)
    emit('answered', o)
  } catch (e) {
    error.value = errorMessage(e, "Couldn't save the answer")
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="followup" role="group" aria-labelledby="fu-q" data-testid="follow-up">
    <p id="fu-q" class="q">❓ {{ followUp.question }}</p>
    <div class="opts">
      <button v-for="o in followUp.options" :key="o.key" type="button" class="btn" :disabled="busy" @click="answer(o)">{{ o.label }}</button>
      <button type="button" class="btn btn-ghost" :disabled="busy" @click="emit('skipped')">Skip</button>
    </div>
    <p v-if="error" class="err" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.followup {
  padding: var(--sp-3);
  border-radius: var(--radius);
  background: var(--info-soft);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.q {
  font-weight: 700;
}
.opts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
}
.err {
  color: var(--danger);
}
</style>
