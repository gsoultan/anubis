/* Dependency-free sparkline. A charting library for one 60-point series would
   cost more than the whole feature is worth, and this keeps the palette under
   our control so it cannot stray into verdict colours. */
export function Sparkline({
  data, w = 88, h = 26, color = 'var(--gold)', fill = true,
}: { data: number[]; w?: number; h?: number; color?: string; fill?: boolean }) {
  if (data.length < 2) return null
  const min = Math.min(...data)
  const max = Math.max(...data)
  const span = max - min || 1
  const step = w / (data.length - 1)
  const pt = (v: number, i: number) =>
    [i * step, h - 2 - ((v - min) / span) * (h - 4)] as const
  const line = data.map((v, i) => pt(v, i).join(',')).join(' ')
  const area = `0,${h} ${line} ${w},${h}`
  const [lx, ly] = pt(data[data.length - 1]!, data.length - 1)

  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} aria-hidden
      style={{ display: 'block', overflow: 'visible' }}>
      {fill && (
        <>
          <defs>
            <linearGradient id={`sg-${color.replace(/\W/g, '')}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity="0.22" />
              <stop offset="100%" stopColor={color} stopOpacity="0" />
            </linearGradient>
          </defs>
          <polygon points={area} fill={`url(#sg-${color.replace(/\W/g, '')})`} />
        </>
      )}
      <polyline points={line} fill="none" stroke={color} strokeWidth="1.5"
        strokeLinecap="round" strokeLinejoin="round" />
      <circle cx={lx} cy={ly} r="2.2" fill={color} />
    </svg>
  )
}
