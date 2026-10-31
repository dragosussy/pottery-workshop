// Photo: resize on the phone, then upload in the background while the form is
// filled in. POST /photos is never retried automatically (contract §1.2);
// the tile offers Retry. A duplicate is never attached, so it's harmless.
import { computed, onBeforeUnmount, ref } from 'vue'
import type { PhotoCreated } from '@/api/types'
import { API_BASE } from '@/api/client'
import { resizeToJpeg } from '@/utils/image'

export type PhotoState = 'empty' | 'resizing' | 'uploading' | 'done' | 'error'

/** XHR so we get upload progress (fetch has none). */
export function uploadJpeg(blob: Blob, onProgress: (f: number) => void): { promise: Promise<PhotoCreated>; abort: () => void } {
  const xhr = new XMLHttpRequest()
  const promise = new Promise<PhotoCreated>((resolve, reject) => {
    xhr.open('POST', `${API_BASE}/photos`)
    xhr.setRequestHeader('Content-Type', 'image/jpeg')
    xhr.setRequestHeader('Accept', 'application/json')
    xhr.timeout = 60_000
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total)
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText) as PhotoCreated)
        } catch {
          reject(new Error('Bad response'))
        }
      } else {
        let msg = `Upload failed (${xhr.status})`
        try {
          msg = (JSON.parse(xhr.responseText) as { error?: { message?: string } }).error?.message ?? msg
        } catch {
          // keep default
        }
        reject(new Error(msg))
      }
    }
    xhr.onerror = () => reject(new Error("Couldn't reach the controller"))
    xhr.ontimeout = () => reject(new Error('The upload timed out'))
    xhr.onabort = () => reject(new Error('aborted'))
    xhr.send(blob)
  })
  return { promise, abort: () => xhr.abort() }
}

export interface PhotoUploadDeps {
  resize?: (f: Blob) => Promise<Blob>
  upload?: typeof uploadJpeg
}

export function usePhotoUpload(deps: PhotoUploadDeps = {}) {
  const resize = deps.resize ?? resizeToJpeg
  const upload = deps.upload ?? uploadJpeg
  const state = ref<PhotoState>('empty')
  const progress = ref(0)
  const photoId = ref<number | null>(null)
  const previewUrl = ref<string | null>(null)
  const error = ref('')
  let blob: Blob | null = null
  let abort: (() => void) | null = null
  let token = 0
  let done: Promise<void> = Promise.resolve()

  function setPreview(b: Blob | null) {
    if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
    previewUrl.value = b && typeof URL.createObjectURL === 'function' ? URL.createObjectURL(b) : null
  }

  async function run(my: number) {
    if (!blob) return
    state.value = 'uploading'
    progress.value = 0
    error.value = ''
    const job = upload(blob, (f) => {
      if (my === token) progress.value = f
    })
    abort = job.abort
    try {
      const res = await job.promise
      if (my !== token) return
      photoId.value = res.id
      state.value = 'done'
    } catch (e) {
      if (my !== token) return
      state.value = 'error'
      error.value = e instanceof Error ? e.message : 'Upload failed'
    } finally {
      if (my === token) abort = null
    }
  }

  function pick(file: File) {
    const my = ++token
    abort?.()
    photoId.value = null
    state.value = 'resizing'
    error.value = ''
    done = (async () => {
      try {
        blob = await resize(file)
      } catch {
        if (my !== token) return
        state.value = 'error'
        error.value = "Couldn't read that photo"
        return
      }
      if (my !== token) return
      setPreview(blob)
      await run(my)
    })()
  }

  /** Manual retry after a failure (never automatic). */
  function retry() {
    if (!blob) return
    const my = ++token
    done = run(my)
  }

  function clear() {
    token++
    abort?.()
    abort = null
    blob = null
    photoId.value = null
    state.value = 'empty'
    progress.value = 0
    error.value = ''
    setPreview(null)
    done = Promise.resolve()
  }

  /** Resolves once any upload in progress has finished (successfully or not). */
  function settled(): Promise<void> {
    return done
  }

  const busy = computed(() => state.value === 'resizing' || state.value === 'uploading')

  onBeforeUnmount(() => {
    if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  })

  return { state, progress, photoId, previewUrl, error, busy, pick, retry, clear, settled }
}
