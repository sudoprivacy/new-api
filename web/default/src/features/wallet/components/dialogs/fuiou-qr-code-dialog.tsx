// sudoapi: Fuiou wallet payment QR code dialog.
import { QRCodeSVG } from 'qrcode.react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

import { getTopupStatus, isApiSuccess } from '../../api'

interface FuiouQrCodeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onPaid: () => void | Promise<void>
  orderId: string | null
  qrCodeUrl: string | null
}

export function FuiouQrCodeDialog(props: FuiouQrCodeDialogProps) {
  const { t } = useTranslation()
  const isOpen = props.open
  const onOpenChange = props.onOpenChange
  const onPaid = props.onPaid
  const orderId = props.orderId
  const qrCodeUrl = props.qrCodeUrl

  useEffect(() => {
    if (!isOpen || !orderId) {
      return
    }

    let stopped = false

    const checkOrderStatus = async () => {
      try {
        const response = await getTopupStatus(orderId)
        if (
          stopped ||
          !isApiSuccess(response) ||
          response.data?.order_status !== 'success'
        ) {
          return
        }

        stopped = true
        toast.success(t('Recharge successful'))
        await onPaid()
        onOpenChange(false)
      } catch {
        // Keep polling; the callback may arrive while a transient query fails.
      }
    }

    void checkOrderStatus()
    const intervalID = window.setInterval(() => {
      void checkOrderStatus()
    }, 3000)

    return () => {
      stopped = true
      window.clearInterval(intervalID)
    }
  }, [isOpen, onOpenChange, onPaid, orderId, t])

  return (
    <Dialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('Payment QR Code')}
      description={t(
        'Scan this QR code with your payment app to complete the payment.'
      )}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      bodyClassName='py-4'
      footer={
        <Button variant='outline' onClick={() => onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      {qrCodeUrl ? (
        <div className='flex justify-center rounded-lg bg-white p-4'>
          <QRCodeSVG value={qrCodeUrl} size={220} />
        </div>
      ) : null}
    </Dialog>
  )
}
