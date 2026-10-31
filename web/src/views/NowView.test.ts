import { afterEach, describe, expect, it, vi } from 'vitest'
import NowView from './NowView.vue'
import { installMockFetch } from '@/test/mockFetch'
import { byTestId, flushPromises, mountWithApp } from '@/test/harness'
import { useLiveStore } from '@/stores/live'
import { useSettingsStore } from '@/stores/settings'
import { setEventSourceCtor } from '@/stores/live'
import { snapshot, type ScenarioName } from '@/mocks/fixtures'

setEventSourceCtor(null)

afterEach(() => {
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

async function mountNow(name: ScenarioName) {
  installMockFetch(name)
  const r = await mountWithApp(NowView)
  const live = useLiveStore()
  await useSettingsStore().load()
  await live.fetchStatus()
  await flushPromises()
  return r
}

const kind = () => byTestId('trim-card').getAttribute('data-kind')

describe('Now: trim card states from fixtures', () => {
  it('empty shelf offers Add pieces', async () => {
    await mountNow('empty')
    expect(kind()).toBe('empty')
    expect(byTestId('trim-card').textContent).toContain('The shelf is empty')
    expect(byTestId('trim-card').textContent).toContain('Add pieces')
  })

  it('ready now shows count, best before and thumbnails', async () => {
    await mountNow('ready_now')
    expect(kind()).toBe('ready_now')
    const t = byTestId('trim-card').textContent!
    expect(t).toContain('3 ready now')
    expect(t).toContain('best before')
    expect(t).not.toContain('come by')
    expect(byTestId('trim-card').querySelectorAll('.thumb')).toHaveLength(3)
    // No studio schedule any more: no setup banner, no at-risk block.
    expect(document.querySelector('[data-testid="studio-banner"]')).toBeNull()
    expect(document.querySelector('[data-testid="at-risk"]')).toBeNull()
  })

  it('learning adds the learning line', async () => {
    await mountNow('learning')
    expect(byTestId('trim-confidence').textContent).toBe('rough guess · learning your shelf (2 of 5)')
  })

  it('calibrated shows a visit window that expands into piece lists', async () => {
    await mountNow('calibrated')
    expect(kind()).toBe('next_visit')
    const btn = byTestId('trim-card').querySelector<HTMLButtonElement>('.visit')!
    expect(btn.getAttribute('aria-expanded')).toBe('false')
    btn.click()
    await flushPromises()
    expect(byTestId('trim-card').textContent).toMatch(/Ready|Maybe|Not yet/)
  })
})

describe('Now: banners', () => {
  it('back from offline: "Back on after 9 days · 3 pieces"', async () => {
    await mountNow('back_from_offline')
    const b = byTestId('back-online-banner').textContent!
    expect(b).toContain('Back on after 9 days')
    expect(b).toContain('3 pieces still listed on the shelf')
  })

  it('stale sensor shows a stale climate card', async () => {
    await mountNow('stale_sensor')
    expect(byTestId('freshness').textContent).toContain('stale')
  })

  it('controller offline: cached data greyed out, "as of"', async () => {
    const snap = snapshot('calibrated')
    localStorage.setItem('hc.status', JSON.stringify({ status: snap.status, saved_at: 0 }))
    localStorage.setItem('hc.shelf', JSON.stringify(snap.shelf))
    localStorage.setItem('hc.settings', JSON.stringify(snap.settings))
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    await mountWithApp(NowView)
    await useLiveStore().fetchStatus()
    await flushPromises()
    const card = byTestId('trim-card')
    expect(card.classList.contains('offline-dim')).toBe(true)
    expect(card.textContent).toMatch(/as of (today|yesterday|\w{3}) \d\d:\d\d/)
    expect(kind()).toBe('next_visit') // the last known card
  })
})
