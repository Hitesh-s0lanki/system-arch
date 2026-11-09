// Recorded by tests/e2e/run_flow.py in disturbuted-rate-limiting.
// A historical measurement, not live telemetry or a performance guarantee.
export const flowSnapshot = {
  date: '6 Nov 2026',
  checks: 66,
  publicSamples: 5,
  controlledSamples: 200,
  publicMedianMs: { direct: 42.19, ram: 51.61, redis: 70.09 },
  addedMedianMs: { ram: 1.98, redis: 2.35 },
} as const
