// GET/PATCH /settings. The studio time zone formats every time in the app,
// so the last settings are cached for the offline state.
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Settings, SettingsPatch } from '@/api/types'
import { api } from '@/api/endpoints'
import { loadJSON, saveJSON } from '@/utils/storage'
import { deviceTimeZone, safeTz } from '@/utils/time'


export const useSettingsStore = defineStore('settings', () => {
  const data = ref<Settings | null>(loadJSON<Settings>('settings'))
  const loading = ref(false)
  const error = ref<unknown>(null)

  /** The time zone used for display: the studio's, or the phone's until known. */
  const tz = computed(() => safeTz(data.value?.studio_tz ?? deviceTimeZone()))

  async function load(): Promise<void> {
    loading.value = true
    try {
      data.value = await api.settings()
      saveJSON('settings', data.value)
      error.value = null
    } catch (e) {
      error.value = e
    } finally {
      loading.value = false
    }
  }

  async function patch(body: SettingsPatch): Promise<Settings> {
    const s = await api.patchSettings(body)
    data.value = s
    saveJSON('settings', s)
    return s
  }

  return { data, loading, error, tz, load, patch }
})
