'use client'

import * as React from 'react'
import { SidebarTrigger, useSidebar } from '@/components/ui/sidebar'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from '@/components/ui/command'
import { Kbd } from '@/components/ui/kbd'
import {
  Search,
  Gauge,
  LayoutDashboard,
  Zap,
  SlidersHorizontal,
  Layers,
  PanelLeft,
  HelpCircle,
  Activity,
  BookOpen,
  Play,
} from 'lucide-react'
import { usePathname, useRouter } from 'next/navigation'

interface AppBarProps {
  title?: string
  breadcrumb?: string[]
}

export function AppBar({ title = 'Overview', breadcrumb = [] }: AppBarProps) {
  const [open, setOpen] = React.useState(false)
  const pathname = usePathname()
  const router = useRouter()
  const { toggleSidebar } = useSidebar()

  const pageTitle = pathname.startsWith('/system-design/rate-limiter/docs')
    ? 'Rate Limiter / Installation guide'
    : pathname.startsWith('/system-design/rate-limiter/fixed-window')
      ? 'Rate Limiter / Fixed Window Counter'
      : pathname.startsWith('/system-design/rate-limiter')
        ? 'Rate Limiter'
        : pathname.startsWith('/system-design/load-tester')
          ? 'Load Tester'
          : pathname.startsWith('/system-design/video-streaming')
            ? 'Video Streaming / StreamFlow'
            : title

  React.useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen((prev) => !prev)
      }
    }

    document.addEventListener('keydown', down)
    return () => document.removeEventListener('keydown', down)
  }, [])

  const runCommand = React.useCallback((command: () => void) => {
    setOpen(false)
    command()
  }, [])

  return (
    <>
      <header className="sticky top-0 z-30 flex h-11 items-center justify-between gap-2.5 border-b border-border bg-background px-4">
        {/* Left — trigger + breadcrumb */}
        <div className="flex min-w-0 items-center gap-2.5">
          <SidebarTrigger className="-ml-1 h-7 w-7 text-muted-foreground" />

          <nav className="flex items-center gap-1 truncate text-[14px] text-muted-foreground">
            <span className="font-medium text-foreground">{pageTitle}</span>
            {breadcrumb.map((crumb, i) => (
              <span key={i} className="flex items-center gap-1">
                <span className="text-muted-foreground/40">/</span>
                <span>{crumb}</span>
              </span>
            ))}
          </nav>
        </div>

        {/* Right — Search Bar */}
        <div className="flex items-center">
          <button
            type="button"
            onClick={() => setOpen(true)}
            className="group relative flex h-8 w-48 items-center justify-between rounded-lg border border-input/60 bg-muted/20 px-2.5 text-[13px] text-muted-foreground transition-colors hover:border-input hover:bg-muted/50 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none sm:w-60 md:w-68"
            aria-label="Search documentation and commands"
          >
            <span className="inline-flex items-center gap-2 truncate">
              <Search className="h-4 w-4 shrink-0 opacity-70 transition-opacity group-hover:opacity-100" />
              <span className="truncate">Search system designs...</span>
            </span>
            <Kbd className="hidden h-5 items-center gap-0.5 rounded border border-border/80 bg-background/80 px-1.5 font-mono text-[11px] font-medium text-muted-foreground select-none sm:inline-flex">
              <span className="text-[11px]">⌘</span>K
            </Kbd>
          </button>
        </div>
      </header>

      {/* Command Palette Dialog */}
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandInput placeholder="Type a command or search..." />
        <CommandList>
          <CommandEmpty>No results found.</CommandEmpty>

          <CommandGroup heading="Navigation">
            <CommandItem onSelect={() => runCommand(() => router.push('/'))}>
              <LayoutDashboard className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Overview Dashboard</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() => router.push('/system-design/rate-limiter'))
              }
            >
              <Zap className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Rate Limiter</span>
              <CommandShortcut>SD</CommandShortcut>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() =>
                  router.push('/system-design/rate-limiter/docs'),
                )
              }
            >
              <BookOpen className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Rate Limiter Installation guide</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() => router.push('/system-design/load-tester'))
              }
            >
              <Gauge className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Load Tester</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() => router.push('/system-design/video-streaming'))
              }
            >
              <Play className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Video Streaming StreamFlow</span>
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Rate Limiting Algorithms">
            <CommandItem
              onSelect={() =>
                runCommand(() =>
                  router.push('/system-design/rate-limiter/fixed-window'),
                )
              }
            >
              <SlidersHorizontal className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Token Bucket Algorithm</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() =>
                  router.push('/system-design/rate-limiter/fixed-window'),
                )
              }
            >
              <Activity className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Leaky Bucket Algorithm</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() =>
                  router.push('/system-design/rate-limiter/fixed-window'),
                )
              }
            >
              <Layers className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Fixed Window Counter</span>
            </CommandItem>
            <CommandItem
              onSelect={() =>
                runCommand(() =>
                  router.push('/system-design/rate-limiter/fixed-window'),
                )
              }
            >
              <Layers className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Sliding Window Log & Counter</span>
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Actions & Help">
            <CommandItem onSelect={() => runCommand(() => toggleSidebar())}>
              <PanelLeft className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Toggle Sidebar</span>
              <CommandShortcut>⌘B</CommandShortcut>
            </CommandItem>
            <CommandItem
              onSelect={() => runCommand(() => router.push('/help'))}
            >
              <HelpCircle className="mr-2 h-4 w-4 text-muted-foreground" />
              <span>Support & Documentation</span>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  )
}
