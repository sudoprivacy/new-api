// sudoapi: Browser-local Playground conversations.
import { useAuthStore } from '@/stores/auth-store'

// Unowned legacy keys are retained for explicit import, never auto-assigned.
export function getPlaygroundStorageKey(
  baseKey: string,
  userId = useAuthStore.getState().auth.user?.id
): string | null {
  return userId == null ? null : `${baseKey}:${userId}`
}
