import { afterEach, describe, expect, it, vi } from 'vitest'
import TrimSheet from './TrimSheet.vue'
import { installMockFetch } from '@/test/mockFetch'
import { byTestId, flushPromises, mountWithApp } from '@/test/harness'
import { clearToasts, useToasts } from '@/composables/useToast'

afterEach(() => {
  clearToasts()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

describe('TrimSheet + Undo', () => {
  it('trims at 3.0 by default, then Undo sends the previous state', async () => {
    const m = installMockFetch('calibrated')
    const piece = m.backend.world.pieces.find((p) => p.state === 'flipped')!
    const { wrapper } = await mountWithApp(TrimSheet, { props: { ids: [piece.id] } })
    expect(document.body.textContent).toContain('Covering it to trim later? Set how dry it is now.')
    byTestId('confirm-trim').click()
    await flushPromises()
    expect(m.writes()[0]).toMatchObject({ method: 'PATCH', path: `/pieces/${piece.id}`, body: { state: 'trimmed', end_dryness: 3 } })
    expect(wrapper.emitted('done')).toBeTruthy()
    const toast = useToasts().toasts[0]!
    expect(toast.message).toBe(`#${piece.number} trimmed`)
    await toast.action!.run()
    await flushPromises()
    expect(m.writes().at(-1)).toMatchObject({ method: 'PATCH', path: `/pieces/${piece.id}`, body: { state: 'flipped' } })
    expect(m.backend.world.pieces.find((p) => p.id === piece.id)!.state).toBe('flipped')
  })

  it('a group trim uses PATCH /pieces with ids', async () => {
    const m = installMockFetch('calibrated')
    const ids = m.backend.world.pieces.filter((p) => p.state === 'flipped').map((p) => p.id)
    await mountWithApp(TrimSheet, { props: { ids } })
    byTestId('confirm-trim').click()
    await flushPromises()
    expect(m.writes()[0]).toMatchObject({ method: 'PATCH', path: '/pieces', body: { ids, state: 'trimmed' } })
  })
})
