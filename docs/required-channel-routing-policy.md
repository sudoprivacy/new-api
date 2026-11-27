<!-- sudoapi: Protect declared gateway channel routes. -->

# Required gateway routes

### Single public key for an SDK comparison

Create a separate, explicit token group with one Messages channel and one
enabled upstream credential. Use a separately issued public token in this
group. Set `SDK_TEST_TOKEN_CHANNEL` to the two database IDs in
`token_id:channel_id` order on every router replica. These are numeric IDs.
Keep the native sudoapi key in the channel credential store. Set up its
private sudoapi routing group with only the test account.

The serving check applies in both directions, including an administrator
selecting a channel, affinity and retry selection. Another token in the
same group cannot use the test channel. The test token only accepts
Messages, Chat Completions and Responses generation routes; the router
makes at most one inference attempt per incoming HTTP request and skips
reset recovery. Keep the token fixed for the whole client conversation.
For rollback disable the public test token, drain in-flight requests, then
change the downstream backend. Start future A/B conversations with new
session IDs. Activate the token only after every serving replica has the
same binding.

Set `REQUIRED_CHANNEL_POLICY_FILE` to a JSON declaration mounted read-only in
the gateway. FujiToken uses Ladder's `deploy/channel-routing-policy.json` as
the single declaration; do not maintain another list of required IDs in code.
The deployment receipt must record its SHA-256 and the image source revision.

With the variable unset, existing behavior is retained. When it is set, an
unreadable, empty, oversized or invalid file blocks channel configuration writes.
Inference and usage accounting do not read this file. Private declaration groups
must also exist in `REQUIRE_EXPLICIT_TOKEN_GROUPS`; serving authorization remains
unchanged.

Supported declaration fields:

- `gateway`: HTTPS gateway origin identifying this declaration.
- `private_groups`: `name`, `allowed_user_ids`, `required_channel_ids`,
  `required_models`. Each required private channel retains only its private group.
- `shared_channels`: `channel_id`, `required_groups`, `required_models`.

The guard preserves Anthropic protocol, required groups/models and enabled
configuration. Single and batch deletes, manual disable, tag edits, full save,
ordinary update and upstream model catalog synchronization use the same model
policy. A prohibited batch or tag edit fails before any selected member changes.
Required rows are locked with the existing SQLite/MySQL/PostgreSQL-compatible
`lockForUpdate` helper inside the write transaction. No schema migration is used.

Real provider health failures may still auto-disable a channel. Configuration
edits may preserve that actual status, but cannot manufacture a new auto-disable.
Removing disabled channels cannot delete declared routes. Additional models and
ordinary metadata remain editable. Changing the declaration is an explicit
operator action before retiring a route; the read-only Ladder `check-routes`
command remains useful for drift checks, including changes made outside APIs.

The file does not change user/key ACLs, account priorities, native micro-pools,
token scopes or backend selection. Its `allowed_user_ids` retains the existing
Ladder audit meaning; this guard does not replace the serving authorization.

Roll out with the reviewed declaration mounted at the configured path, preserving
other gateway environment variables. Verify the file hash in the container,
health and existing routes. Test rejected writes against the real admin API and
read back unchanged channel rows; do not create model traffic for these checks.
Rollback restores the previous image/configuration and read-only mount. Never
restore a database snapshot over live billing writes as an application rollback.
