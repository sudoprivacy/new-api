# sudoapi: Verify real Responses token accounting through chat conversion.
"""Accept a fresh localhost gateway with a real, funded Responses upstream.

Required: LIVE_UPSTREAM_URL, LIVE_UPSTREAM_KEY, LIVE_USAGE_MODEL.
Optional: LIVE_GATEWAY_URL (http://127.0.0.1:18769), LIVE_USAGE_REPORT.
The gateway must be disposable and uninitialized. Keep its database in tmpfs,
disable batched updates, and remove its container after running this test.
No model request is retried. Reports contain counters and no credentials.
"""

import json
import os
import re
import secrets
import time
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import urlencode, urlparse
from urllib.request import ProxyHandler, Request, build_opener


def main():
    base = os.environ.get("LIVE_GATEWAY_URL", "http://127.0.0.1:18769").rstrip("/")
    if urlparse(base).hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise SystemExit("Use a fresh, disposable localhost gateway")
    model = os.environ["LIVE_USAGE_MODEL"]
    upstream = os.environ["LIVE_UPSTREAM_URL"].rstrip("/")
    upstream_key = os.environ["LIVE_UPSTREAM_KEY"]
    opener = build_opener(ProxyHandler({}))
    report = {"passed": False, "realUpstream": True, "model": model, "requests": []}
    action = "setup"

    def request(method, path, body=None, token=None, stream=False):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        req = Request(
            base + path,
            method=method,
            headers=headers,
            data=json.dumps(body).encode() if body is not None else None,
        )
        try:
            response = opener.open(req, timeout=120)
        except HTTPError as error:
            response = error
        with response:
            raw = response.read().decode("utf-8")
            if response.status != 200:
                reason = raw.replace(upstream_key, "[redacted]")
                raise RuntimeError(
                    f"{method} {path}: HTTP {response.status}: {reason[:500]}"
                )
            data = raw if stream else json.loads(raw)
        if not stream and data.get("success") is False:
            raise RuntimeError(
                f"{method} {path}: {data.get('message', 'request rejected')}"
            )
        return data

    try:
        setup = request("GET", "/api/setup")["data"]
        assert not setup["status"] and not setup["root_init"], "Gateway must be fresh"
        password = secrets.token_urlsafe(24)
        request(
            "POST",
            "/api/setup",
            {
                "username": "usagecheck",
                "password": password,
                "confirmPassword": password,
                "SelfUseModeEnabled": True,
            },
        )
        admin = request(
            "POST", "/api/user/login", {"username": "usagecheck", "password": password}
        )["data"]["access_token"]
        request(
            "POST",
            "/api/channel/",
            {
                "mode": "single",
                "channel": {
                    "name": "live-responses-usage",
                    "type": 1,
                    "key": upstream_key,
                    "base_url": upstream,
                    "models": model,
                    "group": "default",
                    "status": 1,
                },
            },
            admin,
        )
        for key, value in {
            "ModelRatio": {model: 1},
            "CompletionRatio": {model: 1},
            "global.chat_completions_to_responses_policy": {
                "enabled": True,
                "all_channels": True,
                "model_patterns": ["^" + re.escape(model) + "$"],
            },
        }.items():
            request(
                "PUT", "/api/option/", {"key": key, "value": json.dumps(value)}, admin
            )
        token_name = "live-usage-" + secrets.token_hex(6)
        issued = request(
            "POST",
            "/api/token/",
            {
                "name": token_name,
                "unlimited_quota": True,
                "expired_time": -1,
                "group": "default",
            },
            admin,
        )["data"]
        token = issued["key"]
        before = request("GET", "/api/user/self", token=admin)["data"]

        action = "real native Responses request"
        marker = "RESPONSE_USAGE_" + secrets.token_hex(6)
        answer = request(
            "POST",
            "/v1/responses",
            {
                "model": model,
                "input": "Reply with exactly " + marker,
                "max_output_tokens": 512,
                "stream": False,
            },
            token,
        )
        text = "".join(
            part.get("text", "")
            for item in answer.get("output", [])
            for part in item.get("content", [])
            if part.get("type") == "output_text"
        )
        assert marker in text, "Native Responses reply did not contain the fresh marker"
        usage = answer["usage"]
        assert usage["input_tokens"] > 0 and usage["output_tokens"] > 0, usage
        report["requests"].append(
            {
                "path": "/v1/responses",
                "stream": False,
                "prompt_tokens": usage["input_tokens"],
                "completion_tokens": usage["output_tokens"],
                "total_tokens": usage["total_tokens"],
            }
        )

        action = "real streaming chat converted to Responses"
        marker = "CHAT_USAGE_" + secrets.token_hex(6)
        raw = request(
            "POST",
            "/v1/chat/completions",
            {
                "model": model,
                "messages": [
                    {"role": "user", "content": "Reply with exactly " + marker}
                ],
                "max_tokens": 512,
                "stream": True,
                "stream_options": {"include_usage": True},
            },
            token,
            stream=True,
        )
        frames = []
        done = False
        for line in raw.splitlines():
            if not line.startswith("data:"):
                continue
            payload = line[5:].strip()
            if payload == "[DONE]":
                done = True
            elif payload:
                frames.append(json.loads(payload))
        assert done and not any("error" in frame for frame in frames), (
            "Incomplete/error SSE"
        )
        text = "".join(
            choice.get("delta", {}).get("content") or ""
            for frame in frames
            for choice in frame.get("choices", [])
        )
        assert marker in text, "Streaming chat reply did not contain the fresh marker"
        usages = [frame["usage"] for frame in frames if frame.get("usage")]
        assert len(usages) == 1, "Expected one final chat usage frame"
        usage = usages[0]
        assert usage["prompt_tokens"] > 0 and usage["completion_tokens"] > 0, usage
        assert "input_tokens" not in usage and "output_tokens" not in usage, (
            "Mixed protocol fields"
        )
        report["requests"].append(
            {
                "path": "/v1/chat/completions",
                "stream": True,
                **{
                    k: usage[k]
                    for k in ("prompt_tokens", "completion_tokens", "total_tokens")
                },
            }
        )

        action = "settlement and request-correlated consumption logs"
        deadline = time.monotonic() + 10
        while True:
            data = request(
                "GET",
                "/api/log/?"
                + urlencode(
                    {"token_name": token_name, "type": 2, "p": 1, "page_size": 10}
                ),
                token=admin,
            )["data"]
            rows = data if isinstance(data, list) else data.get("items", [])
            rows = [row for row in rows if row.get("token_name") == token_name]
            if len(rows) == 2:
                break
            assert time.monotonic() < deadline, (
                "Both completed requests must have consumption logs"
            )
            time.sleep(0.2)
        expected = sorted(
            (v["prompt_tokens"], v["completion_tokens"]) for v in report["requests"]
        )
        actual = sorted((v["prompt_tokens"], v["completion_tokens"]) for v in rows)
        assert actual == expected, {"expected": expected, "logged": actual}
        assert all(row["model_name"] == model and row["quota"] > 0 for row in rows)
        assert len({row["request_id"] for row in rows}) == 2, (
            "Each request needs its own log"
        )
        after = request("GET", "/api/user/self", token=admin)["data"]
        charged = sum(row["quota"] for row in rows)
        assert before["quota"] - after["quota"] == charged, (
            "Quota must settle exactly once"
        )
        assert after["used_quota"] - before["used_quota"] == charged, (
            "Used quota must match logs"
        )
        assert after["request_count"] - before["request_count"] == 2
        report.update(
            {
                "passed": True,
                "chargedQuota": charged,
                "requestCountDelta": 2,
                "loggedTokenCountsMatchReplies": True,
                "exactlyOnceSettlement": True,
            }
        )
        print(json.dumps(report))
    except BaseException as error:
        report.update(
            {"action": action, "error": str(error).replace(upstream_key, "[redacted]")}
        )
        print(json.dumps(report))
        raise
    finally:
        if output := os.environ.get("LIVE_USAGE_REPORT"):
            Path(output).write_text(json.dumps(report, indent=2), encoding="utf-8")


if __name__ == "__main__":
    main()
