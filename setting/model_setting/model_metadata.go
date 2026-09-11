// sudoapi: Per-model capability metadata registry.

package model_setting

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// ModelMetadata stores per-model metadata like context window size, max output
// tokens, and image-input capability.
//
// Managed as a system option (key "ModelMetadata") following the same pattern as
// ModelRatio. Exposed to clients on the OpenAI-style GET /v1/models response
// (see controller.enrichModelMetadata), which is the contract sudocode reads to
// relay each model's image capability to sudowork (ACP _meta.imageCapability).
//
// Image-capability field semantics (image-cap unification, design doc
// "Image handling should be NON-user-facing", Decision 1 / comment #5):
//   - VisionSupported is a *bool on purpose: nil means "not yet known to
//     sudorouter" (omitted from the wire → consumers apply their own default),
//     whereas an explicit false means "this model is documented as text-only".
//     Conflating the two would make a non-vision model look vision-capable.
//   - ImageMaxBytes / ImageMaxDimension are 0 when unknown (omitted from the
//     wire → consumers fall back to a conservative default and log, so the
//     missing entry can be backfilled here). They are the model's documented
//     per-image limits, not a gateway-wide constant.
type ModelMetadata struct {
	ContextWindow     int   `json:"context_window"`
	MaxOutputTokens   int   `json:"max_output_tokens"`
	VisionSupported   *bool `json:"vision_supported,omitempty"`
	ImageMaxBytes     int   `json:"image_max_bytes,omitempty"`
	ImageMaxDimension int   `json:"image_max_dimension,omitempty"`
}

// Documented per-image byte limits, sourced from each provider's API docs.
const (
	imgMaxBytes5MB  = 5 << 20  // 5242880 — Anthropic Messages API per-image hard limit
	imgMaxBytes7MB  = 7 << 20  // Google Gemini inline-image practical limit
	imgMaxBytes20MB = 20 << 20 // OpenAI per-image limit
)

// claudeImageMaxDimension is Anthropic's documented long-edge pixel ceiling.
const claudeImageMaxDimension = 8000

// visionPtr returns a *bool for the VisionSupported field. Use it only when the
// model's vision capability is actually documented; leave the field nil (unset)
// when unknown so the value degrades gracefully downstream.
func visionPtr(v bool) *bool { return &v }

var (
	modelMetadataMap   = make(map[string]ModelMetadata)
	modelMetadataMutex sync.RWMutex
)

