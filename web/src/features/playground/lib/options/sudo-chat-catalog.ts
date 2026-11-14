// sudoapi: Chat-compatible Playground model selection.
import { getPricing } from '@/features/pricing/api'
import type { PricingData } from '@/features/pricing/types'

export async function getChatCatalog(): Promise<PricingData> {
  const catalog = await getPricing()
  if (!catalog.success || !Array.isArray(catalog.data)) {
    throw new Error('Failed to load playground models')
  }
  return catalog
}

export function isChatModel(model: PricingData['data'][number]): boolean {
  const endpoints = model.supported_endpoint_types ?? []
  const nativeOnly = [
    'systemone',
    'seedance',
    'embeddings',
    'jina-rerank',
    'openai-video',
    'image-generation',
  ]
  return (
    endpoints.includes('openai') &&
    !nativeOnly.some((endpoint) => endpoints.includes(endpoint))
  )
}

export function getChatGroupNames(catalog: PricingData): Set<string> {
  const groups = new Set(
    catalog.data.filter(isChatModel).flatMap((model) => model.enable_groups)
  )
  if (catalog.auto_groups?.some((group) => groups.has(group))) {
    groups.add('auto')
  }
  return groups
}
