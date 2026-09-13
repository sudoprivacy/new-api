package dto

import (
	"encoding/json"
	"testing"
)

// A field this struct does not model must survive the decode/encode round trip
// the relay performs outside the pass-through path.
//
// Concretely: `diagnostics` opts a request into Anthropic's cache diagnosis
// (`cache-diagnosis-2026-04-07`). Measured before this fix, a client sending it
// got 4 of 4 diagnoses talking to Anthropic directly and 0 of 4 through this
// gateway, because the field was dropped here. The same silence applies to
// every top-level field Anthropic ships after this struct was last updated.
func TestClaudeRequestPreservesUnknownTopLevelFields(t *testing.T) {
	raw := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":1024,
		"messages":[{"role":"user","content":"hi"}],
		"diagnostics":{"previous_message_id":"msg_123"},
		"some_future_field":[1,2,3]
	}`)

	var req ClaudeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Model != "claude-sonnet-4-6" {
		t.Fatalf("modelled field lost: %q", req.Model)
	}

	encoded, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}

	got, ok := out["diagnostics"]
	if !ok {
		t.Fatalf("diagnostics was dropped; body was %s", encoded)
	}
	if string(got) != `{"previous_message_id":"msg_123"}` {
		t.Fatalf("diagnostics altered: %s", got)
	}
	if _, ok := out["some_future_field"]; !ok {
		t.Fatalf("unknown field was dropped; body was %s", encoded)
	}
	if _, ok := out["messages"]; !ok {
		t.Fatalf("modelled field missing from output: %s", encoded)
	}
}

// With nothing unmodelled present the encoding must be byte-identical to the
// plain struct encoding — the common request keeps its exact previous shape, so
// this change cannot perturb ordinary traffic.
func TestClaudeRequestWithoutExtrasEncodesUnchanged(t *testing.T) {
	raw := []byte(`{"model":"claude-sonnet-4-6","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	var req ClaudeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	withHook, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	type plain ClaudeRequest
	withoutHook, err := json.Marshal((*plain)(&req))
	if err != nil {
		t.Fatalf("marshal plain: %v", err)
	}

	if string(withHook) != string(withoutHook) {
		t.Fatalf("encoding changed for a request with no extras:\n got: %s\nwant: %s", withHook, withoutHook)
	}
}

// A leftover must never overwrite a field the struct owns. The relay clears
// some fields deliberately (service_tier, inference_geo, speed); resurrecting
// them from the original body would defeat that and can cost real money.
func TestClaudeRequestExtrasNeverOverrideModelledFields(t *testing.T) {
	var req ClaudeRequest
	if err := json.Unmarshal([]byte(`{"model":"claude-sonnet-4-6","service_tier":"priority"}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The relay clears it after decoding.
	req.ServiceTier = ""

	encoded, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if _, resurrected := out["service_tier"]; resurrected {
		t.Fatalf("cleared field came back: %s", encoded)
	}
}
