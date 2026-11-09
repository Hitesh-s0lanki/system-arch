import { execFile } from 'node:child_process'
import { access } from 'node:fs/promises'
import path from 'node:path'
import { promisify } from 'node:util'

const execute = promisify(execFile)
type Phase =
  | 'offline'
  | 'checking'
  | 'opening_docker'
  | 'starting_service'
  | 'ready'
  | 'error'
export type StartupStatus = {
  phase: Phase
  message: string
  canStart: boolean
  details?: string
}
type State = { status: StartupStatus; task?: Promise<void> }
const runtime = globalThis as typeof globalThis & { loadTesterStartup?: State }
const state = (runtime.loadTesterStartup ??= {
  status: {
    phase: 'offline',
    message: 'Check Docker and start the load tester here.',
    canStart: true,
  },
})
const active = () => !!state.task
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))
const serviceURL = () =>
  new URL(process.env.LOAD_TESTER_URL || 'http://127.0.0.1:8090')

async function ready(): Promise<boolean> {
  try {
    return (
      await fetch(new URL('ready', serviceURL().href.replace(/\/?$/, '/')), {
        cache: 'no-store',
        signal: AbortSignal.timeout(3000),
      })
    ).ok
  } catch {
    return false
  }
}
async function dockerRunning(): Promise<boolean> {
  try {
    await execute('docker', ['info', '--format', '{{.ServerVersion}}'], {
      timeout: 6000,
      maxBuffer: 65536,
    })
    return true
  } catch {
    return false
  }
}
async function dockerInstalled(): Promise<boolean> {
  try {
    await execute('docker', ['--version'], { timeout: 5000, maxBuffer: 65536 })
    return true
  } catch {
    return false
  }
}
function localConfiguration(): boolean {
  try {
    return ['localhost', '127.0.0.1', '[::1]'].includes(serviceURL().hostname)
  } catch {
    return false
  }
}
export async function status(): Promise<StartupStatus> {
  if (active()) return { ...state.status }
  if (!localConfiguration())
    return {
      phase: 'error',
      message:
        'This connection uses another computer. Automatic startup is available for a local load tester only.',
      canStart: false,
    }
  if (await ready())
    return {
      phase: 'ready',
      message: 'Your load tester is ready.',
      canStart: true,
    }
  if (state.status.phase === 'error') return { ...state.status }
  if (!(await dockerInstalled()))
    return {
      phase: 'offline',
      message:
        'Docker was not found on this computer. Install Docker Desktop, then check again.',
      canStart: false,
    }
  return {
    phase: 'offline',
    message: (await dockerRunning())
      ? 'Docker is ready. Start the load tester to continue.'
      : 'Docker is stopped. We can open Docker Desktop and start the load tester for you.',
    canStart: true,
  }
}
async function start(): Promise<void> {
  try {
    if (!localConfiguration())
      throw new Error(
        'Automatic startup requires a local load tester connection.',
      )
    if (await ready()) {
      state.status = {
        phase: 'ready',
        message: 'Your load tester is ready.',
        canStart: true,
      }
      return
    }
    state.status = {
      phase: 'checking',
      message: 'Checking Docker access…',
      canStart: true,
    }
    if (!(await dockerInstalled()))
      throw new Error(
        'Docker was not found. Install Docker Desktop on this computer, then try again.',
      )
    if (!(await dockerRunning())) {
      state.status = {
        phase: 'opening_docker',
        message: 'Opening Docker Desktop. Waiting for Docker to become ready…',
        canStart: true,
      }
      try {
        await execute('docker', ['desktop', 'start', '--detach'], {
          timeout: 20000,
          maxBuffer: 65536,
        })
      } catch {
        if (process.platform !== 'darwin')
          throw new Error(
            'Could not open Docker Desktop automatically. Open Docker on this computer, then try again.',
          )
        await execute('open', ['-a', 'Docker'], {
          timeout: 10000,
          maxBuffer: 65536,
        })
      }
      const deadline = Date.now() + 120000
      while (!(await dockerRunning())) {
        if (Date.now() > deadline)
          throw new Error(
            'Docker is taking longer than expected to start. Check Docker Desktop for a setup or permission prompt, then try again.',
          )
        await pause(2000)
      }
    }
    state.status = {
      phase: 'starting_service',
      message:
        'Starting your load tester. The first start may take a few minutes while files download…',
      canStart: true,
    }
    const directory = path.resolve(
      process.env.LOAD_TESTER_PROJECT_DIR ||
        path.join(process.cwd(), '../load-tester'),
    )
    const compose = path.join(directory, 'compose.yml')
    await access(compose)
    // Next.js sets PORT for its own server. Do not let that override the
    // backend Compose port (or its .env configuration).
    const dockerEnvironment = { ...process.env }
    delete dockerEnvironment.PORT
    await execute(
      'docker',
      [
        'compose',
        '--file',
        compose,
        'up',
        '--detach',
        '--build',
        'load-tester',
      ],
      {
        cwd: directory,
        env: dockerEnvironment,
        timeout: 600000,
        maxBuffer: 2 * 1024 * 1024,
      },
    )
    const deadline = Date.now() + 60000
    while (!(await ready())) {
      if (Date.now() > deadline)
        throw new Error(
          'The load tester started but is not ready yet. Check Docker Desktop for a port conflict or service error, then try again.',
        )
      await pause(1500)
    }
    state.status = {
      phase: 'ready',
      message: 'Your load tester is ready. You can test your server now.',
      canStart: true,
    }
  } catch (error) {
    const e = error as Error & { stderr?: string; code?: string }
    state.status = {
      phase: 'error',
      message:
        e.message.startsWith('Docker') ||
        e.message.startsWith('Could not') ||
        e.message.startsWith('The load tester') ||
        e.message.startsWith('Automatic')
          ? e.message
          : 'We could not start the load tester. Check that Docker is installed and the load-tester folder is available, then try again.',
      canStart: true,
      details: (e.stderr || e.message).slice(-4000),
    }
  }
}
export function begin(): StartupStatus {
  if (!active()) {
    state.status = {
      phase: 'checking',
      message: 'Checking Docker access…',
      canStart: true,
    }
    state.task = start().finally(() => {
      state.task = undefined
    })
  }
  return { ...state.status }
}
