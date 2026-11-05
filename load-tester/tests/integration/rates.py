#!/usr/bin/env python3
"""Validate fixed RPS, generator saturation, and evidence-based limit suggestions.
Run from load-tester/ against the running Compose deployment. Uses only a local fixture.
"""
import collections
import json
import os
import subprocess
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BASE = os.environ.get('TEST_API_URL', 'http://127.0.0.1:8090')
arrivals = collections.deque()
lock = threading.Lock()

class Target(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_GET(self):
        status = 200
        if self.path == '/limited':
            with lock:
                now = time.monotonic()
                while arrivals and arrivals[0] <= now - 1:
                    arrivals.popleft()
                if len(arrivals) >= 10:
                    status = 429
                else:
                    arrivals.append(now)
        time.sleep(0.2 if self.path == '/slow' else 0.005)
        self.send_response(status)
        self.send_header('Content-Length', '2')
        self.end_headers()
        try:
            self.wfile.write(b'ok')
        except (BrokenPipeError, ConnectionResetError):
            pass
    def log_message(self, *args):
        pass

def api(method, path, body=None, expected=200):
    req = urllib.request.Request(BASE + path, method=method,
        data=json.dumps(body).encode() if body is not None else None,
        headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            code, data = response.status, response.read()
    except urllib.error.HTTPError as error:
        code, data = error.code, error.read()
    assert code == expected, (code, data.decode())
    return json.loads(data)

def run(config):
    test_id = api('POST', '/tests', config, 202)['id']
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        job = api('GET', '/tests/' + test_id)
        if job['status'] in ('completed', 'failed'):
            break
        time.sleep(0.25)
    assert job['status'] == 'completed', job
    logs = api('GET', f'/tests/{test_id}/logs')['logs']
    assert 'unknown fields' not in logs, logs
    report = api('GET', f'/tests/{test_id}/results')['results']
    remaining = subprocess.check_output(['docker', 'ps', '-a', '--filter',
        'label=local.load-tester.job=' + test_id, '--format', '{{.ID}}'], text=True)
    assert not remaining.strip(), remaining
    return job, report

server = ThreadingHTTPServer(('0.0.0.0', 0), Target)
threading.Thread(target=server.serve_forever, daemon=True).start()
target = f'http://host.docker.internal:{server.server_port}'
try:
    api('GET', '/ready')
    config = {'targetUrl': target, 'requestsPerSecond': 3, 'durationSeconds': 3,
        'virtualUsers': 2, 'maxVirtualUsers': 2, 'p95ThresholdMs': 500,
        'requestTimeoutSeconds': 1}
    job, report = run(config)
    step = report['rateAnalysis']['steps'][0]
    assert job['thresholdsPassed'] and 8 <= step['requests'] <= 10, (job, step)
    assert report['rateAnalysis']['recommendedRateLimitRps'] is None, report
    print('PASS fixed 3 RPS; short run correctly withholds suggestion:', step, flush=True)

    config.update(targetUrl=target + '/slow', requestsPerSecond=20, virtualUsers=1, maxVirtualUsers=1)
    job, report = run(config)
    step = report['rateAnalysis']['steps'][0]
    assert not job['thresholdsPassed'] and step['droppedIterations'] > 0, (job, step)
    assert step['status'] == 'inconclusive' and report['rateAnalysis']['recommendedRateLimitRps'] is None, report
    print('PASS generator saturation is inconclusive:', step, flush=True)

    print('Starting two 30-second capacity plateaus against a 10 RPS fixture...', flush=True)
    job, report = run({'targetUrl': target + '/limited',
        'rateSteps': [{'requestsPerSecond': 5, 'durationSeconds': 30},
                      {'requestsPerSecond': 15, 'durationSeconds': 30}],
        'virtualUsers': 5, 'maxVirtualUsers': 10, 'requestTimeoutSeconds': 1,
        'p95ThresholdMs': 500, 'maxErrorRate': 0.01, 'headroomPercent': 20})
    analysis = report['rateAnalysis']
    assert not job['thresholdsPassed'], job
    assert analysis['steps'][0]['status'] == 'passed', analysis
    assert analysis['steps'][1]['status'] == 'failed', analysis
    assert analysis['highestPassingRps'] == 5 and analysis['recommendedRateLimitRps'] == 4, analysis
    assert analysis['boundary'] == 'threshold_crossed', analysis
    print('PASS capacity evidence and 20% headroom:', json.dumps(analysis, indent=2), flush=True)
finally:
    server.shutdown()
    server.server_close()
