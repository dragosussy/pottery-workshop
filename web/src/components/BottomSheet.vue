<script setup lang="ts">
// A modal bottom sheet. Focus moves into it, Tab is trapped inside, Esc and
// the ✕ close it, and a swipe down on the handle/header closes it (a vertical
// gesture, so it never fights the horizontal DrynessSlider).
import { nextTick, onBeforeUnmount, onMounted, ref, useId } from 'vue'

const props = withDefaults(defineProps<{ title: string; dismissible?: boolean }>(), { dismissible: true })
const emit = defineEmits<{ close: [] }>()

const panel = ref<HTMLElement | null>(null)
const titleId = useId()
const dragY = ref(0)
let startY: number | null = null
let previouslyFocused: HTMLElement | null = null

const FOCUSABLE = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusables(): HTMLElement[] {
  return panel.value ? Array.from(panel.value.querySelectorAll<HTMLElement>(FOCUSABLE)) : []
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.dismissible) {
    e.stopPropagation()
    emit('close')
    return
  }
  if (e.key !== 'Tab') return
  const els = focusables()
  if (!els.length) return
  const first = els[0]!
  const last = els[els.length - 1]!
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

function onHandleDown(e: PointerEvent) {
  if (!props.dismissible) return
  startY = e.clientY
  ;(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId)
}
function onHandleMove(e: PointerEvent) {
  if (startY === null) return
  dragY.value = Math.max(0, e.clientY - startY)
}
function onHandleUp() {
  if (startY === null) return
  const shouldClose = dragY.value > 80
  startY = null
  dragY.value = 0
  if (shouldClose) emit('close')
}

onMounted(async () => {
  previouslyFocused = document.activeElement as HTMLElement | null
  document.body.style.overflow = 'hidden'
  await nextTick()
  // Focus the panel itself (not the first input) so the keyboard doesn't pop up.
  panel.value?.focus({ preventScroll: true })
})
onBeforeUnmount(() => {
  document.body.style.overflow = ''
  previouslyFocused?.focus?.({ preventScroll: true })
})
</script>

<template>
  <Teleport to="body">
    <div class="sheet-root" @keydown="onKeydown">
      <div class="sheet-backdrop" aria-hidden="true" @click="dismissible && emit('close')"></div>
      <section
        ref="panel"
        class="sheet"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        tabindex="-1"
        :style="dragY ? { transform: `translateY(${dragY}px)`, transition: 'none' } : undefined"
      >
        <header
          class="sheet-header"
          @pointerdown="onHandleDown"
          @pointermove="onHandleMove"
          @pointerup="onHandleUp"
          @pointercancel="onHandleUp"
        >
          <div class="sheet-handle" aria-hidden="true"></div>
          <div class="sheet-title-row">
            <h2 :id="titleId" class="sheet-title">{{ title }}</h2>
            <button v-if="dismissible" type="button" class="btn btn-ghost sheet-close" aria-label="Close" @click="emit('close')" @pointerdown.stop>
              <span aria-hidden="true">✕</span>
            </button>
          </div>
        </header>
        <div class="sheet-body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="sheet-footer">
          <slot name="footer" />
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.sheet-root {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
}
.sheet-backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  animation: fade 160ms ease-out;
}
.sheet {
  position: relative;
  max-height: 94dvh;
  width: 100%;
  max-width: 640px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  background: var(--bg);
  border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  box-shadow: var(--shadow-lg);
  animation: rise 200ms ease-out;
  transition: transform 160ms ease-out;
  outline: none;
}
.sheet-header {
  padding: var(--sp-2) var(--gutter) 0;
  touch-action: none;
  cursor: grab;
}
.sheet-handle {
  width: 44px;
  height: 5px;
  border-radius: 3px;
  background: var(--border);
  margin: 0 auto var(--sp-2);
}
.sheet-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-2);
}
.sheet-title {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.sheet-close {
  font-size: var(--fs-lg);
}
.sheet-body {
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: var(--sp-3) var(--gutter) var(--sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.sheet-footer {
  padding: var(--sp-3) var(--gutter) calc(var(--sp-3) + env(safe-area-inset-bottom));
  border-top: 1px solid var(--border);
  background: var(--surface);
  display: flex;
  gap: var(--sp-2);
}
.sheet-footer :deep(.btn) {
  flex: 1;
}
@keyframes rise {
  from {
    transform: translateY(40px);
    opacity: 0.4;
  }
}
@keyframes fade {
  from {
    opacity: 0;
  }
}
</style>
