import { ACTIVITY, ADMIN, AGREEMENT_TREND, DEPT_USAGE, KPI } from '../../data/admin'

function Spark({ data, color, w = 560, h = 130 }: { data: number[]; color: string; w?: number; h?: number }) {
  const PAD = 8
  const min = Math.min(...data), max = Math.max(...data)
  const px = (i: number) => PAD + (i / (data.length - 1)) * (w - PAD * 2)
  const py = (v: number) => h - PAD - ((v - min) / (max - min || 1)) * (h - PAD * 2)
  const line = data.map((v, i) => `${i === 0 ? 'M' : 'L'} ${px(i)} ${py(v)}`).join(' ')
  const area = `${line} L ${px(data.length - 1)} ${h - PAD} L ${px(0)} ${h - PAD} Z`
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="block w-full">
      <path d={area} fill={color} opacity="0.12" />
      <path d={line} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round" />
      {data.map((v, i) => (
        <circle key={i} cx={px(i)} cy={py(v)} r={i === data.length - 1 ? 3.5 : 2} fill="hsl(var(--graphite-0))" stroke={color} strokeWidth="1.5" />
      ))}
    </svg>
  )
}

export default function AdminDashboard() {
  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Dashboard / 数据看板</div>
          <div className="mt-1 text-[15px] font-semibold">{ADMIN.scope} · 聚合统计视图</div>
        </div>
        <span className="font-mono-tech text-[11px] text-[hsl(var(--ink-low))]">统计周期 {ADMIN.period} · 范围 全院 4 系 12 班</span>
      </div>

      {/* KPI 卡片 */}
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        {KPI.map((k) => (
          <div key={k.label} className="lift-card hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
            <div className="micro-label">{k.label}</div>
            <div className="mt-1.5 flex items-baseline gap-1.5">
              <span className="font-mono-tech text-[30px] font-bold leading-none text-[hsl(var(--ink-hi))]">{k.value}</span>
              <span className="text-[11px] text-[hsl(var(--ink-low))]">{k.unit}</span>
            </div>
            <div className="mt-2 font-mono-tech text-[11px]" style={{ color: k.up ? 'hsl(var(--bronze))' : 'hsl(var(--rust))' }}>
              {k.delta}
            </div>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {/* 活跃度 */}
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-2">Activity / 近 14 天平台活跃度（日活跃账号）</div>
          <Spark data={ACTIVITY} color="hsl(var(--quench))" />
          <div className="mt-1 flex justify-between font-mono-tech text-[9.5px] text-[hsl(var(--ink-low))]">
            <span>09-08</span><span>09-15</span><span>09-21</span>
          </div>
          <p className="mt-2 text-[11px] text-[hsl(var(--ink-mid))]">周末低谷明显（双日 &lt;110）；上周峰值 210，与第 7 次练习截止日吻合。</p>
        </div>

        {/* AI 与教师评分一致率 */}
        <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div className="micro-label mb-2">Consistency / AI-教师评分一致率 · 周趋势</div>
          <Spark data={AGREEMENT_TREND} color="hsl(var(--bronze))" />
          <div className="mt-1 flex justify-between font-mono-tech text-[9.5px] text-[hsl(var(--ink-low))]">
            <span>W29</span><span>W33</span><span>W38</span>
          </div>
          <p className="mt-2 text-[11px] text-[hsl(var(--ink-mid))]">
            本周 81% 的作业中，AI 评分与教师复核结果一致，较上周提高 4%。
          </p>
        </div>
      </div>

      {/* 院系资源使用 */}
      <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
        <div className="micro-label mb-3">Resource / 院系练习量对比</div>
        <div className="flex flex-col gap-2.5">
          {DEPT_USAGE.map((d) => {
            const per = d.practices / d.students
            const maxPer = Math.max(...DEPT_USAGE.map((x) => x.practices / x.students))
            return (
              <div key={d.dept} className="flex items-center gap-3">
                <span className="w-20 text-[12.5px] text-[hsl(var(--ink-hi))]">{d.dept}</span>
                <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                  <div
                    className="h-full rounded-full"
                    style={{ width: `${(per / maxPer) * 100}%`, backgroundColor: per < 2 ? 'hsl(var(--rust))' : 'hsl(var(--quench))' }}
                  />
                </div>
                <span className="font-mono-tech w-36 text-right text-[11.5px] text-[hsl(var(--ink-mid))]">
                  {d.practices} 张 / 人均 {per.toFixed(1)}
                </span>
              </div>
            )
          })}
        </div>
        <p className="mt-3 text-[11px] text-[hsl(var(--ink-low))]">
          无机非金属系人均 1.2 次，远低于全院均值——详见「资源均衡」页。
        </p>
      </div>
    </div>
  )
}
