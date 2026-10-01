// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/upstream_catalog"
)

func fetchTrustedChannelCatalog(channel *model.Channel) ([]string, map[string]upstream_catalog.Entry, error) {
	if channel.Type != constant.ChannelTypeAnthropic && channel.Type != constant.ChannelTypeOpenAI {
		return nil, nil, fmt.Errorf("trusted catalog sync requires an Anthropic or OpenAI channel")
	}
	base := strings.TrimRight(channel.GetBaseURL(), "/")
	if base == "" {
		base = strings.TrimRight(constant.ChannelBaseURLs[channel.Type], "/")
	}
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	endpoint, err := url.Parse(base + "/models")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid model catalog endpoint")
	}
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, nil, fmt.Errorf("catalog key unavailable")
	}
	headers, err := buildFetchModelsHeaders(channel, strings.TrimSpace(key))
	if err != nil {
		return nil, nil, sanitizeFetchModelsError(err, key)
	}
	ids := []string{}
	entries := map[string]upstream_catalog.Entry{}
	seenCursors := map[string]bool{}
	cursor := ""
	for page := 0; page < 20; page++ {
		q := endpoint.Query()
		q.Set("limit", "1000")
		if cursor != "" {
			q.Set("after_id", cursor)
		}
		endpoint.RawQuery = q.Encode()
		body, err := fetchTrustedCatalogPage(endpoint.String(), channel, headers)
		if err != nil {
			return nil, nil, sanitizeFetchModelsError(err, key)
		}
		if len(body) > 4<<20 {
			return nil, nil, fmt.Errorf("model catalog exceeds response limit")
		}
		var envelope struct {
			Data *[]struct {
				ID              string `json:"id"`
				ContextWindow   int    `json:"context_window"`
				MaxOutputTokens int    `json:"max_output_tokens"`
				MaxInputTokens  int    `json:"max_input_tokens"`
				MaxTokens       int    `json:"max_tokens"`
				VisionSupported *bool  `json:"vision_supported"`
				Capabilities    struct {
					ImageInput struct {
						Supported *bool `json:"supported"`
					} `json:"image_input"`
				} `json:"capabilities"`
				Pricing *upstream_catalog.Pricing `json:"reference_pricing"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if common.Unmarshal(body, &envelope) != nil || envelope.Data == nil || len(*envelope.Data) == 0 {
			return nil, nil, fmt.Errorf("upstream returned an empty or invalid model catalog")
		}
		for _, item := range *envelope.Data {
			if strings.TrimSpace(item.ID) == "" {
				return nil, nil, fmt.Errorf("catalog has an invalid model ID")
			}
			if _, exists := entries[item.ID]; exists {
				continue
			}
			entry := upstream_catalog.Entry{ChannelID: channel.Id, ContextWindow: item.ContextWindow,
				MaxOutputTokens: item.MaxOutputTokens, VisionSupported: item.VisionSupported, Pricing: item.Pricing}
			if item.MaxInputTokens > 0 {
				entry.ContextWindow = item.MaxInputTokens
			}
			if item.MaxTokens > 0 {
				entry.MaxOutputTokens = item.MaxTokens
			}
			if item.Capabilities.ImageInput.Supported != nil {
				entry.VisionSupported = item.Capabilities.ImageInput.Supported
			}
			if entry.ContextWindow < 0 || entry.MaxOutputTokens < 0 {
				return nil, nil, fmt.Errorf("catalog has invalid token limits")
			}
			if entry.Pricing != nil {
				expr, priceErr := entry.Pricing.Expression()
				if priceErr == nil {
					priceErr = billing_setting.SmokeTestExpr(expr)
				}
				if priceErr != nil {
					return nil, nil, fmt.Errorf("invalid catalog reference price for %s", item.ID)
				}
			}
			entries[item.ID] = entry
			ids = append(ids, item.ID)
		}
		if !envelope.HasMore {
			return ids, entries, nil
		}
		if envelope.LastID == "" || seenCursors[envelope.LastID] {
			return nil, nil, fmt.Errorf("catalog pagination did not advance")
		}
		cursor = envelope.LastID
		seenCursors[cursor] = true
	}
	return nil, nil, fmt.Errorf("catalog pagination exceeded its limit")
}

func fetchTrustedCatalogPage(endpoint string, channel *model.Channel, headers http.Header) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog request")
	}
	request.Header = headers.Clone()
	request.Host = headers.Get("Host")
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog proxy")
	}
	// Do not forward channel credentials to a redirected host.
	copyClient := *client
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copyClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("catalog request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("catalog response was interrupted")
	}
	return body, nil
}

func collectTrustedCatalogChanges(channel *model.Channel) ([]string, []string, error) {
	ids, entries, err := fetchTrustedChannelCatalog(channel)
	if err != nil {
		return nil, nil, err
	}
	mapping := normalizeChannelModelMapping(channel)
	for alias, target := range mapping {
		if entry, ok := entries[target]; ok {
			entries[alias] = entry
		}
	}
	if err := model.MergeUpstreamCatalog(channel.Id, entries); err != nil {
		return nil, nil, err
	}
	settings := channel.GetOtherSettings()
	added, removed := collectPendingUpstreamModelChangesFromModels(channel.GetModels(), ids, settings.UpstreamModelUpdateIgnoredModels, mapping)
	return added, removed, nil
}
