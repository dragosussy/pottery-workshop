import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import CheckView from './CheckView.vue'
import { installMockFetch } from '@/test/mockFetch'
import { byTestId, flushPromises, mountWithApp } from '@/test/harness'
import { clearToasts } from '@/composables/useToast'
import { shelfOf, FIXED_NOW } from '@/mocks/fixtures'

afterEach(() => {
  clearToasts()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

const track = () => document.querySelector<HTMLElement>('[role="slider"]')!

describe('Check-in', () => {
  it('orders "check these" first, then the rest by number', async () => {
    const m = installMockFetch('learning')
    setActivePinia(createPinia())
    const shelf = shelfOf(m.backend.world, FIXED_NOW)
    const first = shelf.check_these[0]!.piece_id
    await mountWithApp(CheckView, { path: '/check' })
    const p = shelf.items.find((x) => x.id === first)!
    expect(byTestId('check-card').textContent).toContain(`#${p.number}`)
    expect(document.body.textContent).toContain(`1 of ${shelf.items.length}`)
  })

  it('pre-fills the slider with the last observed value, untouched, and never shows the estimate', async () => {
    const m = installMockFetch('calibrated')
    const piece = m.backend.world.pieces.find((p) => p.state === 'flipped')!
    await mountWithApp(CheckView, { path: `/check?ids=${piece.id}` })
    expect(track().getAttribute('aria-valuenow')).toBe(String(piece.current_dryness))
    expect(document.body.textContent).toContain('same as last time?')
    expect(document.querySelector('[data-testid="estimate-marker"]')).toBeNull()
    expect(document.querySelector('[data-testid="last-marker"]')).not.toBeNull()
  })

  it('Skip sends nothing', async () => {
    const m = installMockFetch('calibrated')
    const ids = m.backend.world.pieces.filter((p) => p.state === 'flipped').slice(0, 2).map((p) => p.id)
    await mountWithApp(CheckView, { path: `/check?ids=${ids.join(',')}` })
    byTestId('skip').click()
    await flushPromises()
    expect(m.writes()).toHaveLength(0)
    expect(document.body.textContent).toContain('2 of 2')
  })

  it('an untouched Save sends the last value, as one item', async () => {
    const m = installMockFetch('calibrated')
    const piece = m.backend.world.pieces.find((p) => p.state === 'flipped')!
    await mountWithApp(CheckView, { path: `/check?ids=${piece.id}` })
    byTestId('save-next').click()
    await flushPromises()
    const w = m.writes()
    expect(w).toHaveLength(1)
    expect(w[0]).toMatchObject({ method: 'POST', path: '/observations', body: { items: [{ piece_id: piece.id, dryness: piece.current_dryness }] } })
    expect(w[0]!.body).not.toHaveProperty('observed_at')
    expect(byTestId('check-done').textContent).toContain('Checked 1 · skipped 0')
  })

  it('a moved slider sends the new value', async () => {
    const m = installMockFetch('calibrated')
    const piece = m.backend.world.pieces.find((p) => p.state === 'flipped')!
    const before = piece.current_dryness!
    await mountWithApp(CheckView, { path: `/check?ids=${piece.id}` })
    track().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    await flushPromises()
    byTestId('save-next').click()
    await flushPromises()
    expect(m.writes()[0]!.body).toMatchObject({ items: [{ dryness: Math.round((before + 0.1) * 10) / 10 }] })
  })

  it('shows a follow-up on the same card before moving on', async () => {
    const m = installMockFetch('calibrated')
    const piece = m.backend.world.pieces.find((p) => p.state === 'flipped')!
    const other = m.backend.world.pieces.find((p) => p.state === 'in')!
    await mountWithApp(CheckView, { path: `/check?ids=${piece.id},${other.id}` })
    for (let i = 0; i < 25; i++) track().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }))
    await flushPromises()
    byTestId('save-next').click()
    await flushPromises()
    expect(byTestId('follow-up').textContent).toContain('Wetter than last time')
    expect(document.body.textContent).toContain('1 of 2')
    Array.from(document.querySelectorAll('button')).find((b) => b.textContent === "It's fine")!.click()
    await flushPromises()
    expect(m.writes().at(-1)).toMatchObject({ method: 'POST', path: expect.stringMatching(/\/follow-up$/), body: { answer: 'its_fine' } })
    expect(document.body.textContent).toContain('2 of 2')
  })

  it('a save with no response is not retried and stays on the piece', async () => {
    const m = installMockFetch('calibrated')
    const piece = m.backend.world.pieces.find((p) => p.state === 'flipped')!
    await mountWithApp(CheckView, { path: `/check?ids=${piece.id}` })
    m.failNextWrite('network')
    byTestId('save-next').click()
    await flushPromises()
    expect(m.writes()).toHaveLength(1)
    expect(byTestId('check-error').textContent).toContain("Couldn't confirm")
    expect(byTestId('save-next').textContent).toContain('Save again')
    expect(document.querySelector('[data-testid="check-card"]')).not.toBeNull()
  })
})
