<!-- sudoapi: Enforce documented deployment tool capabilities before relay billing. -->
# Deployment tool support

`ModelMetadata` accepts an optional `tool_calling_supported` boolean. An explicit
`false` means this gateway deployment accepts plain chat but cannot run tools.
Omission means unknown; `true` and unknown preserve existing request behavior.

Publish `false` only when all routes for that public model ID lack tool support.
If providers expose different capabilities, give the deployments distinct public
IDs. This setting describes the deployed route, not every provider of a model
family. No Llama model names are disabled in code.

OpenAI, Anthropic and Gemini model listings carry the field. Trusted upstream
catalog refreshes preserve an explicit value when a later response omits it.
Operators can correct it through the existing `ModelMetadata` option.

Requests with tool declarations fail with HTTP 400 before billing or inference
when support is false. The check covers Chat Completions, legacy functions,
Messages, Responses, Responses compaction and Gemini. Plain chat stays usable.

## Live acceptance

Run the complete application in a disposable localhost Docker container with its
database in tmpfs. Set `LIVE_UPSTREAM_URL`, `LIVE_UPSTREAM_KEY` and
`LIVE_TEXT_ONLY_MODEL`, then run:

```sh
python tests/live_tool_capability.py
```

The script requires an uninitialized gateway and defaults to port 18764. It
creates a local account and channel, makes a funded plain-chat request, checks
all six tool request shapes return the capability error, and verifies that
quota and request counts remain unchanged for rejected requests. It does not
retry failed inference. Remove the container afterward to discard the upstream
key stored in its disposable database.

`LIVE_SCODE_CONFIG_HOME` optionally writes a config containing only the local
test token for scode's `text_only_endpoint_rejects_agent_work_before_inference`
live PTY test. HTTP integration regressions also run in the normal Go CI suite.
