'use client'

import { useEffect, useMemo, useState } from 'react'
import Image from 'next/image'
import Link from 'next/link'
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  ChartNoAxesCombined,
  Check,
  Expand,
  Layers,
  Lightbulb,
  Container,
  Code2,
} from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { LatencyResults } from './_components/LatencyResults'
import { DocsIndex } from '../_components/DocsIndex'
import { AlgorithmSetup } from './_components/AlgorithmSetup'
import { defaultSettings } from './_components/algorithm-examples'

const algorithms = [
  {
    id: 'fixed-window',
    short: 'Fixed window',
    title: 'Fixed Window Counter',
    tag: 'The simple starting point',
    summary: 'Count requests. Reset at the next clock boundary.',
    memory: 'O(1)',
    accuracy: 'Exact per fixed window',
    steps: [
      'Start a counter for each identity.',
      'Accept requests until the counter reaches the limit.',
      'Reset the counter when the next window begins.',
    ],
    example: '100 requests per minute → request 101 waits for the next minute.',
    use: 'Simple quotas for internal APIs and background jobs.',
    caveat: 'Requests on either side of a boundary can create a 2× burst.',
    image: 'fixed-window.jpg',
    caption:
      'Each window has its own allowance. Watch the boundary between them.',
  },
  {
    id: 'sliding-window-log',
    short: 'Sliding log',
    title: 'Sliding Window Log',
    tag: 'When the rolling limit must be exact',
    summary: 'Keep accepted timestamps. Count only the last window.',
    memory: 'O(limit)',
    accuracy: 'Exact rolling window',
    steps: [
      'Remove timestamps older than the rolling window.',
      'Count the timestamps that remain.',
      'Accept and record the request if there is room.',
    ],
    example:
      '3 requests per 10 seconds → a slot opens when the oldest timestamp expires.',
    use: 'Strict rolling quotas, such as login-attempt controls.',
    caveat: 'More requests mean more timestamps to store and scan.',
    image: 'sliding-window-log.jpg',
    caption:
      'Conceptual timestamp log. Our Redis version stores arrays and indexes identity expiry.',
  },
  {
    id: 'sliding-window-counter',
    short: 'Sliding counter',
    title: 'Sliding Window Counter',
    tag: 'Smoother boundaries, small state',
    summary: 'Blend the previous count with the current count.',
    memory: 'O(1)',
    accuracy: 'Approximate rolling window',
    steps: [
      'Keep the previous and current window counts.',
      'Weight the previous count by the time still overlapping.',
      'Accept when the weighted estimate plus one fits the limit.',
    ],
    example:
      '80 previous + 30 current, 15 seconds into a minute → 80 × 0.75 + 30 = 90.',
    use: 'Compact rolling estimates for busy APIs.',
    caveat:
      'Accuracy depends on how requests were spread through the previous window.',
    image: 'sliding-counter.svg',
    caption: 'At 25% elapsed, 75% of the previous window still contributes.',
  },
  {
    id: 'token-bucket',
    short: 'Token bucket',
    title: 'Token Bucket',
    tag: 'Allow bursts, control the average',
    summary: 'Each request spends a token. Time replenishes them.',
    memory: 'O(1)',
    accuracy: 'Continuous refill',
    steps: [
      'Refill tokens based on time elapsed.',
      'Stop refilling at the bucket capacity.',
      'Spend one token to accept; reject if fewer than one remain.',
    ],
    example:
      'Capacity 10, refill 2/second → a burst of 10, then one new token every 500 ms.',
    use: 'Batch requests and short, legitimate traffic bursts.',
    caveat: 'The full bucket can reach your upstream as an immediate burst.',
    image: 'token-bucket.jpg',
    caption:
      'Capacity controls the burst. Refill rate controls sustained usage.',
  },
  {
    id: 'leaky-bucket',
    short: 'Leaky bucket',
    title: 'Leaky Bucket Meter',
    tag: 'Let recent usage drain over time',
    summary: 'Requests raise a level. Time drains it steadily.',
    memory: 'O(1)',
    accuracy: 'Continuous drain',
    steps: [
      'Drain the stored level for the time elapsed.',
      'Check whether adding one would exceed capacity.',
      'Accept and add one, or reject with a retry delay.',
    ],
    example:
      'Capacity 10, drain 2/second → 10 immediate requests, then room for one every 500 ms.',
    use: 'Immediate admission with a bounded burst and steady recovery.',
    caveat:
      'This is a meter: it does not queue or evenly pace outgoing requests.',
    image: 'leaky-meter.svg',
    caption:
      'A numeric level drains over time. Accepted requests forward immediately.',
  },
]

