// 4A 飞轮进度指示器：Assess → Analyze → Action → Audit
import type { Stage } from '../data/mock'

const STAGES: { key: Stage; label: string; en: string }[] = [
  { key: 'assess', label: '评片', en: 'Assess' },
  { key: 'analyze', label: '识相', en: 'Analyze' },
  { key: 'action', label: '逆推', en: 'Action' },
  { key: 'audit', label: '验证', en: 'Audit' },
]

export default function Flywheel({ current, size = 148 }: { current: Stage; size?: number }) {
  const activeIdx = STAGES.findIndex((s) => s.key === current)
  const r = 42
  const c = 2 * Math.PI * r
  const progress = (activeIdx + 0.5) / STAGES.length

  return (
    <div className="flex flex-col items-center gap-2">
      <svg width={size} height={size} viewBox="0 0 120 120" className="block">
        {/* 底环 */}
        <circle cx="60" cy="60" r={r} fill="none" stroke="hsl(var(--graphite-3))" strokeWidth="5" />
        {/* 进度弧 */}
        <circle
          cx="60" cy="60" r={r} fill="none"
          stroke="hsl(var(--quench))" strokeWidth="5" strokeLinecap="round"
          strokeDasharray={`${c * progress} ${c}`}
          transform="rotate(-90 60 60)"
          style={{ transition: 'stroke-dasharray 600ms cubic-bezier(0.76,0,0.24,1)' }}
        />
        {/* 慢速旋转刻度环 */}
        <g className="flywheel-spin">
          {Array.from({ length: 24 }).map((_, i) => {
            const a = (i / 24) * Math.PI * 2
            const x1 = 60 + Math.cos(a) * 52, y1 = 60 + Math.sin(a) * 52
            const x2 = 60 + Math.cos(a) * 55, y2 = 60 + Math.sin(a) * 55
            return <line key={i} x1={x1} y1={y1} x2={x2} y2={y2} stroke="hsl(var(--ink-low))" strokeWidth="1" opacity="0.6" />
          })}
        </g>
        <text x="60" y="56" textAnchor="middle" fill="hsl(var(--ink-hi))" fontSize="17" fontWeight="700">4A</text>
        <text x="60" y="74" textAnchor="middle" fill="hsl(var(--ink-low))" fontSize="8.5" letterSpacing="2">FLYWHEEL</text>
      </svg>
      <div className="flex items-center gap-1">
        {STAGES.map((s, i) => (
          <div key={s.key} className="flex items-center gap-1">
            <div
              className="flex flex-col items-center rounded px-1.5 py-1 transition-colors"
              style={{
                backgroundColor: i === activeIdx ? 'hsl(var(--quench) / 0.14)' : 'transparent',
              }}
            >
              <span
                className="text-[12px] font-semibold leading-none"
                style={{ color: i === activeIdx ? 'hsl(var(--quench))' : i < activeIdx ? 'hsl(var(--bronze))' : 'hsl(var(--ink-low))' }}
              >
                {s.label}
              </span>
              <span className="micro-label mt-1" style={{ fontSize: 8 }}>{s.en}</span>
            </div>
            {i < STAGES.length - 1 && (
              <span className="text-[10px]" style={{ color: i < activeIdx ? 'hsl(var(--bronze))' : 'hsl(var(--graphite-3))' }}>→</span>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
