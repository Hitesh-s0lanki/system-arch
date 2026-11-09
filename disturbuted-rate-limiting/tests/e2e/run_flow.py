#!/usr/bin/env python3
"""Docker HTTP flow checks and latency measurements; public upstream GETs are bounded."""

import argparse
import concurrent.futures
import hashlib
import http.client
import http.server
import json
import math
import os
import pathlib
import secrets
import socket
import ssl
import statistics
import subprocess
import tempfile
import threading
import time
import urllib.parse
from datetime import datetime, timezone

ROOT = pathlib.Path(__file__).resolve().parents[2]


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True, stderr=subprocess.PIPE).strip()


class Client:
    def __init__(self, url):
        parsed = urllib.parse.urlsplit(url)
        self.host = parsed.hostname
        self.base_path = parsed.path or "/"
        if parsed.scheme == "https":
            self.connection = http.client.HTTPSConnection(parsed.hostname, parsed.port, timeout=15,
                                                         context=ssl.create_default_context())
        else:
            self.connection = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=15)

    def request(self, path=None, method="GET", body=None, headers=None):
        started = time.perf_counter_ns()
        self.connection.request(method, path or self.base_path, body=body,
                                headers={"Accept-Encoding": "identity", **(headers or {})})
        response = self.connection.getresponse()
        ttfb = (time.perf_counter_ns() - started) / 1e6
        payload = response.read()
        total = (time.perf_counter_ns() - started) / 1e6
        return {"status": response.status, "ttfb_ms": ttfb, "total_ms": total,
                "bytes": len(payload), "sha256": hashlib.sha256(payload).hexdigest(),
                "headers": dict((k.lower(), v) for k, v in response.getheaders()), "body": payload}

    def close(self):
        self.connection.close()


