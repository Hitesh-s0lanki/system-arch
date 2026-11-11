import { begin, gateway, localAccess, status } from '../local'
import {
  validateDemoConfig,
  type DemoConfig,
} from '@/lib/rate-limiter-demo-config'

export async function GET(request: Request) {
  if (!localAccess(request))
    return Response.json({ available: false }, { status: 403 })
  return Response.json(
    { available: true, gateway, ...(await status()) },
    { headers: { 'Cache-Control': 'no-store' } },
  )
}

export async function POST(request: Request) {
  if (!localAccess(request))
    return Response.json(
      { error: 'Docker controls are available on localhost only.' },
      { status: 403 },
    )
  try {
    const raw = await request.text()
    if (raw.length > 8192)
      return Response.json(
        { error: 'Configuration is too large.' },
        { status: 413 },
      )
    const { config, action } = JSON.parse(raw)
    if (!['start', 'stop'].includes(action))
      return Response.json({ error: 'Invalid action.' }, { status: 400 })
    const issue = validateDemoConfig(config)
    if (issue) return Response.json({ error: issue }, { status: 400 })
    return Response.json(
      { gateway, ...begin(config as DemoConfig, action === 'stop') },
      {
        status: 202,
      },
    )
  } catch (error) {
    return Response.json(
      { error: error instanceof Error ? error.message : 'Invalid request.' },
      { status: 400 },
    )
  }
}
