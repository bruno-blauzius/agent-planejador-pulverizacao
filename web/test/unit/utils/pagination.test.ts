import { describe, expect, it } from 'vitest'
import { clampPage, pageSlots } from '../../../app/utils/pagination'

const label = (page: number, pageCount: number) =>
  pageSlots(page, pageCount).map((slot) => (slot.kind === 'gap' ? '…' : String(slot.value)))

describe('pageSlots', () => {
  it.each([
    [1, 1, ['1']],
    [1, 3, ['1', '2', '3']],
    [1, 10, ['1', '2', '…', '10']],
    [5, 10, ['1', '…', '4', '5', '6', '…', '10']],
    [10, 10, ['1', '…', '9', '10']],
    [0, 0, ['1']],
  ])('page %i of %i', (page, pageCount, expected) => {
    expect(label(page, pageCount)).toEqual(expected)
  })
})

describe('clampPage', () => {
  it.each([
    [0, 5, 1],
    [3, 5, 3],
    [9, 5, 5],
    [2, 0, 1],
  ])('clamps %i within %i pages to %i', (value, pageCount, expected) => {
    expect(clampPage(value, pageCount)).toBe(expected)
  })
})
