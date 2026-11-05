import type { NextRequest } from 'next/server'

type Context = { params: Promise<{ path: string[] }> }
const idPattern = /^[a-f0-9]{32}$/
async function proxy(request: NextRequest, context: Context) {
  const { path } = await context.params
  const allowed =
    (path.length === 1 && ['ready', 'tests'].includes(path[0])) ||
    (path[0] === 'tests' &&
      idPattern.test(path[1] || '') &&
      (path.length === 2 ||
        (path.length === 3 &&
          ['results', 'metrics', 'logs', 'cancel'].includes(path[2]))))
  const mutation =
    (path.length === 1 && path[0] === 'tests') ||
    (path.length === 3 && path[2] === 'cancel')
  if (
    !allowed ||
    (request.method === 'POST' && !mutation) ||
    (request.method === 'GET' && path[2] === 'cancel')
  )
    return Response.json({ error: 'Endpoint not found' }, { status: 404 })
  if (request.method === 'POST') {
    const origin = request.headers.get('origin')
    if (origin) {
      try {
        if (new URL(origin).host !== request.headers.get('host')) {
          return Response.json(
            { error: 'Cross-origin requests are not allowed' },
            { status: 403 },
          )
        }
      } catch {
        return Response.json(
          { error: 'Invalid request origin' },
          { status: 403 },
        )
      }
    }
  }

  try {
    const base = new URL(process.env.LOAD_TESTER_URL || 'http://127.0.0.1:8090')
    if (!['http:', 'https:'].includes(base.protocol))
      throw new Error('Invalid service URL')
    const url = new URL(path.join('/'), base.href.replace(/\/?$/, '/'))
    for (const key of ['limit', 'offset']) {
      const value = request.nextUrl.searchParams.get(key)
      if (value !== null) url.searchParams.set(key, value)
    }
    const body = request.method === 'POST' ? await request.text() : undefined
    if (body && Buffer.byteLength(body) > 128 * 1024)
      return Response.json({ error: 'Request too large' }, { status: 413 })
    const upstream = await fetch(url, {
      method: request.method,
      body,
      headers: { 'Content-Type': 'application/json' },
      cache: 'no-store',
      signal: AbortSignal.timeout(60000),
    })
    return new Response(upstream.body, {
      status: upstream.status,
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': 'no-store',
      },
    })
  } catch {
    return Response.json(
      {
        error:
          'Load tester unavailable. Start it with docker compose up -d in load-tester, then retry.',
      },
      { status: 503, headers: { 'Cache-Control': 'no-store' } },
    )
  }
}
export const GET = proxy
export const POST = proxy
