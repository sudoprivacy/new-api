/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

import { API_ENDPOINTS } from './constants'
import {
  getChatCatalog,
  getChatGroupNames,
  isChatModel,
} from './lib/options/sudo-chat-catalog'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  GroupOption,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group: string): Promise<ModelOption[]> {
  const [res, catalog] = await Promise.all([
    api.get(API_ENDPOINTS.USER_MODELS, { params: { group } }),
    getChatCatalog(),
  ])
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  const chatModels = new Set(
    catalog.data.filter(isChatModel).map((model) => model.model_name)
  )
  return data.data
    .filter((model: string) => chatModels.has(model))
    .map((model: string) => ({
      label: model,
      value: model,
    }))
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const [res, catalog] = await Promise.all([
    api.get(API_ENDPOINTS.USER_GROUPS),
    getChatCatalog(),
  ])
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  const chatGroups = getChatGroupNames(catalog)
  return Object.entries(groupData)
    .filter(([group]) => chatGroups.has(group))
    .map(([group, info]) => ({
      label: group,
      value: group,
      ratio: info.ratio,
      desc: info.desc,
    }))
}
