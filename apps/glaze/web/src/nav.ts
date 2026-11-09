import type { Router } from 'vue-router'

/** Returns to the page you came from, or to `fallback` when opened directly. */
export function goBack(router: Router, fallback: string) {
  if (window.history.state?.back) router.back()
  else void router.replace(fallback)
}