class Backend(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    calls = 0
    lock = threading.Lock()

    def setup(self):
        super().setup()
        self.connection.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)

    def log_message(self, *_):
        pass

    def do_GET(self):
        self.respond()

    def do_POST(self):
        self.respond()

    def respond(self):
        with self.lock:
            type(self).calls += 1
        parsed = urllib.parse.urlsplit(self.path)
        query = urllib.parse.parse_qs(parsed.query)
        if parsed.path == "/slow":
            time.sleep(.6)
        time.sleep(min(float(query.get("work_ms", [0])[0]), 10) / 1000)
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        payload = json.dumps({"method": self.command, "path": parsed.path, "query": parsed.query,
                              "body": body.decode(), "headers": dict(self.headers)}).encode()
        self.send_response(201 if self.command == "POST" else 200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.send_header("X-Backend", "flow-fixture")
        self.end_headers()
        try:
            self.wfile.write(payload)
        except (BrokenPipeError, ConnectionResetError):
            pass


def stats(results):
    values = sorted(r["total_ms"] for r in results)
    quantile = lambda q: values[max(0, math.ceil(q * len(values)) - 1)]
    return {"samples": len(values), "mean_ms": statistics.mean(values), "p50_ms": statistics.median(values),
            "p95_ms": quantile(.95), "p99_ms": quantile(.99),
            "ttfb_p50_ms": statistics.median(r["ttfb_ms"] for r in results),
            "min_ms": values[0], "max_ms": values[-1]}


def route(pattern, name, limit=100000, identity="ip", per_route=False, capacity=100):
    return {"pattern": pattern, "policy": {"id": name, "identity": identity, "per_route": per_route},
            "limit": limit, "window": "1h", "max_keys": capacity}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--upstream", required=True, help="Public HTTPS upstream; only sequential GETs are sent")
    parser.add_argument("--samples", type=int, default=200, help="Controlled local samples per path")
    parser.add_argument("--report", default="reports/flow-results.json")
    args = parser.parse_args()
    public = urllib.parse.urlsplit(args.upstream)
    if public.scheme != "https" or not public.hostname or public.query or public.fragment or public.username:
        parser.error("use an HTTPS URL without query, fragment, or credentials")
    if not 20 <= args.samples <= 1000:
        parser.error("controlled samples must be between 20 and 1000")
    report = {"started_at": datetime.now(timezone.utc).isoformat(), "public_upstream": args.upstream,
              "methodology": {"controlled": "Interleaved sequential requests, warmed persistent connections, 2ms backend work; includes Docker networking.",
                              "public": "Six admitted sequential GETs per proxy (one cold plus five warm), six direct GETs, then local rejections. No public concurrency/load test.",
                              "limitations": "Public N=5 warm samples cannot establish p95/p99. WAN/CDN/TLS/Docker variability prevents isolating limiter cost from public timings."},
              "cases": [], "controlled_latency": {}, "public_latency": {}}
    created, clients = [], []
    name = "rl-flow-" + secrets.token_hex(4)
    token_a, token_b = secrets.token_urlsafe(32), secrets.token_urlsafe(32)
    credentials = json.dumps([{"token": token_a, "tenant_id": "test", "user_id": "user-a", "api_key_id": "key-a"},
                              {"token": token_b, "tenant_id": "test", "user_id": "user-b", "api_key_id": "key-b"}])
    backend = http.server.ThreadingHTTPServer(("0.0.0.0", 0), Backend)
    backend.daemon_threads = True
    threading.Thread(target=backend.serve_forever, daemon=True).start()
    report["environment"] = {"client": "Python http.client on host", "proxy": "Go Docker image rate-limiter:local",
                             "backend": "Controlled host HTTP fixture + actual public HTTPS origin",
                             "redis": "Redis 7.4 in Docker", "concurrency": "8 controlled workers, 40 requests"}

    def new_client(url):
        client = Client(url)
        clients.append(client)
        return client

    def check(name, condition, detail=None):
        report["cases"].append({"name": name, "passed": bool(condition), "detail": detail})
        if not condition:
            raise AssertionError(name + ": " + str(detail))

    try:
        docker("network", "create", name)
        redis_name = name + "-redis"
        docker("run", "-d", "--name", redis_name, "--network", name, "--network-alias", "redis",
               "redis:7.4-alpine", "redis-server", "--save", "", "--appendonly", "no")
        created.append(redis_name)
        with tempfile.TemporaryDirectory(prefix=".tmp-flow-", dir=ROOT) as temp:
            def start_proxy(label, mode, upstream, rules, timeout="200ms"):
                config = {"version": 1, "listen": ":8080", "upstream": upstream,
                          "storage": {"mode": mode, "redis_url": "redis://redis:6379/0", "namespace": name + "-" + label, "timeout": "500ms"},
                          "cleanup_interval": "100ms", "upstream_timeout": timeout, "anonymous_tenant": "public",
                          "trusted_proxies": [], "forward_authorization": False, "routes": rules}
                path = pathlib.Path(temp) / (label + ".json")
                path.write_text(json.dumps(config))
                container = name + "-" + label
                command = ["run", "-d", "--name", container, "--network", name, "-p", "127.0.0.1::8080",
                           "-v", str(path) + ":/etc/ratelimit/proxy.json:ro", "-e", "RATE_LIMIT_CREDENTIALS=" + credentials]
                if os.uname().sysname == "Linux":
                    command += ["--add-host", "host.docker.internal:host-gateway"]
                docker(*command, "rate-limiter:local")
                created.append(container)
                port = docker("port", container, "8080/tcp").split(":")[-1]
                client = new_client("http://127.0.0.1:" + port)
                deadline = time.monotonic() + 15
                while True:
                    try:
                        if client.request("/readyz")["status"] == 200:
                            break
                    except (OSError, http.client.HTTPException):
                        client.close()
                    if time.monotonic() >= deadline:
                        raise RuntimeError("proxy startup failed: " + docker("logs", container))
                    time.sleep(.1)
                return client, container, port

            local = "http://host.docker.internal:" + str(backend.server_port)
            rules = [route("/", "performance"), route("GET /quota", "quota", 3),
                     route("GET /concurrent", "concurrent", 10), route("GET /auth", "auth", 2, "user"),
                     route("GET /capacity", "capacity", 1, "user", capacity=1),
                     route("GET /users", "users", 1, "user"), route("GET /credential", "credential", 1, "api_key"),
                     route("GET /items/{id}", "items", 1, "user", True),
                     route("GET /other-items/{id}", "items", 1, "user", True), route("GET /reset", "reset", 1)]
            rules[-1]["window"] = "1s"
            controlled = {}
            for mode in ("ram", "redis"):
                client, container, port = start_proxy("controlled-" + mode, mode, local, rules)
                controlled[mode] = (client, container, port)
                headers_a, headers_b = {"Authorization": "Bearer " + token_a}, {"Authorization": "Bearer " + token_b}
                before = Backend.calls
                responses = [client.request("/quota", headers={"X-Forwarded-For": "198.51.100." + str(i)}) for i in range(5)]
                check(mode + ": allowance and spoof resistance", [r["status"] for r in responses] == [200, 200, 200, 429, 429])
                check(mode + ": blocked requests never reach backend", Backend.calls - before == 3)
                check(mode + ": Retry-After", all(int(r["headers"].get("retry-after", "0")) > 0 for r in responses[3:]))
                for label, headers, status in [("missing", {}, 401), ("invalid", {"Authorization": "Bearer invalid"}, 401),
                                                ("forged", {"X-User-ID": "user-a"}, 401), ("valid", headers_a, 200)]:
                    check(mode + ": authentication " + label, client.request("/auth", headers=headers)["status"] == status)
                check(mode + ": independent users", [client.request("/users", headers=h)["status"] for h in (headers_a, headers_b, headers_a)] == [200, 200, 429])
                check(mode + ": credential quota", [client.request("/credential", headers=headers_a)["status"] for _ in range(2)] == [200, 429])
                check(mode + ": capacity", [client.request("/capacity", headers=h)["status"] for h in (headers_a, headers_b)] == [200, 503])
                check(mode + ": canonical route grouping", [client.request(p, headers=headers_a)["status"] for p in ("/items/1", "/items/2", "/other-items/1")] == [200, 429, 200])
                before = Backend.calls
                def concurrent_request(_):
                    temporary = Client("http://127.0.0.1:" + port)
                    try: return temporary.request("/concurrent")["status"]
                    finally: temporary.close()
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    statuses = list(pool.map(concurrent_request, range(40)))
                check(mode + ": concurrent quota", statuses.count(200) == 10 and statuses.count(429) == 30, {"allowed": statuses.count(200), "blocked": statuses.count(429)})
                check(mode + ": concurrent backend count", Backend.calls - before == 10)
                response = client.request("/echo?x=1&x=2", method="POST", body="payload", headers={"X-User-ID": "forged", "Authorization": "Bearer secret"})
                echo = json.loads(response["body"])
                check(mode + ": method body query and response", response["status"] == 201 and echo["method"] == "POST" and echo["body"] == "payload" and echo["query"] == "x=1&x=2" and response["headers"].get("x-backend") == "flow-fixture")
                forwarded = {k.lower(): v for k, v in echo["headers"].items()}
                check(mode + ": strip unverified identity and token", "x-user-id" not in forwarded and "authorization" not in forwarded)
                check(mode + ": upstream timeout", client.request("/slow")["status"] == 504)
                check(mode + ": upgrades unsupported", client.request("/", headers={"Connection": "Upgrade", "Upgrade": "websocket"})["status"] == 501)
                time.sleep(1 - time.time() % 1 + .03)
                check(mode + ": reset first allowance", client.request("/reset")["status"] == 200)
                check(mode + ": reset exhausted", client.request("/reset")["status"] == 429)
                time.sleep(1 - time.time() % 1 + .03)
                check(mode + ": next window", client.request("/reset")["status"] == 200)

            direct = new_client("http://127.0.0.1:" + str(backend.server_port))
            measurements = {"direct": [], "ram": [], "redis": []}
            paths = {"direct": direct, **{mode: pair[0] for mode, pair in controlled.items()}}
            for client in paths.values():
                for _ in range(5): client.request("/echo?work_ms=2")
            for index in range(args.samples):
                order = list(paths)
                order = order[index % 3:] + order[:index % 3]
                for label in order:
                    response = paths[label].request("/echo?work_ms=2")
                    check_status = response["status"] == 200
                    if not check_status: raise AssertionError("performance request rejected")
                    measurements[label].append(response)
            report["controlled_latency"] = {k: stats(v) for k, v in measurements.items()}
            report["controlled_latency"]["paired_added_ms"] = {
                mode: stats([{"total_ms": measurements[mode][i]["total_ms"] - measurements["direct"][i]["total_ms"], "ttfb_ms": 0}
                             for i in range(args.samples)]) for mode in ("ram", "redis")}

            public_direct = new_client(args.upstream)
            public_proxies = {}
            for mode in ("ram", "redis"):
                client, _, _ = start_proxy("public-" + mode, mode,
                    urllib.parse.urlunsplit((public.scheme, public.netloc, "", "", "")),
                    [route("/", "public-ip", 6)], "10s")
                public_proxies[mode] = client
            public_samples = {"direct": [], "ram": [], "redis": []}
            public_clients = {"direct": public_direct, **public_proxies}
            cold = {}
            for label, client in public_clients.items():
                response = client.request(public.path or "/")
                check(label + ": public HTTPS response", response["status"] == 200, response["status"])
                cold[label] = {k: v for k, v in response.items() if k not in ("body", "headers")}
            for index in range(5):
                order = list(public_clients)
                order = order[index % 3:] + order[:index % 3]
                for label in order:
                    response = public_clients[label].request(public.path or "/")
                    check(label + ": public warm GET " + str(index + 1), response["status"] == 200)
                    public_samples[label].append(response)
                    time.sleep(.1)
            report["public_latency"] = {label: {"cold": cold[label], "warm": stats(values),
                "response_bytes": [r["bytes"] for r in values], "response_hashes": sorted(set(r["sha256"] for r in values))}
                for label, values in public_samples.items()}
            # Five warm public requests are enough for a smoke comparison,
            # not defensible tail-latency estimates.
            for values in report["public_latency"].values():
                values["warm"].pop("p95_ms", None)
                values["warm"].pop("p99_ms", None)
            for mode, client in public_proxies.items():
                blocked = [client.request(public.path or "/", headers={"X-Forwarded-For": "198.51.100." + str(i)}) for i in range(50)]
                check(mode + ": public origin IP quota cannot be spoofed", all(r["status"] == 429 for r in blocked))
                report["public_latency"][mode]["blocked"] = stats(blocked)
            check("public response bodies match direct", all(set(r["sha256"] for r in public_samples[mode]) == set(r["sha256"] for r in public_samples["direct"]) for mode in public_proxies))

            redis_client = controlled["redis"][0]
            docker("stop", redis_name)
            try:
                before = Backend.calls
                check("Redis outage returns 503", redis_client.request("/echo")["status"] == 503)
                check("Redis outage does not reach backend", Backend.calls == before)
                check("Redis readiness fails", redis_client.request("/readyz")["status"] == 503)
                check("Redis liveness remains healthy", redis_client.request("/healthz")["status"] == 200)
            finally:
                docker("start", redis_name)
            deadline = time.monotonic() + 10
            while redis_client.request("/readyz")["status"] != 200:
                if time.monotonic() > deadline: raise RuntimeError("Redis did not recover")
                time.sleep(.1)
            check("Redis recovery forwards again", redis_client.request("/echo")["status"] == 200)
            report["success"] = True
    except Exception as error:
        report["success"] = False
        report["error"] = str(error)
        raise
    finally:
        for client in clients: client.close()
        backend.shutdown()
        backend.server_close()
        for container in reversed(created):
            try: docker("rm", "-f", container)
            except subprocess.CalledProcessError: pass
        try: docker("network", "rm", name)
        except subprocess.CalledProcessError: pass
        report["completed_at"] = datetime.now(timezone.utc).isoformat()
        output = ROOT / args.report
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, indent=2) + "\n")
        print("Flow report:", output)
        print("Passed cases:", sum(case["passed"] for case in report["cases"]), "/", len(report["cases"]))
        print("Controlled latency:", json.dumps(report["controlled_latency"], indent=2))
        print("Public latency:", json.dumps(report["public_latency"], indent=2))


if __name__ == "__main__":
    main()
