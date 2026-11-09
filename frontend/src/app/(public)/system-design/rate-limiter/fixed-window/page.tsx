import type { Metadata } from 'next'
import { RateLimiterArchitecture } from '../_components/RateLimiterArchitecture'

export const metadata: Metadata = {
  title: 'Rate Limiter | Fixed Window Counter',
}

export default function FixedWindowPage() {
  return <RateLimiterArchitecture />
}
