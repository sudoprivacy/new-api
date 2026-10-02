# sudoapi: Enforce documented deployment tool capabilities before relay billing.
"""Run against a fresh localhost gateway backed by a real funded upstream.

Required: LIVE_UPSTREAM_URL, LIVE_UPSTREAM_KEY, LIVE_TEXT_ONLY_MODEL.
Optional: LIVE_GATEWAY_URL (http://127.0.0.1:18764), LIVE_SCODE_CONFIG_HOME.
The gateway must be disposable: this initializes its root account and settings.
Store its database in tmpfs and remove the container after acceptance.
"""

import json
import os
from pathlib import Path
import secrets
from urllib.error import HTTPError
from urllib.parse import urlparse
from urllib.request import Request, ProxyHandler, build_opener


def main():
    base = os.environ.get("LIVE_GATEWAY_URL", "http://127.0.0.1:18764").rstrip("/")
    if urlparse(base).hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise SystemExit("Use a disposable localhost gateway")
    model = os.environ["LIVE_TEXT_ONLY_MODEL"]
    upstream = os.environ["LIVE_UPSTREAM_URL"]
    upstream_key = os.environ["LIVE_UPSTREAM_KEY"]
    opener = build_opener(ProxyHandler({}))

    def request(method, path, body=None, token=None, expected=200):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        req = Request(base + path, data=json.dumps(body).encode() if body is not None else None,
                      headers=headers, method=method)
        try:
            response = opener.open(req, timeout=90)
        except HTTPError as error:
            response = error
        with response:
            data = json.load(response)
            assert response.status == expected, (path, response.status, data.get("error", data.get("message")))
        assert data.get("success", True), (path, data.get("message"))
        return data

    setup = request("GET", "/api/setup")["data"]
    assert not setup["status"] and not setup["root_init"], "Gateway must be fresh and disposable"
    password = secrets.token_urlsafe(24)
    request("POST", "/api/setup", {"username": "livecheck", "password": password,
            "confirmPassword": password, "SelfUseModeEnabled": True})
    login = request("POST", "/api/user/login", {"username": "livecheck", "password": password})["data"]
    admin = login["access_token"]
    request("POST", "/api/channel/", {"mode": "single", "channel": {
        "name": "live-tool-capability", "type": 1, "key": upstream_key,
        "base_url": upstream, "models": model, "group": "default", "status": 1,
    }}, admin)
    for key, value in {
        "ModelMetadata": {model: {"tool_calling_supported": False}},
        "ModelRatio": {model: 1}, "CompletionRatio": {model: 1},
    }.items():
        request("PUT", "/api/option/", {"key": key, "value": json.dumps(value)}, admin)
    token = request("POST", "/api/token/", {"name": "live-capability", "unlimited_quota": True,
                    "expired_time": -1, "group": "default"}, admin)["data"]["key"]
    catalog = request("GET", "/v1/models", token=token)["data"]
    assert next(item for item in catalog if item["id"] == model)["tool_calling_supported"] is False

    plain = {"model": model, "messages": [{"role": "user", "content": "What is 2+2? Answer with just the number."}],
             "max_tokens": 128, "stream": False}
    answer = request("POST", "/v1/chat/completions", plain, token)
    assert answer["choices"][0]["message"]["content"].strip() == "4", "Unexpected real upstream answer"
    assert answer["usage"]["total_tokens"] > 0
    print("PASS: real upstream plain chat answered 4 with nonzero token usage", flush=True)
    before = request("GET", "/api/user/self", token=admin)["data"]
    function = {"name": "weather", "description": "Weather", "parameters": {"type": "object", "properties": {}}}
    checks = [
        ("/v1/chat/completions", {**plain, "tools": [{"type": "function", "function": function}]}),
        ("/v1/chat/completions", {**plain, "functions": [function]}),
        ("/v1/messages", {**plain, "tools": [{"name": "weather", "input_schema": function["parameters"]}]}),
        ("/v1/responses", {"model": model, "input": "2+2", "tools": [{"type": "function", **function}]}),
        ("/v1/responses/compact", {"model": model, "input": [{"role": "user", "content": "2+2"}],
                                   "tools": [{"type": "function", **function}]}),
        (f"/v1beta/models/{model}:generateContent", {"contents": [{"parts": [{"text": "2+2"}]}],
                                                   "tools": [{"functionDeclarations": [function]}]}),
    ]
    for path, body in checks:
        result = request("POST", path, body, token, expected=400)
        assert "does not support tool calling" in result["error"]["message"], (path, result)
    after = request("GET", "/api/user/self", token=admin)["data"]
    for field in ("quota", "used_quota", "request_count"):
        assert before[field] == after[field], (field, before[field], after[field])
    print("PASS: all six tool request shapes rejected; quota, used quota and request count unchanged", flush=True)
    if directory := os.environ.get("LIVE_SCODE_CONFIG_HOME"):
        target = Path(directory)
        target.mkdir(parents=True, exist_ok=True)
        config = {"auth_modes": {"proxy": {"sudorouter": {
            "baseUrl": base + "/v1", "apiKey": token,
        }}}, "models": {}}
        (target / "sudocode.json").write_text(json.dumps(config), encoding="utf-8")
        print("Wrote isolated CLI config containing only the disposable local token", flush=True)


if __name__ == "__main__":
    main()
