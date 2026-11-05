import http from 'k6/http';
import { Counter, Trend, Rate } from 'k6/metrics';

const config = JSON.parse(open('./config.json'));
const statuses = new Counter('response_status');
const successful = new Trend('successful_latency', true);
const successfulCount = new Counter('successful_requests');
const limited = new Trend('rate_limited_latency', true);
const other = new Trend('other_error_latency', true);
const errors = new Rate('unsuccessful_requests');
const transport = new Counter('transport_failures');
const timeouts = new Counter('request_timeouts');
const thresholds = { unsuccessful_requests: [`rate<=${config.maxErrorRate}`] };
if (config.p95ThresholdMs > 0) thresholds.successful_latency = [`p(95)<=${config.p95ThresholdMs}`];
const scenarios = {};
const steps = config.rateSteps || [];
if (steps.length || config.requestsPerSecond) {
  const rates = steps.length ? steps : [{requestsPerSecond: config.requestsPerSecond, durationSeconds: config.durationSeconds}];
  let startTime = 0;
  rates.forEach((step, index) => {
    const name = steps.length ? `step_${index + 1}` : 'benchmark';
    scenarios[name] = {
      executor: 'constant-arrival-rate', rate: step.requestsPerSecond, timeUnit: '1s',
      duration: `${step.durationSeconds}s`, startTime: `${startTime}s`,
      preAllocatedVUs: config.virtualUsers, maxVUs: config.maxVirtualUsers,
      gracefulStop: `${config.requestTimeoutSeconds}s`,
    };
    thresholds[`unsuccessful_requests{scenario:${name}}`] = [`rate<=${config.maxErrorRate}`];
    thresholds[`dropped_iterations{scenario:${name}}`] = ['count==0'];
    thresholds[`http_reqs{scenario:${name}}`] = [`count>=${Math.ceil(step.requestsPerSecond * step.durationSeconds * 0.95)}`];
    if (config.p95ThresholdMs > 0) {
      thresholds[`successful_requests{scenario:${name}}`] = ['count>=1'];
      thresholds[`successful_latency{scenario:${name}}`] = [`p(95)<=${config.p95ThresholdMs}`];
    }
    // Drain outstanding requests before starting the next plateau.
    startTime += step.durationSeconds + config.requestTimeoutSeconds;
  });
} else {
  scenarios.benchmark = {
    executor: 'constant-vus', vus: config.virtualUsers,
    duration: `${config.durationSeconds}s`, gracefulStop: `${config.requestTimeoutSeconds}s`,
  };
}
export const options = {
  scenarios,
  discardResponseBodies: true,
  maxRedirects: 0,
  systemTags: ['status', 'method', 'name', 'error_code', 'scenario', 'expected_response'],
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(95)', 'p(99)', 'count'],
  thresholds,
};
export default function () {
  const r = http.request(config.method, config.targetUrl, config.body || null, {
    headers: config.headers || {}, timeout: `${config.requestTimeoutSeconds}s`,
    tags: { name: 'target' },
  });
  statuses.add(1, { status: String(r.status) });
  const ok = r.status >= 200 && r.status < 300;
  errors.add(!ok);
  successfulCount.add(ok ? 1 : 0);
  if (ok) successful.add(r.timings.duration);
  else if (r.status === 429) limited.add(r.timings.duration);
  else other.add(r.timings.duration);
  transport.add(r.status === 0 ? 1 : 0);
  timeouts.add(r.error_code === 1050 ? 1 : 0);
}
export function handleSummary(data) {
  return { 'summary.json': JSON.stringify(data) };
}
