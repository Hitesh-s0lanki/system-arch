import type { NextRequest } from 'next/server'
import { begin, status } from './startup'

function denied(request: NextRequest): boolean {
  const host = request.headers.get('host') || ''
  try {
    if (
      !['localhost', '127.0.0.1', '[::1]'].includes(
        new URL('http://' + host).hostname,
      )
    )
      return true
    const origin = request.headers.get('origin')
    return !!origin && new URL(origin).host !== host
  } catch {
    return true
  }
}
export async function GET(request: NextRequest) {
  if (denied(request))
    return Response.json(
      { error: 'Local access is required to manage Docker.' },
      { status: 403 },
    )
  return Response.json(await status(), {
    headers: { 'Cache-Control': 'no-store' },
  })
}
export async function POST(request: NextRequest) {
  if (denied(request))
    return Response.json(
      { error: 'Local access is required to manage Docker.' },
      { status: 403 },
    )
  return Response.json(begin(), {
    status: 202,
    headers: { 'Cache-Control': 'no-store' },
  })
}