// defaultModelMetadata provides sensible defaults for well-known models.
// Token values sourced from official documentation as of June 2026.
// Image-capability values sourced from each provider's documented per-image
// limits; models whose image limits are not confidently documented are left
// unset (graceful degradation — see ModelMetadata doc comment).
// Admins can override via PUT /api/option/ with key "ModelMetadata".
var defaultModelMetadata = map[string]ModelMetadata{
	// ---- Anthropic Claude (all vision-capable; 5 MB / 8000 px per Messages API) ----
	"claude-fable-5-1":           {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-fable-5":             {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-opus-5":              {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-opus-4-8":            {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-opus-4-7":            {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-opus-4-6":            {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-opus-4-5":            {ContextWindow: 200000, MaxOutputTokens: 64000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-opus-4-5-20251101":   {ContextWindow: 200000, MaxOutputTokens: 64000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-sonnet-5":            {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-sonnet-4-6":          {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-sonnet-4-5":          {ContextWindow: 200000, MaxOutputTokens: 64000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-sonnet-4-5-20250929": {ContextWindow: 200000, MaxOutputTokens: 64000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-haiku-4-5":           {ContextWindow: 200000, MaxOutputTokens: 64000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},
	"claude-haiku-4-5-20251001":  {ContextWindow: 200000, MaxOutputTokens: 64000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes5MB, ImageMaxDimension: claudeImageMaxDimension},

	// ---- OpenAI GPT (vision-capable; 20 MB per image, no documented px cap) ----
	"gpt-5.5":            {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.5-pro":        {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.6-luna":       {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.6-sol":        {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.6-terra":      {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.4":            {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.4-pro":        {ContextWindow: 1000000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.4-mini":       {ContextWindow: 400000, MaxOutputTokens: 128000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.4-nano":       {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.3-codex":      {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.3-chat":       {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.2":            {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.2-pro":        {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.2-codex":      {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.2-chat":       {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.1":            {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.1-codex":      {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.1-codex-max":  {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.1-codex-mini": {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5.1-chat":       {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5":              {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5-pro":          {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5-codex":        {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5-chat":         {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5-mini":         {ContextWindow: 128000, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-5-nano":         {ContextWindow: 128000, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-4.5-preview":    {ContextWindow: 128000, MaxOutputTokens: 16384, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-4.1":            {ContextWindow: 1047576, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-4.1-mini":       {ContextWindow: 1047576, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-4.1-nano":       {ContextWindow: 1047576, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-4o":             {ContextWindow: 128000, MaxOutputTokens: 16384, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"gpt-4o-mini":        {ContextWindow: 128000, MaxOutputTokens: 16384, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},

	// ---- OpenAI o-series (o1-mini / o3-mini are text-only) ----
	"o4-mini":               {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"o3":                    {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"o3-mini":               {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(false)},
	"o3-pro":                {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"o3-deep-research":      {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"o4-mini-deep-research": {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"o1":                    {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},
	"o1-mini":               {ContextWindow: 128000, MaxOutputTokens: 65536, VisionSupported: visionPtr(false)},
	"o1-pro":                {ContextWindow: 200000, MaxOutputTokens: 100000, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes20MB},

	// ---- Google Gemini (vision-capable; ~7 MB inline image) ----
	"gemini-2.5-flash":               {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-2.5-flash-lite":          {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-2.5-flash-image":         {ContextWindow: 32768, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-2.5-pro":                 {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3-flash-preview":         {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3-flash-preview-sale":    {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3-flash-preview-stable":  {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3-pro-preview":           {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3-pro-image":             {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3-pro-image-preview":     {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.1-flash-lite":          {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.1-flash-lite-preview":  {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.1-flash-image":         {ContextWindow: 65536, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.1-flash-image-preview": {ContextWindow: 65536, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.1-pro-preview":         {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.1-pro-preview-sale":    {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	"gemini-3.5-flash":               {ContextWindow: 1048576, MaxOutputTokens: 65536, VisionSupported: visionPtr(true), ImageMaxBytes: imgMaxBytes7MB},
	// Live/native-audio models: image-input capability not documented — left unset.
	"gemini-live-2.5-flash-native-audio":                 {ContextWindow: 131072, MaxOutputTokens: 8192},
	"gemini-live-2.5-flash-preview-native-audio-09-2025": {ContextWindow: 131072, MaxOutputTokens: 8192},
	"gemini-2.5-flash-native-audio-preview-12-2025":      {ContextWindow: 131072, MaxOutputTokens: 8192},

	// ---- DeepSeek (text-only) ----
	"deepseek-v3":             {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"deepseek-v3-0324":        {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"deepseek-v3.2":           {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"deepseek-v3.2-exp":       {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"deepseek-v3.2-azure":     {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"deepseek-v3.2-speciale":  {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"deepseek-v4-flash":       {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"deepseek-v4-flash-azure": {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"deepseek-v4-pro":         {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},

	// ---- xAI Grok (vision-capable; documented per-image byte cap not confirmed → unset) ----
	"grok-4":                      {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4-1-fast-reasoning":     {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4-1-fast-non-reasoning": {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4-20-reasoning":         {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4-20-non-reasoning":     {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4-fast-reasoning":       {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4-fast-non-reasoning":   {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},
	"grok-4.3":                    {ContextWindow: 131072, MaxOutputTokens: 32768, VisionSupported: visionPtr(true)},

	// ---- Moonshot / Kimi (text-only) ----
	"Kimi-K2.5":        {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"Kimi-K2.6":        {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"Kimi-K2-Thinking": {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"Kimi-K3":          {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},

	// ---- MiniMax (text-only) ----
	"MiniMax-M2.1":           {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"MiniMax-M2.5":           {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"MiniMax-M2.5-highspeed": {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},
	"MiniMax-M3":             {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(false)},

	// ---- Alibaba Qwen (only -vl- variants are vision-capable) ----
	"qwen-max":                       {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen-plus":                      {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen-vl-max":                    {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(true)},
	"qwen-vl-plus":                   {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(true)},
	"qwen2.5-32b-instruct":           {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen2.5-72b-instruct":           {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen2.5-vl-72b-instruct":        {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(true)},
	"qwen3-235b-a22b":                {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3-30b-a3b":                  {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3-32b":                      {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3-coder-30b-a3b-instruct":   {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3-coder-480b-a35b-instruct": {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3-coder-plus":               {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3.5-397b-a17b":              {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3.5-flash":                  {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3.5-plus":                   {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwen3.6-plus":                   {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},
	"qwq-plus":                       {ContextWindow: 131072, MaxOutputTokens: 8192, VisionSupported: visionPtr(false)},

	// ---- Zhipu GLM (vision capability not confidently documented → unset) ----
	"glm-4.7": {ContextWindow: 128000, MaxOutputTokens: 4096},
	"glm-5":   {ContextWindow: 128000, MaxOutputTokens: 4096},
	"glm-5.1": {ContextWindow: 128000, MaxOutputTokens: 4096},
	"glm-5.2": {ContextWindow: 128000, MaxOutputTokens: 4096},

	// ---- Meta Llama (Llama-4 multimodal; Llama-3.3 text-only) ----
	"Llama-3.3-70B-Instruct":                 {ContextWindow: 131072, MaxOutputTokens: 4096, VisionSupported: visionPtr(false)},
	"Llama-4-Maverick-17B-128E-Instruct-FP8": {ContextWindow: 131072, MaxOutputTokens: 4096, VisionSupported: visionPtr(true)},

	// ---- Doubao (the -vision- variant is vision-capable; others left unset) ----
	"doubao-seed-1-6-251015":        {ContextWindow: 131072, MaxOutputTokens: 16384},
	"doubao-seed-1-6-vision-250815": {ContextWindow: 131072, MaxOutputTokens: 16384, VisionSupported: visionPtr(true)},
	"doubao-seed-2-0-pro-260215":    {ContextWindow: 131072, MaxOutputTokens: 16384},

	// ---- Embeddings (text-only, context window only, no output) ----
	"text-embedding-3-large": {ContextWindow: 8191, MaxOutputTokens: 0, VisionSupported: visionPtr(false)},
	"text-embedding-3-small": {ContextWindow: 8191, MaxOutputTokens: 0, VisionSupported: visionPtr(false)},
	"text-embedding-ada-002": {ContextWindow: 8191, MaxOutputTokens: 0, VisionSupported: visionPtr(false)},
	"text-embedding-v3":      {ContextWindow: 8192, MaxOutputTokens: 0, VisionSupported: visionPtr(false)},
	"text-embedding-v4":      {ContextWindow: 8192, MaxOutputTokens: 0, VisionSupported: visionPtr(false)},
}

func init() {
	modelMetadataMap = mergeModelMetadata(nil)
}

// mergeModelMetadata layers admin overrides on top of the table this build ships.
//
// Capability data describes the models themselves, not operator policy, so a
// model added in a later release has to show up even on a deployment that has
// already saved the "ModelMetadata" option. Taking the stored value as the whole
// table instead would freeze it at whatever was saved and leave every model added
// afterwards with no metadata on /v1/models — clients then fall back to their own
// bundled guess for a model the gateway actually knows about.
//
// It always builds a new map: modelMetadataMap must never alias
// defaultModelMetadata, or an override would rewrite the shipped table.
func mergeModelMetadata(overrides map[string]ModelMetadata) map[string]ModelMetadata {
	merged := make(map[string]ModelMetadata, len(defaultModelMetadata)+len(overrides))
	for name, meta := range defaultModelMetadata {
		merged[name] = meta
	}
	for name, meta := range overrides {
		merged[name] = meta
	}
	return merged
}

// GetModelMetadata returns metadata for a specific model, or nil if not found.
func GetModelMetadata(modelName string) *ModelMetadata {
	modelMetadataMutex.RLock()
	defer modelMetadataMutex.RUnlock()
	if m, ok := modelMetadataMap[modelName]; ok {
		return &m
	}
	return nil
}

// GetAllModelMetadata returns a copy of the full metadata map.
func GetAllModelMetadata() map[string]ModelMetadata {
	modelMetadataMutex.RLock()
	defer modelMetadataMutex.RUnlock()
	result := make(map[string]ModelMetadata, len(modelMetadataMap))
	for k, v := range modelMetadataMap {
		result[k] = v
	}
	return result
}

// UpdateModelMetadataByJSONString applies the "ModelMetadata" system option on top
// of the shipped table. Entries in the option win per model; models it does not
// mention keep the values this build ships.
func UpdateModelMetadataByJSONString(jsonStr string) error {
	var overrides map[string]ModelMetadata
	if err := common.Unmarshal([]byte(jsonStr), &overrides); err != nil {
		return err
	}
	modelMetadataMutex.Lock()
	defer modelMetadataMutex.Unlock()
	modelMetadataMap = mergeModelMetadata(overrides)
	return nil
}

// ModelMetadata2JSONString serializes the current metadata map to JSON.
func ModelMetadata2JSONString() string {
	modelMetadataMutex.RLock()
	defer modelMetadataMutex.RUnlock()
	data, _ := common.Marshal(modelMetadataMap)
	return string(data)
}
