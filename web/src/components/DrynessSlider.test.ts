import { describe, expect, it, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import DrynessSlider from './DrynessSlider.vue'
import { pointer, stubRect } from '@/test/pointer'

// Track: 0 at x = 18, 5 at x = 318 (300px wide, inset 18) → 6px per 0.1.
const RECT = { left: 0, top: 72, width: 336, height: 56 }
const xOf = (v: number) => 18 + (v / 5) * 300

function setup(props: Record<string, unknown> = {}, start = 2.3) {
  const value = ref(start)
  const touched = ref(false)
  const commits: number[] = []
  const Host = defineComponent({
    setup: () => () =>
      h(DrynessSlider, {
        modelValue: value.value,
        'onUpdate:modelValue': (v: number) => (value.value = v),
        touched: touched.value,
        'onUpdate:touched': (v: boolean) => (touched.value = v),
        onCommit: (v: number) => commits.push(v),
        label: 'How dry is #6?',
        ...props,
      }),
  })
  const wrapper = mount(Host, { attachTo: document.body })
  const track = wrapper.get('[role="slider"]')
  stubRect(track.element, RECT)
  ;(track.element as HTMLElement).setPointerCapture = vi.fn()
  return { wrapper, track, value, touched, commits }
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.useRealTimers()
})

