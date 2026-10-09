import { COHORT_BENCHMARK, DEFECT_BOOK, FLYWHEEL_STATS, PRACTICE_CURVE } from '../data/mock'

function Curve({ data }: { data: number[] }) {
  const W = 560, H = 170, PAD = 26
  const min = 50, max = 100
  const px = (i: number) => PAD + (i / (data.length - 1)) * (W - PAD * 2)
  const py = (v: number) => H - PAD - ((v - min) / (max - min)) * (H - PAD * 2)
  const path = data.map((v, i) => `${i === 0 ? 'M' : 'L'} ${px(i)} ${py(v)}`).join(' ')

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="block w-full">
      {[60, 75, 90].map((g) => (
        <g key={g}>
          <line x1={PAD} y1={py(g)} x2={W - PAD} y2={py(g)} stroke="hsl(var(--graphite-3))" strokeDasharray="3 4" />
          <text x={W - PAD + 2} y={py(g) + 3} fill="hsl(var(--ink-low))" fontSize="9" fontFamily="JetBrains Mono, monospace">{g}</text>
        </g>
      ))}
      {/* 校内教学示例目标线 */}
      <line x1={PAD} y1={py(90)} x2={W - PAD} y2={py(90)} stroke="hsl(var(--bronze))" strokeDasharray="6 4" strokeWidth="1.2" />
      <text x={PAD} y={py(90) - 5} fill="hsl(var(--bronze))" fontSize="9" fontFamily="JetBrains Mono, monospace">示例练习目标 90</text>
      <path d={path} fill="none" stroke="hsl(var(--quench))" strokeWidth="2" strokeLinejoin="round" />
      {data.map((v, i) => (
        <g key={i}>
          <circle cx={px(i)} cy={py(v)} r={i === data.length - 1 ? 4 : 3} fill="hsl(var(--graphite-0))" stroke="hsl(var(--quench))" strokeWidth="2" />
          <text x={px(i)} y={py(v) - 8} textAnchor="middle" fill="hsl(var(--ink-mid))" fontSize="9.5" fontFamily="JetBrains Mono, monospace">{v}</text>
          <text x={px(i)} y={H - 8} textAnchor="middle" fill="hsl(var(--ink-low))" fontSize="9" fontFamily="JetBrains Mono, monospace">#{i + 1}</text>
        </g>
      ))}
    </svg>
  )
}

export default function ProfileModule() {
  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="border-b border-[hsl(var(--metal-line))] pb-4">
        <div className="micro-label">Profile / 成长档案</div>
        <div className="mt-1 text-[15px] font-semibold">练习曲线 · 缺陷本 · 4A 飞轮 <span className="text-[11px] font-normal text-[hsl(var(--ink-low))]">（演示数据）</span></div>
      </div>

      {/* 统计行 */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {[
          { label: '评片练习', value: FLYWHEEL_STATS.assess, unit: '次' },
          { label: '识相分析', value: FLYWHEEL_STATS.analyze, unit: '次' },
          { label: '逆向论证', value: FLYWHEEL_STATS.action, unit: '次' },
          { label: '飞轮闭环', value: FLYWHEEL_STATS.completed, unit: '轮', accent: true },
        ].map((s) => (
          <div key={s.label} className="lift-card hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
            <div className="micro-label">{s.label}</div>
            <div className="mt-1.5 flex items-baseline gap-1">
              <span
                className="font-mono-tech text-[30px] font-bold leading-none"
                style={{ color: s.accent ? 'hsl(var(--bronze))' : 'hsl(var(--ink-hi))' }}
              >
                {s.value}
              </span>
              <span className="text-[11px] text-[hsl(var(--ink-low))]">{s.unit}</span>
            </div>
          </div>
        ))}
      </div>

      {/* 练习曲线 + 对标 */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_260px]">
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-2">Score Curve / 近 7 次练习总分</div>
          <Curve data={PRACTICE_CURVE} />
        </div>
        <div className="hairline flex flex-col justify-center rounded-md bg-[hsl(var(--graphite-1))] p-5">
          <div className="micro-label">Cohort / 同届匿名对标</div>
          <div className="mt-2 font-mono-tech text-[40px] font-bold leading-none text-[hsl(var(--quench))]">
            {COHORT_BENCHMARK.percentile}
            <span className="text-[16px] text-[hsl(var(--ink-low))]"> 分位</span>
          </div>
          <div className="mt-3 text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]">
            {COHORT_BENCHMARK.note}；同届均值 {COHORT_BENCHMARK.avgScore} 分，你最近三次均高于均值。
          </div>
          <div className="mt-3 border-t border-[hsl(var(--metal-line))] pt-3 text-[11px] text-[hsl(var(--ink-low))]">
            匿名聚合数据，不含他人明细（权限边界）。
          </div>
        </div>
      </div>

      {/* 缺陷本 */}
      <div>
        <div className="micro-label mb-3">Defect Book / 个人缺陷本 · 按累计次数排序</div>
        <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
          <div className="flex items-center gap-4 px-4 py-2.5">
            <span className="micro-label w-20">缺陷类型</span>
            <span className="micro-label w-16">累计</span>
            <span className="micro-label w-[180px]">近 7 次练习出现次数</span>
            <span className="micro-label w-16 text-right">复发率</span>
          </div>
          {DEFECT_BOOK.map((d) => {
            const peak = Math.max(...d.trend)
            return (
              <div key={d.type} className="flex items-center gap-4 px-4 py-3">
                <span className="w-20 text-[13px] text-[hsl(var(--ink-hi))]">{d.type}</span>
                <span className="font-mono-tech w-16 text-[12px] text-[hsl(var(--ink-mid))]">×{d.count} 次</span>
                {/* 迷你趋势条：高度按行内最大次数归一化，避免溢出行高 */}
                <div className="flex h-6 w-[180px] items-end gap-[3px]">
                  {d.trend.map((v, i) => (
                    <div
                      key={i}
                      className="flex-1 rounded-sm"
                      style={{
                        height: v === 0 ? '3px' : `${Math.max(14, (v / peak) * 100)}%`,
                        backgroundColor:
                          v === 0
                            ? 'hsl(var(--graphite-3) / 0.5)'
                            : i === d.trend.length - 1
                              ? 'hsl(var(--rust))'
                              : 'hsl(var(--graphite-3))',
                      }}
                      title={`第 ${i + 1} 次练习：${v} 处`}
                    />
                  ))}
                </div>
                <span
                  className="font-mono-tech w-16 text-right text-[12px]"
                  style={{ color: d.rate > 0.3 ? 'hsl(var(--rust))' : d.rate > 0 ? '#d59a3d' : 'hsl(var(--bronze))' }}
                >
                  复发 {Math.round(d.rate * 100)}%
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
