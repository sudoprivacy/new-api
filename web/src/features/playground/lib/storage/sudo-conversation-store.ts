// sudoapi: Browser-local Playground conversations.
import { nanoid } from 'nanoid'
import { z } from 'zod'
import { createStore } from 'zustand/vanilla'

import { useAuthStore } from '@/stores/auth-store'

import { STORAGE_KEYS } from '../../constants'
import type { Message } from '../../types'
import {
  loadMessages,
  normalizeStoredMessages,
  prepareStoredMessages,
} from './storage'
import { getPlaygroundStorageKey } from './sudo-storage-scope'

const MAX_CHATS = 30
const MAX_HISTORY_BYTES = 4 * 1024 * 1024
const historySchema = z.object({
  version: z.literal(1),
  activeId: z.string(),
  legacyImported: z.boolean().optional(),
  chats: z
    .array(
      z.object({
        id: z.string().min(1),
        updatedAt: z.number().finite(),
        draft: z.string().default(''),
        messages: z.unknown(),
      })
    )
    .min(1)
    .max(MAX_CHATS),
})

export type BrowserChat = {
  id: string
  updatedAt: number
  draft: string
  messages: Message[]
}

type History = {
  version: 1
  activeId: string
  legacyImported?: boolean
  chats: BrowserChat[]
}

export type HistoryError = 'storage' | 'conflict' | 'limit' | 'corrupt' | null

type ConversationState = {
  history: History
  isLoadingMessages: boolean
  error: HistoryError
  hasLegacy: boolean
  hydrate: () => void
  flush: () => void
  updateMessages: (
    updater: Message[] | ((messages: Message[]) => Message[])
  ) => void
  setDraft: (draft: string) => void
  newChat: () => boolean
  selectChat: (id: string) => boolean
  deleteChat: () => boolean
  importLegacy: () => boolean
}

function emptyChat(): BrowserChat {
  return { id: nanoid(), updatedAt: Date.now(), draft: '', messages: [] }
}

