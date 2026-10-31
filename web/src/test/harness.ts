// Mount a component with Pinia, a memory router (all app routes) and a body
// to teleport sheets into.
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Component } from 'vue'
import { routes } from '@/router'

export async function mountWithApp(component: Component, opts: { props?: Record<string, unknown>; path?: string } = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(opts.path ?? '/')
  await router.isReady()
  const wrapper = mount(component, { props: opts.props, global: { plugins: [pinia, router] }, attachTo: document.body })
  await flushPromises()
  return { wrapper, router, pinia }
}

/** Find a button in the whole document (sheets are teleported) by its text. */
export function buttonByText(text: string | RegExp): HTMLButtonElement {
  const all = Array.from(document.querySelectorAll('button'))
  const b = all.find((x) => (typeof text === 'string' ? x.textContent?.trim() === text : text.test(x.textContent ?? '')))
  if (!b) throw new Error(`No button "${text}" in: ${all.map((x) => x.textContent?.trim()).join(' | ')}`)
  return b
}

export function byTestId<T extends Element = HTMLElement>(id: string): T {
  const el = document.querySelector<T>(`[data-testid="${id}"]`)
  if (!el) throw new Error(`No [data-testid="${id}"]`)
  return el
}

export { flushPromises }
