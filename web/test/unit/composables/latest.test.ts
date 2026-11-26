import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useLatestValue } from '~/composables/internal/latest'

describe('useLatestValue', () => {
  it('shows the loaded data until a value is set, and the set value until the next load', () => {
    const source = ref('loaded')
    const latest = useLatestValue(source)
    expect(latest.value.value).toBe('loaded')
    latest.set('action')
    expect(latest.value.value).toBe('action')
    source.value = 'reloaded'
    expect(latest.value.value).toBe('reloaded')
  })

  it('keeps the action result over a load that started before it', async () => {
    const source = ref('before')
    const latest = useLatestValue(source)
    let finish: () => void = () => {}
    const load = latest.trackLoad(() => new Promise<void>((resolve) => { finish = () => { source.value = 'late get'; resolve() } }))
    latest.set('action')
    finish()
    await load
    expect(latest.value.value).toBe('action')
  })

  it('lets a load that started after the action replace it', async () => {
    const source = ref('before')
    const latest = useLatestValue(source)
    latest.set('action')
    await latest.trackLoad(async () => { source.value = 'newer' })
    expect(latest.value.value).toBe('newer')
  })

  it('forgets the value when the context changes, even with a stale load in flight', async () => {
    const context = ref('a')
    const source = ref('x')
    const latest = useLatestValue(source, () => context.value)
    latest.set('action')
    context.value = 'b'
    expect(latest.value.value).toBe('x')
    latest.set('second')
    expect(latest.value.value).toBe('second')
  })
})
