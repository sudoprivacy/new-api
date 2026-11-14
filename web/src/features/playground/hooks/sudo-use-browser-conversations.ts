// sudoapi: Browser-local Playground conversations.
import { useEffect, useMemo } from 'react'
import { useStore } from 'zustand'

import { useAuthStore } from '@/stores/auth-store'

import { createBrowserConversationStore } from '../lib/storage/sudo-conversation-store'

export function useBrowserConversations() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const store = useMemo(() => createBrowserConversationStore(userId), [userId])
  const state = useStore(store)

  useEffect(() => {
    store.getState().hydrate()
    const flush = () => store.getState().flush()
    window.addEventListener('pagehide', flush)
    return () => {
      window.removeEventListener('pagehide', flush)
      flush()
    }
  }, [store])

  const active = state.history.chats.find(
    (chat) => chat.id === state.history.activeId
  )
  return {
    ...state,
    messages: active?.messages ?? [],
    draft: active?.draft ?? '',
  }
}
