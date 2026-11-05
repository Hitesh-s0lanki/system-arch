interface LogoMarkProps {
  size?: number
  className?: string
}

/** SVG icon mark — node graph representing a layered system architecture */
export function LogoMark({ size = 22, className }: LogoMarkProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 20 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden="true"
    >
      {/* Top node — client / entry point */}
      <circle cx="10" cy="2.5" r="2" fill="currentColor" />
      {/* Middle nodes — servers / services */}
      <circle cx="4" cy="11" r="1.75" fill="currentColor" fillOpacity="0.85" />
      <circle cx="16" cy="11" r="1.75" fill="currentColor" fillOpacity="0.85" />
      {/* Bottom node — data layer */}
      <circle cx="10" cy="17.5" r="1.75" fill="currentColor" fillOpacity="0.65" />
      {/* Connections */}
      <line x1="10" y1="4.5"  x2="4.8"  y2="9.5"  stroke="currentColor" strokeWidth="1.1" strokeOpacity="0.5" strokeLinecap="round" />
      <line x1="10" y1="4.5"  x2="15.2" y2="9.5"  stroke="currentColor" strokeWidth="1.1" strokeOpacity="0.5" strokeLinecap="round" />
      <line x1="4"  y1="12.8" x2="9.2"  y2="15.9" stroke="currentColor" strokeWidth="1.1" strokeOpacity="0.5" strokeLinecap="round" />
      <line x1="16" y1="12.8" x2="10.8" y2="15.9" stroke="currentColor" strokeWidth="1.1" strokeOpacity="0.5" strokeLinecap="round" />
    </svg>
  )
}

interface LogoProps {
  showName?: boolean
  className?: string
}

export function Logo({ showName = true, className }: LogoProps) {
  return (
    <span className={`flex items-center gap-2 ${className ?? ''}`}>
      <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-[5px] bg-primary text-primary-foreground">
        <LogoMark size={14} />
      </span>
      {showName && (
        <span className="text-[13px] font-semibold tracking-[-0.01em] text-foreground">
          SystemArch
        </span>
      )}
    </span>
  )
}

