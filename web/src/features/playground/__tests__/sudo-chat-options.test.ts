import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createElement } from 'react'
// sudoapi: Chat-compatible Playground model selection.
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Playground } from '..'
import { getUserGroups, getUserModels } from '../api'
import { getInputControlState } from '../lib/input/input-control-utils'
import {
  getGroupFallback,
  getModelFallback,
} from '../lib/options/playground-option-utils'
import { saveConfig } from '../lib/storage/storage'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
  getFreshAuthHeaders: vi.fn(async () => ({})),
}))

const catalog = {
  success: true,
  auto_groups: ['default', 'OpenAI'],
  data: [
    {
      model_name: 'MS-Decision-1',
      enable_groups: ['default'],
      supported_endpoint_types: ['openai', 'systemone'],
    },
    {
      model_name: 'seedance-video',
      enable_groups: ['default'],
      supported_endpoint_types: ['seedance'],
    },
    {
      model_name: 'gpt-4o',
      enable_groups: ['OpenAI'],
      supported_endpoint_types: ['openai'],
    },
    {
      model_name: 'native-response',
      enable_groups: ['OpenAI'],
      supported_endpoint_types: ['openai-response'],
    },
  ],
}

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
  vi.mocked(api.get).mockImplementation(async (path) => {
    if (path === '/api/pricing') return { data: catalog }
    if (path === '/api/user/models') {
      return {
        data: {
          success: true,
          data: [
            'MS-Decision-1',
            'seedance-video',
            'gpt-4o',
            'native-response',
          ],
        },
      }
    }
    if (path === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: {
            default: { ratio: 1 },
            OpenAI: { ratio: 1 },
            auto: { ratio: 1 },
          },
        },
      }
    }
    throw new Error('Unexpected API path')
  })
  vi.mocked(api.post).mockResolvedValue({
    data: {
      id: 'default-chat',
      object: 'chat.completion',
      model: 'gpt-4o',
      created: 1,
      choices: [
        {
          index: 0,
          finish_reason: 'stop',
          message: { role: 'assistant', content: 'Default chat works' },
        },
      ],
    },
  })
})

test('chat options exclude decision, video and response-only endpoints', async () => {
  expect(await getUserModels('auto')).toEqual([
    { label: 'gpt-4o', value: 'gpt-4o' },
  ])
})

test('groups without chat routes are excluded while the authorized auto group stays available', async () => {
  expect((await getUserGroups()).map((group) => group.value)).toEqual([
    'OpenAI',
    'auto',
  ])
})

test('an unavailable saved group falls back to the server-authorized auto group', () => {
  expect(
    getGroupFallback(
      [
        { label: 'OpenAI', value: 'OpenAI', ratio: 1 },
        { label: 'Auto', value: 'auto', ratio: 1 },
      ],
      'default'
    )
  ).toBe('auto')
})

test('an invalid saved model uses the configured default when it is available', () => {
  expect(
    getModelFallback(
      [
        { label: 'First', value: 'expensive-first' },
        { label: 'GPT', value: 'gpt-4o' },
      ],
      'MS-Decision-1'
    )
  ).toBe('gpt-4o')
})

test('an existing available model and group remain selected', () => {
  expect(
    getModelFallback(
      [{ label: 'Existing', value: 'existing-model' }],
      'existing-model'
    )
  ).toBeNull()
  expect(
    getGroupFallback(
      [
        { label: 'Existing', value: 'existing-group', ratio: 1 },
        { label: 'Auto', value: 'auto', ratio: 1 },
      ],
      'existing-group'
    )
  ).toBeNull()
})

test('catalog failure keeps the chat selector from presenting unchecked models', async () => {
  vi.mocked(api.get).mockImplementation(async (path) =>
    path === '/api/pricing'
      ? { data: { success: false, data: [] } }
      : { data: { success: true, data: ['MS-Decision-1'] } }
  )
  await expect(getUserModels('default')).rejects.toThrow()
})

