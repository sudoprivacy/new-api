// sudoapi: Fuiou payment.

import * as z from 'zod'

import { removeTrailingSlash } from './utils'
import { FuiouBillingSettings } from "../types";

export const paymentFuiouSchema = {
  FuiouPubKey: z.string(),
  FuiouPriKey: z.string(),
  FuiouMerchant: z.string(),
  FuiouUrl: z.string(),
  FuiouCallback: z.string(),
  FuiouUnitPrice: z.coerce.number().min(0),
}

export type SettingUpdate = {
  key: string
  value: string | number | boolean
}

export function sanitizeFuiou(
  values: FuiouBillingSettings
): FuiouBillingSettings {
  return {
    FuiouPubKey: values.FuiouPubKey.trim(),
    FuiouPriKey: values.FuiouPriKey.trim(),
    FuiouMerchant: values.FuiouMerchant.trim(),
    FuiouUrl: removeTrailingSlash(values.FuiouUrl),
    FuiouCallback: removeTrailingSlash(values.FuiouCallback),
    FuiouUnitPrice: values.FuiouUnitPrice,
  }
}

export function appendFuiouSettingUpdates(
  updates: SettingUpdate[],
  sanitized: FuiouBillingSettings,
  initial: FuiouBillingSettings
) {
  if (sanitized.FuiouPubKey && sanitized.FuiouPubKey !== initial.FuiouPubKey) {
    updates.push({ key: 'FuiouPubKey', value: sanitized.FuiouPubKey })
  }

  if (sanitized.FuiouPriKey && sanitized.FuiouPriKey !== initial.FuiouPriKey) {
    updates.push({ key: 'FuiouPriKey', value: sanitized.FuiouPriKey })
  }

  if (sanitized.FuiouMerchant !== initial.FuiouMerchant) {
    updates.push({ key: 'FuiouMerchant', value: sanitized.FuiouMerchant })
  }

  if (sanitized.FuiouUrl !== initial.FuiouUrl) {
    updates.push({ key: 'FuiouUrl', value: sanitized.FuiouUrl })
  }

  if (sanitized.FuiouCallback !== initial.FuiouCallback) {
    updates.push({ key: 'FuiouCallback', value: sanitized.FuiouCallback })
  }

  if (sanitized.FuiouUnitPrice !== initial.FuiouUnitPrice) {
    updates.push({ key: 'FuiouUnitPrice', value: sanitized.FuiouUnitPrice })
  }
}
