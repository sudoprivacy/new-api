# sudoapi: Preserve Claude protocol fields and tool inputs.
"""Verify a disposable localhost gateway against a funded Claude upstream.

Required: LIVE_UPSTREAM_URL, LIVE_UPSTREAM_KEY.
Optional: LIVE_GATEWAY_URL (http://127.0.0.1:19081),
LIVE_CLAUDE_MODEL (claude-opus-5-5), LIVE_CLAUDE_REPORT.
LIVE_CLAUDE_MESSAGE_EFFORT defaults to require. Set reject only when verifying
a known upstream refusal: it must identify messages.2.output_config, proving
the gateway preserved that field. The test then checks ordinary continuation.
LIVE_UPSTREAM_HTTP_PROTOCOL may select http1 for the disposable channel.
Set LIVE_CLAUDE_INVOKE_MODEL=1 only for a transparent Bedrock InvokeModel
bridge, which requires the beta value in anthropic_beta instead of a header.
Creates the fresh gateway's root, native Claude channel, and local API token.
Runs three or four dependent API requests; the upstream must support strict
thinking binding. Never point this at a production gateway.
"""

import copy
import hashlib
import json
import os
from pathlib import Path
import secrets
from urllib.error import HTTPError
from urllib.parse import urlparse
from urllib.request import ProxyHandler, Request, build_opener


def main():
    base = os.environ.get("LIVE_GATEWAY_URL", "http://127.0.0.1:19081").rstrip("/")
    if urlparse(base).hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise SystemExit("Use a fresh disposable localhost gateway")
    model = os.environ.get("LIVE_CLAUDE_MODEL", "claude-opus-5-5")
    message_effort = os.environ.get("LIVE_CLAUDE_MESSAGE_EFFORT", "require")
    assert message_effort in {"require", "reject"}
    opener = build_opener(ProxyHandler({}))
    results = []

    def request(method, path, body=None, token=None, expected=200, protocol=False):
        headers = {"Content-Type": "application/json", "User-Agent": "new-api-live-thinking"}
        if token:
            headers["Authorization"] = "Bearer " + token
        if protocol:
            headers.update({"anthropic-version": "2023-06-01", "anthropic-beta":
                            "thinking-binding-controls-2026-08-01,mid-conversation-output-config-2026-07-01"})
        payload = json.dumps(body, ensure_ascii=False, separators=(",", ":")).encode() if body is not None else None
        req = Request(base + path, data=payload, headers=headers, method=method)
        try:
            response = opener.open(req, timeout=120)
        except HTTPError as error:
            response = error
        with response:
            value = json.load(response)
            if protocol:
                results.append({"http": response.status,
                                "request_id": response.headers.get("x-oneapi-request-id"),
                                "request_sha256": hashlib.sha256(payload).hexdigest(),
                                "input_transformations": value.get("input_transformations"),
                                "usage": value.get("usage"), "error": value.get("error")})
            assert response.status == expected, (path, response.status, value.get("error", value.get("message")))
        assert value.get("success", True), (path, value.get("message"))
        return value

    setup = request("GET", "/api/setup")["data"]
    assert not setup["status"] and not setup["root_init"], "Gateway must be fresh and disposable"
    password = secrets.token_urlsafe(24)
    request("POST", "/api/setup", {"username": "livecheck", "password": password,
            "confirmPassword": password, "SelfUseModeEnabled": True})
    login = request("POST", "/api/user/login", {"username": "livecheck", "password": password})["data"]
    admin = login["access_token"]
    request("POST", "/api/channel/", {"mode": "single", "channel": {
        "name": "live-claude-thinking", "type": 14, "key": os.environ["LIVE_UPSTREAM_KEY"],
        "base_url": os.environ["LIVE_UPSTREAM_URL"], "models": model, "group": "default", "status": 1,
        "setting": json.dumps({"pass_through_body_enabled": False,
                               "http_protocol": os.environ.get("LIVE_UPSTREAM_HTTP_PROTOCOL", "auto")}),
    }}, admin)
    for key in ("ModelRatio", "CompletionRatio"):
        request("PUT", "/api/option/", {"key": key, "value": json.dumps({model: 1})}, admin)
    token = request("POST", "/api/token/", {"name": "live-claude-thinking", "unlimited_quota": True,
                    "expired_time": -1, "group": "default"}, admin)["data"]["key"]
    nonce = secrets.token_hex(8)
    n = next(i for i in range(1, 23 * 29 * 31) if i % 23 == 7 and i % 29 == 11 and i % 31 == 13)
    original = {"model": model, "max_tokens": 2048, "stream": False,
                "thinking": {"type": "adaptive", "display": "summarized",
                             "block_binding": {"prefix_mismatch_behavior": "error"}},
                "output_config": {"effort": "high"}, "system": "Arithmetic session " + nonce,
                "metadata": {"user_id": "binding-" + nonce}, "messages": [{"role": "user", "content":
                    "Find the smallest positive integer n with n mod 23 = 7, n mod 29 = 11, and n mod 31 = 13. "
                    "Return only BINDING:" + nonce + ":<n>, replacing <n> with the number."}]}
    if os.environ.get("LIVE_CLAUDE_INVOKE_MODEL") == "1":
        original["anthropic_beta"] = ["thinking-binding-controls-2026-08-01",
                                      "mid-conversation-output-config-2026-07-01"]
    try:
        first = request("POST", "/v1/messages", original, token, protocol=True)
        assert any(b.get("type") == "thinking" and b.get("signature") for b in first["content"]), "Require real signed thinking"
        assert first.get("input_transformations") == [], "The binding protocol must be acknowledged without transformations"
        text = "".join(b.get("text", "") for b in first["content"] if b.get("type") == "text").strip()
        assert text == f"BINDING:{nonce}:{n}", text
        continuation = copy.deepcopy(original)
        continuation["messages"] += [{"role": "assistant", "content": first["content"]},
                                     {"role": "system", "content": [], "output_config": {"effort": "low"}},
                                     {"role": "user", "content": "Using that n, return only NEXT:<n+17>, replacing the placeholder."}]
        if message_effort == "reject":
            refusal = request("POST", "/v1/messages", continuation, token, expected=400, protocol=True)
            assert "messages.2.output_config" in refusal.get("error", {}).get("message", ""), refusal
            # This explicit compatibility arm checks the unchanged conversation
            # without the unsupported feature. Production requests are never
            # rewritten or retried by this test.
            continuation["messages"].pop(2)
        second = request("POST", "/v1/messages", continuation, token, protocol=True)
        assert second.get("input_transformations") == [], "An unchanged prefix must preserve its signed thinking"
        text = "".join(b.get("text", "") for b in second["content"] if b.get("type") == "text").strip()
        assert text == f"NEXT:{n + 17}", text
        edited = copy.deepcopy(continuation)
        edited["system"] += " Changed before the signed block."
        rejected = request("POST", "/v1/messages", edited, token, expected=400, protocol=True)
        assert "bound to a different conversation" in json.dumps(rejected), rejected
        assert len(results) == (4 if message_effort == "reject" else 3)
        print("PASS: real signed response, message effort " + ("refusal preserved" if message_effort == "reject" else "accepted")
              + ", unchanged continuation, edited-prefix refusal", flush=True)
    finally:
        if report := os.environ.get("LIVE_CLAUDE_REPORT"):
            Path(report).write_text(json.dumps({"model": model, "message_effort": message_effort, "results": results}, indent=2), encoding="utf-8")


if __name__ == "__main__":
    main()
