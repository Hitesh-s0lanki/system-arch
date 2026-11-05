import type { Metadata } from 'next'
import React from 'react'
import { Geist, Geist_Mono } from 'next/font/google'
import { TooltipProvider } from '@/components/ui/tooltip'
import './globals.css'

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
})

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
})

export const metadata: Metadata = {
  title: {
    default: 'SystemArch',
    template: '%s · SystemArch',
  },
  description: 'Learn system design through interactive concepts — rate limiters, load balancers, caches, and more.',
  metadataBase: new URL('https://systemarch.dev'),
  openGraph: {
    title: 'SystemArch',
    description: 'Learn system design through interactive concepts.',
    siteName: 'SystemArch',
    type: 'website',
  },
  twitter: {
    card: 'summary_large_image',
    title: 'SystemArch',
    description: 'Learn system design through interactive concepts.',
  },
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col">
        <TooltipProvider>{children}</TooltipProvider>
      </body>
    </html>
  )
}
