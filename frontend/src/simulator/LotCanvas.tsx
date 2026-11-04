import { memo, useState } from 'react'
import type { LayoutSpot } from '@/api/types'
import { VehicleShape } from '@/components/brand/VehicleArt'
import { cn } from '@/lib/utils'
import { type Geometry, SIDE_LANE_W, SPOT_D, SPOT_W, type SpotGeo } from './geometry'
import { useSim } from './store'
import type { SimVehicle } from './types'

const letterColor: Record<string, string> = { C: '#f59e0b', R: '#3b82f6', L: '#8b5cf6' }

// Drawn sizes in the lot: motorcycle small, car medium, truck long (SRS 11.1), all fitting a bay.
const SPRITE_SCALE: Record<string, number> = { MOTORCYCLE: 1.6, CAR: 1.3, TRUCK: 1.15 }

export function LotCanvas() {
  const geo = useSim((s) => s.geometry)
  const layout = useSim((s) => s.layout)
  const floor = useSim((s) => s.floor)
  if (!geo || !layout) return null
  const freeCount = layout.freeCount
  const floors = layout.floors
  return (
    <svg
      viewBox={`0 0 ${geo.width} ${geo.height}`}
      className="block h-auto w-full select-none"
      role="img"
      aria-label={`Top-down view of the car park, floor ${floor}. ${freeCount} spots open.`}
    >
      <defs>
        <pattern id="hatch" width="10" height="10" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
          <rect width="10" height="10" fill="#4b5563" />
          <line x1="0" y1="0" x2="0" y2="10" stroke="#fbbf24" strokeWidth="4" opacity="0.55" />
        </pattern>
        <pattern id="grass" width="24" height="24" patternUnits="userSpaceOnUse">
          <rect width="24" height="24" fill="#74c27a" />
          <circle cx="5" cy="7" r="1.2" fill="#5fae66" />
          <circle cx="17" cy="15" r="1.2" fill="#5fae66" />
          <circle cx="11" cy="21" r="1" fill="#8fd394" />
        </pattern>
      </defs>
      <StaticLot geo={geo} spots={layout.rows.filter((r) => r.floor === floor).flatMap((r) => r.spots)} />
      <PBoard free={freeCount} />
      {floors.length > 1 && (
        <g transform={`translate(${geo.rightX} ${geo.lotTop - 70})`}>
          <rect x={-46} y={-18} width={92} height={30} rx={8} fill="#0f172a" opacity={0.75} />
          <text y={2} textAnchor="middle" fill="#fff" fontSize={13} fontWeight={700}>
            Ramp · floor {floor}
          </text>
        </g>
      )}
      <Gates geo={geo} />
      <Vehicles />
    </svg>
  )
}

// The lot itself only changes with the layout, so memo() skips redrawing it on every animation frame.
const StaticLot = memo(function StaticLot({ geo, spots }: { geo: Geometry; spots: LayoutSpot[] }) {
  const lotX = geo.leftX - SIDE_LANE_W / 2
  const lotW = geo.rightX + SIDE_LANE_W / 2 - lotX
  const bays = [...geo.spots.values()]
  const statusOf = new Map(spots.map((s) => [s.code, s]))
  return (
    <g>
      <rect width={geo.width} height={geo.height} fill="url(#grass)" />
      {/* landscaping (decoration only) */}
      <Trees geo={geo} />
      {/* asphalt: lot, IN road (bottom left) and OUT road (bottom right) */}
      <rect x={lotX} y={geo.lotTop - 46} width={lotW} height={geo.lotBottom - geo.lotTop + 46} rx={18} fill="#3f4651" />
      <rect x={lotX} y={geo.lotBottom - 20} width={SIDE_LANE_W} height={geo.height - geo.lotBottom + 20} fill="#3f4651" />
      <rect x={geo.rightX - SIDE_LANE_W / 2} y={geo.lotBottom - 20} width={SIDE_LANE_W} height={geo.height - geo.lotBottom + 20} fill="#3f4651" />
      {/* grass islands between bands */}
      {geo.bands.slice(1).map((b) => (
        <rect key={b.index} x={geo.spotsX0 + 4} y={b.y - 48} width={geo.spotsX1 - geo.spotsX0 - 8} height={40} rx={12} fill="url(#grass)" />
      ))}
      {/* lane markings */}
      {geo.bands.map((b) => (
        <g key={b.index}>
          <line x1={geo.spotsX0} x2={geo.spotsX1} y1={b.laneY} y2={b.laneY} stroke="#cbd5e1" strokeWidth={2} strokeDasharray="14 12" opacity={0.5} />
          {[0.22, 0.5, 0.78].map((f) => (
            <Arrow key={f} x={geo.spotsX0 + (geo.spotsX1 - geo.spotsX0) * f} y={b.laneY} angle={0} />
          ))}
        </g>
      ))}
      {Array.from({ length: Math.max(2, geo.bands.length + 1) }, (_, i) => {
        const y = geo.lotBottom - 60 - i * ((geo.lotBottom - geo.lotTop) / Math.max(2, geo.bands.length + 1))
        return (
          <g key={i}>
            <Arrow x={geo.leftX} y={y} angle={-90} />
            <Arrow x={geo.rightX} y={y} angle={90} />
          </g>
        )
      })}
      {/* bays */}
      {bays.map((b) => (
        <Bay key={b.spot.code} g={b} spot={statusOf.get(b.spot.code) ?? b.spot} />
      ))}
      {/* road labels */}
      <text x={geo.leftX} y={geo.height - 14} textAnchor="middle" fill="#f8fafc" fontSize={15} fontWeight={800} letterSpacing={2}>
        IN
      </text>
      <text x={geo.rightX} y={geo.height - 14} textAnchor="middle" fill="#f8fafc" fontSize={15} fontWeight={800} letterSpacing={2}>
        OUT
      </text>
    </g>
  )
})

