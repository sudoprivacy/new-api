<!-- sudoapi: Verify real Responses token accounting through chat conversion. -->

# Responses usage acceptance

Responses carries `input_tokens` and `output_tokens`. Chat Completions carries
`prompt_tokens` and `completion_tokens`. Decode the Responses fields before
converting or settling a request. A positive `total_tokens` value alone does not
prove that billing received the input and output counts.

`tests/live_responses_usage.py` runs two real requests against an uninitialized,
disposable localhost gateway:

1. Create an isolated administrator, funded-upstream channel, and client token.
2. Send a native Responses request and check its fresh reply and positive usage.
3. Stream Chat Completions through the Responses conversion path, checking its
   fresh reply, final usage frame, and Chat Completions field names.
4. Find both consumption logs by the newly issued token. Match their input and
   output counts to the replies, require positive charges, and reconcile the
   administrator's remaining quota, used quota, and request count exactly.

Use a gateway built from the candidate source. Bind port 3000 to loopback only,
mount `/data` as tmpfs, and set `BATCH_UPDATE_ENABLED=false`. Supply a real,
funded upstream that supports `/v1/responses`; the test does not emulate model
responses. Set the following environment variables without writing keys to a
file or command history:

- `LIVE_GATEWAY_URL`: disposable gateway, default `http://127.0.0.1:18769`.
- `LIVE_UPSTREAM_URL`: upstream origin, without `/v1`.
- `LIVE_UPSTREAM_KEY`: upstream API key.
- `LIVE_USAGE_MODEL`: model available on that upstream.
- `LIVE_USAGE_REPORT`: optional path for counters and acceptance results.

Run `python tests/live_responses_usage.py`. Remove the owned container afterward
to discard its temporary administrator, client token, channel key, and database.
The script never retries a model request. On failure, preserve the report and
diagnose the failure before starting another acceptance run.

The protocol decoding and conversion fixtures also run in the existing backend
CI job, including the independent `relaykit` module tests. Live acceptance is an
additional check; skipped or synthetic tests do not count as real API evidence.
