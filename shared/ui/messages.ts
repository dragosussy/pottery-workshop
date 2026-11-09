// The message shown along the bottom of the screen (see MessageBar.vue).
import { reactive } from 'vue'

export const bar = reactive({
  message: null as { text: string; error: boolean; undo?: () => Promise<unknown> } | null,
})

let timer: ReturnType<typeof setTimeout> | undefined

/** Shows a message; with `undo`, it carries an Undo button for a few seconds. */
export function say(text: string, error = false, undo?: () => Promise<unknown>) {
  bar.message = { text, error, undo }
  clearTimeout(timer)
  timer = setTimeout(() => (bar.message = null), error || undo ? 8000 : 5000)
}

export function sayError(e: unknown) {
  say(e instanceof Error ? e.message : String(e), true)
}
