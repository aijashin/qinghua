import { useMemo, useState } from 'react'
import { ACTION_COLOR, AUDIT_LOGS, BALANCE, type AuditLog } from '../../data/admin'

function Scatter() {
  const W = 560, H = 300, PAD = 44
  const px = (will: number) => PAD + will * (W - PAD * 2)
  const py = (p: number) => H - PAD - (p / 8) * (H - PAD * 2)

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="block w-full">
      {/* 高意愿低练习象限高亮 */}
      <rect x={px(0.75)} y={py(8)} width={W - PAD - px(0.75)} height={py(3) - py(8)} fill="hsl(var(--rust))" opacity="0.07" />
      <text x={W - PAD - 4} y={py(7.85)} textAnchor="end" fill="hsl(var(--rust))" fontSize="10" fontFamily="JetBrains Mono, monospace">高意愿 · 低练习区</text>

      {/* 参考线 */}
      <line x1={px(0.75)} y1={PAD} x2={px(0.75)} y2={H - PAD} stroke="hsl(var(--graphite-3))" strokeDasharray="4 4" />
      <line x1={PAD} y1={py(3)} x2={W - PAD} y2={py(3)} stroke="hsl(var(--graphite-3))" strokeDasharray="4 4" />
      <text x={W - PAD} y={py(3) - 4} textAnchor="end" fill="hsl(var(--ink-low))" fontSize="9" fontFamily="JetBrains Mono, monospace">人均 3 次</text>

      {/* 轴 */}
      <line x1={PAD} y1={H - PAD} x2={W - PAD} y2={H - PAD} stroke="hsl(var(--metal-line))" />
      <line x1={PAD} y1={PAD} x2={PAD} y2={H - PAD} stroke="hsl(var(--metal-line))" />
      {[0.4, 0.6, 0.8].map((w) => (
        <text key={w} x={px(w)} y={H - PAD + 14} textAnchor="middle" fill="hsl(var(--ink-low))" fontSize="9" fontFamily="JetBrains Mono, monospace">{w.toFixed(1)}</text>
      ))}
      {[2, 4, 6, 8].map((p) => (
        <text key={p} x={PAD - 8} y={py(p) + 3} textAnchor="end" fill="hsl(var(--ink-low))" fontSize="9" fontFamily="JetBrains Mono, monospace">{p}</text>
      ))}
      <text x={W - PAD} y={H - PAD + 26} textAnchor="end" fill="hsl(var(--ink-mid))" fontSize="10">意愿指数 →</text>
      <text x={PAD - 30} y={PAD - 8} fill="hsl(var(--ink-mid))" fontSize="10">↑ 人均练习</text>

      {/* 班级点 */}
      {BALANCE.map((b) => (
        <g key={b.className}>
          <circle
            cx={px(b.will)} cy={py(b.practice)} r={b.gap ? 8 : 6}
            fill={b.gap ? 'hsl(var(--rust) / 0.25)' : 'hsl(var(--quench) / 0.2)'}
            stroke={b.gap ? 'hsl(var(--rust))' : 'hsl(var(--quench))'} strokeWidth="1.5"
          />
          <text x={px(b.will)} y={py(b.practice) - 12} textAnchor="middle" fill={b.gap ? 'hsl(var(--rust))' : 'hsl(var(--ink-mid))'} fontSize="10.5">
            {b.className}
          </text>
        </g>
      ))}
    </svg>
  )
}

const ACTION_FILTERS: (AuditLog['action'] | '全部')[] = ['全部', '改分', '导出', '细则发布', '标定', '账号']

export default function AdminBalanceAudit() {
  const [filter, setFilter] = useState<(typeof ACTION_FILTERS)[number]>('全部')
  const logs = useMemo(() => AUDIT_LOGS.filter((l) => filter === '全部' || l.action === filter), [filter])
  const gaps = BALANCE.filter((b) => b.gap)

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="border-b border-[hsl(var(--metal-line))] pb-4">
        <div className="micro-label">Balance & Audit / 资源均衡 · 审计日志</div>
        <div className="mt-1 text-[15px] font-semibold">识别「高意愿低练习」班级 · 全操作留痕追溯</div>
      </div>

      {/* 资源均衡散点 */}
      <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
        <div className="micro-label mb-2">Resource Balance / 班级意愿-练习分布</div>
        <Scatter />
        <div className="mt-3 flex flex-col gap-2">
          {gaps.map((g) => (
            <div key={g.className} className="flex flex-wrap items-center gap-3 rounded border border-[hsl(var(--rust)/0.4)] bg-[hsl(var(--rust)/0.06)] px-3 py-2">
              <span className="text-[12.5px] font-medium text-[hsl(var(--rust))]">{g.className}</span>
              <span className="text-[11.5px] text-[hsl(var(--ink-mid))]">
                意愿 {Math.round(g.will * 100)}% 但人均仅 {g.practice} 次（教师 {g.teacher}）
              </span>
              <button className="ml-auto rounded border border-[hsl(var(--quench))] px-2.5 py-1 text-[11px] text-[hsl(var(--quench))] transition-transform active:scale-[0.98]">
                投放题库与训练资源包
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* 审计日志 */}
      <div>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <span className="micro-label mr-2">Audit Log / 审计日志（只读 · 不可篡改）</span>
          {ACTION_FILTERS.map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className="rounded border px-2.5 py-1 text-[11.5px] transition-colors"
              style={{
                borderColor: filter === f ? 'hsl(var(--quench))' : 'hsl(var(--metal-line))',
                color: filter === f ? 'hsl(var(--quench))' : 'hsl(var(--ink-low))',
                backgroundColor: filter === f ? 'hsl(var(--quench) / 0.08)' : 'transparent',
              }}
            >
              {f}
            </button>
          ))}
        </div>
        <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
          {logs.map((l, i) => (
            <div key={i} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2.5">
              <span className="font-mono-tech w-24 text-[11px] text-[hsl(var(--ink-low))]">{l.time}</span>
              <span className="font-mono-tech w-20 text-[11px] text-[hsl(var(--ink-mid))]">{l.actor}</span>
              <span
                className="rounded border px-1.5 py-0.5 text-[10.5px]"
                style={{ borderColor: `${ACTION_COLOR[l.action]}`, color: ACTION_COLOR[l.action] }}
              >
                {l.action}
              </span>
              <span className="flex-1 text-[12px] text-[hsl(var(--ink-hi))]">{l.target}</span>
              <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">#{l.hash}</span>
            </div>
          ))}
        </div>
        <p className="mt-2 text-[10.5px] text-[hsl(var(--ink-low))]">
          每条记录附哈希指纹，支持链式追溯；改分 / 导出 / 细则发布三类高危操作全量留痕。
        </p>
      </div>
    </div>
  )
}
