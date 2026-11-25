#!/usr/bin/env python3
"""Send requests through the Gateway and retain every result, including errors."""

import argparse
import csv
import json
import sys
import urllib.error
import urllib.request
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:8888/version")
    parser.add_argument("--count", type=int, choices=range(100, 201), default=200)
    parser.add_argument("--output", type=Path, default=Path("evidence/gateway-90-10"))
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    counts = Counter()
    errors = 0
    started = datetime.now(timezone.utc).isoformat()
    # Bypass system HTTP proxies: the default target is a local port-forward.
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with (args.output / "requests.csv").open("w", newline="", encoding="utf-8") as stream:
        writer = csv.writer(stream)
        writer.writerow(["request", "status", "version", "pod", "body", "error"])
        for number in range(1, args.count + 1):
            status, version, pod, body, error = "", "", "", "", ""
            try:
                request = urllib.request.Request(args.url, headers={
                    "Connection": "close", "X-Request-ID": f"gateway-90-10-{number}"
                })
                with client.open(request, timeout=10) as response:
                    status = response.status
                    version = response.headers.get("X-App-Version", "")
                    pod = response.headers.get("X-Pod-Name", "")
                    body = response.read().decode("utf-8")
                if status != 200 or version not in ("v1", "v2"):
                    error = "Expected HTTP 200 and X-App-Version v1 or v2"
            except (urllib.error.URLError, OSError) as exc:
                error = str(exc)
            if error:
                errors += 1
            else:
                counts[version] += 1
            writer.writerow([number, status, version, pod, body.strip(), error])
            stream.flush()
    summary = {
        "started_at_utc": started,
        "finished_at_utc": datetime.now(timezone.utc).isoformat(),
        "url": args.url,
        "requests": args.count,
        "counts": {version: counts[version] for version in ("v1", "v2")},
        "percentages": {version: counts[version] / args.count * 100 for version in ("v1", "v2")},
        "errors": errors,
    }
    result = json.dumps(summary, ensure_ascii=False, indent=2) + "\n"
    (args.output / "summary.json").write_text(result, encoding="utf-8")
    print(result, end="")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
