import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  classifyGesture,
  createRepeater,
  fineFactor,
  GESTURE,
  keyToValue,
  positionToRaw,
  SliderGesture,
  snap,
  type TrackGeometry,
} from './usePointerSlider'

const scale = { min: 0, max: 5, step: 0.1 }
// 0 at x=20, 5 at x=320: 300px for 5 units = 60px per unit, 6px per 0.1.
const geo: TrackGeometry = { left: 20, width: 300, centerY: 100 }
const xOf = (v: number) => geo.left + (v / 5) * geo.width

describe('classifyGesture: capture thresholds', () => {
  it('stays pending for small movement', () => {
    expect(classifyGesture(0, 0)).toBe('pending')
    expect(classifyGesture(6, 10)).toBe('pending') // thresholds are "passes", not "reaches"
    expect(classifyGesture(-5, -9)).toBe('pending')
  })
  it('captures once horizontal passes 6px before vertical passes 10px', () => {
    expect(classifyGesture(7, 0)).toBe('drag')
    expect(classifyGesture(-7, 9)).toBe('drag')
  })
  it('hands vertical movement to the page scroll', () => {
    expect(classifyGesture(0, 11)).toBe('scroll')
    expect(classifyGesture(5, -11)).toBe('scroll')
  })
  it('lets the dominant axis win when both pass in one jump', () => {
    expect(classifyGesture(30, 12)).toBe('drag')
    expect(classifyGesture(8, 40)).toBe('scroll')
  })
})

describe('fineFactor: fine scrubbing below the track', () => {
  it.each([
    [-50, 1],
    [0, 1],
    [59, 1],
    [60, 0.5],
    [119, 0.5],
    [120, 0.25],
    [400, 0.25],
  ])('%spx below → %s×', (d, f) => {
    expect(fineFactor(d)).toBe(f)
  })
})

describe('snap / positionToRaw', () => {
  it('snaps to 0.1 without float drift', () => {
    expect(snap(2.849999, scale)).toBe(2.8)
    expect(snap(0.1 + 0.2, scale)).toBe(0.3)
    expect(snap(-3, scale)).toBe(0)
    expect(snap(9, scale)).toBe(5)
  })
  it('maps x to a value and clamps outside the track', () => {
    expect(positionToRaw(xOf(2.5), geo, scale)).toBeCloseTo(2.5)
    expect(positionToRaw(0, geo, scale)).toBe(0)
    expect(positionToRaw(1000, geo, scale)).toBe(5)
  })
})

