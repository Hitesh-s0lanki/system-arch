import { demoTokens, gateway, localAccess, status } from '../local'

export async function POST(request: Request) {
  if (!localAccess(request))
    return Response.json(
      { error: 'Request testing is available on localhost only.' },
      { status: 403 },
    )
  try {
    const raw = await request.text()
    if (raw.length > 20000)
      return Response.json({ error: 'Request is too large.' }, { status: 413 })
    const {
      method,
      path,
      headers: inputHeaders,
      body,
      client,
    } = JSON.parse(raw)
    if (
      !['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].includes(
        method,
      ) ||
      typeof path !== 'string' ||
      !path.startsWith('/') ||
      path.startsWith('//') ||
      /[\\\r\n#]/.test(path) ||
      typeof body !== 'string' ||
      body.length > 16384 ||
      ![0, 1].includes(client)
    ) {
      return Response.json(
        { error: 'Check the method, path, body, and client.' },
        { status: 400 },
      )
    }
    const url = new URL(path, gateway)
    if (
      url.origin !== gateway ||
      ['/healthz', '/readyz', '/metrics'].includes(url.pathname)
    )
      return Response.json(
        { error: 'Use an application path such as / or /api/orders.' },
        { status: 400 },
      )
    const current = await status()
    if (current.phase !== 'running')
      return Response.json(
        { error: 'Start the Docker demo first.' },
        { status: 409 },
      )
    if (
      !inputHeaders ||
      typeof inputHeaders !== 'object' ||
      Array.isArray(inputHeaders) ||
      Object.keys(inputHeaders).length > 20
    )
      return Response.json(
        { error: 'Headers must be a JSON object with at most 20 entries.' },
        { status: 400 },
      )
    const headers = new Headers()
    for (const [name, value] of Object.entries(inputHeaders)) {
      if (
        typeof value !== 'string' ||
        /^(host|connection|content-length|transfer-encoding|upgrade|authorization|cookie|proxy-.*|x-forwarded-.*|x-ratelimit-.*|forwarded)$/i.test(
          name,
        )
      ) {
        return Response.json(
          { error: `Header ${name} is reserved or invalid.` },
          { status: 400 },
        )
      }
      headers.set(name, value)
    }
    if (current.config?.identity !== 'ip')
      headers.set('Authorization', `Bearer ${demoTokens[client]}`)
    const started = performance.now()
    const response = await fetch(url, {
      method,
      headers,
      body: ['GET', 'HEAD'].includes(method) ? undefined : body,
      redirect: 'manual',
      cache: 'no-store',
      signal: AbortSignal.timeout(12000),
    })
    const reader = response.body?.getReader()
    const chunks: Uint8Array[] = []
    let size = 0
    if (reader) {
      try {
        while (size < 65536) {
          const { value, done } = await reader.read()
          if (done) break
          chunks.push(value.subarray(0, 65536 - size))
          size += value.length
        }
      } finally {
        await reader.cancel()
      }
    }
    const text = Buffer.concat(chunks).toString('utf8')
    return Response.json({
      status: response.status,
      durationMs: Math.round(performance.now() - started),
      headers: Object.fromEntries(response.headers),
      body: text,
      truncated: size >= 65536,
    })
  } catch (error) {
    return Response.json(
      { error: error instanceof Error ? error.message : 'Request failed.' },
      { status: 400 },
    )
  }
}
