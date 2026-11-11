'use client'

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuAction,
  SidebarMenuItem,
  SidebarSeparator,
  SidebarMenuSub,
  SidebarMenuSubItem,
  SidebarMenuSubButton,
  useSidebar,
} from '@/components/ui/sidebar'
import { ChevronDown, Gauge, HelpCircle, Play, Zap } from 'lucide-react'
import { Logo } from '@/components/Logo'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useState } from 'react'

const systemDesignNav = [
  { label: 'Load Tester', icon: Gauge, href: '/system-design/load-tester' },
  {
    label: 'Video Streaming',
    icon: Play,
    href: '/system-design/video-streaming',
  },
]

export function AppSidebar() {
  const pathname = usePathname()
  const [rateLimiterOpen, setRateLimiterOpen] = useState(true)
  const { state, setOpen, setOpenMobile } = useSidebar()
  const rateLimiterLinks = [
    { label: 'Overview', href: '/system-design/rate-limiter' },
    { label: 'Algorithms', href: '/system-design/rate-limiter/algorithms' },
    { label: 'Installation guide', href: '/system-design/rate-limiter/docs' },
    { label: 'Proxy Mode', href: '/system-design/rate-limiter/proxy-mode' },
    { label: 'SDK Mode', href: '/system-design/rate-limiter/sdk-mode' },
    { label: 'Gateway Mode', href: '/system-design/rate-limiter/gateway-mode' },
    {
      label: 'Distributed Mode',
      href: '/system-design/rate-limiter/distributed-mode',
    },
  ]
  return (
    <Sidebar collapsible="icon">
      {/* ── Brand ── */}
      <SidebarHeader className="h-11 justify-center border-b border-sidebar-border px-4">
        <Link href="/">
          <Logo />
        </Link>
      </SidebarHeader>

      {/* ── Navigation ── */}
      <SidebarContent className="px-2 py-2">
        <SidebarGroup className="p-0">
          <SidebarGroupLabel className="mb-0.5 px-2 text-[10.5px] font-semibold tracking-[0.07em] text-muted-foreground/50 uppercase group-data-[collapsible=icon]:hidden">
            System Design
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu className="gap-px">
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Rate Limiter"
                  render={<Link href="/system-design/rate-limiter" />}
                  onClick={() => {
                    setRateLimiterOpen(true)
                    setOpenMobile(false)
                  }}
                  className="h-[30px] gap-2 rounded-md px-2 text-[13px] font-normal text-muted-foreground"
                >
                  <Zap className="size-[14px] shrink-0" strokeWidth={1.6} />
                  <span>Rate Limiter</span>
                </SidebarMenuButton>
                <SidebarMenuAction
                  aria-label={
                    rateLimiterOpen
                      ? 'Collapse Rate Limiter navigation'
                      : 'Expand Rate Limiter navigation'
                  }
                  aria-expanded={rateLimiterOpen}
                  aria-controls="rate-limiter-navigation"
                  onClick={() => {
                    if (state === 'collapsed') setOpen(true)
                    setRateLimiterOpen((value) => !value)
                  }}
                >
                  <ChevronDown
                    className={`size-3.5 transition-transform ${rateLimiterOpen ? '' : '-rotate-90'}`}
                  />
                </SidebarMenuAction>
                <div id="rate-limiter-navigation" hidden={!rateLimiterOpen}>
                  <SidebarMenuSub>
                    {rateLimiterLinks.map((item) => (
                      <SidebarMenuSubItem key={item.href}>
                        <SidebarMenuSubButton
                          render={<Link href={item.href} />}
                          isActive={pathname === item.href}
                          aria-current={
                            pathname === item.href ? 'page' : undefined
                          }
                          onClick={() => setOpenMobile(false)}
                          className="text-[13px]"
                        >
                          {item.label}
                        </SidebarMenuSubButton>
                      </SidebarMenuSubItem>
                    ))}
                  </SidebarMenuSub>
                </div>
              </SidebarMenuItem>
              {systemDesignNav.map((item) => (
                <SidebarMenuItem key={item.label}>
                  <SidebarMenuButton
                    render={<Link href={item.href} />}
                    isActive={pathname.startsWith(item.href)}
                    aria-current={
                      pathname.startsWith(item.href) ? 'page' : undefined
                    }
                    onClick={() => setOpenMobile(false)}
                    tooltip={item.label}
                    className="h-[30px] gap-2 rounded-md px-2 text-[13px] font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  >
                    <item.icon
                      className="h-[14px] w-[14px] shrink-0"
                      strokeWidth={1.6}
                    />
                    <span className="leading-none">{item.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      {/* ── Footer ── */}
      <SidebarFooter className="px-2 pb-3">
        <SidebarSeparator className="mb-2" />
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              render={<a href="/help" />}
              tooltip="Support"
              className="h-[30px] gap-2 rounded-md px-2 text-[13px] font-normal text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              <HelpCircle
                className="h-[14px] w-[14px] shrink-0"
                strokeWidth={1.6}
              />
              <span className="leading-none">Support</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
