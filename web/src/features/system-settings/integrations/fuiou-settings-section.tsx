// sudoapi: Fuiou payment.

import type { Control, FieldValues, Path } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import { safeNumberFieldProps } from '../utils/numeric-field'
import { FuiouBillingSettings } from '../types'

type FuiouSettingsSectionProps<TFieldValues extends FieldValues> = {
  control: Control<TFieldValues>
}

export function FuiouSettingsSection<
  TFieldValues extends FieldValues & FuiouBillingSettings,
>(props: FuiouSettingsSectionProps<TFieldValues>) {
  const { t } = useTranslation()

  return (
    <div className='space-y-4'>
      <div>
        <h3 className='text-lg font-medium'>{t('Fuiou Gateway')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t('Configuration for Fuiou payment integration')}
        </p>
      </div>

      <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
        <p className='mb-2 font-medium'>{t('Fuiou Callback Configuration:')}</p>
        <ul className='list-inside list-disc space-y-1'>
          <li>
            {t('Fuiou callback URL')}{': '}
            <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
              {'<' +
                t('Fuiou notification callback address') +
                '>/api/fuiou/callback'}
            </code>
          </li>
          <li>
            {t(
              'Fuiou payment is enabled when merchant ID, public key, private key, endpoint, and callback address are configured.'
            )}
          </li>
        </ul>
      </div>

      <div className='grid gap-6 md:grid-cols-3'>
        <FormField
          control={props.control}
          name={'FuiouUrl' as Path<TFieldValues>}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Fuiou endpoint')}</FormLabel>
              <FormControl>
                <Input
                  placeholder='https://pay.example.com'
                  {...field}
                  onChange={(event) => field.onChange(event.target.value)}
                />
              </FormControl>
              <FormDescription>
                {t('Base URL of the Fuiou service')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={props.control}
          name={'FuiouCallback' as Path<TFieldValues>}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Fuiou notification callback address')}</FormLabel>
              <FormControl>
                <Input
                  placeholder='https://gateway.example.com'
                  {...field}
                  onChange={(event) => field.onChange(event.target.value)}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Public base address for Fuiou payment callbacks. Only enter the site root domain, for example https://api.example.com'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={props.control}
          name={'FuiouMerchant' as Path<TFieldValues>}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Fuiou merchant ID')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('Enter Fuiou merchant ID')}
                  autoComplete='off'
                  {...field}
                  onChange={(event) => field.onChange(event.target.value)}
                />
              </FormControl>
              <FormDescription>
                {t('Fuiou merchant ID')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <div className='grid gap-6 md:grid-cols-2'>
        <FormField
          control={props.control}
          name={'FuiouPubKey' as Path<TFieldValues>}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Fuiou public key')}</FormLabel>
              <FormControl>
                <Textarea
                  rows={8}
                  placeholder={t('Enter new public key to update')}
                  autoComplete='off'
                  {...field}
                  onChange={(event) => field.onChange(event.target.value)}
                />
              </FormControl>
              <FormDescription>
                {t('Base64-encoded RSA public key from Fuiou')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={props.control}
          name={'FuiouPriKey' as Path<TFieldValues>}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Fuiou merchant private key')}</FormLabel>
              <FormControl>
                <Textarea
                  rows={8}
                  placeholder={t('Enter new private key to update')}
                  autoComplete='new-password'
                  {...field}
                  onChange={(event) => field.onChange(event.target.value)}
                />
              </FormControl>
              <FormDescription>
                {t('Base64-encoded RSA merchant private key from Fuiou')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={props.control}
        name={'FuiouUnitPrice' as Path<TFieldValues>}
        render={({ field }) => (
          <FormItem className='max-w-sm'>
            <FormLabel>
              {t('Fuiou exchange rate (CNY / USD)')}
            </FormLabel>
            <FormControl>
              <Input
                type='number'
                step='0.01'
                min={0}
                {...safeNumberFieldProps(field)}
              />
            </FormControl>
            <FormDescription>
              {t('For example, 7.3 means $1 requires paying ￥7.3')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}