function Bay({ g, spot }: { g: SpotGeo; spot: LayoutSpot }) {
  const oos = spot.status === 'OUT_OF_SERVICE'
  const occupied = spot.status === 'OCCUPIED'
  const labelY = g.top ? g.y + 16 : g.y + SPOT_D - 10
  return (
    <g>
      <rect x={g.x} y={g.y} width={SPOT_W} height={SPOT_D} fill={oos ? 'url(#hatch)' : occupied ? '#394049' : '#454c57'} />
      {/* bay lines: two sides plus the back */}
      <path
        d={
          g.top
            ? `M${g.x} ${g.y + SPOT_D} V${g.y} H${g.x + SPOT_W} V${g.y + SPOT_D}`
            : `M${g.x} ${g.y} V${g.y + SPOT_D} H${g.x + SPOT_W} V${g.y}`
        }
        fill="none"
        stroke="#f8fafc"
        strokeWidth={2.5}
      />
      <text x={g.x + 7} y={labelY} fill="#e2e8f0" fontSize={12} fontWeight={700} fontFamily="ui-monospace, monospace">
        {spot.code}
      </text>
      <g transform={`translate(${g.x + SPOT_W - 15} ${labelY - 4})`}>
        <rect x={-8} y={-8} width={16} height={16} rx={4} fill={letterColor[spot.letter] ?? '#64748b'} />
        <text y={4.5} textAnchor="middle" fill="#fff" fontSize={11} fontWeight={800}>
          {spot.letter}
        </text>
      </g>
      {oos && (
        <text x={g.cx} y={g.y + SPOT_D / 2 + 4} textAnchor="middle" fill="#fde68a" fontSize={10} fontWeight={700}>
          CLOSED
        </text>
      )}
    </g>
  )
}

function Arrow({ x, y, angle }: { x: number; y: number; angle: number }) {
  return (
    <g transform={`translate(${x} ${y}) rotate(${angle})`} opacity={0.85}>
      <path d="M-12 0 H8 M2 -6 L9 0 L2 6" stroke="#fbbf24" strokeWidth={3} fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </g>
  )
}

function Trees({ geo }: { geo: Geometry }) {
  const pts: [number, number][] = [
    [28, 60],
    [geo.width - 30, 60],
    [28, geo.lotBottom - 40],
    [geo.width - 30, geo.lotBottom - 60],
    [geo.width / 2 - 40, geo.height - 40],
    [geo.width / 2 + 60, geo.height - 60],
    [geo.width / 2 + 10, 34],
  ]
  return (
    <g aria-hidden>
      <rect x={geo.leftX + SIDE_LANE_W / 2 + 30} y={geo.lotBottom + 26} width={geo.rightX - geo.leftX - SIDE_LANE_W - 60} height={geo.height - geo.lotBottom - 40} rx={18} fill="#5fae66" opacity={0.6} />
      {pts.map(([x, y], i) => (
        <g key={i}>
          <circle cx={x + 2} cy={y + 3} r={17} fill="rgb(15 23 42 / 0.15)" />
          <circle cx={x} cy={y} r={17} fill="#3f8f47" />
          <circle cx={x - 5} cy={y - 5} r={8} fill="#5fae66" />
        </g>
      ))}
    </g>
  )
}

function PBoard({ free }: { free: number }) {
  const full = free === 0
  return (
    <g transform="translate(16 14)" role="status" aria-label={full ? 'Car park full' : `${free} spots open`}>
      <rect width={168} height={64} rx={12} fill="#0f172a" stroke="#334155" strokeWidth={2} />
      <rect x={8} y={8} width={48} height={48} rx={9} fill={full ? '#dc2626' : '#1e40af'} />
      <text x={32} y={45} textAnchor="middle" fill="#fff" fontSize={36} fontWeight={900}>
        P
      </text>
      <text x={110} y={34} textAnchor="middle" fill={full ? '#f87171' : '#4ade80'} fontSize={full ? 24 : 28} fontWeight={900} fontFamily="ui-monospace, monospace">
        {full ? 'FULL' : free}
      </text>
      <text x={110} y={52} textAnchor="middle" fill="#94a3b8" fontSize={11} fontWeight={700} letterSpacing={2}>
        {full ? 'NO SPACES' : 'OPEN'}
      </text>
    </g>
  )
}

