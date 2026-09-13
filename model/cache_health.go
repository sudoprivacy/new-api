package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ChannelCacheHealth is one (channel, hour) bucket of prompt-cache behaviour.
//
// Reuse is the number worth watching. Anthropic bills a cache read at 0.1x and
// a cache write at 1.25x, so a channel that halves its reuse roughly doubles
// the input cost of identical work — invisibly, because every request still
// succeeds.
type ChannelCacheHealth struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	// Hour bucket, "2006-01-02 15" in the server's local zone. Empty when the
	// caller asked for totals per channel instead of a time series.
	Hour         string `json:"hour,omitempty"`
	Requests     int64  `json:"requests"`
	CacheRead    int64  `json:"cache_read_tokens"`
	CacheWrite   int64  `json:"cache_write_tokens"`
	PromptTokens int64  `json:"uncached_prompt_tokens"`
	Quota        int64  `json:"quota"`
	// ReusePct is read/(read+write) as a percentage, rounded to one decimal.
	// 0 with traffic present means nothing was cached at all — a different
	// failure from a low hit rate, and usually a different cause (see
	// NoCacheRequests).
	ReusePct float64 `json:"reuse_pct"`
	// WeightedUnits is write*1.25 + read*0.1: the two prices put on one scale
	// so channels are directly comparable.
	WeightedUnits int64 `json:"weighted_units"`
	// NoCacheRequests counts requests that reported neither a read nor a write.
	// A large share here means the traffic is not using prompt caching at all
	// (e.g. it arrived on an OpenAI-compatible path, which carries no
	// cache_control), not that the cache is missing.
	NoCacheRequests int64 `json:"no_cache_requests"`
}

// cacheHealthMaxRows bounds a single report. The cache fields live inside the
// `other` JSON blob rather than in columns, so this aggregates in Go instead of
// SQL — portable across the MySQL/PostgreSQL/SQLite backends new-api supports,
// at the cost of scanning rows. The cap turns "this query melted the database"
// into a truncated flag the caller can see.
const cacheHealthMaxRows = 200000

// GetChannelCacheHealth aggregates prompt-cache behaviour per channel over a
// time window, optionally bucketed by hour.
//
// Bucketing by hour is not a nicety. Averaged over days, a fixed bug keeps
// poisoning the mean: one of our own upstreams read as 30% reuse in a 3-day
// total — worse than a vendor at 41% — while the hourly series showed
// 29% -> 0% -> 94% as two client bugs were fixed. The total pointed at a
// problem that no longer existed.
//
// Returns the buckets and whether the row cap was hit.
func GetChannelCacheHealth(startTimestamp, endTimestamp int64, channelId int, modelName string, byHour bool) ([]*ChannelCacheHealth, bool, error) {
	if endTimestamp <= 0 {
		endTimestamp = time.Now().Unix()
	}
	if startTimestamp <= 0 {
		return nil, false, fmt.Errorf("start_timestamp is required")
	}
	if startTimestamp >= endTimestamp {
		return nil, false, fmt.Errorf("start_timestamp must be before end_timestamp")
	}

	tx := DB.Model(&Log{}).
		Select("channel_id, created_at, model_name, prompt_tokens, quota, other").
		Where("type = ?", LogTypeConsume).
		Where("created_at BETWEEN ? AND ?", startTimestamp, endTimestamp)
	if channelId != 0 {
		tx = tx.Where("channel_id = ?", channelId)
	}
	if modelName != "" {
		tx = tx.Where("model_name LIKE ?", "%"+modelName+"%")
	}

	var rows []Log
	if err := tx.Limit(cacheHealthMaxRows + 1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	truncated := len(rows) > cacheHealthMaxRows
	if truncated {
		rows = rows[:cacheHealthMaxRows]
	}

	type key struct {
		channel int
		hour    string
	}
	buckets := make(map[key]*ChannelCacheHealth)
	for i := range rows {
		row := &rows[i]
		hour := ""
		if byHour {
			hour = time.Unix(row.CreatedAt, 0).Format("2006-01-02 15")
		}
		k := key{channel: row.ChannelId, hour: hour}
		bucket := buckets[k]
		if bucket == nil {
			bucket = &ChannelCacheHealth{ChannelId: row.ChannelId, Hour: hour}
			buckets[k] = bucket
		}
		bucket.Requests++
		bucket.PromptTokens += int64(row.PromptTokens)
		bucket.Quota += int64(row.Quota)

		read, write := cacheTokensFromOther(row.Other)
		bucket.CacheRead += read
		bucket.CacheWrite += write
		if read == 0 && write == 0 {
			bucket.NoCacheRequests++
		}
	}

	names := channelNamesById()
	out := make([]*ChannelCacheHealth, 0, len(buckets))
	for _, bucket := range buckets {
		if total := bucket.CacheRead + bucket.CacheWrite; total > 0 {
			bucket.ReusePct = float64(int64(float64(bucket.CacheRead)/float64(total)*1000+0.5)) / 10
		}
		bucket.WeightedUnits = int64(float64(bucket.CacheWrite)*1.25 + float64(bucket.CacheRead)*0.1)
		bucket.ChannelName = names[bucket.ChannelId]
		out = append(out, bucket)
	}
	sortCacheHealth(out)
	return out, truncated, nil
}

// cacheTokensFromOther pulls the read/write cache counts out of a log's `other`
// blob. The wire names are `cache_tokens` (read) and `cache_creation_tokens`
// (write) — deliberately not symmetric, which is easy to get backwards.
func cacheTokensFromOther(other string) (read int64, write int64) {
	if other == "" {
		return 0, 0
	}
	parsed, err := common.StrToMap(other)
	if err != nil || parsed == nil {
		return 0, 0
	}
	return numberFromAny(parsed["cache_tokens"]), numberFromAny(parsed["cache_creation_tokens"])
}

// numberFromAny coerces a JSON number to int64. Values decoded from the blob
// arrive as float64, but a backend or a future writer may hand back an integer
// type, so both are accepted rather than silently counted as zero.
func numberFromAny(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func sortCacheHealth(rows []*ChannelCacheHealth) {
	// Channel ascending, then hour ascending — a stable order the UI can render
	// directly and a test can assert on.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			a, b := rows[j-1], rows[j]
			if a.ChannelId < b.ChannelId || (a.ChannelId == b.ChannelId && a.Hour <= b.Hour) {
				break
			}
			rows[j-1], rows[j] = rows[j], rows[j-1]
		}
	}
}

func channelNamesById() map[int]string {
	names := make(map[int]string)
	var channels []struct {
		Id   int
		Name string
	}
	if err := DB.Model(&Channel{}).Select("id, name").Find(&channels).Error; err != nil {
		return names
	}
	for _, channel := range channels {
		names[channel.Id] = channel.Name
	}
	return names
}
