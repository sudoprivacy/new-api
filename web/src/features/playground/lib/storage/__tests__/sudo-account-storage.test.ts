// sudoapi: Browser-local Playground conversations.
import { beforeEach, expect, test } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { STORAGE_KEYS } from '../../../constants'
import { loadConfig, saveConfig } from '../storage'

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'first', role: 1 })
})

test('switching accounts keeps model settings private to their owner', () => {
  saveConfig({ model: 'first-model' })
  useAuthStore.getState().auth.setUser({ id: 2, username: 'second', role: 1 })
  expect(loadConfig()).toEqual({})
  saveConfig({ model: 'second-model' })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'first', role: 1 })
  expect(loadConfig()).toEqual({ model: 'first-model' })
})

test('unowned legacy settings are retained without adopting them into the next account', () => {
  const legacy = JSON.stringify({ version: 1, data: { model: 'legacy-model' } })
  localStorage.setItem(STORAGE_KEYS.CONFIG, legacy)
  expect(loadConfig()).toEqual({})
  expect(localStorage.getItem(STORAGE_KEYS.CONFIG)).toBe(legacy)
})
