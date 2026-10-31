// Resize a camera photo on the phone before upload (contract §3.6): 1600px on
// the long edge, JPEG 0.8. Drawing through createImageBitmap with
// imageOrientation "from-image" bakes the rotation in, and the canvas drops
// EXIF (including GPS). Works over plain HTTP.

export const MAX_EDGE = 1600
export const QUALITY = 0.8

export function fitWithin(w: number, h: number, max = MAX_EDGE): { width: number; height: number } {
  const scale = Math.min(1, max / Math.max(w, h))
  return { width: Math.max(1, Math.round(w * scale)), height: Math.max(1, Math.round(h * scale)) }
}

async function decode(file: Blob): Promise<ImageBitmap | HTMLImageElement> {
  if (typeof createImageBitmap === 'function') {
    try {
      return await createImageBitmap(file, { imageOrientation: 'from-image' })
    } catch {
      try {
        return await createImageBitmap(file)
      } catch {
        // fall through to <img>
      }
    }
  }
  const url = URL.createObjectURL(file)
  try {
    const img = new Image()
    img.decoding = 'async'
    img.src = url
    await img.decode()
    return img
  } finally {
    URL.revokeObjectURL(url)
  }
}

export async function resizeToJpeg(file: Blob, max = MAX_EDGE, quality = QUALITY): Promise<Blob> {
  const src = await decode(file)
  const w = 'naturalWidth' in src ? src.naturalWidth : src.width
  const h = 'naturalHeight' in src ? src.naturalHeight : src.height
  const size = fitWithin(w, h, max)
  const canvas = document.createElement('canvas')
  canvas.width = size.width
  canvas.height = size.height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('No canvas')
  ctx.drawImage(src, 0, 0, size.width, size.height)
  if ('close' in src) src.close()
  return new Promise<Blob>((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('Could not encode the photo'))), 'image/jpeg', quality),
  )
}