const viewsBySection: Record<string, string> = {
  overview: 'learn',
  diagram: 'learn',
  'how-it-works': 'learn',
  example: 'learn',
  docker: 'docker',
  sdk: 'sdk',
  latency: 'benchmarks',
  comparison: 'compare',
}

export default function AlgorithmsPage() {
  const [activeId, setActiveId] = useState(algorithms[0].id)
  const [view, setView] = useState('learn')
  const [settings, setSettings] = useState(defaultSettings)
  const [pendingAnchor, setPendingAnchor] = useState<{ id: string } | null>(
    null,
  )
  const indexSections = useMemo(
    () =>
      [
        ['algorithms', 'Choose an algorithm'],
        ['overview', 'Overview'],
        ...(view === 'learn'
          ? [
              ['diagram', 'Diagram'],
              ['how-it-works', 'How it works'],
              ['example', 'Example'],
            ]
          : []),
        ['docker', 'Docker configuration'],
        ['sdk', 'Go SDK'],
        ['latency', 'Benchmarks'],
        ['comparison', 'Compare'],
      ] as [string, string][],
    [view],
  )
  useEffect(() => {
    const readHash = () => {
      const id = window.location.hash.slice(1)
      if (viewsBySection[id]) {
        setView(viewsBySection[id])
        setPendingAnchor({ id })
      }
    }
    const frame = requestAnimationFrame(readHash)
    window.addEventListener('hashchange', readHash)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('hashchange', readHash)
    }
  }, [])
  function navigateIndex(id: string) {
    if (viewsBySection[id]) setView(viewsBySection[id])
    setPendingAnchor({ id })
  }
  useEffect(() => {
    if (!pendingAnchor) return
    const frame = requestAnimationFrame(() => {
      document
        .getElementById(pendingAnchor.id)
        ?.scrollIntoView({ block: 'start' })
      history.replaceState(null, '', `#${pendingAnchor.id}`)
      setPendingAnchor(null)
    })
    return () => cancelAnimationFrame(frame)
  }, [pendingAnchor, view])

  return (
    <div className="mx-auto w-full max-w-7xl min-w-0 px-5 py-7 md:px-8 md:py-10 lg:pe-60">
      <DocsIndex
        sections={indexSections}
        onNavigate={navigateIndex}
        contextKey={`${activeId}:${view}`}
        activeSection={
          (
            {
              docker: 'docker',
              sdk: 'sdk',
              benchmarks: 'latency',
              compare: 'comparison',
            } as Record<string, string>
          )[view]
        }
      />
      <details className="mb-4 rounded-lg border border-border p-3 lg:hidden">
        <summary className="cursor-pointer rounded-sm text-xs font-medium focus-visible:outline-2 focus-visible:outline-ring">
          On this page
        </summary>
        <nav
          aria-label="On this page mobile"
          className="mt-2 grid grid-cols-2 gap-1"
        >
          {indexSections.map(([id, label]) => (
            <a
              key={id}
              href={`#${id}`}
              onClick={(event) => {
                event.preventDefault()
                navigateIndex(id)
              }}
              className="rounded-md px-2 py-2 text-xs text-muted-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
            >
              {label}
            </a>
          ))}
        </nav>
      </details>
      <Link
        href="/system-design/rate-limiter"
        className="inline-flex min-h-9 items-center gap-2 rounded-md text-xs text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
      >
        <ArrowLeft className="size-3.5" />
        Rate limiter
      </Link>
      <header className="mt-5 mb-8 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="mb-2 text-xs font-medium tracking-widest text-primary uppercase">
            The algorithm field guide
          </p>
          <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
            Rate limiting, made visual.
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">
            Five approaches. Pick one, see the flow, understand the tradeoff.
          </p>
        </div>
        <span className="inline-flex items-center gap-2 rounded-full border border-border px-3 py-1.5 text-xs text-muted-foreground">
          <Check className="size-3.5" />
          All five support RAM & Redis
        </span>
      </header>

      <Tabs
        value={activeId}
        onValueChange={(value) => setActiveId(String(value))}
        className="scroll-mt-20 gap-5"
        id="algorithms"
      >
        <div className="min-w-0 overflow-x-auto rounded-xl border border-border bg-muted/30 p-1">
          <TabsList
            aria-label="Rate limiting algorithms"
            className="h-auto! w-full min-w-max justify-start gap-1 bg-transparent p-0"
          >
            {algorithms.map((algo) => (
              <TabsTrigger
                key={algo.id}
                value={algo.id}
                aria-label={algo.title}
                className="min-h-11 shrink-0 rounded-lg px-4 py-2.5 text-sm shadow-none! data-active:bg-primary data-active:text-primary-foreground"
              >
                <span className="text-sm font-medium">{algo.short}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        <p className="-mt-3 text-[11px] text-muted-foreground sm:hidden">
          Scroll sideways to see all five algorithms.
        </p>
        {algorithms.map((algo) => (
          <TabsContent key={algo.id} value={algo.id} className="min-w-0">
            <Tabs
              value={view}
              onValueChange={(value) => setView(String(value))}
              className="gap-6"
            >
              <div className="min-w-0 overflow-x-auto border-b border-border pb-1">
                <TabsList
                  aria-label="Explore algorithm"
                  variant="line"
                  className="h-11! w-max min-w-full justify-start gap-1 pb-1"
                >
                  <TabsTrigger
                    value="learn"
                    className="min-h-9 flex-none px-2 text-xs sm:text-sm"
                  >
                    <BookOpen />
                    Overview
                  </TabsTrigger>
                  <TabsTrigger
                    value="docker"
                    className="min-h-10 flex-none px-3 text-xs sm:text-sm"
                  >
                    <Container />
                    Docker
                  </TabsTrigger>
                  <TabsTrigger
                    value="sdk"
                    className="min-h-10 flex-none px-3 text-xs sm:text-sm"
                  >
                    <Code2 />
                    Go SDK
                  </TabsTrigger>
                  <TabsTrigger
                    value="benchmarks"
                    className="min-h-9 flex-none px-2 text-xs sm:text-sm"
                  >
                    <ChartNoAxesCombined />
                    Benchmarks
                  </TabsTrigger>
                  <TabsTrigger
                    value="compare"
                    className="min-h-9 flex-none px-2 text-xs sm:text-sm"
                  >
                    <Layers />
                    Compare
                  </TabsTrigger>
                </TabsList>
              </div>

              <TabsContent value="learn" className="min-w-0">
                <article id="overview" className="min-w-0 scroll-mt-20">
                  <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <p className="mb-1 text-xs font-medium text-primary">
                        {algo.tag}
                      </p>
                      <h2 className="text-2xl font-semibold tracking-tight">
                        {algo.title}
                      </h2>
                      <p className="mt-2 text-sm text-muted-foreground">
                        {algo.summary}
                      </p>
                    </div>
                    <div className="flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                      <span className="rounded-md bg-muted px-2.5 py-1.5">
                        {algo.memory} memory / identity
                      </span>
                      <span className="rounded-md bg-muted px-2.5 py-1.5">
                        {algo.accuracy}
                      </span>
                    </div>
                  </div>
                  <figure
                    id="diagram"
                    className="scroll-mt-20 overflow-hidden rounded-2xl border border-border"
                  >
                    <Dialog>
                      <DialogTrigger
                        aria-label={`Expand ${algo.title} diagram`}
                        className="group relative block w-full cursor-zoom-in bg-[#0d1117] focus-visible:outline-2 focus-visible:-outline-offset-4 focus-visible:outline-ring"
                      >
                        <Image
                          src={`/algorithms/${algo.image}`}
                          alt={`${algo.title}: ${algo.summary}`}
                          width={1376}
                          height={768}
                          sizes="(max-width: 768px) 100vw, 1000px"
                          unoptimized={algo.image.endsWith('.svg')}
                          className="h-auto w-full"
                        />
                        <span className="absolute end-3 bottom-3 inline-flex items-center gap-2 rounded-lg bg-black/75 px-3 py-2 text-xs text-white">
                          <Expand className="size-3.5" />
                          Expand diagram
                        </span>
                      </DialogTrigger>
                      <DialogContent className="max-h-[92dvh] overflow-y-auto motion-reduce:animate-none sm:max-w-6xl">
                        <DialogTitle className="pe-10 font-semibold">
                          {algo.title}
                        </DialogTitle>
                        <DialogDescription>{algo.caption}</DialogDescription>
                        <div
                          className="overflow-x-auto rounded-lg bg-[#0d1117]"
                          tabIndex={0}
                          role="region"
                          aria-label="Full size diagram"
                        >
                          <Image
                            src={`/algorithms/${algo.image}`}
                            alt={`${algo.title}: ${algo.summary}`}
                            width={1376}
                            height={768}
                            unoptimized
                            className="h-auto w-full min-w-[700px]"
                          />
                        </div>
                        <p className="text-xs text-muted-foreground">
                          Scroll sideways on smaller screens to read the full
                          diagram.
                        </p>
                      </DialogContent>
                    </Dialog>
                    <figcaption className="bg-muted/20 px-4 py-3 text-xs leading-5 text-muted-foreground">
                      {algo.caption}
                    </figcaption>
                  </figure>
                  <div className="mt-7 grid gap-7 lg:grid-cols-[1.2fr_1fr]">
                    <section id="how-it-works" className="scroll-mt-20">
                      <h3 className="mb-4 text-sm font-semibold">
                        Three steps. One decision.
                      </h3>
                      <ol className="space-y-4">
                        {algo.steps.map((step, index) => (
                          <li
                            key={step}
                            className="flex gap-3 text-sm leading-6"
                          >
                            <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs text-muted-foreground">
                              {index + 1}
                            </span>
                            <span>{step}</span>
                          </li>
                        ))}
                      </ol>
                    </section>
                    <aside className="space-y-4 rounded-xl bg-muted/40 p-5">
                      <div>
                        <h3 className="mb-1 flex items-center gap-2 text-xs font-semibold">
                          <Check className="size-3.5 text-primary" />
                          Good for
                        </h3>
                        <p className="text-sm leading-6 text-muted-foreground">
                          {algo.use}
                        </p>
                      </div>
                      <div>
                        <h3 className="mb-1 flex items-center gap-2 text-xs font-semibold">
                          <Lightbulb className="size-3.5 text-primary" />
                          Keep in mind
                        </h3>
                        <p className="text-sm leading-6 text-muted-foreground">
                          {algo.caveat}
                        </p>
                      </div>
                    </aside>
                  </div>
                  <div
                    id="example"
                    className="mt-6 scroll-mt-20 rounded-xl border border-border p-4"
                  >
                    <p className="mb-1 text-[11px] font-semibold tracking-wide text-primary uppercase">
                      A quick example
                    </p>
                    <p className="text-sm leading-6">{algo.example}</p>
                  </div>
                  <button
                    type="button"
                    onClick={() => setView('benchmarks')}
                    className="mt-6 inline-flex min-h-10 items-center gap-2 rounded-lg border border-border px-4 text-sm font-medium hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                  >
                    See RAM vs Redis latency
                    <ArrowRight className="size-4" />
                  </button>
                </article>
              </TabsContent>
              <TabsContent value="docker" className="min-w-0">
                <AlgorithmSetup
                  algorithm={activeId}
                  mode="docker"
                  settings={settings}
                  onChange={setSettings}
                />
              </TabsContent>
              <TabsContent value="sdk" className="min-w-0">
                <AlgorithmSetup
                  algorithm={activeId}
                  mode="sdk"
                  settings={settings}
                  onChange={setSettings}
                />
              </TabsContent>
              <TabsContent value="benchmarks" className="min-w-0">
                <LatencyResults activeAlgorithm={activeId} />
              </TabsContent>
              <TabsContent
                id="comparison"
                value="compare"
                className="min-w-0 scroll-mt-20"
              >
                <h2 className="text-xl font-semibold">Pick by the tradeoff.</h2>
                <p className="mt-2 mb-5 text-sm text-muted-foreground">
                  Same goal. Different ways to count recent usage.
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  {algorithms.map((item) => (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => {
                        setActiveId(item.id)
                        setView('learn')
                      }}
                      className="rounded-xl border border-border p-5 text-start hover:border-primary focus-visible:outline-2 focus-visible:outline-ring"
                    >
                      <span className="flex items-center justify-between gap-2 text-sm font-semibold">
                        {item.short}
                        <ArrowRight className="size-4 text-primary" />
                      </span>
                      <span className="mt-2 block text-sm leading-6 text-muted-foreground">
                        {item.use}
                      </span>
                      <span className="mt-3 block text-xs text-muted-foreground">
                        {item.memory} memory · {item.accuracy}
                      </span>
                    </button>
                  ))}
                </div>
                <p className="mt-5 text-xs text-muted-foreground">
                  All implementations reject immediately when full. The leaky
                  bucket is a meter, not a queue.
                </p>
              </TabsContent>
            </Tabs>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
