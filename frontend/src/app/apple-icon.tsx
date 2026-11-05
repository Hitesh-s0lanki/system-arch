import { ImageResponse } from 'next/og'

export const size = { width: 180, height: 180 }
export const contentType = 'image/png'

export default function AppleIcon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: 180,
          height: 180,
          borderRadius: 40,
          background: '#9b4819',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <svg width="110" height="110" viewBox="0 0 20 20" fill="none">
          {/* Top node (client) */}
          <circle cx="10" cy="2.5" r="2" fill="white" />
          {/* Middle-left node (server A) */}
          <circle cx="4" cy="11" r="2" fill="white" opacity="0.85" />
          {/* Middle-right node (server B) */}
          <circle cx="16" cy="11" r="2" fill="white" opacity="0.85" />
          {/* Bottom node (database) */}
          <circle cx="10" cy="17.5" r="2" fill="white" opacity="0.7" />
          {/* Connections */}
          <line x1="10" y1="4.5" x2="4.8" y2="9.3" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
          <line x1="10" y1="4.5" x2="15.2" y2="9.3" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
          <line x1="4" y1="13" x2="9.2" y2="15.8" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
          <line x1="16" y1="13" x2="10.8" y2="15.8" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
        </svg>
      </div>
    ),
    { ...size },
  )
}

