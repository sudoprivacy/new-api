// sudoapi: Browser-local Playground conversations.
import { MessageSquarePlus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'

import type {
  BrowserChat,
  HistoryError,
} from '../../lib/storage/sudo-conversation-store'

type Props = {
  chats: BrowserChat[]
  activeId: string
  disabled: boolean
  error: HistoryError
  hasLegacy: boolean
  onNewChat: () => void
  onSelectChat: (id: string) => void
  onImportLegacy: () => void
}

export function ConversationToolbar(props: Props) {
  const { t } = useTranslation()
  const [importOpen, setImportOpen] = useState(false)
  let errorMessage = ''
  if (props.error === 'storage') {
    errorMessage = t(
      'Chats could not be saved in this browser. Your current chat is still open.'
    )
  }
  if (props.error === 'conflict') {
    errorMessage = t(
      'Saved chats changed in another tab. Keep any unsaved text, then reload this page.'
    )
  }
  if (props.error === 'limit') {
    errorMessage = t(
      'Recent chats is full. Delete a saved chat before starting another.'
    )
  }
  if (props.error === 'corrupt') {
    errorMessage = t(
      'Saved chats could not be read. The original data has been kept.'
    )
  }

  return (
    <div className='mx-auto w-full max-w-4xl shrink-0 space-y-2 px-4 py-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Button disabled={props.disabled} onClick={props.onNewChat} size='sm'>
          <MessageSquarePlus aria-hidden='true' className='size-4' />
          {t('New chat')}
        </Button>
        <select
          aria-label={t('Recent chats')}
          className='border-input bg-background h-9 min-w-0 flex-1 truncate rounded-md border px-2 text-sm sm:max-w-sm'
          disabled={props.disabled}
          onChange={(event) => props.onSelectChat(event.target.value)}
          value={props.activeId}
        >
          {[...props.chats]
            .sort((a, b) => b.updatedAt - a.updatedAt)
            .map((chat) => {
              const title = chat.messages.find(
                (message) => message.from === 'user'
              )?.versions[0]?.content
              return (
                <option key={chat.id} value={chat.id}>
                  {title?.trim().slice(0, 60) ||
                    chat.draft.trim().slice(0, 60) ||
                    t('New chat')}
                </option>
              )
            })}
        </select>
        <span className='text-muted-foreground text-xs'>
          {t('Saved in this browser')}
        </span>
      </div>
      {props.hasLegacy && (
        <Button
          disabled={props.disabled}
          onClick={() => setImportOpen(true)}
          size='sm'
          variant='outline'
        >
          {t('Import older chat')}
        </Button>
      )}
      {errorMessage && (
        <p role='alert' className='text-destructive text-sm'>
          {errorMessage}
        </p>
      )}
      <ConfirmDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        title={t('Import older chat?')}
        desc={t(
          'This browser has a chat from an older version. Import it only if it belongs to you. The original copy will be kept.'
        )}
        confirmText={t('Import')}
        handleConfirm={() => {
          props.onImportLegacy()
          setImportOpen(false)
        }}
      />
    </div>
  )
}
