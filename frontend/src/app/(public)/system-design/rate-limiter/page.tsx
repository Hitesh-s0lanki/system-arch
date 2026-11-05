import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowRight, Zap } from 'lucide-react'

export const metadata: Metadata = { title: 'Rate Limiter' }
export default function RateLimiterPage() {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-8 sm:px-7">
      <div className="mb-3 flex items-center gap-2 text-xs font-medium text-primary">
        <Zap className="size-4" /> RATE LIMITER
      </div>
      <h1>Rate Limiter</h1>
      <p className="mt-3 text-sm">
        Rate limiter controls are not connected to the dashboard yet.
      </p>
      <div className="mt-6 rounded-xl border border-border bg-card p-5">
        <h2>Looking to test your server?</h2>
        <p className="mt-2 text-sm">
          Run capacity tests, validate requests per second, and view suggested
          limits in Load Tester.
        </p>
        <Link
          href="/system-design/load-tester"
          className="mt-4 inline-flex min-h-9 items-center gap-2 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground focus-visible:outline-2 focus-visible:outline-ring"
        >
          Open Load Tester <ArrowRight className="size-4" />
        </Link>
      </div>
    </div>
  )
}
