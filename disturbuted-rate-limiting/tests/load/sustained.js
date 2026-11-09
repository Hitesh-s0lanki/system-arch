import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Targets must be isolated test gateways. This also accepts comma-separated
// replica URLs so a run exercises a shared Redis namespace across gateways.
const targets = (__ENV.TARGETS || 'http://127.0.0.1:8088').split(',');
const unexpected = new Rate('unexpected_responses');
const expectedStatus = Number(__ENV.EXPECT_STATUS || 200);
export const options = {
  scenarios: {
    sustained: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 100),
      timeUnit: '1s',
      duration: __ENV.DURATION || '2m',
      preAllocatedVUs: Number(__ENV.VUS || 32),
      maxVUs: Number(__ENV.MAX_VUS || 128),
    },
  },
  thresholds: {
    unexpected_responses: ['rate<0.001'],
    http_req_duration: [`p(95)<${__ENV.P95_MS || 50}`, `p(99)<${__ENV.P99_MS || 100}`],
    dropped_iterations: ['count==0'],
  },
};
export default function () {
  const target = targets[(__VU + __ITER) % targets.length];
  const headers = __ENV.TOKEN ? { Authorization: `Bearer ${__ENV.TOKEN}` } : {};
  const r = http.get(`${target}/load`, { headers, timeout: '5s' });
  const ok = check(r, { 'expected status': response => response.status === expectedStatus });
  unexpected.add(!ok);
  sleep(0.001);
}
