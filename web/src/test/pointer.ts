// jsdom has no PointerEvent; build one from MouseEvent with the extra fields.
export function pointer(type: string, init: { x: number; y?: number; id?: number; pointerType?: string; button?: number }): Event {
  const e = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: init.x, clientY: init.y ?? 100, button: init.button ?? 0 })
  Object.defineProperty(e, 'pointerId', { value: init.id ?? 1 })
  Object.defineProperty(e, 'pointerType', { value: init.pointerType ?? 'touch' })
  return e
}

/** Give an element a fixed layout box. */
export function stubRect(el: Element, rect: { left: number; top: number; width: number; height: number }) {
  el.getBoundingClientRect = () =>
    ({ ...rect, right: rect.left + rect.width, bottom: rect.top + rect.height, x: rect.left, y: rect.top, toJSON: () => ({}) }) as DOMRect
}