function Gates({ geo }: { geo: Geometry }) {
  // The barrier is up while 'open until' is still in the future.
  const now = useSim((s) => s.now)
  const inOpen = useSim((s) => s.inBarrierOpenUntil) > now
  const outOpen = useSim((s) => s.outBarrierOpenUntil) > now
  const half = SIDE_LANE_W / 2
  return (
    <g>
      {/* IN: ticket kiosk + barrier */}
      <g transform={`translate(${geo.leftX - half - 30} ${geo.gateStopY - 18})`}>
        <rect width={26} height={40} rx={5} fill="#f8fafc" stroke="#1e40af" strokeWidth={2} />
        <rect x={5} y={6} width={16} height={10} rx={2} fill="#1e40af" />
        <rect x={7} y={22} width={12} height={3} rx={1} fill="#94a3b8" />
        <text x={13} y={56} textAnchor="middle" fill="#0f172a" fontSize={10} fontWeight={800}>
          TICKET
        </text>
      </g>
      <Barrier x={geo.leftX - half} y={geo.barrierInY} length={SIDE_LANE_W} open={inOpen} />
      {/* PAY kiosk + OUT barrier */}
      <g transform={`translate(${geo.rightX + half + 4} ${geo.payStopY - 22})`}>
        <rect width={30} height={44} rx={5} fill="#f8fafc" stroke="#059669" strokeWidth={2} />
        <rect x={5} y={6} width={20} height={12} rx={2} fill="#059669" />
        <circle cx={15} cy={30} r={5} fill="#a7f3d0" stroke="#059669" />
        <text x={15} y={60} textAnchor="middle" fill="#0f172a" fontSize={10} fontWeight={800}>
          PAY
        </text>
      </g>
      <Barrier x={geo.rightX + half} y={geo.barrierOutY} length={SIDE_LANE_W} open={outOpen} flip />
    </g>
  )
}

function Barrier({ x, y, length, open, flip }: { x: number; y: number; length: number; open: boolean; flip?: boolean }) {
  const angle = open ? (flip ? 80 : -80) : 0
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect x={flip ? -2 : -8} y={-8} width={10} height={16} rx={2} fill="#1f2937" />
      <g style={{ transform: `rotate(${angle}deg)`, transition: 'transform 450ms ease', transformBox: 'view-box' }}>
        <g transform={flip ? 'scale(-1 1)' : undefined}>
          <rect x={0} y={-3} width={length - 4} height={6} rx={3} fill="#f8fafc" stroke="#991b1b" strokeWidth={1} />
          {[0.15, 0.4, 0.65].map((f) => (
            <rect key={f} x={(length - 4) * f} y={-3} width={(length - 4) * 0.12} height={6} fill="#dc2626" />
          ))}
        </g>
      </g>
    </g>
  )
}

function Vehicles() {
  const vehicles = useSim((s) => s.vehicles)
  const selected = useSim((s) => s.selectedVehicleId)
  const select = useSim((s) => s.selectVehicle)
  const [hover, setHover] = useState<string | null>(null)
  return (
    <g>
      {vehicles.map((v) => (
        <VehicleSprite
          key={v.id}
          v={v}
          selected={selected === v.id}
          hovered={hover === v.id}
          onHover={setHover}
          onClick={() => v.state === 'PARKED' && select(selected === v.id ? null : v.id)}
        />
      ))}
    </g>
  )
}

function VehicleSprite({
  v,
  selected,
  hovered,
  onHover,
  onClick,
}: {
  v: SimVehicle
  selected: boolean
  hovered: boolean
  onHover: (id: string | null) => void
  onClick: () => void
}) {
  const deg = (v.heading * 180) / Math.PI
  const clickable = v.state === 'PARKED'
  return (
    <g
      transform={`translate(${v.x} ${v.y})`}
      className={cn(clickable && 'cursor-pointer')}
      onPointerEnter={() => onHover(v.id)}
      onPointerLeave={() => onHover(null)}
      onClick={onClick}
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      aria-label={clickable ? `${v.plate}, parked. Show ticket` : undefined}
      onKeyDown={(e) => {
        if (clickable && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault()
          onClick()
        }
      }}
    >
      {selected && <circle r={34} fill="none" stroke="#fbbf24" strokeWidth={3} strokeDasharray="6 4" />}
      <g transform={`rotate(${deg}) scale(${SPRITE_SCALE[v.vehicleType]})`}>
        <VehicleShape type={v.vehicleType} hue={v.hue} ghost={v.state === 'TURNING_BACK'} />
      </g>
      {(hovered || selected || v.state === 'AT_ENTRY' || v.state === 'AT_PAY') && (
        <g transform="translate(0 -34)" pointerEvents="none">
          <rect x={-44} y={-12} width={88} height={20} rx={6} fill="#0f172a" opacity={0.88} />
          <text y={3} textAnchor="middle" fill="#fff" fontSize={11} fontWeight={700} fontFamily="ui-monospace, monospace">
            {v.plate}
          </text>
        </g>
      )}
      <title>{v.plate}</title>
    </g>
  )
}