describe('DrynessSlider', () => {
  it('exposes slider semantics with the label as value text', () => {
    const { track } = setup({}, 2.8)
    expect(track.attributes('aria-label')).toBe('How dry is #6?')
    expect(track.attributes('aria-valuemin')).toBe('0')
    expect(track.attributes('aria-valuemax')).toBe('5')
    expect(track.attributes('aria-valuenow')).toBe('2.8')
    expect(track.attributes('aria-valuetext')).toContain('Leather hard · a touch soft')
    expect(track.attributes('aria-valuetext')).toContain('in the trim band')
    expect(track.attributes('tabindex')).toBe('0')
  })

  it('shows the untouched state until the value is moved', async () => {
    const { wrapper, track, touched } = setup()
    expect(wrapper.find('.ds').classes()).toContain('is-untouched')
    expect(wrapper.get('[data-testid="untouched-hint"]').text()).toBe('same as last time?')
    await track.trigger('keydown', { key: 'ArrowRight' })
    expect(touched.value).toBe(true)
    expect(wrapper.find('.ds').classes()).not.toContain('is-untouched')
    expect(wrapper.find('[data-testid="untouched-hint"]').exists()).toBe(false)
  })

  it('shows the big readout label', () => {
    const { wrapper } = setup({}, 2.4)
    expect(wrapper.get('[data-testid="readout"]').text()).toContain('Soft leather hard')
    expect(wrapper.get('[data-testid="readout"]').text()).toContain('firming up')
  })

  it('draws the last-observed ring and the estimate tick only when given', () => {
    const plain = setup()
    expect(plain.wrapper.find('[data-testid="last-marker"]').exists()).toBe(false)
    expect(plain.wrapper.find('[data-testid="estimate-marker"]').exists()).toBe(false)
    plain.wrapper.unmount()

    const { wrapper } = setup({ lastValue: 2.3, estimate: 2.6 })
    expect(wrapper.find('[data-testid="last-marker"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="estimate-marker"]').attributes('style')).toContain('--p: 0.52')
    expect(wrapper.find('[data-testid="trim-band"]').exists()).toBe(true)
  })

  it('keyboard: arrows ±0.1, PageUp/PageDown ±1', async () => {
    const { track, value } = setup({}, 2.3)
    await track.trigger('keydown', { key: 'ArrowRight' })
    expect(value.value).toBe(2.4)
    await track.trigger('keydown', { key: 'PageUp' })
    expect(value.value).toBe(3.4)
    await track.trigger('keydown', { key: 'ArrowLeft' })
    expect(value.value).toBe(3.3)
    await track.trigger('keydown', { key: 'PageDown' })
    expect(value.value).toBe(2.3)
  })

  it('a tap moves the thumb to the tapped point', async () => {
    const { track, value, touched, commits } = setup({}, 1.0)
    track.element.dispatchEvent(pointer('pointerdown', { x: xOf(3.2) }))
    track.element.dispatchEvent(pointer('pointerup', { x: xOf(3.2) }))
    await Promise.resolve()
    expect(value.value).toBe(3.2)
    expect(touched.value).toBe(true)
    expect(commits).toEqual([3.2])
  })

  it('a vertical swipe over the track never changes the value', async () => {
    const { track, value, touched } = setup({}, 2.3)
    const x = xOf(4)
    track.element.dispatchEvent(pointer('pointerdown', { x, y: 100 }))
    track.element.dispatchEvent(pointer('pointermove', { x: x + 3, y: 110 }))
    track.element.dispatchEvent(pointer('pointermove', { x: x + 4, y: 140 }))
    track.element.dispatchEvent(pointer('pointercancel', { x: x + 4, y: 140 })) // the browser took over scrolling
    track.element.dispatchEvent(pointer('pointerup', { x: x + 4, y: 160 }))
    await Promise.resolve()
    expect(value.value).toBe(2.3)
    expect(touched.value).toBe(false)
  })

  it('drags, shows the floating readout, captures the pointer and commits on release', async () => {
    const { wrapper, track, value, commits } = setup({}, 2.0)
    const x0 = xOf(2)
    track.element.dispatchEvent(pointer('pointerdown', { x: x0, y: 100 }))
    track.element.dispatchEvent(pointer('pointermove', { x: x0 + 12, y: 101 }))
    await wrapper.vm.$nextTick()
    expect(value.value).toBe(2.2)
    expect((track.element as HTMLElement).setPointerCapture).toHaveBeenCalled()
    expect(wrapper.find('.ds-bubble').exists()).toBe(true)
    // Fine mode: finger 130px below the track centre → ¼×.
    track.element.dispatchEvent(pointer('pointermove', { x: x0 + 12, y: 100 + 130 }))
    track.element.dispatchEvent(pointer('pointermove', { x: x0 + 36, y: 100 + 130 })) // 24px at ¼× = 0.1
    await wrapper.vm.$nextTick()
    expect(value.value).toBe(2.3)
    expect(wrapper.find('.ds-fine').text()).toContain('¼')
    track.element.dispatchEvent(pointer('pointerup', { x: x0 + 36, y: 230 }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.ds-bubble').exists()).toBe(false)
    expect(commits).toEqual([2.3])
  })

  it('nudge buttons step by 0.1 and repeat while held', async () => {
    vi.useFakeTimers()
    const { wrapper, value } = setup({}, 2.3)
    const plus = wrapper.get('[data-testid="nudge-plus"]')
    plus.element.dispatchEvent(pointer('pointerdown', { x: 0 }))
    expect(value.value).toBe(2.4)
    vi.advanceTimersByTime(400 + 90 * 2)
    expect(value.value).toBe(2.7)
    plus.element.dispatchEvent(pointer('pointerup', { x: 0 }))
    vi.advanceTimersByTime(1000)
    expect(value.value).toBe(2.7)
    // A pointer click after pointerdown must not double-step…
    plus.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    expect(value.value).toBe(2.7)
    // …but a keyboard activation (detail 0) steps once.
    wrapper.get('[data-testid="nudge-minus"]').element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 0 }))
    await wrapper.vm.$nextTick()
    expect(value.value).toBe(2.6)
  })

  it('disables the minus button at 0 and plus at 5', () => {
    const lo = setup({}, 0)
    expect(lo.wrapper.get('[data-testid="nudge-minus"]').attributes('disabled')).toBeDefined()
    lo.wrapper.unmount()
    const hi = setup({}, 5)
    expect(hi.wrapper.get('[data-testid="nudge-plus"]').attributes('disabled')).toBeDefined()
  })

  it('vibrates when crossing a landmark or the trim band edge, if vibrate exists', async () => {
    const vibrate = vi.fn()
    Object.defineProperty(navigator, 'vibrate', { value: vibrate, configurable: true })
    const { track } = setup({}, 2.7)
    await track.trigger('keydown', { key: 'ArrowRight' }) // enters the band
    expect(vibrate).toHaveBeenCalledTimes(1)
    await track.trigger('keydown', { key: 'ArrowRight' }) // 2.9: nothing
    expect(vibrate).toHaveBeenCalledTimes(1)
    await track.trigger('keydown', { key: 'ArrowRight' }) // 3.0: landmark
    expect(vibrate).toHaveBeenCalledTimes(2)
    // @ts-expect-error cleanup
    delete navigator.vibrate
  })

  it('ignores input when disabled', async () => {
    const { track, value } = setup({ disabled: true }, 2.3)
    expect(track.attributes('aria-disabled')).toBe('true')
    await track.trigger('keydown', { key: 'ArrowRight' })
    expect(value.value).toBe(2.3)
  })
})
