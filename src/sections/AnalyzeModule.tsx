import { useEffect, useState } from 'react'
import MicroCanvas from '../components/MicroCanvas'
import { CHAIN, PHASES, PREDICT } from '../data/mock'

export default function AnalyzeModule() {
  const [stage, setStage] = useState(0)
  const [openLink, setOpenLink] = useState<number | null>(1) // 默认展开第二段链条

  useEffect(() => {
    const t = [1000, 1700, 2400].map((d, i) => setTimeout(() => setStage(i + 1), d))
    return () => t.forEach(clearTimeout)
  }, [])

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="flex items-center justify-between border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Act II / 识相悟道</div>
          <div className="mt-1 text-[15px] font-semibold">组织线索 · 性能关联 · 因果解释</div>
        </div>
        {stage === 0 && (
          <span className="shimmer-chip rounded border border-[hsl(var(--quench)/0.4)] px-2.5 py-1 font-mono-tech text-[11px] text-[hsl(var(--quench))]">
            READING
          </span>
        )}
      </div>

      {/* 示意关注区域 + 组织线索 */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_300px]">
        <figure>
          <MicroCanvas
            overlay={stage >= 1 ? 'segment' : 'none'}
            phases={PHASES}
            scanning={stage === 0}
            className="hairline rounded-md"
          />
          <figcaption className="mt-1.5 flex justify-between">
            <span className="micro-label" style={{ color: 'hsl(var(--quench))' }}>Visual Hints / 组织关注区域示意</span>
            <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">演示覆盖层 · 非像素分割</span>
          </figcaption>
        </figure>

        <div className="flex flex-col gap-3">
          <div className="micro-label">Structure Clues / 可见组织线索</div>
          {PHASES.map((p, i) => (
            <div
              key={p.name}
              className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-3"
              style={stage >= 1 ? { animation: `stage-in 480ms ${i * 120}ms cubic-bezier(0.76,0,0.24,1) both` } : { opacity: 0.35 }}
            >
              <div className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 rounded-sm" style={{ backgroundColor: p.color }} />
                <span className="text-[13px] text-[hsl(var(--ink-hi))]">{p.name}</span>
                <span className="ml-auto font-mono-tech text-[15px] font-semibold" style={{ color: p.color }}>
                  {stage >= 1 ? p.level : '—'}
                </span>
              </div>
              <div className="mt-2 h-1 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                <div
                  className="h-full rounded-full"
                  style={{ width: stage >= 1 ? `${p.displayWidth}%` : 0, backgroundColor: p.color, transition: 'width 900ms cubic-bezier(0.76,0,0.24,1)' }}
                />
              </div>
              <p className="mt-2 text-[11px] leading-relaxed text-[hsl(var(--ink-mid))]">{p.note}</p>
              <div className="micro-label mt-1" style={{ fontSize: 8 }}>{p.latin}</div>
            </div>
          ))}
        </div>
      </div>

      {/* 性能关联 */}
      {stage >= 2 && (
        <div className="stage-in">
          <div className="micro-label mb-3">Property Link / 性能关联分析（示例）</div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            {PREDICT.map((p) => (
              <div key={p.label} className="lift-card hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
                <div className="micro-label">{p.label}</div>
                <div className="mt-1.5 font-mono-tech text-[20px] font-semibold text-[hsl(var(--quench))]">{p.value}</div>
                <div className="mt-2 border-t border-[hsl(var(--metal-line))] pt-2 text-[11px] text-[hsl(var(--ink-low))]">{p.note}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 四维因果链 */}
      {stage >= 3 && (
        <div className="stage-in">
          <div className="micro-label mb-3">Causal Chain / 成分 → 工艺 → 组织 → 性能</div>
          <div className="grid grid-cols-1 gap-0 md:grid-cols-4">
            {CHAIN.map((c, i) => (
              <div key={c.key} className="relative">
                <button
                  onClick={() => setOpenLink(openLink === i ? null : i)}
                  className="lift-card hairline flex w-full flex-col rounded-md bg-[hsl(var(--graphite-1))] p-4 text-left"
                  style={{ minHeight: 180 }}
                >
                  <div className="flex items-baseline justify-between">
                    <span className={`text-[16px] font-bold ${c.accent}`}>{c.key}</span>
                    <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">0{i + 1}</span>
                  </div>
                  <div className="micro-label mt-0.5">{c.title.split(' ')[1]}</div>
                  <p className={`mt-2.5 text-[12px] leading-relaxed text-[hsl(var(--ink-mid))] ${openLink === i ? '' : 'line-clamp-3'}`}>
                    {c.body}
                  </p>
                  <span className="mt-auto pt-2 text-[10px]" style={{ color: 'hsl(var(--quench))' }}>
                    {openLink === i ? '收起依据 ▴' : '查看依据 ▾'}
                  </span>
                </button>
                {i < CHAIN.length - 1 && (
                  <div className="absolute -right-3 top-1/2 z-10 hidden -translate-y-1/2 text-[hsl(var(--ink-low))] md:block">→</div>
                )}
              </div>
            ))}
          </div>
          <p className="mt-4 border-l-2 border-[hsl(var(--quench))] pl-3 text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]">
            追问一下：图像中的疑似片状纹理到底是组织特征，还是制样假象？你会补充什么图像或工艺信息来区分？
          </p>
        </div>
      )}
    </div>
  )
}