describe('SliderGesture', () => {
  it('a tap with no movement moves the thumb to the tapped point', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(4), 100, 1.0, geo)
    expect(g.move(xOf(4) + 3, 103, geo).phase).toBe('pending')
    expect(g.up(xOf(4) + 3, geo)).toBe(4.1) // +3px ≈ 0.05 → rounds to 4.1
  })

  it('a tap on the thumb itself changes nothing', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(2) + 10, 100, 2.0, geo)
    expect(g.up(xOf(2) + 10, geo)).toBeUndefined()
  })

  it('a vertical scroll never changes the value', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(1), 100, 3.0, geo)
    expect(g.move(xOf(1) + 4, 108, geo)).toMatchObject({ phase: 'pending' })
    const r = g.move(xOf(1) + 5, 115, geo)
    expect(r.released).toBe(true)
    expect(r.value).toBeUndefined()
    // Further moves and the release are ignored.
    expect(g.move(xOf(4), 300, geo).value).toBeUndefined()
    expect(g.up(xOf(4), geo)).toBeUndefined()
  })

  it('dragging from the thumb moves relatively (no jump)', () => {
    const g = new SliderGesture(scale)
    const x0 = xOf(2) + 15 // grabbed slightly right of centre
    g.down(x0, 100, 2.0, geo)
    const r = g.move(x0 + 12, 100, geo) // 12px = 0.2
    expect(r).toMatchObject({ phase: 'drag', captured: true, value: 2.2 })
    expect(g.move(x0 + 60, 102, geo).value).toBe(3.0) // 60px = 1.0
  })

  it('dragging from elsewhere jumps to the finger, then follows', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(4), 100, 1.0, geo)
    expect(g.move(xOf(4) + 7, 100, geo).value).toBe(4.1)
    expect(g.move(xOf(4) - 23, 100, geo).value).toBe(3.6)
  })

  it('moving the finger down slows the value: ½× after 60px, ¼× after 120px', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(2), 100, 2.0, geo)
    g.move(xOf(2) + 7, 100, geo) // capture: raw = 2.0 + 7px
    const base = g.move(xOf(2) + 7, 100, geo).value!
    // 1×: +60px → +1.0
    let x = xOf(2) + 7
    let r = g.move((x += 60), 100 + 30, geo)
    expect(r.factor).toBe(1)
    expect(r.value).toBeCloseTo(base + 1, 5)
    // ½×: +60px → +0.5
    r = g.move((x += 60), 100 + 70, geo)
    expect(r.factor).toBe(0.5)
    expect(r.value).toBeCloseTo(base + 1.5, 5)
    // ¼×: −60px → −0.25 (raw), shown rounded
    r = g.move((x -= 60), 100 + 130, geo)
    expect(r.factor).toBe(0.25)
    expect(r.value).toBeCloseTo(snap(base + 1.25, scale), 5)
    // Six steps of 6px at ¼× = 0.25 → fine positioning to a single step is easy.
    const before = r.value!
    for (let i = 0; i < 4; i++) r = g.move((x += 6), 230, geo)
    expect(r.value! - before).toBeCloseTo(0.1, 5)
  })

  it('clamps at the ends and responds immediately on reversal', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(4.8), 100, 4.8, geo)
    g.move(xOf(4.8) + 7, 100, geo)
    expect(g.move(xOf(4.8) + 200, 100, geo).value).toBe(5)
    // Coming back 6px moves off the end straight away (raw was clamped).
    expect(g.move(xOf(4.8) + 194, 100, geo).value).toBe(4.9)
  })

  it('cancel stops the gesture', () => {
    const g = new SliderGesture(scale)
    g.down(xOf(1), 100, 1, geo)
    g.move(xOf(1) + 10, 100, geo)
    g.cancel()
    expect(g.phase).toBe('idle')
    expect(g.move(xOf(3), 100, geo).value).toBeUndefined()
  })
})

describe('keyboard', () => {
  it('arrows ±0.1, PageUp/PageDown ±1.0, Home/End', () => {
    expect(keyToValue('ArrowRight', 2.8, scale)).toBe(2.9)
    expect(keyToValue('ArrowUp', 2.8, scale)).toBe(2.9)
    expect(keyToValue('ArrowLeft', 2.8, scale)).toBe(2.7)
    expect(keyToValue('ArrowDown', 0, scale)).toBe(0)
    expect(keyToValue('PageUp', 2.8, scale)).toBe(3.8)
    expect(keyToValue('PageDown', 0.4, scale)).toBe(0)
    expect(keyToValue('Home', 2.8, scale)).toBe(0)
    expect(keyToValue('End', 2.8, scale)).toBe(5)
    expect(keyToValue('a', 2.8, scale)).toBeNull()
  })
})

describe('nudge hold-repeat', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('fires once on press, then repeats after the delay until released', () => {
    vi.useFakeTimers()
    const fire = vi.fn()
    const r = createRepeater(fire)
    r.start()
    expect(fire).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(GESTURE.repeatDelay - 1)
    expect(fire).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1)
    expect(fire).toHaveBeenCalledTimes(2)
    vi.advanceTimersByTime(GESTURE.repeatInterval * 3)
    expect(fire).toHaveBeenCalledTimes(5)
    r.stop()
    vi.advanceTimersByTime(5000)
    expect(fire).toHaveBeenCalledTimes(5)
  })

  it('stops by itself at the end of the scale', () => {
    vi.useFakeTimers()
    let v = 4.8
    const r = createRepeater(() => {
      const next = snap(v + 0.1, scale)
      if (next === v) return false
      v = next
    })
    r.start()
    vi.advanceTimersByTime(5000)
    expect(v).toBe(5)
    expect(r.active).toBe(false)
  })
})
