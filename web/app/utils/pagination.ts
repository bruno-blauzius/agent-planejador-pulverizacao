export type PageSlot = { kind: 'page', value: number } | { kind: 'gap', key: string }

/** First, last and the neighbours of the current page; gaps become "…". */
export function pageSlots(page: number, pageCount: number): PageSlot[] {
  const total = Math.max(pageCount, 1)
  const wanted = new Set([1, total, page - 1, page, page + 1].filter((value) => value >= 1 && value <= total))
  const sorted = [...wanted].sort((a, b) => a - b)
  const result: PageSlot[] = []
  sorted.forEach((value, index) => {
    const previous = sorted[index - 1]
    if (previous !== undefined && value - previous > 1) result.push({ kind: 'gap', key: `gap-${value}` })
    result.push({ kind: 'page', value })
  })
  return result
}

/** Keeps a requested page inside 1..pageCount. */
export function clampPage(value: number, pageCount: number): number {
  return Math.min(Math.max(value, 1), Math.max(pageCount, 1))
}
