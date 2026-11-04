import type { VehicleType } from '@/api/types'

// Original top-down vehicle artwork (no third-party assets). Drawn facing +x, centred on 0,0,
// so the simulator can rotate it by its heading. Sizes are in simulator units.
export const VEHICLE_SIZE: Record<VehicleType, { length: number; width: number }> = {
  MOTORCYCLE: { length: 30, width: 12 },
  CAR: { length: 50, width: 24 },
  TRUCK: { length: 78, width: 28 },
}

export function VehicleShape({ type, hue, ghost }: { type: VehicleType; hue: number; ghost?: boolean }) {
  const body = `hsl(${hue} 70% 48%)`
  const dark = `hsl(${hue} 65% 32%)`
  const light = `hsl(${hue} 75% 64%)`
  const opacity = ghost ? 0.45 : 1

  if (type === 'MOTORCYCLE') {
    return (
      <g opacity={opacity}>
        <ellipse cx={0} cy={0} rx={15} ry={4} fill="#1f2937" />
        <rect x={-9} y={-4.5} width={16} height={9} rx={4} fill={body} />
        <line x1={9} y1={-6} x2={9} y2={6} stroke="#111827" strokeWidth={2} strokeLinecap="round" />
        <circle cx={-1} cy={0} r={4.2} fill={dark} stroke="#f8fafc" strokeWidth={1} />
        <circle cx={14} cy={0} r={1.4} fill="#fde68a" />
      </g>
    )
  }

  if (type === 'TRUCK') {
    return (
      <g opacity={opacity}>
        <rect x={-39} y={-14} width={78} height={28} rx={4} fill="rgb(15 23 42 / 0.25)" transform="translate(1.5 2)" />
        {/* cargo box */}
        <rect x={-39} y={-14} width={54} height={28} rx={3} fill="#e2e8f0" stroke="#94a3b8" strokeWidth={1} />
        <line x1={-27} y1={-14} x2={-27} y2={14} stroke="#cbd5e1" strokeWidth={1} />
        <line x1={-13} y1={-14} x2={-13} y2={14} stroke="#cbd5e1" strokeWidth={1} />
        <line x1={1} y1={-14} x2={1} y2={14} stroke="#cbd5e1" strokeWidth={1} />
        {/* cab */}
        <rect x={17} y={-13} width={22} height={26} rx={5} fill={body} />
        <rect x={29} y={-10.5} width={7} height={21} rx={2} fill="#1e293b" />
        <rect x={19} y={-11} width={8} height={22} rx={2} fill={light} />
        <rect x={37} y={-11} width={2} height={4} rx={1} fill="#fde68a" />
        <rect x={37} y={7} width={2} height={4} rx={1} fill="#fde68a" />
      </g>
    )
  }

  return (
    <g opacity={opacity}>
      <rect x={-25} y={-12} width={50} height={24} rx={7} fill="rgb(15 23 42 / 0.25)" transform="translate(1.5 2)" />
      <rect x={-25} y={-12} width={50} height={24} rx={7} fill={body} />
      {/* roof */}
      <rect x={-10} y={-9} width={20} height={18} rx={4} fill={light} />
      {/* windscreen and rear window */}
      <path d="M10 -9 L17 -10 Q19 0 17 10 L10 9 Z" fill="#1e293b" />
      <path d="M-10 -9 L-15 -9.5 Q-16.5 0 -15 9.5 L-10 9 Z" fill="#334155" />
      {/* mirrors and lights */}
      <rect x={9} y={-14} width={4} height={2.5} rx={1} fill={dark} />
      <rect x={9} y={11.5} width={4} height={2.5} rx={1} fill={dark} />
      <rect x={22} y={-10} width={2.5} height={5} rx={1} fill="#fef3c7" />
      <rect x={22} y={5} width={2.5} height={5} rx={1} fill="#fef3c7" />
      <rect x={-24.5} y={-10} width={2} height={4.5} rx={1} fill="#f87171" />
      <rect x={-24.5} y={5.5} width={2} height={4.5} rx={1} fill="#f87171" />
    </g>
  )
}

/** Small standalone vehicle picture, e.g. on the registration type buttons. */
export function VehicleIcon({ type, hue = 220, size = 64 }: { type: VehicleType; hue?: number; size?: number }) {
  const { length } = VEHICLE_SIZE[type]
  const s = 70 / Math.max(length, 50)
  return (
    <svg width={size} height={size * 0.5} viewBox="-40 -20 80 40" aria-hidden>
      <g transform={`scale(${s})`}>
        <VehicleShape type={type} hue={hue} />
      </g>
    </svg>
  )
}
