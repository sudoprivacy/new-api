// sudoapi: Prompt cache health reporting for gateway administrators.

import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { api } from '@/lib/api'

type CacheBucket = {
  channel_id: number
  channel_name: string
  hour?: string
  requests: number
  cache_read_tokens: number
  cache_write_tokens: number
  no_cache_requests: number
  reuse_pct: number
}

type CacheReport = { buckets: CacheBucket[] | null; truncated: boolean }

export function CacheHealthDialog() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const report = useQuery({
    queryKey: ['channel-cache-health'],
    enabled: open,
    staleTime: 0,
    queryFn: async () => {
      const end = Math.floor(Date.now() / 1000)
      const response = await api.get<{
        success: boolean
        data: CacheReport
      }>('/api/log/cache_health', {
        params: {
          start_timestamp: end - 86400,
          end_timestamp: end,
          model_name: 'claude',
          by_hour: true,
        },
      })
      if (!response.data.success) throw new Error('Cache report failed')
      return response.data.data
    },
  })

  return (
    <>
      <Button variant='outline' size='sm' onClick={() => setOpen(true)}>
        {t('Prompt cache health')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-4xl'>
          <DialogHeader>
            <DialogTitle>{t('Prompt cache health')}</DialogTitle>
            <DialogDescription>
              {t(
                'Claude requests in the last 24 hours, grouped by channel and server-local hour. Reuse is cached reads divided by cached reads plus writes.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Button
            variant='outline'
            size='sm'
            disabled={report.isFetching}
            onClick={() => void report.refetch()}
          >
            {t('Refresh')}
          </Button>
          {report.isPending && <p role='status'>{t('Loading...')}</p>}
          {report.isError && (
            <p role='alert'>
              {t('Unable to load prompt cache health. Please retry.')}
            </p>
          )}
          {report.data && !report.isError && (
            <CacheHealthReport report={report.data} />
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}

export function CacheHealthReport(props: { report: CacheReport }) {
  const { t } = useTranslation()
  const rows = props.report.buckets ?? []
  return (
    <>
      {props.report.truncated && (
        <p role='alert' className='text-destructive'>
          {t(
            'This report is incomplete. Reuse percentages are hidden because the row limit was reached.'
          )}
        </p>
      )}
      {rows.length === 0 ? (
        <p>{t('No Claude requests in this time window.')}</p>
      ) : (
        <div className='overflow-x-auto'>
          <table className='w-full text-left text-sm'>
            <caption className='sr-only'>{t('Prompt cache health')}</caption>
            <thead>
              <tr>
                {[
                  t('Channel'),
                  t('Hour'),
                  t('Requests'),
                  t('Cache read tokens'),
                  t('Cache write tokens'),
                  t('Cache reuse'),
                  t('Requests without cache'),
                ].map((label) => (
                  <th key={label} scope='col' className='p-2'>
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                let reuse = t('No cache usage')
                if (props.report.truncated) {
                  reuse = t('Incomplete report')
                } else if (row.cache_read_tokens + row.cache_write_tokens > 0) {
                  reuse = `${row.reuse_pct.toFixed(1)}%`
                }
                return (
                  <tr
                    key={`${row.channel_id}-${row.hour}`}
                    className='border-t'
                  >
                    <td className='p-2'>
                      {row.channel_name || row.channel_id}
                    </td>
                    <td className='p-2 whitespace-nowrap'>{row.hour}</td>
                    <td className='p-2 tabular-nums'>
                      {row.requests.toLocaleString()}
                    </td>
                    <td className='p-2 tabular-nums'>
                      {row.cache_read_tokens.toLocaleString()}
                    </td>
                    <td className='p-2 tabular-nums'>
                      {row.cache_write_tokens.toLocaleString()}
                    </td>
                    <td className='p-2 tabular-nums'>{reuse}</td>
                    <td className='p-2 tabular-nums'>
                      {row.no_cache_requests.toLocaleString()}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}
