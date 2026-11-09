import { describe, expect, it } from 'vitest'
import { firingLabel, formatDate, layerDetail, pieceTitle, positionText, stackText } from './format'

describe('wording', () => {
  it('names tiles by number and pots by record', () => {
    expect(pieceTitle({ kind: 'tile', number: 142, id: 9 })).toBe('Tile 142')
    expect(pieceTitle({ kind: 'pot', number: null, id: 9 })).toBe('Pot #9')
  })
  it('keeps a date on its own day', () => {
    expect(formatDate('2026-11-09')).toBe('9 Nov 2026')
    expect(formatDate(null)).toBe('Date unknown')
  })
  it('leaves out what is not known', () => {
    expect(firingLabel({ fired_on: '2026-11-09', cone: '6', schedule: 'slow cool' })).toBe('9 Nov 2026 · cone 6 · slow cool')
    expect(firingLabel({ fired_on: null, cone: '', schedule: '' })).toBe('Date unknown')
    expect(positionText({ level: 'top', spot: 'edge' })).toBe('Top shelf, edge')
    expect(positionText({ level: null, spot: null })).toBe('')
    expect(layerDetail({ stage: null, method: null, coats: null, thickness: null, area: '' })).toBe('')
  })
  it('describes a layer as steps to repeat', () => {
    expect(layerDetail({ stage: 'bisque', method: 'brush', coats: 3, thickness: 3, area: 'rim' })).toBe('Bisque · Brush, 3 coats · Medium (3.0) · rim')
    expect(layerDetail({ stage: null, method: 'brush', coats: 1, thickness: null, area: '' })).toBe('Brush, 1 coat')
  })
  it('shows the stack in order', () => {
    expect(stackText([{ material: 'Slip' }, { material: 'Celadon' }])).toBe('Slip → Celadon')
  })
})
