import { ImageResponse } from 'next/og'

export const size = { width: 1200, height: 630 }
export const contentType = 'image/png'

export default function OGImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: 1200,
          height: 630,
          background: '#faf8f6',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-start',
          justifyContent: 'flex-end',
          padding: '64px 72px',
          fontFamily: 'sans-serif',
        }}
      >
        {/* Logo mark */}
        <div
          style={{
            width: 64,
            height: 64,
            borderRadius: 16,
            background: '#9b4819',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            marginBottom: 32,
          }}
        >
          <svg width="38" height="38" viewBox="0 0 20 20" fill="none">
            <circle cx="10" cy="2.5" r="2" fill="white" />
            <circle cx="4" cy="11" r="2" fill="white" opacity="0.85" />
            <circle cx="16" cy="11" r="2" fill="white" opacity="0.85" />
            <circle cx="10" cy="17.5" r="2" fill="white" opacity="0.7" />
            <line x1="10" y1="4.5" x2="4.8" y2="9.3" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
            <line x1="10" y1="4.5" x2="15.2" y2="9.3" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
            <line x1="4" y1="13" x2="9.2" y2="15.8" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
            <line x1="16" y1="13" x2="10.8" y2="15.8" stroke="white" strokeWidth="1.2" strokeOpacity="0.6" />
          </svg>
        </div>

        {/* Title */}
        <div
          style={{
            fontSize: 64,
            fontWeight: 700,
            color: '#1a1210',
            letterSpacing: '-0.03em',
            lineHeight: 1.1,
            marginBottom: 20,
          }}
        >
          SystemArch
        </div>

        {/* Subtitle */}
        <div
          style={{
            fontSize: 26,
            fontWeight: 400,
            color: '#6b5a52',
            letterSpacing: '-0.01em',
            lineHeight: 1.4,
          }}
        >
          Learn system design through interactive concepts
        </div>

        {/* Decorative node graph - top right */}
        <svg
          style={{ position: 'absolute', top: 60, right: 72, opacity: 0.07 }}
          width="420"
          height="420"
          viewBox="0 0 420 420"
          fill="none"
        >
          <circle cx="210" cy="60" r="28" fill="#9b4819" />
          <circle cx="90" cy="200" r="22" fill="#9b4819" />
          <circle cx="210" cy="200" r="22" fill="#9b4819" />
          <circle cx="330" cy="200" r="22" fill="#9b4819" />
          <circle cx="150" cy="340" r="18" fill="#9b4819" />
          <circle cx="270" cy="340" r="18" fill="#9b4819" />
          <line x1="210" y1="88" x2="100" y2="180" stroke="#9b4819" strokeWidth="6" />
          <line x1="210" y1="88" x2="210" y2="178" stroke="#9b4819" strokeWidth="6" />
          <line x1="210" y1="88" x2="320" y2="180" stroke="#9b4819" strokeWidth="6" />
          <line x1="90" y1="222" x2="145" y2="323" stroke="#9b4819" strokeWidth="5" />
          <line x1="210" y1="222" x2="158" y2="323" stroke="#9b4819" strokeWidth="5" />
          <line x1="210" y1="222" x2="265" y2="323" stroke="#9b4819" strokeWidth="5" />
          <line x1="330" y1="222" x2="275" y2="323" stroke="#9b4819" strokeWidth="5" />
        </svg>
      </div>
    ),
    { ...size },
  )
}

