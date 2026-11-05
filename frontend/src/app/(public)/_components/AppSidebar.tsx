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
  SidebarMenuItem,
  SidebarSeparator,
} from '@/components/ui/sidebar'
import { Gauge, HelpCircle, Zap } from 'lucide-react'
import { Logo } from '@/components/Logo'
import Link from 'next/link'
import { usePathname } from 'next/navigation'

const systemDesignNav = [
  { label: 'Rate Limiter', icon: Zap, href: '/system-design/rate-limiter' },
  { label: 'Load Tester', icon: Gauge, href: '/system-design/load-tester' },
]

export function AppSidebar() {
  const pathname = usePathname()
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
              {systemDesignNav.map((item) => (
                <SidebarMenuItem key={item.label}>
                  <SidebarMenuButton
                    render={<Link href={item.href} />}
                    isActive={pathname.startsWith(item.href)}
                    aria-current={
                      pathname.startsWith(item.href) ? 'page' : undefined
                    }
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
