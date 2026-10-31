import { afterEach, describe, expect, it, vi } from 'vitest'
import AddPiecesSheet from './AddPiecesSheet.vue'
import { installMockFetch } from '@/test/mockFetch'
import { buttonByText, byTestId, flushPromises, mountWithApp } from '@/test/harness'
import { useToasts, clearToasts } from '@/composables/useToast'
import type { PhotoCreated } from '@/api/types'

afterEach(() => {
  clearToasts()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

function controllableUpload() {
  let resolve!: (v: PhotoCreated) => void
  let reject!: (e: Error) => void
  const upload = vi.fn(() => ({
    promise: new Promise<PhotoCreated>((res, rej) => {
      resolve = res
      reject = rej
    }),
    abort: vi.fn(),
  }))
  return { upload, resolve: (id: number) => resolve({ id, width: 1, height: 1, created_at: '' }), reject: (m: string) => reject(new Error(m)) }
}

async function pickPhoto() {
  const input = document.querySelector<HTMLInputElement>('input[type="file"]')!
  Object.defineProperty(input, 'files', { value: [new File(['x'], 'p.jpg', { type: 'image/jpeg' })], configurable: true })
  input.dispatchEvent(new Event('change'))
  await flushPromises()
}

describe('AddPiecesSheet', () => {
  it('adds pieces and shows the new numbers in large type', async () => {
    const m = installMockFetch('ready_now')
    await mountWithApp(AddPiecesSheet)
    const more = document.querySelector<HTMLButtonElement>('[aria-label="One more"]')!
    more.click()
    await flushPromises()
    more.click()
    await flushPromises()
    byTestId('add-submit').click()
    await flushPromises()
    const post = m.writes().find((c) => c.path === '/pieces')!
    expect(post.body).toMatchObject({ quantity: 3, form: 'mug', initial_dryness: 1, size: 'medium', thickness: 'medium' })
    expect(post.body).not.toHaveProperty('added_at') // "Now" sends no time
    expect(useToasts().toasts[0]!.big).toBe('#5 #6 #7')
  })

  it('waits for a photo upload in progress, then attaches it', async () => {
    const m = installMockFetch('calibrated')
    const up = controllableUpload()
    const resize = vi.fn(async (f: Blob) => f)
    await mountWithApp(AddPiecesSheet, { props: { photoDeps: { resize, upload: up.upload } } })
    await pickPhoto()
    expect(document.body.textContent).toContain('Uploading')
    byTestId('add-submit').click()
    await flushPromises()
    expect(byTestId('add-submit').textContent).toContain('Waiting for photo')
    expect(m.writes().filter((c) => c.path === '/pieces')).toHaveLength(0)
    up.resolve(11)
    await flushPromises()
    expect(m.writes().find((c) => c.path === '/pieces')!.body).toMatchObject({ photo_id: 11 })
  })

  it('never blocks on a failed upload: offers Retry, and Add goes without the photo', async () => {
    const m = installMockFetch('calibrated')
    const up = controllableUpload()
    await mountWithApp(AddPiecesSheet, { props: { photoDeps: { resize: async (f: Blob) => f, upload: up.upload } } })
    await pickPhoto()
    up.reject("Couldn't reach the controller")
    await flushPromises()
    expect(document.body.textContent).toContain('Retry')
    expect(up.upload).toHaveBeenCalledTimes(1) // never retried automatically
    byTestId('add-submit').click()
    await flushPromises()
    const post = m.writes().find((c) => c.path === '/pieces')!
    expect(post.body).not.toHaveProperty('photo_id')
    expect(useToasts().toasts[0]!.message).toContain('without the photo')
  })

  it('a POST with no response is not retried: refetch the shelf and ask to check', async () => {
    const m = installMockFetch('calibrated')
    await mountWithApp(AddPiecesSheet)
    m.failNextWrite('network-after')
    const shelfBefore = m.calls.filter((c) => c.path === '/shelf').length
    byTestId('add-submit').click()
    await flushPromises()
    expect(m.writes().filter((c) => c.path === '/pieces')).toHaveLength(1)
    expect(byTestId('add-unconfirmed').textContent).toContain("Couldn't confirm")
    expect(m.calls.filter((c) => c.path === '/shelf').length).toBeGreaterThan(shelfBefore)
  })

  it('an HTTP error says nothing was saved and lets the user try again', async () => {
    const m = installMockFetch('calibrated')
    await mountWithApp(AddPiecesSheet)
    // Out of range dryness is impossible from the slider; use a bad photo id instead.
    m.backend.world.photos.clear()
    m.fetchMock.mockImplementationOnce(async () => new Response(JSON.stringify({ error: { code: 'internal', message: 'Boom' } }), { status: 500 }))
    byTestId('add-submit').click()
    await flushPromises()
    expect(document.body.textContent).toContain('Boom')
    expect(buttonByText('Add')).toBeTruthy()
  })
})
