import { SidebarProvider, SidebarInset } from '@/components/ui/sidebar'
import { AppSidebar } from './_components/AppSidebar'
import { AppBar } from './_components/AppBar'

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset className="flex flex-col">
        <AppBar title="Overview" />
        <main className="flex flex-1 overflow-y-auto">
          {children}
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

