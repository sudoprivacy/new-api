// sudoapi: Fuiou payment.

import { useCallback, useEffect, useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'

import {
  getTopupStatus,
  isApiSuccess,
  requestFuiouPayment,
} from '@/features/wallet/api.ts'

export function usePaymentFuiou(onOrderSuccess?: () => void | Promise<void>) {
  const [creatingOrder, setCreatingOrder] = useState(false)
  const [orderID, setOrderID] = useState<string | null>(null)
  const [orderInfo, setOrderInfo] = useState<string | null>(null)

  const processPaymentFuiou = useCallback(
    async (amount: number, payment_method: string): Promise<boolean> => {
      setCreatingOrder(true)
      try {
        const response = await requestFuiouPayment({ amount, payment_method })
        if (isApiSuccess(response) && response.data?.order_id && response.data?.order_info) {
          setOrderID(response.data.order_id)
          setOrderInfo(response.data.order_info)
          return true
        }
        toast.error(typeof response.data === 'string' ? response.data : i18next.t('Payment request failed'))
      } catch {
        toast.error(i18next.t('Payment request failed'))
      } finally {
        setCreatingOrder(false)
      }
      return false
    },
    []
  )

  useEffect(() => {
    if (!orderID) {
      return
    }
    let cancelled = false
    let timeoutID: number | undefined

    const checkOrderStatus = async () => {
      try {
        const response = await getTopupStatus(orderID)
        if (cancelled) {
          return
        }
        if (isApiSuccess(response)) {
          switch (response.data?.order_status) {
            case 'success':
              setOrderID(null)
              setOrderInfo(null)
              toast.success(i18next.t('Recharge successful'))
              await onOrderSuccess?.()
              return
            case 'expired':
            case 'failed':
              setOrderID(null)
              setOrderInfo(null)
              toast.error(i18next.t('Payment expired or failed'))
              return
          }
        }
      } catch {
      }
      if (!cancelled) {
        timeoutID = window.setTimeout(() => {
          void checkOrderStatus()
        }, 3000)
      }
    }
    void checkOrderStatus()

    return () => {
      cancelled = true
      window.clearTimeout(timeoutID)
    }
  }, [orderID, onOrderSuccess])

  const onQRCodeClose = () => {
    setOrderID(null)
    setOrderInfo(null)
  }

  return {
    qrCodeValue: orderInfo,
    onQRCodeClose,
    processPaymentFuiou,
    creatingOrder,
  }
}
