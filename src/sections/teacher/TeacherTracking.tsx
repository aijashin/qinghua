import { useState } from 'react'
import { RING_LABEL, STUDENT_TRACKS, type RingKey } from '../../data/teacher'

const RINGS: RingKey[] = ['assess', 'analyze', 'action', 'audit']
const STUCK_COLOR: Record<RingKey, string> = {
  assess: 'hsl(var(--quench))',
  analyze: '#d59a3d',
  action: 'hsl(var(--rust))',
  audit: 'hsl(var(--bronze))',
}

export default function TeacherTracking() {
  const [filter, setFilter] = useState<RingKey | 'all'>('all')
  const list = STUDENT_TRACKS.filter((s) => filter === 'all' || s.stuck === filter)

  const stuckCount = (r: RingKey) => STUDENT_TRACKS.filter((s) => s.stuck === r).length

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="border-b border-[hsl(var(--metal-line))] pb-4">
        <div className="micro-label">Tracking / 个体追踪</div>
        <div className="mt-1 text-[15px] font-semibold">每位学生卡在「评 / 识 / 逆 / 验」哪一环</div>
      </div>

      {/* 环节过滤器 */}
      <div className="flex flex-wrap gap-2">
        <button
          onClick={() => setFilter('all')}
          className="rounded border px-3 py-1.5 text-[12px] transition-colors"
          style={{
            borderColor: filter === 'all' ? 'hsl(var(--quench))' : 'hsl(var(--metal-line))',
            color: filter === 'all' ? 'hsl(var(--quench))' : 'hsl(var(--ink-mid))',
            backgroundColor: filter === 'all' ? 'hsl(var(--quench) / 0.08)' : 'transparent',
          }}
        >
          全部 · {STUDENT_TRACKS.length}
        </button>
        {RINGS.map((r) => (
          <button
            key={r}
            onClick={() => setFilter(r)}
            className="rounded border px-3 py-1.5 text-[12px] transition-colors"
            style={{
              borderColor: filter === r ? STUCK_COLOR[r] : 'hsl(var(--metal-line))',
              color: filter === r ? STUCK_COLOR[r] : 'hsl(var(--ink-mid))',
              backgroundColor: filter === r ? `${STUCK_COLOR[r]}14` : 'transparent',
            }}
          >
            卡在{RING_LABEL[r]} · {stuckCount(r)}
          </button>
        ))}
      </div>

      {/* 学生列表 */}
      <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
        {list.map((s) => {
          const stuckIdx = RINGS.indexOf(s.stuck)
          return (
            <div key={s.name} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
              <span className="w-12 text-[13px] font-medium text-[hsl(var(--ink-hi))]">{s.name}</span>
              <span className="font-mono-tech w-10 text-[13px] text-[hsl(var(--quench))]">{s.score}</span>

              {/* 四环状态条 */}
              <div className="flex items-center gap-1">
                {RINGS.map((r, i) => {
                  const state = i < stuckIdx ? 'done' : i === stuckIdx ? 'stuck' : 'todo'
                  return (
                    <div key={r} className="flex items-center gap-1">
                      <div
                        className="flex h-6 items-center rounded px-1.5 font-mono-tech text-[9.5px] tracking-wide"
                        style={{
                          backgroundColor:
                            state === 'done' ? 'hsl(var(--bronze) / 0.15)' :
                            state === 'stuck' ? `${STUCK_COLOR[r]}22` : 'hsl(var(--graphite-2))',
                          color:
                            state === 'done' ? 'hsl(var(--bronze))' :
                            state === 'stuck' ? STUCK_COLOR[r] : 'hsl(var(--ink-low))',
                          border: state === 'stuck' ? `1px solid ${STUCK_COLOR[r]}` : '1px solid transparent',
                        }}
                      >
                        {state === 'done' ? '✓ ' : state === 'stuck' ? '◆ ' : ''}{RING_LABEL[r]}
                      </div>
                      {i < RINGS.length - 1 && <span className="text-[9px] text-[hsl(var(--graphite-3))]">→</span>}
                    </div>
                  )
                })}
              </div>

              <span className="font-mono-tech text-[11px] text-[hsl(var(--ink-low))]">{s.practices} 练</span>
              <span className="flex-1 text-right text-[11px] text-[hsl(var(--ink-low))]">
                高频缺陷 <span className="text-[hsl(var(--ink-mid))]">{s.defectTop}</span> · 活跃 {s.lastActive}
              </span>
            </div>
          )
        })}
      </div>

      <p className="border-l-2 border-[hsl(var(--quench))] pl-3 text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]">
        卡在「逆推」的 2 人与 2 周未解锁名单完全重合；魏澜、周彤近一周无新练习且卡在「评片」——建议优先约谈这两位，基础动作问题比思维问题更影响后续环节。
      </p>
    </div>
  )
}
