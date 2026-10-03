// sudoapi: Prompt cache health reporting for gateway administrators.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { CacheHealthDialog, CacheHealthReport } from '../cache-health-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

afterEach(cleanup)

const bucket = {
  channel_id: 42,
  channel_name: 'FujiToken',
  hour: '2026-10-03 12',
  requests: 20,
  cache_read_tokens: 800,
  cache_write_tokens: 200,
  no_cache_requests: 2,
  reuse_pct: 80,
}

describe('cache health report', () => {
  it('shows channel reuse and distinguishes requests without cache usage', () => {
    render(
      <CacheHealthReport
        report={{
          truncated: false,
          buckets: [
            bucket,
            {
              ...bucket,
              channel_id: 30,
              channel_name: 'No cache',
              cache_read_tokens: 0,
              cache_write_tokens: 0,
            },
          ],
        }}
      />
    )
    expect(screen.getByText('FujiToken')).toBeInTheDocument()
    expect(screen.getByText('80.0%')).toBeInTheDocument()
    expect(screen.getByText('No cache usage')).toBeInTheDocument()
  })

  it('hides misleading percentages when the server reports truncated data', () => {
    render(
      <CacheHealthReport report={{ truncated: true, buckets: [bucket] }} />
    )
    expect(screen.getByRole('alert')).toHaveTextContent('incomplete')
    expect(screen.queryByText('80.0%')).not.toBeInTheDocument()
    expect(screen.getByText('Incomplete report')).toBeInTheDocument()
  })

  it('explains an empty time window', () => {
    render(<CacheHealthReport report={{ truncated: false, buckets: null }} />)
    expect(
      screen.getByText('No Claude requests in this time window.')
    ).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('loads only after opening and allows retry after a failed request', async () => {
    const request = vi
      .spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({
        data: { success: true, data: { buckets: [bucket], truncated: false } },
      })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <CacheHealthDialog />
      </QueryClientProvider>
    )
    expect(request).not.toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', { name: 'Prompt cache health' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load')
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByText('80.0%')).toBeInTheDocument()
    const params = request.mock.calls[1][1]?.params
    expect(params.model_name).toBe('claude')
    expect(params.by_hour).toBe(true)
    expect(params.end_timestamp - params.start_timestamp).toBe(86400)
    client.clear()
  })
})
