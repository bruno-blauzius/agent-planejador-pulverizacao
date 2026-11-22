import type { Field, FieldBlock } from '~/types/field'

const byName = new Intl.Collator('pt-BR', { numeric: true, sensitivity: 'base' })

/**
 * Groups fields by block. The API has no order field, so the order is set here
 * to be stable and predictable: blocks alphabetical (natural, pt-BR) with the
 * fields without a block last, and fields by name ("Talhão 2" before "Talhão 10").
 * The block area is the display sum of its fields.
 */
export function groupFieldsByBlock(fields: readonly Field[]): FieldBlock[] {
  const groups = new Map<string | null, Field[]>()
  for (const field of fields) {
    const group = groups.get(field.block)
    if (group) group.push(field)
    else groups.set(field.block, [field])
  }
  return [...groups.entries()]
    .sort(([a], [b]) => Number(a === null) - Number(b === null) || byName.compare(a ?? '', b ?? ''))
    .map(([name, items]) => ({
      name,
      areaHa: Math.round(items.reduce((sum, field) => sum + field.areaHa, 0) * 10) / 10,
      fields: [...items].sort((a, b) => byName.compare(a.name, b.name)),
    }))
}
