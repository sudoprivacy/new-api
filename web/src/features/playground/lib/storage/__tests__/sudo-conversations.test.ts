// sudoapi: Browser-local Playground conversations.
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { STORAGE_KEYS } from '../../../constants'
import type { Message } from '../../../types'
import { createBrowserConversationStore } from '../sudo-conversation-store'

const prompt: Message = {
  key: 'first',
  from: 'user',
  versions: [{ id: 'v1', content: 'First request' }],
}

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'first', role: 1 })
  vi.useFakeTimers()
})
afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

test('starting another chat preserves messages and drafts when switching back', () => {
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  const firstId = store.getState().history.activeId
  store.getState().updateMessages([prompt])
  store.getState().setDraft('Unsent followup')
  expect(store.getState().newChat()).toBe(true)
  const second = store.getState().history.chats[0]
  expect(second.messages).toEqual([])
  expect(second.draft).toBe('')
  store.getState().selectChat(firstId)
  expect(
    store.getState().history.chats.find((chat) => chat.id === firstId)
  ).toMatchObject({ messages: [prompt], draft: 'Unsent followup' })
})

test('reloading restores the active chat and an update that was waiting for debounce', () => {
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  store.getState().updateMessages([prompt])
  store.getState().newChat()
  store.getState().setDraft('Second draft')
  store.getState().flush()
  const reloaded = createBrowserConversationStore(1)
  reloaded.getState().hydrate()
  expect(reloaded.getState().history).toEqual(store.getState().history)
})

test('deleting the active chat preserves the other chat', () => {
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  store.getState().updateMessages([prompt])
  const originalId = store.getState().history.activeId
  store.getState().newChat()
  store.getState().deleteChat()
  expect(store.getState().history.chats).toHaveLength(1)
  expect(store.getState().history.activeId).toBe(originalId)
  expect(store.getState().history.chats[0].messages).toEqual([prompt])
})

test('deleting the last chat leaves an empty usable conversation', () => {
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  store.getState().updateMessages([prompt])
  store.getState().deleteChat()
  expect(store.getState().history.chats).toHaveLength(1)
  expect(store.getState().history.chats[0].messages).toEqual([])
})

test('another account cannot restore or overwrite the first accounts history', () => {
  const first = createBrowserConversationStore(1)
  first.getState().hydrate()
  first.getState().updateMessages([prompt])
  first.getState().flush()
  useAuthStore.getState().auth.setUser({ id: 2, username: 'second', role: 1 })
  const second = createBrowserConversationStore(2)
  second.getState().hydrate()
  expect(second.getState().history.chats[0].messages).toEqual([])
  first.getState().updateMessages([])
  expect(first.getState().history.chats[0].messages).toEqual([prompt])
})

test('legacy history requires explicit import and its original copy is retained', () => {
  const legacy = JSON.stringify({ version: 1, data: [prompt] })
  localStorage.setItem(STORAGE_KEYS.MESSAGES, legacy)
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  expect(store.getState().history.chats[0].messages).toEqual([])
  expect(store.getState().hasLegacy).toBe(true)
  expect(store.getState().importLegacy()).toBe(true)
  expect(store.getState().history.chats[0].messages).toEqual([prompt])
  expect(localStorage.getItem(STORAGE_KEYS.MESSAGES)).toBe(legacy)
  const reloaded = createBrowserConversationStore(1)
  reloaded.getState().hydrate()
  expect(reloaded.getState().hasLegacy).toBe(false)
  expect(reloaded.getState().importLegacy()).toBe(false)
})

test('a storage failure keeps the current conversation and original saved history', () => {
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  store.getState().updateMessages([prompt])
  store.getState().flush()
  const saved = localStorage.getItem('playground_conversations:1')
  const activeId = store.getState().history.activeId
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Full', 'QuotaExceededError')
  })
  expect(store.getState().newChat()).toBe(false)
  expect(store.getState().error).toBe('storage')
  expect(store.getState().history.activeId).toBe(activeId)
  expect(localStorage.getItem('playground_conversations:1')).toBe(saved)
})

test('another tabs write is kept instead of being overwritten by a stale tab', () => {
  const first = createBrowserConversationStore(1)
  const second = createBrowserConversationStore(1)
  first.getState().hydrate()
  second.getState().hydrate()
  first.getState().updateMessages([prompt])
  first.getState().flush()
  const saved = localStorage.getItem('playground_conversations:1')
  second.getState().setDraft('Other tab draft')
  second.getState().flush()
  expect(second.getState().error).toBe('conflict')
  expect(localStorage.getItem('playground_conversations:1')).toBe(saved)
  expect(second.getState().history.chats[0].draft).toBe('Other tab draft')
})

test('malformed history is retained and backed up before a new save', () => {
  localStorage.setItem('playground_conversations:1', '{broken')
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  expect(store.getState().error).toBe('corrupt')
  expect(localStorage.getItem('playground_conversations:1')).toBe('{broken')
  store.getState().setDraft('Recovered draft')
  store.getState().flush()
  expect(localStorage.getItem('playground_conversations:1:recovery')).toBe(
    '{broken'
  )
  expect(store.getState().error).toBeNull()
})

test('new chat on an empty conversation does not fill history with empty entries', () => {
  const store = createBrowserConversationStore(1)
  store.getState().hydrate()
  store.getState().newChat()
  expect(store.getState().history.chats).toHaveLength(1)
})
