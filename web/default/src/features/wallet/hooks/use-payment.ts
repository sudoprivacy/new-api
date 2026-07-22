/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import i18next from 'i18next'
import { useState, useCallback } from 'react'
import { toast } from 'sonner'

import {
  calculateFuiouAmount,
  requestFuiouPayment,
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isFuiouPayment,
  isStripePayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'

interface FuiouQrCodePayment {
  orderId: string
  qrCodeUrl: string
}

// ============================================================================
// Payment Hook
// ============================================================================

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)
  const [fuiouQrCodePayment, setFuiouQrCodePayment] = useState<FuiouQrCodePayment | null>(null)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)

        const isFuiou = isFuiouPayment(paymentType)
        const isStripe = isStripePayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)

        let response
        if (isFuiou) {
          response = await calculateFuiouAmount({ amount: topupAmount })
        } else if (isStripe) {
          response = await calculateStripeAmount({ amount: topupAmount })
        } else if (isPancake) {
          response = await calculateWaffoPancakeAmount({ amount: topupAmount })
        } else {
          response = await calculateAmount({ amount: topupAmount })
        }

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = Number.parseFloat(response.data)
          setAmount(calculatedAmount)
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        return 0
      } catch {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setProcessing(true)

        const isFuiou = isFuiouPayment(paymentType)
        const isStripe = isStripePayment(paymentType)
        const amount = Math.floor(topupAmount)

        let response
        if (isFuiou) {
          response = await requestFuiouPayment({
            amount,
            payment_method: paymentType,
          })
        } else if (isStripe) {
          response = await requestStripePayment({
            amount,
            payment_method: 'stripe',
          })
        } else {
          response = await requestPayment({
            amount,
            payment_method: paymentType,
          })
        }

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        const responseData = response.data

        // Handle Stripe payment
        if (
          isStripe &&
          responseData &&
          typeof responseData === 'object' &&
          'pay_link' in responseData &&
          typeof responseData.pay_link === 'string'
        ) {
          window.open(responseData.pay_link, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        if (
          isFuiou &&
          responseData &&
          typeof responseData === 'object' &&
          'order_info' in responseData &&
          typeof responseData.order_info === 'string' &&
          'order_id' in responseData &&
          typeof responseData.order_id === 'string'
        ) {
          setFuiouQrCodePayment({
            orderId: responseData.order_id,
            qrCodeUrl: responseData.order_info,
          })
          toast.success(
            i18next.t(
              'Payment QR code generated. Please scan it to complete the payment.'
            )
          )
          return true
        }

        // Handle non-Stripe payment
        if (!isStripe && !isFuiou && responseData) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, responseData)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        return false
      } catch {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  const clearFuiouQrCode = useCallback(() => {
    setFuiouQrCodePayment(null)
  }, [])

  return {
    amount,
    calculating,
    processing,
    fuiouQrCodePayment,
    calculatePaymentAmount,
    processPayment,
    clearFuiouQrCode,
    setAmount,
  }
}
