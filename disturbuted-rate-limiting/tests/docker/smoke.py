#!/usr/bin/env python3
"""Exercise the built image in isolated Compose stacks; requires Docker Compose v2."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
ALGORITHMS = ("fixed_window", "sliding_window_log", "sliding_window_counter", "token_bucket", "leaky_bucket")
# Ignore host HTTP proxy settings for requests to loopback containers.
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def request(base, path, expected):
    try:
        response = HTTP.open(base + path, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        if response.code != expected:
            raise AssertionError(f"{base}{path}: expected {expected}, got {response.code}: {body!r}")
        if expected == 429 and not response.headers.get("Retry-After"):
            raise AssertionError("Rate-limited response omitted Retry-After")
        if expected == 200 and path.startswith('/check/'):
            forwarded = json.loads(body)
            if forwarded.get("path") != path:
                raise AssertionError(f"Backend did not receive original path: {forwarded!r}")


def exercise(mode):
    project = "rl-smoke-" + uuid.uuid4().hex[:12]
    with tempfile.TemporaryDirectory(prefix="rl-smoke-") as directory:
        config = json.loads((ROOT / "configs/proxy.json").read_text())
        config["routes"] = []
        for algorithm in ALGORITHMS:
            route = {
                "pattern": "/check/" + algorithm,
                "policy": {"id": algorithm, "identity": "ip", "per_route": False},
                "algorithm": algorithm,
                "limit": 3,
                "window": "1h",
                "max_keys": 100,
                "revision": "ci-v1",
            }
            if algorithm in ("token_bucket", "leaky_bucket"):
                route["capacity"] = 3
            config["routes"].append(route)
        config_path = Path(directory) / "proxy.json"
        config_path.write_text(json.dumps(config))
        override = Path(directory) / "compose.yaml"
        # Override both the port and mount, so no developer stack/config is reused.
        gateway = f"""    ports: !override [\"127.0.0.1::8080\"]
    volumes:
      - {json.dumps(str(config_path) + ':/etc/ratelimit/proxy.json:ro')}
    environment:
      RATE_LIMIT_STORAGE: {mode}
      RATE_LIMIT_NAMESPACE: {project}
      RATE_LIMIT_CREDENTIALS: '[]'
      RATE_LIMIT_UPSTREAM: http://backend:8080
"""
        override.write_text("services:\n  gateway:\n" + gateway + "  replica:\n"
                            "    extends:\n      file: " + json.dumps(str(ROOT / "compose.yaml")) + "\n      service: gateway\n"
                            + gateway + ("    depends_on:\n      redis: {condition: service_healthy}\n      backend: {condition: service_healthy}\n" if mode == "redis" else ""))
        command = ["docker", "compose", "--project-directory", str(ROOT), "-p", project, "-f", str(ROOT / "compose.yaml")]
        if mode == "redis":
            command += ["-f", str(ROOT / "compose.redis.yaml")]
        command += ["-f", str(override)]
        env = dict(os.environ, RATE_LIMIT_IMAGE=os.environ.get("RATE_LIMIT_IMAGE", "rate-limiter:ci"))

        def compose(*args, capture=False):
            return subprocess.run(command + list(args), cwd=ROOT, env=env, check=True,
                                  stdout=subprocess.PIPE if capture else None, text=True).stdout

        try:
            compose("up", "-d", "--no-build", "--wait", "--wait-timeout", "90")
            urls = ["http://" + compose("port", service, "8080", capture=True).strip() for service in ("gateway", "replica")]
            for url in urls:
                request(url, "/healthz", 200)
                request(url, "/readyz", 200)
            for algorithm in ALGORITHMS:
                path = "/check/" + algorithm
                if mode == "redis":
                    for index in range(3):
                        request(urls[index % 2], path, 200)
                    for url in urls:
                        request(url, path, 429)
                else:
                    for url in urls:
                        for _ in range(3):
                            request(url, path, 200)
                        request(url, path, 429)
            compose("restart", "gateway")
            # Docker may reassign the ephemeral host port on restart.
            deadline = time.monotonic() + 30
            while True:
                try:
                    urls[0] = "http://" + compose("port", "gateway", "8080", capture=True).strip()
                    request(urls[0], "/readyz", 200)
                    break
                except (OSError, AssertionError):
                    if time.monotonic() >= deadline:
                        raise
                    time.sleep(0.2)
            for algorithm in ALGORITHMS:
                request(urls[0], "/check/" + algorithm, 429 if mode == "redis" else 200)
            if mode == "redis":
                compose("stop", "redis")
                for url in urls:
                    request(url, "/healthz", 200)
                    request(url, "/readyz", 503)
                    for algorithm in ALGORITHMS:
                        request(url, "/check/" + algorithm, 503)
            print(f"PASS {mode}: forwarding, all five quotas, replicas, restart" + (", Redis outage" if mode == "redis" else ""), flush=True)
        except BaseException:
            subprocess.run(command + ["logs", "--no-color"], cwd=ROOT, env=env, check=False)
            raise
        finally:
            compose("down", "-v", "--remove-orphans")


if __name__ == "__main__":
    exercise("ram")
    exercise("redis")
