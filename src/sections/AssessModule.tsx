import { useEffect, useMemo, useState } from 'react'
import MicroCanvas from '../components/MicroCanvas'
import { DEFECTS, RUBRIC, STUDENT, TOTAL_FULL, TOTAL_SCORE, NEXT_GOAL } from '../data/mock'

// 分阶段揭示：扫描 → 评分 → 缺陷 → 建议
const STAGE_DELAY = [900, 1400, 1800]

export default function AssessModule() {
  const [stage, setStage] = useState(0) // 0 扫描中 1 评分 2 缺陷 3 完整
  const [activeDefect, setActiveDefect] = useState<number | null>(1)
  const [view, setView] = useState<'split' | 'anno'>('split')

  useEffect(() => {
    const timers = STAGE_DELAY.map((d, i) => setTimeout(() => setStage(i + 1), d))
    return () => timers.forEach(clearTimeout)
  }, [])

  const worst = useMemo(() => RUBRIC.filter((r) => r.level === 'bad')[0], [])
  const scorePct = Math.round((TOTAL_SCORE / TOTAL_FULL) * 100)

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      {/* 样本信息条 */}
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Sample / 本次试样</div>
          <div className="mt-1 text-[15px] font-semibold text-[hsl(var(--ink-hi))]">
            {STUDENT.sample.material} · 第 {STUDENT.sample.index} 次练习
          </div>
        </div>
        {[
          ['工艺', STUDENT.sample.process],
          ['腐蚀剂', STUDENT.sample.etchant],
          ['倍数', STUDENT.sample.magnification],
          ['日期', STUDENT.sample.date],
        ].map(([k, v]) => (
          <div key={k}>
            <div className="micro-label">{k}</div>
            <div className="mt-1 font-mono-tech text-[12px] text-[hsl(var(--ink-mid))]">{v}</div>
          </div>
        ))}
      </div>

      {/* 评分总览 + 最优先改进点 */}
      {stage >= 1 ? (
        <div className="stage-in grid grid-cols-1 gap-4 lg:grid-cols-[240px_1fr]">
          {/* 总分 */}
          <div className="hairline flex flex-col items-center justify-center rounded-md bg-[hsl(var(--graphite-1))] p-5">
            <div className="micro-label">Teaching Demo / 教学示例评分</div>
            <div className="mt-2 flex items-baseline gap-1">
              <span className="font-mono-tech text-[52px] font-bold leading-none text-[hsl(var(--quench))]">{TOTAL_SCORE}</span>
              <span className="font-mono-tech text-[16px] text-[hsl(var(--ink-low))]">/ {TOTAL_FULL}</span>
            </div>
            <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
              <div
                className="h-full rounded-full bg-[hsl(var(--quench))]"
                style={{ width: `${scorePct}%`, transition: 'width 800ms cubic-bezier(0.76,0,0.24,1)' }}
              />
            </div>
            <div className="mt-2 text-center text-[11px] text-[hsl(var(--ink-mid))]">校内练习示例细则 · 不代表国赛正式成绩</div>
          </div>

          {/* 教练结论：最优先改进点 */}
          <div className="hairline rounded-md border-l-2 bg-[hsl(var(--graphite-1))] p-5" style={{ borderLeftColor: 'hsl(var(--rust))' }}>
            <div className="flex items-center gap-2">
              <span className="micro-label" style={{ color: 'hsl(var(--rust))' }}>Priority Fix / 最优先改进点</span>
            </div>
            <p className="mt-2 text-[14px] leading-relaxed text-[hsl(var(--ink-hi))]">
              <span className="font-semibold" style={{ color: 'hsl(var(--rust))' }}>{worst.name}</span>
              是本片最大失分项（{worst.got}/{worst.full}）。2 条粗划痕贯穿视场——问题不在抛光，而在砂纸过渡。
            </p>
            <p className="mt-2 text-[13px] leading-relaxed text-[hsl(var(--ink-mid))]">
              你觉得这一步划痕是怎么产生的？回想一下：从 400# 换到 800# 时，有没有在低倍下确认上一道磨痕已完全消除？
            </p>
          </div>
        </div>
      ) : (
        <div className="hairline flex items-center gap-3 rounded-md bg-[hsl(var(--graphite-1))] p-5">
          <span className="shimmer-chip rounded border border-[hsl(var(--quench)/0.4)] px-2.5 py-1 font-mono-tech text-[11px] text-[hsl(var(--quench))]">
            ANALYZING
          </span>
          <span className="text-[13px] text-[hsl(var(--ink-mid))]">
            正在展示预置的评片示例结果
            <span className="caret-blink ml-1" />
          </span>
        </div>
      )}

      {/* 图像对照 */}
      <div>
        <div className="mb-3 flex items-center justify-between">
          <div className="micro-label">Field View / 视场对照</div>
          <div className="flex gap-1 rounded border border-[hsl(var(--metal-line))] p-0.5">
            {([['split', '左右对照'], ['anno', 'AI 标注']] as const).map(([k, label]) => (
              <button
                key={k}
                onClick={() => setView(k)}
                className="rounded px-3 py-1 text-[12px] transition-colors"
                style={{
                  backgroundColor: view === k ? 'hsl(var(--graphite-3))' : 'transparent',
                  color: view === k ? 'hsl(var(--ink-hi))' : 'hsl(var(--ink-low))',
                }}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        {view === 'split' ? (
          <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
            <figure>
              <MicroCanvas overlay="none" className="hairline rounded-md" />
              <figcaption className="mt-1.5 flex justify-between">
                <span className="micro-label">Original / 原图</span>
                <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">IMG_0417.TIF</span>
              </figcaption>
            </figure>
            <figure>
              <MicroCanvas
                overlay="annotate"
                anchors={DEFECTS.map((d) => ({ id: d.id, x: d.x, y: d.y, severity: d.severity }))}
                activeAnchor={activeDefect}
                scanning={stage === 0}
                className="hairline rounded-md"
              />
              <figcaption className="mt-1.5 flex justify-between">
                <span className="micro-label" style={{ color: 'hsl(var(--quench))' }}>Example Annotation / 示例标注图</span>
                <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">{DEFECTS.length} 处缺陷锚点</span>
              </figcaption>
            </figure>
          </div>
        ) : (
          <MicroCanvas
            overlay="annotate"
            anchors={DEFECTS.map((d) => ({ id: d.id, x: d.x, y: d.y, severity: d.severity }))}
            activeAnchor={activeDefect}
            className="hairline rounded-md"
          />
        )}
      </div>

      {/* 逐项评分 */}
      {stage >= 1 && (
        <div className="stage-in">
          <div className="micro-label mb-3">Rubric Breakdown / 逐项评分</div>
          <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
            {RUBRIC.map((r) => (
              <div key={r.code} className="px-4 py-3">
                <div className="flex items-center gap-3">
                  <span className="font-mono-tech w-11 text-[11px] text-[hsl(var(--ink-low))]">{r.code}</span>
                  <span className="w-40 text-[13px] text-[hsl(var(--ink-hi))]">{r.name}</span>
                  <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                    <div
                      className="h-full rounded-full"
                      style={{
                        width: `${(r.got / r.full) * 100}%`,
                        backgroundColor:
                          r.level === 'bad' ? 'hsl(var(--rust))' : r.level === 'warn' ? '#d59a3d' : 'hsl(var(--bronze))',
                        transition: 'width 700ms cubic-bezier(0.76,0,0.24,1)',
                      }}
                    />
                  </div>
                  <span className="font-mono-tech w-14 text-right text-[12px] text-[hsl(var(--ink-mid))]">
                    {r.got}<span className="text-[hsl(var(--ink-low))]">/{r.full}</span>
                  </span>
                  <span className="micro-label w-14 text-right" style={{ fontSize: 9 }}>依据</span>
                </div>
                <div className="ml-14 mt-2 rounded border-l-2 border-[hsl(var(--quench))] bg-[hsl(var(--graphite-2))] px-3 py-2 text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]">
                  {r.basis}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 缺陷列表 */}
      {stage >= 2 && (
        <div className="stage-in">
          <div className="micro-label mb-3">Defects / 缺陷定位（示例缺陷本）</div>
          <div className="flex flex-col gap-2">
            {DEFECTS.map((d) => {
              const active = activeDefect === d.id
              const color = d.severity === 'high' ? 'hsl(var(--rust))' : d.severity === 'mid' ? '#d59a3d' : 'hsl(var(--ink-low))'
              return (
                <div
                  key={d.id}
                  onMouseEnter={() => setActiveDefect(d.id)}
                  className="hairline rounded-md bg-[hsl(var(--graphite-1))] px-4 py-3 transition-colors"
                  style={{ borderColor: active ? color : undefined }}
                >
                  <div className="flex items-center gap-3">
                    <span
                      className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold ${d.severity === 'high' && active ? 'anchor-ping' : ''}`}
                      style={{ borderColor: color, color }}
                    >
                      {d.id}
                    </span>
                    <span className="text-[13px] font-medium text-[hsl(var(--ink-hi))]">{d.type}</span>
                    <span className="micro-label" style={{ color }}>{d.severity === 'high' ? 'HIGH' : d.severity === 'mid' ? 'MID' : 'LOW'}</span>
                  </div>
                  <div className="mt-3 grid grid-cols-1 gap-2 border-t border-[hsl(var(--metal-line))] pt-3 text-[12px] leading-relaxed md:grid-cols-3">
                    <div>
                      <div className="micro-label mb-1">现象</div>
                      <p className="text-[hsl(var(--ink-mid))]">{d.desc}</p>
                    </div>
                    <div>
                      <div className="micro-label mb-1">成因推测</div>
                      <p className="text-[hsl(var(--ink-mid))]">{d.cause}</p>
                    </div>
                    <div>
                      <div className="micro-label mb-1" style={{ color: 'hsl(var(--bronze))' }}>纠正方法</div>
                      <p className="text-[hsl(var(--ink-hi))]">{d.fix}</p>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* 下次练习目标 */}
      {stage >= 3 && (
        <div className="stage-in hairline rounded-md border-l-2 bg-[hsl(var(--graphite-1))] p-4" style={{ borderLeftColor: 'hsl(var(--bronze))' }}>
          <div className="micro-label" style={{ color: 'hsl(var(--bronze))' }}>Next Goal / 下一次练习可验证的小目标</div>
          <p className="mt-1.5 text-[13px] text-[hsl(var(--ink-hi))]">{NEXT_GOAL}</p>
        </div>
      )}
    </div>
  )
}
