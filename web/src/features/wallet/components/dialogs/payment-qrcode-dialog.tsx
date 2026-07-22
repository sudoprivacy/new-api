// sudoapi: Fuiou payment.

import { QRCodeSVG } from 'qrcode.react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

export function PaymentQRCodeDialog(props: {
  onClose: () => void
  qrCodeValue: string | null
}) {
  const {t} = useTranslation()
  return (
    <Dialog
      open={!!props.qrCodeValue}
      onOpenChange={(open) => {
        if (!open) props.onClose?.()
      }}
      title={t('Payment QR Code')}
      description={t('Scan this QR code with your payment app to complete the payment.')}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      bodyClassName='py-4'
      footer={
        <Button variant='outline' onClick={props.onClose}>{t('Close')}</Button>
      }
    >
      {props.qrCodeValue ? (
        <div className='flex justify-center rounded-lg bg-white p-4'>
          <QRCodeSVG value={props.qrCodeValue} size={220}/>
        </div>
      ) : null}
    </Dialog>
  )
}
