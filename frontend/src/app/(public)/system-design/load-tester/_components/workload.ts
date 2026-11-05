// Keep load selection out of the form while respecting the backend's minimum
// sample size. Duplicates at small maximums must not create invalid rate steps.
export function automaticLoads(maximum: number): number[] {
  if (!Number.isInteger(maximum) || maximum < 1 || maximum > 1000) return []
  return [...new Set([Math.ceil(maximum / 4), Math.ceil(maximum / 2), maximum])]
}
export function timeAtLoad(rate: number, seconds: number): number {
  return Math.max(seconds, Math.ceil(120 / rate))
}
