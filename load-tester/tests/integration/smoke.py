#!/usr/bin/env python3
"""Exercise a running Compose deployment with an isolated local HTTP fixture.
Run from load-tester/: python3 tests/integration/smoke.py
"""
import json
import os
import subprocess
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BASE = os.environ.get('TEST_API_URL', 'http://127.0.0.1:8090')

class Target(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_GET(self):
        time.sleep(1.2 if self.path == '/slow' else 0.01)
        code = 429 if self.path == '/limited' else 500 if self.path == '/error' else 200
        self.send_response(code)
        self.send_header('Content-Length', '2')
        self.end_headers()
        try:
            self.wfile.write(b'ok')
        except (BrokenPipeError, ConnectionResetError):
            pass
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        code = 200 if body == b'{"probe":true}' and self.headers.get('X-Probe') == 'test-secret' else 400
        self.send_response(code)
        self.send_header('Content-Length', '2')
        self.end_headers()
        self.wfile.write(b'ok')
    def log_message(self, *args):
        pass


def api(method, path, body=None, expected=200):
    req = urllib.request.Request(BASE + path, method=method,
        data=json.dumps(body).encode() if body is not None else None,
        headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            code, data = response.status, response.read()
    except urllib.error.HTTPError as error:
        code, data = error.code, error.read()
    assert code == expected, (method, path, code, data.decode())
    return json.loads(data)


def poll(test_id, statuses, timeout=180):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        job = api('GET', '/tests/' + test_id)
        if job['status'] in statuses:
            return job
        time.sleep(0.2)
    raise AssertionError(('timeout', test_id, job))


def create(path='/', duration=2):
    return api('POST', '/tests', {'targetUrl': target + path, 'method': 'GET',
        'virtualUsers': 1, 'durationSeconds': duration,
        'requestTimeoutSeconds': 1, 'maxErrorRate': 0}, 202)['id']


def no_container(test_id):
    output = subprocess.check_output(['docker', 'ps', '-a', '--filter',
        'label=local.load-tester.job=' + test_id, '--format', '{{.ID}}'], text=True)
    assert not output.strip(), ('container leaked', test_id, output)


server = ThreadingHTTPServer(('0.0.0.0', 0), Target)
threading.Thread(target=server.serve_forever, daemon=True).start()
target = f'http://host.docker.internal:{server.server_port}'
try:
    api('GET', '/ready')
    api('POST', '/tests', {'targetUrl': 'file:///etc/passwd'}, 400)
    for path, status in [('/', '200'), ('/limited', '429'), ('/error', '500'), ('/slow', '0')]:
        test_id = create(path)
        job = poll(test_id, {'completed', 'failed'})
        assert job['status'] == 'completed', job
        assert job['thresholdsPassed'] == (status == '200'), job
        report = api('GET', f'/tests/{test_id}/results')['results']
        assert report['statusCounts'].get(status, 0) > 0, report
        if status == '200':
            assert report['metrics']['successful_latency']['values']['p(95)'] > 0
        if path == '/slow':
            assert report['metrics']['request_timeouts']['values']['count'] > 0
        points = api('GET', f'/tests/{test_id}/metrics?limit=2')
        assert len(points['items']) == 2 and points['hasMore'], points
        api('GET', f'/tests/{test_id}/logs')
        no_container(test_id)
        print('PASS', path, job['exitCode'], report['statusCounts'], flush=True)

    posted = api('POST', '/tests', {'targetUrl': target, 'method': 'POST',
        'headers': {'X-Probe': 'test-secret'}, 'body': '{"probe":true}',
        'virtualUsers': 1, 'durationSeconds': 1}, 202)['id']
    assert poll(posted, {'completed', 'failed'})['thresholdsPassed'], 'POST body/header forwarding failed'
    no_container(posted)
    print('PASS POST body and headers', flush=True)

    # A target on the shared network must resolve by container name too.
    fixture_name = 'load-tester-smoke-target'
    subprocess.run(['docker', 'run', '-d', '--name', fixture_name, '--network',
        os.environ.get('K6_NETWORK', 'load-tester'), 'nginx:alpine'],
        check=True, stdout=subprocess.DEVNULL)
    time.sleep(1)
    try:
        networked = api('POST', '/tests', {'targetUrl': f'http://{fixture_name}:80',
            'virtualUsers': 1, 'durationSeconds': 1}, 202)['id']
        job = poll(networked, {'completed', 'failed'})
        assert job['status'] == 'completed' and job['thresholdsPassed'], job
        no_container(networked)
        print('PASS shared-network target', flush=True)
    finally:
        subprocess.run(['docker', 'rm', '-f', fixture_name], check=True, stdout=subprocess.DEVNULL)

    # Unreachable host port is a completed measurement with transport errors.
    unreachable = api('POST', '/tests', {'targetUrl': 'http://host.docker.internal:1',
        'virtualUsers': 1, 'durationSeconds': 1}, 202)['id']
    job = poll(unreachable, {'completed', 'failed'})
    assert job['status'] == 'completed' and not job['thresholdsPassed'], job
    assert api('GET', f'/tests/{unreachable}/results')['results']['statusCounts'].get('0', 0) > 0
    no_container(unreachable)
    print('PASS unreachable target', flush=True)

    running = create(duration=30)
    poll(running, {'running'})
    queued = create()
    api('POST', f'/tests/{queued}/cancel', expected=202)
    assert poll(queued, {'cancelled'})['status'] == 'cancelled'
    api('POST', f'/tests/{running}/cancel', expected=202)
    poll(running, {'cancelled'})
    no_container(running)
    no_container(queued)
    print('PASS running and queued cancellation', flush=True)

    # Simulate a crash: k6 is a sibling container and must be reconciled on startup.
    interrupted = create(duration=30)
    poll(interrupted, {'running'})
    queued = create()
    time.sleep(1)
    subprocess.run(['docker', 'compose', 'kill', '-s', 'SIGKILL', 'load-tester'], check=True)
    subprocess.run(['docker', 'compose', 'up', '-d'], check=True)
    for _ in range(100):
        try:
            api('GET', '/ready')
            break
        except (OSError, AssertionError):
            time.sleep(0.2)
    job = poll(interrupted, {'failed'})
    assert 'restarted' in job.get('error', ''), job
    no_container(interrupted)
    assert poll(queued, {'completed', 'failed'})['status'] == 'completed'
    no_container(queued)
    print('PASS crash recovery, persisted queue, and container cleanup', flush=True)
    print('All Docker end-to-end checks passed.', flush=True)
finally:
    server.shutdown()
    server.server_close()
