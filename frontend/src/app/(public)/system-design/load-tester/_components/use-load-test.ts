'use client'
import { useCallback, useEffect, useState } from 'react'
import {
  activeStatus,
  api,
  type Job,
  type Report,
  type TestConfig,
} from './types'

export function useLoadTest() {
  const [connection, setConnection] = useState<
    'checking' | 'online' | 'offline'
  >('checking')
  const [history, setHistory] = useState<Job[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [job, setJob] = useState<Job | null>(null)
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [retry, setRetry] = useState(0)
  const refresh = useCallback(async () => {
    const [ready, tests] = await Promise.allSettled([
      api('/ready'),
      api<{ items: Job[] }>('/tests?limit=8'),
    ])
    setConnection(ready.status === 'fulfilled' ? 'online' : 'offline')
    if (tests.status === 'fulfilled') setHistory(tests.value.items)
  }, [])
  useEffect(() => {
    let alive = true
    async function initialize() {
      await refresh()
      if (!alive) return
      const query = new URLSearchParams(window.location.search).get('test')
      let saved: string | null = null
      try {
        saved =
          window.localStorage.getItem('load-tester-test') ||
          window.localStorage.getItem('rate-limiter-test')
      } catch {}
      const id = query || saved
      if (id && /^[a-f0-9]{32}$/.test(id)) setSelectedId(id)
    }
    void initialize()
    return () => {
      alive = false
    }
  }, [refresh])
  useEffect(() => {
    if (!selectedId) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      try {
        const current = await api<Job>('/tests/' + selectedId, {
          signal: controller.signal,
        })
        if (controller.signal.aborted) return
        setJob(current)
        setError('')
        if (activeStatus(current)) timer = setTimeout(poll, 2000)
        else {
          if (current.status === 'completed') {
            const result = await api<Report>(
              '/tests/' + selectedId + '/results',
              { signal: controller.signal },
            )
            if (!controller.signal.aborted) setReport(result)
          }
          if (!controller.signal.aborted) void refresh()
        }
      } catch (e) {
        if (!controller.signal.aborted)
          setError(e instanceof Error ? e.message : 'Unable to fetch test')
      }
    }
    void poll()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [selectedId, refresh, retry])
  const select = useCallback((id: string) => {
    setJob(null)
    setReport(null)
    setError('')
    setSelectedId(id)
    setRetry((x) => x + 1)
    try {
      window.localStorage.setItem('load-tester-test', id)
    } catch {}
    const url = new URL(window.location.href)
    url.searchParams.set('test', id)
    window.history.replaceState(null, '', url)
  }, [])
  async function start(config: TestConfig) {
    setPending(true)
    setError('')
    try {
      const created = await api<Job>('/tests', {
        method: 'POST',
        body: JSON.stringify(config),
      })
      select(created.id)
      setJob(created)
      void refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to start test')
    } finally {
      setPending(false)
    }
  }
  async function cancel() {
    if (!job) return
    setCancelling(true)
    try {
      const updated = await api<Job>('/tests/' + job.id + '/cancel', {
        method: 'POST',
      })
      setJob(updated)
      setRetry((x) => x + 1)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to cancel test')
    } finally {
      setCancelling(false)
    }
  }
  return {
    connection,
    history,
    job,
    report,
    error,
    pending,
    cancelling,
    selectedId,
    select,
    start,
    cancel,
    refresh,
    retry: () => setRetry((x) => x + 1),
  }
}
