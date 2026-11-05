import type { Metadata } from 'next'
import { LoadTesterWorkbench } from './_components/LoadTesterWorkbench'

export const metadata: Metadata = { title: 'Load Tester' }
export default function LoadTesterPage() {
  return <LoadTesterWorkbench />
}
