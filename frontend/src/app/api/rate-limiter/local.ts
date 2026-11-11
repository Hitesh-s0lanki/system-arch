import { execFile } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { randomUUID } from 'node:crypto'
import { promisify } from 'node:util'
import {
  demoProxyConfig,
  type DemoConfig,
} from '@/lib/rate-limiter-demo-config'

const exec = promisify(execFile)
const project = path.resolve(
  process.env.RATE_LIMITER_PROJECT_DIR ||
    path.join(process.cwd(), '../disturbuted-rate-limiting'),
)
const configPath = path.join(project, '.local-demo/proxy.json')
const port = Number(process.env.RATE_LIMITER_DEMO_PORT || 8089)
if (!Number.isInteger(port) || port < 1024 || port > 65535)
  throw new Error('Invalid RATE_LIMITER_DEMO_PORT')
export const gateway = `http://127.0.0.1:${port}`
export const demoTokens = [
  'local-demo-client-a-000000000000000000000',
  'local-demo-client-b-000000000000000000000',
]

type State = {
  phase: 'stopped' | 'starting' | 'running' | 'stopping' | 'error'
  config?: DemoConfig
  error?: string
  message?: string
}
const shared = globalThis as typeof globalThis & {
  rateLimiterDemoState?: State
}
function state() {
  return (shared.rateLimiterDemoState ??= { phase: 'stopped' })
}

export function localAccess(request: Request): boolean {
  if (process.env.VERCEL) return false
  try {
    const host = request.headers.get('host') || ''
    if (
      !['localhost', '127.0.0.1', '[::1]'].includes(
        new URL(`http://${host}`).hostname,
      )
    )
      return false
    const origin = request.headers.get('origin')
    if (request.method !== 'GET' && !origin) return false
    return !origin || new URL(origin).host === host
  } catch {
    return false
  }
}

function compose(config: DemoConfig, args: string[]) {
  return exec(
    'docker',
    [
      'compose',
      '--project-name',
      'rate-limiter-ui',
      '-f',
      'compose.yaml',
      ...(config.storage === 'redis' ? ['-f', 'compose.redis.yaml'] : []),
      ...args,
    ],
    {
      cwd: project,
      timeout: 240000,
      maxBuffer: 2 * 1024 * 1024,
      env: {
        ...process.env,
        PROXY_PORT: String(port),
        RATE_LIMIT_CONFIG: configPath,
        RATE_LIMIT_UPSTREAM:
          config.target === 'demo' ? 'http://backend:8080' : config.upstream,
        RATE_LIMIT_NAMESPACE: 'local-demo',
        REDIS_URL: 'redis://redis:6379/0',
        RATE_LIMIT_CREDENTIALS: JSON.stringify(
          demoTokens.map((token, i) => ({
            token,
            tenant_id: 'demo',
            user_id: `user-${i + 1}`,
            api_key_id: `key-${i + 1}`,
          })),
        ),
      },
    },
  )
}

async function ensureDocker(config: DemoConfig) {
  try {
    await exec('docker', ['--version'], { timeout: 5000, maxBuffer: 65536 })
  } catch {
    throw new Error(
      'Docker was not found. Install Docker Desktop, then try again.',
    )
  }
  async function running() {
    try {
      await exec('docker', ['info', '--format', '{{.ServerVersion}}'], {
        timeout: 6000,
        maxBuffer: 65536,
      })
      return true
    } catch {
      return false
    }
  }
  if (await running()) return
  shared.rateLimiterDemoState = {
    phase: 'starting',
    config,
    message: 'Opening Docker Desktop. Waiting for Docker to become ready…',
  }
  try {
    await exec('docker', ['desktop', 'start', '--detach'], {
      timeout: 20000,
      maxBuffer: 65536,
    })
  } catch {
    if (process.platform !== 'darwin')
      throw new Error(
        'Could not open Docker Desktop automatically. Open Docker, then try again.',
      )
    await exec('open', ['-a', 'Docker'], { timeout: 10000, maxBuffer: 65536 })
  }
  const deadline = Date.now() + 120000
  while (!(await running())) {
    if (Date.now() >= deadline)
      throw new Error(
        'Docker is taking longer than expected. Check Docker Desktop for a setup prompt, then try again.',
      )
    await new Promise((resolve) => setTimeout(resolve, 2000))
  }
}

export async function status() {
  const current = state()
  if (current.phase === 'running') {
    try {
      const response = await fetch(`${gateway}/readyz`, {
        cache: 'no-store',
        signal: AbortSignal.timeout(1500),
      })
      if (!response.ok) throw new Error('not ready')
    } catch {
      // Do not overwrite a newer start/stop operation while readiness was pending.
      if (state() === current)
        shared.rateLimiterDemoState = {
          ...current,
          phase: 'error',
          error: 'Gateway is offline. Start Docker to try again.',
        }
    }
  }
  return state()
}

export function begin(config: DemoConfig, stop = false) {
  if (['starting', 'stopping'].includes(state().phase))
    throw new Error('Docker is busy. Wait for the current operation.')
  shared.rateLimiterDemoState = {
    phase: stop ? 'stopping' : 'starting',
    config,
  }
  void (async () => {
    try {
      if (stop) {
        await compose(config, ['down', '--remove-orphans'])
        shared.rateLimiterDemoState = { phase: 'stopped' }
      } else {
        await ensureDocker(config)
        shared.rateLimiterDemoState = {
          phase: 'starting',
          config,
          message:
            'Starting the proxy and sample API. First build may take a few minutes.',
        }
        await mkdir(path.dirname(configPath), { recursive: true })
        await writeFile(
          configPath,
          JSON.stringify(demoProxyConfig(config, randomUUID()), null, 2) + '\n',
        )
        await compose(config, [
          'up',
          '-d',
          '--build',
          '--force-recreate',
          '--remove-orphans',
          '--wait',
          '--wait-timeout',
          '60',
        ])
        shared.rateLimiterDemoState = { phase: 'running', config }
      }
    } catch (error) {
      const failure = error as Error & { stderr?: string }
      shared.rateLimiterDemoState = {
        phase: 'error',
        config,
        error: (failure.stderr || failure.message).slice(-3000),
      }
    }
  })()
  return state()
}
