// sudoapi: Browser-local Playground conversations.
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { Playground } from '..'
import { getUserGroups, getUserModels, sendChatCompletion } from '../api'
import { DEFAULT_CONFIG, STORAGE_KEYS } from '../constants'
import { loadConfig, saveConfig } from '../lib/storage/storage'
import type { ChatCompletionResponse } from '../types'

vi.mock('../api', () => ({
  getUserModels: vi.fn(async () => [
    { label: 'Sonnet test', value: 'sonnet-test' },
  ]),
  getUserGroups: vi.fn(async () => [
    { label: 'Demo', value: 'demo', ratio: 1 },
  ]),
  sendChatCompletion: vi.fn(),
}))

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'first', role: 1 })
  saveConfig({
    ...DEFAULT_CONFIG,
    group: 'demo',
    model: 'sonnet-test',
    stream: false,
    temperature: 0.42,
  })
  vi.mocked(sendChatCompletion)
    .mockReset()
    .mockResolvedValue({
      id: 'response',
      object: 'chat.completion',
      model: 'sonnet-test',
      created: 1,
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'Saved reply' },
        },
      ],
    })
})

function renderPlayground(staleTime = 0) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Playground />
    </QueryClientProvider>
  )
}

test('a fresh cache from another account cannot supply its models and groups', async () => {
  vi.mocked(getUserGroups)
    .mockResolvedValueOnce([
      { label: 'First group', value: 'first-group', ratio: 1 },
    ])
    .mockResolvedValueOnce([
      { label: 'Second group', value: 'second-group', ratio: 1 },
    ])
  vi.mocked(getUserModels).mockImplementation(async () => {
    const model =
      useAuthStore.getState().auth.user?.id === 1
        ? 'first-model'
        : 'second-model'
    return [{ label: model, value: model }]
  })
  renderPlayground(Infinity)
  await waitFor(() =>
    expect(loadConfig()).toMatchObject({
      group: 'first-group',
      model: 'first-model',
    })
  )
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'second', role: 1 })
  )
  await waitFor(() =>
    expect(loadConfig()).toMatchObject({
      group: 'second-group',
      model: 'second-model',
    })
  )
  vi.mocked(getUserModels).mockResolvedValue([
    { label: 'Sonnet test', value: 'sonnet-test' },
  ])
})

test('new chat starts a fresh request while preserving previous messages, drafts and model settings', async () => {
  const user = userEvent.setup()
  renderPlayground()
  const input = await screen.findByRole('textbox')
  await user.type(input, 'First request{Enter}')
  await screen.findByText('Saved reply')
  await user.type(input, 'Unsent followup')
  const select = screen.getByRole('combobox', { name: 'Recent chats' })
  const firstId = (select as HTMLSelectElement).value
  await user.click(screen.getByRole('button', { name: 'New chat' }))
  expect(screen.queryByText('Saved reply')).not.toBeInTheDocument()
  expect(screen.getByRole('textbox')).toHaveValue('')
  expect(loadConfig()).toMatchObject({
    model: 'sonnet-test',
    temperature: 0.42,
  })
  await user.type(screen.getByRole('textbox'), 'Second request{Enter}')
  await waitFor(() => expect(sendChatCompletion).toHaveBeenCalledTimes(2))
  expect(vi.mocked(sendChatCompletion).mock.calls[1][0].messages).toEqual([
    { role: 'user', content: 'Second request' },
  ])
  await screen.findByText('Saved reply')
  await user.selectOptions(select, firstId)
  expect(screen.getByRole('textbox')).toHaveValue('Unsent followup')
  expect(
    within(screen.getByRole('log')).getByText('First request')
  ).toBeInTheDocument()
})

test('deleting a chat requires confirmation and preserves the other saved chat', async () => {
  const user = userEvent.setup()
  renderPlayground()
  await user.type(await screen.findByRole('textbox'), 'Keep this draft')
  await user.click(screen.getByRole('button', { name: 'New chat' }))
  await user.type(screen.getByRole('textbox'), 'Delete this draft')
  await user.click(screen.getByRole('button', { name: 'Delete chat' }))
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(screen.getByRole('textbox')).toHaveValue('Delete this draft')
  await user.click(screen.getByRole('button', { name: 'Delete chat' }))
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  expect(screen.getByRole('textbox')).toHaveValue('Keep this draft')
  expect(
    screen
      .getByRole('combobox', { name: 'Recent chats' })
      .querySelectorAll('option')
  ).toHaveLength(1)
})

test('another signed-in account starts empty and returning restores the original draft', async () => {
  const user = userEvent.setup()
  renderPlayground()
  await user.type(await screen.findByRole('textbox'), 'Private first draft')
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'second', role: 1 })
  )
  await waitFor(() => expect(screen.getByRole('textbox')).toHaveValue(''))
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 1, username: 'first', role: 1 })
  )
  await waitFor(() =>
    expect(screen.getByRole('textbox')).toHaveValue('Private first draft')
  )
})

test('legacy messages are shown only after an explicit import confirmation', async () => {
  localStorage.setItem(
    STORAGE_KEYS.MESSAGES,
    JSON.stringify({
      version: 1,
      data: [
        {
          key: 'legacy',
          from: 'user',
          versions: [{ id: 'v1', content: 'Older private request' }],
        },
      ],
    })
  )
  const user = userEvent.setup()
  renderPlayground()
  await user.click(
    await screen.findByRole('button', { name: 'Import older chat' })
  )
  expect(screen.queryByText('Older private request')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Import' }))
  expect(
    await within(screen.getByRole('log')).findByText('Older private request')
  ).toBeInTheDocument()
  expect(localStorage.getItem(STORAGE_KEYS.MESSAGES)).toContain(
    'Older private request'
  )
})

test('new chat and history switching stay disabled until an active request is stopped', async () => {
  let complete: (response: ChatCompletionResponse) => void = () => undefined
  vi.mocked(sendChatCompletion).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve
      })
  )
  const user = userEvent.setup()
  renderPlayground()
  await user.type(await screen.findByRole('textbox'), 'Pending request{Enter}')
  await waitFor(() => expect(sendChatCompletion).toHaveBeenCalledTimes(1))
  expect(screen.getByRole('button', { name: 'New chat' })).toBeDisabled()
  expect(screen.getByRole('combobox', { name: 'Recent chats' })).toBeDisabled()
  await user.click(screen.getAllByRole('button', { name: /^Stop/ })[0])
  expect(screen.getByRole('button', { name: 'New chat' })).toBeEnabled()
  await user.click(screen.getByRole('button', { name: 'New chat' }))
  await act(async () =>
    complete({
      id: 'late',
      object: 'chat.completion',
      model: 'sonnet-test',
      created: 1,
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'Late old answer' },
        },
      ],
    })
  )
  expect(screen.queryByText('Late old answer')).not.toBeInTheDocument()
})
