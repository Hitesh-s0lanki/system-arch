import { test, expect } from '@playwright/test'
import {
  automaticLoads,
  timeAtLoad,
} from '../../src/app/(public)/system-design/load-tester/_components/workload'

test('automatic loads respect the selected ceiling and collect enough samples within the job budget', () => {
  for (const ceiling of [1, 2, 3, 4, 5, 10, 20, 99, 1000]) {
    const loads = automaticLoads(ceiling)
    expect(loads.at(-1)).toBe(ceiling)
    expect(new Set(loads).size).toBe(loads.length)
    expect(
      loads.every(
        (load, index) =>
          load >= 1 &&
          load <= ceiling &&
          (index === 0 || load > loads[index - 1]),
      ),
    ).toBe(true)
    for (const load of loads) {
      expect(timeAtLoad(load, 30)).toBeGreaterThanOrEqual(30)
      expect(timeAtLoad(load, 30) * load).toBeGreaterThanOrEqual(120)
    }
    const duration =
      loads.reduce((sum, load) => sum + timeAtLoad(load, 30), 0) +
      loads.length * 5
    expect(duration).toBeLessThanOrEqual(300)
  }
})

test('invalid maximums cannot generate a workload', () => {
  for (const invalid of [0, -1, 1001, 2.5, Number.NaN, Infinity])
    expect(automaticLoads(invalid)).toEqual([])
})
