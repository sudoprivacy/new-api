package model

import (
	"encoding/json"
	"testing"
)

// The two wire names are not symmetric — `cache_tokens` is the READ and
// `cache_creation_tokens` is the WRITE. Getting them backwards inverts every
// reuse figure the report produces, and the result still looks plausible, so
// it is worth a test rather than a comment.
func TestCacheTokensFromOtherReadsTheRightFields(t *testing.T) {
	read, write := cacheTokensFromOther(`{"cache_tokens":900,"cache_creation_tokens":100}`)
	if read != 900 || write != 100 {
		t.Fatalf("read/write swapped: got read=%d write=%d, want 900/100", read, write)
	}
}

// A log without cache fields is the "no caching at all" case, not an error.
// It must report zero rather than failing the whole report.
func TestCacheTokensFromOtherToleratesMissingAndMalformed(t *testing.T) {
	for _, other := range []string{
		"",
		"{}",
		"not json",
		`{"cache_tokens":null}`,
		`{"request_path":"/v1/chat/completions"}`,
	} {
		read, write := cacheTokensFromOther(other)
		if read != 0 || write != 0 {
			t.Fatalf("other=%q should yield 0/0, got %d/%d", other, read, write)
		}
	}
}

// Values decoded from the blob normally arrive as float64, but a json.Number
// (or a plain int, if a future writer builds the map directly) must count too —
// silently treating them as zero would under-report reuse.
func TestNumberFromAnyAcceptsEveryJSONNumericShape(t *testing.T) {
	cases := map[string]struct {
		value any
		want  int64
	}{
		"float64":     {float64(1234), 1234},
		"int":         {int(1234), 1234},
		"int64":       {int64(1234), 1234},
		"json.Number": {json.Number("1234"), 1234},
		"string":      {"1234", 0}, // not a number on the wire; must not guess
		"nil":         {nil, 0},
	}
	for name, tc := range cases {
		if got := numberFromAny(tc.value); got != tc.want {
			t.Fatalf("%s: got %d, want %d", name, got, tc.want)
		}
	}
}

// Ordering is part of the contract: the UI renders the slice directly, so a
// map-iteration order would make the table jump between refreshes.
func TestSortCacheHealthIsStableByChannelThenHour(t *testing.T) {
	rows := []*ChannelCacheHealth{
		{ChannelId: 42, Hour: "2026-09-13 21"},
		{ChannelId: 30, Hour: "2026-09-13 22"},
		{ChannelId: 42, Hour: "2026-09-13 20"},
		{ChannelId: 30, Hour: "2026-09-13 20"},
	}
	sortCacheHealth(rows)
	want := []struct {
		channel int
		hour    string
	}{
		{30, "2026-09-13 20"},
		{30, "2026-09-13 22"},
		{42, "2026-09-13 20"},
		{42, "2026-09-13 21"},
	}
	for i, w := range want {
		if rows[i].ChannelId != w.channel || rows[i].Hour != w.hour {
			t.Fatalf("row %d: got %d/%s, want %d/%s", i, rows[i].ChannelId, rows[i].Hour, w.channel, w.hour)
		}
	}
}
