import Link from 'next/link'
import {
  ArrowRight,
  ArrowUpRight,
  Gauge,
  Layers,
  Play,
  Zap,
} from 'lucide-react'

const designs = [
  {
    title: 'Rate Limiter',
    category: 'Traffic control',
    description:
      'Control requests with a fixed-window counter. Explore identity policies, local RAM counters, and shared quotas with Redis.',
    icon: Zap,
    technologies: ['Go', 'Redis', 'Docker'],
    flow: ['Client', 'Gateway', 'Backend'],
    href: '/system-design/rate-limiter',
    action: 'Explore rate limiter',
    external: false,
    detail: 'Overview, installation guide, and interactive request flow',
  },
  {
    title: 'Load Tester',
    category: 'Performance testing',
    description:
      'Run HTTP workloads with k6 containers. Configure request rates, inspect latency and errors, and explore capacity under load.',
    icon: Gauge,
    technologies: ['Go', 'k6', 'Docker'],
    flow: ['Workload', 'k6 workers', 'Results'],
    href: '/system-design/load-tester',
    action: 'Open load tester',
    external: false,
    detail: 'Workload configuration and test results',
  },
  {
    title: 'Video Streaming',
    category: 'Media delivery',
    description:
      'Upload MP4 videos and transcode them with FFmpeg into HLS renditions for adaptive bitrate playback.',
    icon: Play,
    technologies: ['Java', 'Spring Boot', 'FFmpeg'],
    flow: ['Upload', 'Transcode', 'HLS playback'],
    href: '/system-design/video-streaming',
    action: 'Explore video streaming',
    external: false,
    detail: 'StreamFlow overview, features, and live app',
  },
]

export function DashboardContent() {
  return (
    <div className="mx-auto w-full max-w-7xl min-w-0 px-5 py-8 md:px-10 md:py-12">
      <header className="max-w-2xl">
        <div className="mb-4 flex items-center gap-2 text-xs font-medium tracking-widest text-muted-foreground uppercase">
          <Layers className="size-4" aria-hidden="true" /> SystemArch
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          System designs, implemented
        </h1>
        <p className="mt-4 text-base leading-7 text-muted-foreground">
          Explore the systems built in this project. Open an implementation to
          understand its request flow, try the available tools, and follow the
          setup guides.
        </p>
      </header>

      <section aria-labelledby="implementations-title" className="mt-10">
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <h2 id="implementations-title" className="text-sm font-medium">
            Implemented systems
          </h2>
          <span className="text-xs text-muted-foreground">
            {designs.length} implementations
          </span>
        </div>
        <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
          {designs.map((design) => (
            <Link
              key={design.title}
              href={design.href}
              target={design.external ? '_blank' : undefined}
              rel={design.external ? 'noreferrer' : undefined}
              aria-label={`${design.action}${design.external ? ' on GitHub (opens in a new tab)' : ''}`}
              className="group flex min-w-0 flex-col rounded-xl border border-border bg-card p-6 transition-colors hover:border-primary/40 hover:bg-accent/30 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"
            >
              <div className="flex items-center justify-between gap-3">
                <span className="flex size-10 items-center justify-center rounded-lg border border-primary/15 bg-primary/5 text-primary">
                  <design.icon
                    className="size-5"
                    strokeWidth={1.6}
                    aria-hidden="true"
                  />
                </span>
                <span className="text-xs text-muted-foreground">
                  {design.category}
                </span>
              </div>
              <h3 className="mt-5 text-lg font-semibold tracking-tight">
                {design.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {design.description}
              </p>
              <div className="mt-5 flex flex-wrap gap-2">
                {design.technologies.map((technology) => (
                  <span
                    key={technology}
                    className="rounded-md border border-border px-2 py-1 text-xs text-muted-foreground"
                  >
                    {technology}
                  </span>
                ))}
              </div>
              <div className="mt-6 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg bg-muted/40 px-3 py-3 text-xs text-muted-foreground">
                {design.flow.map((step, index) => (
                  <span key={step} className="inline-flex items-center gap-2">
                    {index > 0 && (
                      <ArrowRight
                        className="size-3 text-muted-foreground/60"
                        aria-hidden="true"
                      />
                    )}
                    {step}
                  </span>
                ))}
              </div>
              <div className="mt-auto pt-6">
                <p className="text-xs leading-5 text-muted-foreground">
                  {design.detail}
                </p>
                <span className="mt-3 flex items-center justify-between gap-3 text-sm font-medium text-primary">
                  {design.action}
                  {design.external ? (
                    <ArrowUpRight className="size-4" aria-hidden="true" />
                  ) : (
                    <ArrowRight className="size-4" aria-hidden="true" />
                  )}
                </span>
              </div>
            </Link>
          ))}
        </div>
      </section>
    </div>
  )
}
