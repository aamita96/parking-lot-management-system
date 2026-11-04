import { VehicleShape } from './VehicleArt'

// Original illustration: a small top-down car park with a P sign. Pure SVG, free to use.
export function HeroIllustration({ className }: { className?: string }) {
  const bays = [0, 1, 2, 3, 4]
  const parkedTop: [number, 'CAR' | 'MOTORCYCLE' | 'TRUCK', number][] = [
    [0, 'CAR', 215],
    [2, 'MOTORCYCLE', 28],
    [3, 'CAR', 350],
  ]
  const parkedBottom: [number, 'CAR' | 'MOTORCYCLE' | 'TRUCK', number][] = [
    [1, 'CAR', 150],
    [4, 'TRUCK', 40],
  ]
  return (
    <svg viewBox="0 0 420 300" className={className} role="img" aria-label="Illustration of a car park seen from above">
      <defs>
        <linearGradient id="hero-sky" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor="#dbeafe" />
          <stop offset="1" stopColor="#eff6ff" />
        </linearGradient>
      </defs>
      <rect width="420" height="300" rx="28" fill="url(#hero-sky)" />
      {/* grass and trees */}
      <rect x="18" y="18" width="384" height="264" rx="20" fill="#86c98a" />
      {[
        [40, 40],
        [380, 44],
        [36, 262],
        [386, 258],
      ].map(([x, y], i) => (
        <g key={i}>
          <circle cx={x} cy={y} r="15" fill="#4e9f55" />
          <circle cx={x - 4} cy={y - 4} r="7" fill="#6fbf73" />
        </g>
      ))}
      {/* asphalt */}
      <rect x="60" y="56" width="300" height="190" rx="12" fill="#3f4651" />
      {/* bays */}
      {bays.map((i) => (
        <g key={i}>
          <rect x={72 + i * 56} y="62" width="52" height="64" fill="none" stroke="#f8fafc" strokeWidth="2" />
          <rect x={72 + i * 56} y="176" width="52" height="64" fill="none" stroke="#f8fafc" strokeWidth="2" />
        </g>
      ))}
      {/* lane arrows */}
      {[110, 200, 290].map((x) => (
        <path key={x} d={`M${x} 151 h22 m-7 -6 l7 6 l-7 6`} stroke="#fbbf24" strokeWidth="3" fill="none" strokeLinecap="round" />
      ))}
      {parkedTop.map(([i, t, h]) => (
        <g key={`t${i}`} transform={`translate(${98 + i * 56} 94) rotate(-90) scale(0.95)`}>
          <VehicleShape type={t} hue={h} />
        </g>
      ))}
      {parkedBottom.map(([i, t, h]) => (
        <g key={`b${i}`} transform={`translate(${98 + i * 56} 208) rotate(90) scale(${t === 'TRUCK' ? 0.72 : 0.95})`}>
          <VehicleShape type={t} hue={h} />
        </g>
      ))}
      {/* moving car in the lane */}
      <g transform="translate(160 151) scale(0.9)">
        <VehicleShape type="CAR" hue={2} />
      </g>
      {/* P sign */}
      <g transform="translate(330 14)">
        <rect x="-3" y="30" width="6" height="40" fill="#64748b" />
        <rect x="-26" y="-6" width="52" height="44" rx="10" fill="#1e40af" stroke="#fff" strokeWidth="3" />
        <text x="0" y="29" textAnchor="middle" fontSize="34" fontWeight="800" fill="#fff" fontFamily="system-ui, sans-serif">
          P
        </text>
      </g>
    </svg>
  )
}
