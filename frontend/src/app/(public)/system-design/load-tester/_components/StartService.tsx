'use client'

import { useEffect, useRef, useState } from 'react'
import { Loader2, Power, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api } from './types'

type Status = {
  phase: string
  message: string
  canStart: boolean
  details?: string
}
const working = (phase: string) =>
  ['checking', 'opening_docker', 'starting_service'].includes(phase)
export function StartService({ onReady }: { onReady: () => void }) {
  const [state, setState] = useState<Status>({
    phase: 'checking',
    message: 'Checking Docker on this computer…',
    canStart: true,
  })
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(true)
  const onReadyRef = useRef(onReady)
  useEffect(() => {
    onReadyRef.current = onReady
  }, [onReady])
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let alive = true
    let timer: ReturnType<typeof setTimeout>
    async function check() {
      try {
        const value = await api<Status>('/control')
        if (!alive) return
        setState(value)
        setError('')
        setChecking(false)
        if (value.phase === 'ready') onReadyRef.current()
        else if (working(value.phase)) timer = setTimeout(check, 1500)
      } catch (e) {
        if (alive) {
          setChecking(false)
          setState((previous) => ({ ...previous, phase: 'error' }))
          setError(e instanceof Error ? e.message : 'Unable to check Docker.')
        }
      }
    }
    void check()
    return () => {
      alive = false
      clearTimeout(timer)
    }
  }, [attempt])
  async function start() {
    setError('')
    setChecking(true)
    try {
      const value = await api<Status>('/control', {
        method: state.canStart ? 'POST' : 'GET',
      })
      setState(value)
      setChecking(false)
      setAttempt((value) => value + 1)
    } catch (e) {
      setChecking(false)
      setError(
        e instanceof Error ? e.message : 'Unable to start the load tester.',
      )
    }
  }
  const busy = checking || working(state.phase)
  return (
    <section
      className="rounded-xl border border-border bg-card p-4"
      aria-label="Start load tester"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h2 className="text-sm">
            {busy ? 'Getting ready to test' : 'Get ready to test'}
          </h2>
          <p className="mt-1 text-sm" role="status" aria-live="polite">
            {error || state.message}
          </p>
        </div>
        <Button
          type="button"
          onClick={() => void start()}
          disabled={busy}
          size="lg"
        >
          {busy ? (
            <Loader2 className="animate-spin" />
          ) : state.canStart ? (
            <Power />
          ) : (
            <RefreshCw />
          )}
          {busy
            ? 'Starting…'
            : state.canStart
              ? 'Start load tester'
              : 'Check again'}
        </Button>
      </div>
      {state.details && (
        <details className="mt-3 text-xs text-muted-foreground">
          <summary className="cursor-pointer">Technical details</summary>
          <pre className="mt-2 max-h-40 overflow-auto break-all whitespace-pre-wrap">
            {state.details}
          </pre>
        </details>
      )}
    </section>
  )
}
