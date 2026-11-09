// Whether the kiln is firing, for the screens that show it wherever you are.
import { ref } from 'vue'
import { api } from './api'

export const kilnFiring = ref(false)

export async function refreshKilnStatus() {
  try {
    kilnFiring.value = (await api.firings()).some((f) => f.state === 'firing')
  } catch {
    // Only a decoration: the screen being opened reports the connection itself.
  }
}
