#!/usr/bin/env python3
"""Run at most six explicit Qoder transport probes and print a redacted report."""

import argparse
import json
import sys
import time
import urllib.error
import urllib.request


def request(url, token, payload=None):
    body = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(url, data=body, method="POST" if body else "GET")
    req.add_header("Authorization", "Bearer " + token)
    if body:
        req.add_header("Content-Type", "application/json")
    started = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            return response.status, round(time.monotonic() - started, 3), response.read(512)
    except urllib.error.HTTPError as error:
        return error.code, round(time.monotonic() - started, 3), error.read(512)
    except urllib.error.URLError as error:
        return 0, round(time.monotonic() - started, 3), str(error.reason).encode()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--api-key", required=True)
    parser.add_argument("--model", required=True)
    args = parser.parse_args()
    base = args.base_url.rstrip("/")
    probes = [
        ("models", base + "/v1/models", None),
        ("chat", base + "/v1/chat/completions", {"model": args.model, "messages": [{"role": "user", "content": "ping"}], "stream": False}),
        ("responses", base + "/v1/responses", {"model": args.model, "input": "ping", "stream": False}),
    ]
    report = []
    for name, url, payload in probes:
        status, seconds, sample = request(url, args.api_key, payload)
        report.append({"probe": name, "status": status, "seconds": seconds, "sample": sample[:120].decode("utf-8", "replace")})
    print(json.dumps({"requests": len(report), "report": report}, ensure_ascii=False, indent=2))
    return 0 if all(item["status"] for item in report) else 1


if __name__ == "__main__":
    sys.exit(main())
