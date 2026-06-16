/**
 * Unit tests for useFreshData composable
 *
 * **Validates: Requirements 11.1, 11.3**
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, ref, nextTick } from 'vue'
import { useFreshData } from './useFreshData'

describe('useFreshData', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('calls fetcher on mount', async () => {
    const fetcher = vi.fn().mockResolvedValue(undefined)

    const TestComponent = defineComponent({
      setup() {
        const { ensureFresh, lastFetchedAt } = useFreshData(fetcher)
        return { ensureFresh, lastFetchedAt }
      },
      template: '<div></div>',
    })

    mount(TestComponent)
    await nextTick()
    // Allow the async ensureFresh to complete
    await vi.runAllTimersAsync()

    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('sets lastFetchedAt after successful fetch', async () => {
    const fetcher = vi.fn().mockResolvedValue(undefined)
    vi.setSystemTime(new Date('2024-01-01T00:00:30.000Z'))

    const TestComponent = defineComponent({
      setup() {
        const { ensureFresh, lastFetchedAt } = useFreshData(fetcher)
        return { ensureFresh, lastFetchedAt }
      },
      template: '<div></div>',
    })

    const wrapper = mount(TestComponent)
    await nextTick()
    await vi.runAllTimersAsync()

    expect(wrapper.vm.lastFetchedAt).toBe(Date.now())
  })

  it('does not re-fetch if data is fresh (< 30s old)', async () => {
    const fetcher = vi.fn().mockResolvedValue(undefined)
    vi.setSystemTime(new Date('2024-01-01T00:00:00.000Z'))

    const TestComponent = defineComponent({
      setup() {
        const { ensureFresh, lastFetchedAt } = useFreshData(fetcher)
        return { ensureFresh, lastFetchedAt }
      },
      template: '<div></div>',
    })

    const wrapper = mount(TestComponent)
    await nextTick()
    await vi.runAllTimersAsync()

    expect(fetcher).toHaveBeenCalledTimes(1)

    // Advance time by only 10 seconds (still fresh)
    vi.setSystemTime(new Date('2024-01-01T00:00:10.000Z'))

    // Manually call ensureFresh
    await wrapper.vm.ensureFresh()

    // Should not have re-fetched since data is still fresh
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('re-fetches if data is stale (> 30s old)', async () => {
    const fetcher = vi.fn().mockResolvedValue(undefined)
    vi.setSystemTime(new Date('2024-01-01T00:00:00.000Z'))

    const TestComponent = defineComponent({
      setup() {
        const { ensureFresh, lastFetchedAt } = useFreshData(fetcher)
        return { ensureFresh, lastFetchedAt }
      },
      template: '<div></div>',
    })

    const wrapper = mount(TestComponent)
    await nextTick()
    await vi.runAllTimersAsync()

    expect(fetcher).toHaveBeenCalledTimes(1)

    // Advance time by 31 seconds (stale)
    vi.setSystemTime(new Date('2024-01-01T00:00:31.000Z'))

    // Manually call ensureFresh
    await wrapper.vm.ensureFresh()

    // Should have re-fetched since data is stale
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('re-fetches exactly at the 30s boundary', async () => {
    const fetcher = vi.fn().mockResolvedValue(undefined)
    vi.setSystemTime(new Date('2024-01-01T00:00:00.000Z'))

    const TestComponent = defineComponent({
      setup() {
        const { ensureFresh, lastFetchedAt } = useFreshData(fetcher)
        return { ensureFresh, lastFetchedAt }
      },
      template: '<div></div>',
    })

    const wrapper = mount(TestComponent)
    await nextTick()
    await vi.runAllTimersAsync()

    expect(fetcher).toHaveBeenCalledTimes(1)

    // Advance exactly 30001ms (just past the threshold)
    vi.setSystemTime(new Date('2024-01-01T00:00:30.001Z'))

    await wrapper.vm.ensureFresh()

    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('ensureFresh updates lastFetchedAt to current time after fetch', async () => {
    const fetcher = vi.fn().mockResolvedValue(undefined)
    vi.setSystemTime(new Date('2024-01-01T00:00:00.000Z'))

    const TestComponent = defineComponent({
      setup() {
        const { ensureFresh, lastFetchedAt } = useFreshData(fetcher)
        return { ensureFresh, lastFetchedAt }
      },
      template: '<div></div>',
    })

    const wrapper = mount(TestComponent)
    await nextTick()
    await vi.runAllTimersAsync()

    const firstFetchTime = wrapper.vm.lastFetchedAt

    // Advance time past threshold
    vi.setSystemTime(new Date('2024-01-01T00:01:00.000Z'))

    await wrapper.vm.ensureFresh()

    expect(wrapper.vm.lastFetchedAt).toBeGreaterThan(firstFetchTime)
    expect(wrapper.vm.lastFetchedAt).toBe(Date.now())
  })
})