export function createBrowserConversationStore(userId: number | undefined) {
  const storageKey = getPlaygroundStorageKey('playground_conversations', userId)
  const first = emptyChat()
  let expectedRaw: string | null = null
  let recoveryRaw: string | null = null
  let dirty = false
  let timer: ReturnType<typeof setTimeout> | undefined

  const store = createStore<ConversationState>()((set, get) => {
    const persist = (history: History): boolean => {
      if (!storageKey) return false
      try {
        // Preserve another tab's changes and the current in-memory draft.
        if (localStorage.getItem(storageKey) !== expectedRaw) {
          set({ error: 'conflict' })
          return false
        }
        const serialized = JSON.stringify({
          ...history,
          chats: history.chats.map((chat) => ({
            ...chat,
            messages: prepareStoredMessages(chat.messages),
          })),
        })
        if (new Blob([serialized]).size > MAX_HISTORY_BYTES) {
          set({ error: 'storage' })
          return false
        }
        if (recoveryRaw !== null) {
          localStorage.setItem(`${storageKey}:recovery`, recoveryRaw)
          recoveryRaw = null
        }
        localStorage.setItem(storageKey, serialized)
        expectedRaw = serialized
        dirty = false
        set({ error: null })
        return true
      } catch {
        set({ error: 'storage' })
        return false
      }
    }

    const commit = (history: History): boolean => {
      if (
        useAuthStore.getState().auth.user?.id !== userId ||
        get().isLoadingMessages
      ) {
        return false
      }
      clearTimeout(timer)
      if (!persist(history)) return false
      set({ history })
      return true
    }

    const changeActiveChat = (
      update: (chat: BrowserChat) => BrowserChat
    ): void => {
      if (
        useAuthStore.getState().auth.user?.id !== userId ||
        get().isLoadingMessages
      ) {
        return
      }
      const history = get().history
      const chats = history.chats.map((chat) =>
        chat.id === history.activeId
          ? { ...update(chat), updatedAt: Date.now() }
          : chat
      )
      set({ history: { ...history, chats } })
      dirty = true
      clearTimeout(timer)
      timer = setTimeout(() => get().flush(), 500)
    }

    return {
      history: { version: 1, activeId: first.id, chats: [first] },
      isLoadingMessages: true,
      error: null,
      hasLegacy: false,
      hydrate: () => {
        if (!get().isLoadingMessages) return
        try {
          if (storageKey) expectedRaw = localStorage.getItem(storageKey)
          if (expectedRaw) {
            if (new Blob([expectedRaw]).size > MAX_HISTORY_BYTES) {
              throw new Error('History too large')
            }
            const saved = historySchema.parse(JSON.parse(expectedRaw))
            if (
              new Set(saved.chats.map((chat) => chat.id)).size !==
              saved.chats.length
            ) {
              throw new Error('Duplicate chat')
            }
            const chats = saved.chats.map((chat) => ({
              ...chat,
              messages: normalizeStoredMessages(chat.messages),
            }))
            if (!chats.some((chat) => chat.id === saved.activeId)) {
              throw new Error('Missing active chat')
            }
            set({ history: { ...saved, chats } })
          } else if (useAuthStore.getState().auth.user?.id === userId) {
            const messages = loadMessages()
            if (messages?.length) {
              set({
                history: {
                  version: 1,
                  activeId: first.id,
                  chats: [{ ...first, messages }],
                },
              })
            }
          }
          set({
            hasLegacy:
              !get().history.legacyImported &&
              localStorage.getItem(STORAGE_KEYS.MESSAGES) !== null,
          })
        } catch {
          recoveryRaw = expectedRaw
          set({ error: 'corrupt' })
        }
        set({ isLoadingMessages: false })
      },
      flush: () => {
        clearTimeout(timer)
        if (dirty) persist(get().history)
      },
      updateMessages: (updater) =>
        changeActiveChat((chat) => ({
          ...chat,
          messages:
            typeof updater === 'function' ? updater(chat.messages) : updater,
        })),
      setDraft: (draft) => changeActiveChat((chat) => ({ ...chat, draft })),
      newChat: () => {
        if (get().isLoadingMessages) return false
        const history = get().history
        const active = history.chats.find(
          (chat) => chat.id === history.activeId
        )
        if (!active?.messages.length && !active?.draft) return true
        if (history.chats.length >= MAX_CHATS) {
          set({ error: 'limit' })
          return false
        }
        const chat = emptyChat()
        return commit({
          ...history,
          activeId: chat.id,
          chats: [chat, ...history.chats],
        })
      },
      selectChat: (id) => {
        const history = get().history
        return (
          history.chats.some((chat) => chat.id === id) &&
          commit({ ...history, activeId: id })
        )
      },
      deleteChat: () => {
        const history = get().history
        const remaining = history.chats.filter(
          (chat) => chat.id !== history.activeId
        )
        const chats = remaining.length ? remaining : [emptyChat()]
        return commit({ ...history, activeId: chats[0].id, chats })
      },
      importLegacy: () => {
        try {
          const raw = localStorage.getItem(STORAGE_KEYS.MESSAGES)
          if (!raw || raw.length > MAX_HISTORY_BYTES) return false
          const messages = normalizeStoredMessages(JSON.parse(raw))
          const history = get().history
          if (!messages.length || history.legacyImported) return false
          if (history.chats.length >= MAX_CHATS) {
            set({ error: 'limit' })
            return false
          }
          const chat = { ...emptyChat(), messages }
          if (
            !commit({
              ...history,
              activeId: chat.id,
              chats: [chat, ...history.chats],
              legacyImported: true,
            })
          ) {
            return false
          }
          // Original unowned records remain intact as a migration backup.
          set({ hasLegacy: false })
          return true
        } catch {
          set({ error: 'corrupt' })
          return false
        }
      },
    }
  })
  return store
}
