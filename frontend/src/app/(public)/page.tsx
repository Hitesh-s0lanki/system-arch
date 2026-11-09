import type { Metadata } from 'next'
import { DashboardContent } from './_components/DashboardContent'

export const metadata: Metadata = {
  title: 'SystemArch | Implemented systems',
  description:
    'Explore the rate limiter, load tester, and video streaming implementations in SystemArch.',
}

export default function DashboardPage() {
  return <DashboardContent />
}