test.each(['fresh browser', 'saved decision model'])(
  '%s starts a usable chat without manually choosing another group or model',
  async (scenario) => {
    if (scenario === 'saved decision model') {
      saveConfig({ group: 'default', model: 'MS-Decision-1', stream: false })
    } else saveConfig({ stream: false })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      createElement(QueryClientProvider, { client }, createElement(Playground))
    )
    const user = userEvent.setup()
    await waitFor(() =>
      expect(screen.getAllByRole('button', { name: /^Send/ })[0]).toBeDisabled()
    )
    await user.type(
      await screen.findByRole('textbox'),
      'Hello from default chat'
    )
    await waitFor(() =>
      expect(screen.getAllByRole('button', { name: /^Send/ })[0]).toBeEnabled()
    )
    await user.keyboard('{Enter}')
    await screen.findByText('Default chat works')
    expect(vi.mocked(api.post).mock.calls.at(-1)?.[1]).toMatchObject({
      model: 'gpt-4o',
      group: 'auto',
    })
  }
)

test('a cached model list cannot enable Send while the new group is loading', () => {
  expect(
    getInputControlState({
      disabled: false,
      hasStopHandler: false,
      groups: [{ label: 'Auto', value: 'auto', ratio: 1 }],
      models: [{ label: 'Old', value: 'old-model' }],
      isModelLoading: true,
      text: 'New question',
    }).canSubmit
  ).toBe(false)
})

test.each(['Enter', 'starter prompt'])(
  '%s waits for the chat catalog and preserves the draft until a usable route is ready',
  async (trigger) => {
    const originalGet = vi.mocked(api.get).getMockImplementation()
    if (!originalGet) throw new Error('Missing API fixture')
    let releaseCatalog: (value: { data: typeof catalog }) => void = () =>
      undefined
    const pendingCatalog = new Promise<{ data: typeof catalog }>((resolve) => {
      releaseCatalog = resolve
    })
    vi.mocked(api.get).mockImplementation((...args) =>
      args[0] === '/api/pricing' ? pendingCatalog : originalGet(...args)
    )
    saveConfig({ stream: false })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      createElement(QueryClientProvider, { client }, createElement(Playground))
    )
    const user = userEvent.setup()
    const input = await screen.findByRole('textbox')
    const prompt = trigger === 'Enter' ? 'Wait for a chat route' : 'Code'
    if (trigger === 'Enter') {
      await user.type(input, `${prompt}{Enter}`)
    } else {
      await user.click(screen.getByRole('button', { name: /^Code$/ }))
    }
    expect(api.post).not.toHaveBeenCalled()
    expect(input).toHaveValue(prompt)
    await act(async () => releaseCatalog({ data: catalog }))
    await waitFor(() =>
      expect(screen.getAllByRole('button', { name: /^Send/ })[0]).toBeEnabled()
    )
    await user.click(input)
    await user.keyboard('{Enter}')
    await screen.findByText('Default chat works')
  }
)

test('an empty model response keeps Send disabled but lets the user choose another group', async () => {
  const originalGet = vi.mocked(api.get).getMockImplementation()
  if (!originalGet) throw new Error('Missing API fixture')
  vi.mocked(api.get).mockImplementation((...args) =>
    args[0] === '/api/user/models'
      ? Promise.resolve({ data: { success: true, data: [] } })
      : originalGet(...args)
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    createElement(QueryClientProvider, { client }, createElement(Playground))
  )
  const user = userEvent.setup()
  await user.type(await screen.findByRole('textbox'), 'Keep this draft')
  await waitFor(() =>
    expect(
      screen
        .getAllByRole('combobox', { hidden: true })
        .find((element) => element.tagName === 'BUTTON')
    ).toBeEnabled()
  )
  expect(screen.getAllByRole('button', { name: /^Send/ })[0]).toBeDisabled()
})
