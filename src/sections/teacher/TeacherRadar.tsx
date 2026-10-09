import {
  ALERTS, COHORT_COMPARE, HEAT_MATRIX, HEAT_ORGS, PASS_RATES, TEACHER, TOP_DEFECTS,
} from '../../data/teacher'

const HEAT_MAX = Math.max(...HEAT_MATRIX.flat())

function heatColor(v: number) {
  if (v === 0) return 'hsl(var(--graphite-2))'
  const t = v / HEAT_MAX
  // 石墨 → 锈红 渐变
  return `color-mix(in srgb, hsl(var(--rust)) ${Math.round(15 + t * 75)}%, hsl(var(--graphite-2)))`
}

export default function TeacherRadar() {
  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Radar / 学情雷达</div>
          <div className="mt-1 text-[15px] font-semibold">{TEACHER.className} · n = {TEACHER.students}</div>
        </div>
        <span className="font-mono-tech text-[11px] text-[hsl(var(--ink-low))]">统计周期 {TEACHER.weekRange} · 样本 {TEACHER.students} 人 / 187 张片子</span>
      </div>

      {/* 异常预警置顶 */}
      <div className="flex flex-col gap-2">
        {ALERTS.map((a, i) => {
          const color = a.level === 'high' ? 'hsl(var(--rust))' : a.level === 'mid' ? '#d59a3d' : 'hsl(var(--ink-low))'
          return (
            <div key={i} className="hairline flex items-start gap-3 rounded-md border-l-2 bg-[hsl(var(--graphite-1))] px-4 py-3" style={{ borderLeftColor: color }}>
              <span className="mt-0.5 shrink-0 rounded border px-1.5 py-0.5 font-mono-tech text-[9px] tracking-wider" style={{ borderColor: color, color }}>
                {a.level === 'high' ? 'HIGH' : a.level === 'mid' ? 'MID' : 'LOW'}
              </span>
              <div className="flex-1 text-[12.5px] leading-relaxed text-[hsl(var(--ink-hi))]">{a.text}</div>
              <span className="shrink-0 font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">{a.scope}</span>
            </div>
          )
        })}
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {/* 高频缺陷 TOP 榜 */}
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-3">Top Defects / 高频缺陷榜 · 按出现人次</div>
          <div className="flex flex-col gap-2.5">
            {TOP_DEFECTS.map((d, i) => (
              <div key={d.type} className="flex items-center gap-3">
                <span className="font-mono-tech w-5 text-[12px] text-[hsl(var(--ink-low))]">{i + 1}</span>
                <span className="w-20 text-[12.5px] text-[hsl(var(--ink-hi))]">{d.type}</span>
                <div className="h-2 flex-1 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                  <div
                    className="h-full rounded-full"
                    style={{ width: `${d.pct * 100}%`, backgroundColor: i < 2 ? 'hsl(var(--rust))' : i < 4 ? '#d59a3d' : 'hsl(var(--ink-low))' }}
                  />
                </div>
                <span className="font-mono-tech w-16 text-right text-[12px] text-[hsl(var(--quench))]">{d.count} 人次</span>
                <span className="w-32 truncate text-right text-[10.5px] text-[hsl(var(--ink-low))]">{d.note}</span>
              </div>
            ))}
          </div>
        </div>

        {/* 4A 通过率 */}
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-3">4A Pass Rate / 各环节通过率</div>
          <div className="flex flex-col gap-3">
            {PASS_RATES.map((p) => {
              const low = p.rate < 0.5
              return (
                <div key={p.stage}>
                  <div className="mb-1 flex justify-between text-[12px]">
                    <span className="text-[hsl(var(--ink-mid))]">{p.stage}</span>
                    <span className="font-mono-tech" style={{ color: low ? 'hsl(var(--rust))' : 'hsl(var(--bronze))' }}>
                      {Math.round(p.rate * 100)}%
                    </span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                    <div
                      className="h-full rounded-full"
                      style={{ width: `${p.rate * 100}%`, backgroundColor: low ? 'hsl(var(--rust))' : 'hsl(var(--bronze))' }}
                    />
                  </div>
                </div>
              )
            })}
          </div>
          <p className="mt-3 border-l-2 border-[hsl(var(--rust))] pl-2.5 text-[11.5px] leading-relaxed text-[hsl(var(--ink-mid))]">
            「逆推 → 验证」出现明显断层（42% → 36%）：学生能做题但不愿回炉验证。建议把 Audit 纳入平时分。
          </p>
        </div>

        {/* 组织识别错误热力图 */}
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-3">Confusion Heatmap / 组织识别错误热力</div>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-center">
              <thead>
                <tr>
                  <th className="p-1 text-left text-[10px] font-normal text-[hsl(var(--ink-low))]">真实 ↓ 误判 →</th>
                  {HEAT_ORGS.map((o) => (
                    <th key={o} className="p-1 text-[10.5px] font-normal text-[hsl(var(--ink-mid))]">{o}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {HEAT_MATRIX.map((row, ri) => (
                  <tr key={ri}>
                    <td className="p-1 text-left text-[10.5px] text-[hsl(var(--ink-mid))]">{HEAT_ORGS[ri]}</td>
                    {row.map((v, ci) => (
                      <td key={ci} className="p-0.5">
                        <div
                          className="flex h-9 items-center justify-center rounded font-mono-tech text-[12px]"
                          style={{
                            backgroundColor: ri === ci ? 'hsl(var(--graphite-0))' : heatColor(v),
                            color: v > 0 ? 'hsl(var(--ink-hi))' : 'hsl(var(--ink-low))',
                            border: ri === ci ? '1px dashed hsl(var(--metal-line))' : 'none',
                          }}
                        >
                          {ri === ci ? '·' : v || '·'}
                        </div>
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="mt-3 text-[11.5px] leading-relaxed text-[hsl(var(--ink-mid))]">
            误判集中于 <span style={{ color: 'hsl(var(--rust))' }}>回火索氏体 → 珠光体（6 例）</span>——二者都呈层片感，建议下一课用 1000× 对比图专项讲解。
          </p>
        </div>

        {/* 历届纵向对比 */}
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-3">Cohort Trend / 历届纵向对比（同期第 7 次练习均分）</div>
          <div className="flex items-end justify-around gap-4 pt-2" style={{ height: 180 }}>
            {COHORT_COMPARE.map((c) => {
              const isCurrent = c.cohort === '2023 届'
              return (
                <div key={c.cohort} className="flex flex-1 flex-col items-center gap-2">
                  <span className="font-mono-tech text-[16px] font-bold" style={{ color: isCurrent ? 'hsl(var(--quench))' : 'hsl(var(--ink-mid))' }}>
                    {c.avg}
                  </span>
                  <div
                    className="w-full max-w-[90px] rounded-t"
                    style={{
                      height: `${(c.avg / 100) * 130}px`,
                      backgroundColor: isCurrent ? 'hsl(var(--quench) / 0.7)' : 'hsl(var(--graphite-3))',
                      border: isCurrent ? '1px solid hsl(var(--quench))' : '1px solid hsl(var(--metal-line))',
                    }}
                  />
                  <div className="text-center">
                    <div className="text-[11.5px] text-[hsl(var(--ink-hi))]">{c.cohort}</div>
                    <div className="font-mono-tech text-[9.5px] text-[hsl(var(--ink-low))]">n={c.n}</div>
                  </div>
                </div>
              )
            })}
          </div>
          <p className="mt-2 text-[11.5px] text-[hsl(var(--ink-mid))]">
            本届同期均分较上届 +5，平台引入后两届连续上升（n 见柱下标注）。
          </p>
        </div>
      </div>
    </div>
  )
}
