import {
  FileText,
  FolderOpen,
  Users,
  BarChart2,
  Zap,
  ArrowUpRight,
  TrendingUp,
  Clock,
  CheckCircle2,
  AlertCircle,
} from 'lucide-react'

// ── Types ─────────────────────────────────────────────────────────────────────

interface StatCard {
  label: string
  value: string
  delta: string
  up: boolean
  icon: React.ElementType
}

interface ActivityItem {
  id: string
  title: string
  meta: string
  time: string
  status: 'done' | 'pending' | 'alert'
}

// ── Data ──────────────────────────────────────────────────────────────────────

const stats: StatCard[] = [
  { label: 'Projects', value: '24', delta: '+3 this month', up: true, icon: FolderOpen },
  { label: 'Team members', value: '12', delta: '+1 this week', up: true, icon: Users },
  { label: 'Documents', value: '138', delta: '+12 today', up: true, icon: FileText },
  { label: 'Pipelines', value: '7', delta: '2 paused', up: false, icon: Zap },
]

const sections = [
  { id: 'overview', label: 'Overview', active: true },
  { id: 'stats', label: 'At a glance' },
  { id: 'activity', label: 'Activity' },
  { id: 'quick-access', label: 'Go to' },
]

const activity: ActivityItem[] = [
  { id: '1', title: 'Deployment pipeline completed', meta: 'prod-v2.4.1', time: '2m ago', status: 'done' },
  { id: '2', title: 'New team member invited', meta: 'sarah@example.com', time: '15m ago', status: 'pending' },
  { id: '3', title: 'Document review overdue', meta: 'System Architecture.pdf', time: '1h ago', status: 'alert' },
  { id: '4', title: 'Analytics report generated', meta: 'October 2026', time: '3h ago', status: 'done' },
  { id: '5', title: 'Integration sync failed', meta: 'GitHub → SystemArch', time: '5h ago', status: 'alert' },
  { id: '6', title: 'Project milestone reached', meta: 'Phase 1 complete', time: '1d ago', status: 'done' },
]

const quickLinks = [
  { label: 'View all projects', href: '/projects', icon: FolderOpen },
  { label: 'Browse documents', href: '/documents', icon: FileText },
  { label: 'Team management', href: '/team', icon: Users },
  { label: 'Analytics & reports', href: '/analytics', icon: BarChart2 },
]

// ── Helpers ───────────────────────────────────────────────────────────────────

function StatusDot({ status }: { status: ActivityItem['status'] }) {
  if (status === 'done')
    return <CheckCircle2 className="mt-px h-3.5 w-3.5 shrink-0 text-primary" strokeWidth={1.75} />
  if (status === 'alert')
    return <AlertCircle className="mt-px h-3.5 w-3.5 shrink-0 text-destructive" strokeWidth={1.75} />
  return <Clock className="mt-px h-3.5 w-3.5 shrink-0 text-muted-foreground/50" strokeWidth={1.75} />
}

// ── Component ─────────────────────────────────────────────────────────────────

export function DashboardContent() {
  return (
    <div className="flex flex-1 overflow-y-auto">

      {/* ── Main content ──────────────────────────── */}
      <div className="flex-1">
        <div className="mx-auto max-w-2xl px-10 py-10">

          {/* Page heading */}
          <div id="overview" className="mb-9">
            <h1 className="text-[22px] font-semibold tracking-[-0.02em] text-foreground leading-snug">
              Workspace overview
            </h1>
            <p className="mt-2 text-[14px] leading-relaxed text-muted-foreground">
              Track projects, monitor pipelines, and collaborate with your team — all from one place.
            </p>
          </div>

          {/* Stats */}
          <div id="stats" className="mb-9">
            <h2 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.07em] text-muted-foreground/60">
              At a glance
            </h2>
            <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
              {stats.map((s) => (
                <div
                  key={s.label}
                  className="rounded-lg border border-border bg-card px-4 py-3.5"
                >
                  <div className="flex items-center justify-between">
                    <span className="text-[11px] font-medium text-muted-foreground/70">{s.label}</span>
                    <s.icon className="h-3.5 w-3.5 text-muted-foreground/30" strokeWidth={1.5} />
                  </div>
                  <p className="mt-2 text-[22px] font-semibold tabular-nums tracking-tight text-foreground leading-none">
                    {s.value}
                  </p>
                  <p className={`mt-1.5 flex items-center gap-1 text-[11px] font-medium ${s.up ? 'text-primary' : 'text-muted-foreground/60'}`}>
                    {s.up
                      ? <TrendingUp className="h-3 w-3" strokeWidth={2} />
                      : <Clock className="h-3 w-3" strokeWidth={2} />
                    }
                    {s.delta}
                  </p>
                </div>
              ))}
            </div>
          </div>

          {/* Activity */}
          <div id="activity" className="mb-9">
            <h2 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.07em] text-muted-foreground/60">
              Activity
            </h2>
            <div className="rounded-lg border border-border divide-y divide-border">
              {activity.map((item) => (
                <div key={item.id} className="flex items-start gap-3 px-4 py-2.5">
                  <StatusDot status={item.status} />
                  <div className="flex-1 min-w-0">
                    <p className="text-[13px] font-normal text-foreground leading-snug">{item.title}</p>
                    <p className="text-[11px] text-muted-foreground/60 mt-0.5 leading-none">{item.meta}</p>
                  </div>
                  <span className="shrink-0 text-[11px] text-muted-foreground/50 whitespace-nowrap pt-0.5 tabular-nums">
                    {item.time}
                  </span>
                </div>
              ))}
            </div>
          </div>

          {/* Quick Access */}
          <div id="quick-access">
            <h2 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.07em] text-muted-foreground/60">
              Go to
            </h2>
            <div className="flex flex-col divide-y divide-border rounded-lg border border-border">
              {quickLinks.map((link) => (
                <a
                  key={link.label}
                  href={link.href}
                  className="group flex items-center justify-between px-4 py-2.5 text-[13px] text-foreground transition-colors hover:bg-muted/40"
                >
                  <span className="flex items-center gap-2.5">
                    <link.icon className="h-3.5 w-3.5 text-muted-foreground/50" strokeWidth={1.6} />
                    <span className="font-normal">{link.label}</span>
                  </span>
                  <ArrowUpRight className="h-3.5 w-3.5 text-muted-foreground/40 opacity-0 transition-opacity group-hover:opacity-100" />
                </a>
              ))}
            </div>
          </div>

        </div>
      </div>

      {/* ── Right "On this page" ──────────────────── */}
      <aside className="hidden w-52 shrink-0 self-start sticky top-0 border-l border-border xl:block">
        <div className="px-5 py-8">
          <p className="mb-3 text-[11px] font-semibold uppercase tracking-[0.07em] text-muted-foreground/50">
            On this page
          </p>
          <nav className="flex flex-col">
            {sections.map((s) => (
              <a
                key={s.id}
                href={`#${s.id}`}
                className={`border-l py-[5px] pl-3 text-[13px] transition-colors leading-snug ${
                  s.active
                    ? 'border-primary font-medium text-primary'
                    : 'border-transparent font-normal text-muted-foreground hover:border-border hover:text-foreground'
                }`}
              >
                {s.label}
              </a>
            ))}
          </nav>
        </div>
      </aside>

    </div>
  )
}
