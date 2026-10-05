#!/usr/bin/env python3
"""One bounded readiness observation. Does not restart or send notifications."""
import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def check(endpoint, timeout=3):
    parsed = urllib.parse.urlsplit(endpoint)
    local = parsed.hostname in ("localhost", "127.0.0.1", "::1")
    if not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ("", "/") or (parsed.scheme != "https" and not (local and parsed.scheme == "http")):
        return False
    if not 1 <= timeout <= 10:
        return False
    try:
        opener = urllib.request.build_opener(NoRedirect())
        with opener.open(endpoint.rstrip("/") + "/readyz", timeout=timeout) as response:
            body = response.read(1025)
            return response.status == 200 and len(body) <= 1024 and json.loads(body) == {"status": "ready"}
    except urllib.error.HTTPError as error:
        error.close()
        return False
    except (OSError, ValueError):
        return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--endpoint", default="http://127.0.0.1:1234")
    parser.add_argument("--timeout", type=float, default=3)
    args = parser.parse_args()
    ready = check(args.endpoint, args.timeout)
    print(json.dumps({"status": "ready" if ready else "unavailable"}))
    return 0 if ready else 1


if __name__ == "__main__":
    sys.exit(main())
