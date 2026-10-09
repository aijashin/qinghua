import { useEffect, useRef, useState } from 'react'
import { DIALOGUE, ROUTE_TREE, UNLOCKED_PLAN, type TreeNode } from '../data/mock'

interface Msg {
  role: 'coach' | 'me' | 'system'
  text: string
}

function Node({ node, depth = 0, unlocked }: { node: TreeNode; depth?: number; unlocked: boolean }) {
  const locked = node.status === 'locked' && !unlocked
  const color =
    node.status === 'recommended' ? 'hsl(var(--bronze))' : locked ? 'hsl(var(--ink-low))' : 'hsl(var(--quench))'
  return (
    <div>
      <div
        className="flex items-center gap-2 rounded border px-3 py-2"
        style={{
          borderColor: locked ? 'hsl(var(--graphite-3))' : `${color}`,
          backgroundColor: node.status === 'recommended' ? 'hsl(var(--bronze) / 0.08)' : 'hsl(var(--graphite-1))',
          opacity: locked ? 0.55 : 1,
        }}
      >
        <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: color }} />
        <div className="min-w-0">
          <div className="truncate text-[12.5px] font-medium text-[hsl(var(--ink-hi))]">
            {locked ? '？' : ''}{node.label}
            {locked && <span className="ml-1.5 font-mono-tech text-[9px] tracking-wider text-[hsl(var(--ink-low))]">LOCKED</span>}
          </div>
          <div className="truncate text-[10.5px] text-[hsl(var(--ink-low))]">{locked ? '完成追问论证后解锁' : node.detail}</div>
        </div>
      </div>
      {node.children && (
        <div className="ml-4 border-l border-[hsl(var(--metal-line))] pl-3 pt-2" style={{ marginTop: 8 }}>
          <div className="flex flex-col gap-2 pb-1">
            {node.children.map((c) => (
              <Node key={c.id} node={c} depth={depth + 1} unlocked={unlocked} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export default function ActionModule() {
  const [round, setRound] = useState(0) // 当前论证明轮次
  const [msgs, setMsgs] = useState<Msg[]>([])
  const [input, setInput] = useState('')
  const [typing, setTyping] = useState(false)
  const [unlocked, setUnlocked] = useState(false)
  const messagesRef = useRef<HTMLDivElement>(null)

  // 开场：教练抛出第 1 问
  useEffect(() => {
    const t = setTimeout(() => {
      setMsgs([
        { role: 'system', text: '逆向求证 · 第 1/3 轮论证' },
        { role: 'coach', text: DIALOGUE[0].question },
      ])
    }, 600)
    return () => clearTimeout(t)
  }, [])

  useEffect(() => {
    const box = messagesRef.current
    if (box) box.scrollTop = box.scrollHeight
  }, [msgs, typing])

  const submit = () => {
    const text = input.trim()
    if (!text || typing || unlocked) return
    const q = DIALOGUE[round]
    setMsgs((m) => [...m, { role: 'me', text }])
    setInput('')
    setTyping(true)

    setTimeout(() => {
      const hit = q.keywords.some((k) => text.toLowerCase().includes(k.toLowerCase()))
      if (hit) {
        // 论证通过 → 点评 + 推进
        const next = round + 1
        const pass: Msg[] = [{ role: 'coach', text: `✓ ${q.coach}` }]
        if (next < DIALOGUE.length) {
          pass.push({ role: 'system', text: `逆向求证 · 第 ${next + 1}/3 轮论证` })
          pass.push({ role: 'coach', text: DIALOGUE[next].question })
          setRound(next)
        } else {
          pass.push({ role: 'system', text: '3 轮论证完成 · 完整工艺方案已解锁' })
          pass.push({ role: 'coach', text: '论证链闭合。但注意：方案不是终点——按它制样后回「鹰眼评片」验证组织，才算走完一轮 4A 飞轮。' })
          setUnlocked(true)
        }
        setMsgs((m) => [...m, ...pass])
      } else {
        // 未命中 → 苏格拉底式再追问
        setMsgs((m) => [...m, { role: 'coach', text: `再想想。${q.hint}` }])
      }
      setTyping(false)
    }, 1100)
  }

  return (
    <div className="flex h-full flex-col gap-5 overflow-hidden p-6">
      <div className="flex items-center justify-between border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Act III / 逆向求证</div>
          <div className="mt-1 text-[15px] font-semibold">性能 → 工艺 · 苏格拉底式论证 <span className="text-[11px] font-normal text-[hsl(var(--ink-low))]">（预置交互示例）</span></div>
        </div>
        <span
          className="rounded border px-2.5 py-1 font-mono-tech text-[11px]"
          style={{
            borderColor: unlocked ? 'hsl(var(--bronze))' : 'hsl(var(--metal-line))',
            color: unlocked ? 'hsl(var(--bronze))' : 'hsl(var(--ink-low))',
          }}
        >
          {unlocked ? 'PLAN UNLOCKED' : `${round}/3 轮论证`}
        </span>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-5 xl:grid-cols-[340px_1fr]">
        {/* 工艺路线树 */}
        <div className="min-h-0 overflow-y-auto pr-1">
          <div className="micro-label mb-3">Route Tree / 候选工艺路线</div>
          <Node node={ROUTE_TREE} unlocked={unlocked} />
          {unlocked && (
            <div className="stage-in mt-4">
              <div className="micro-label mb-2" style={{ color: 'hsl(var(--bronze))' }}>Unlocked Plan / 完整方案</div>
              <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
                {UNLOCKED_PLAN.map((p, i) => (
                  <div key={p.step} className="px-3 py-2.5">
                    <div className="flex items-center gap-2">
                      <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">S{i + 1}</span>
                      <span className="text-[12px] font-semibold text-[hsl(var(--bronze))]">{p.step}</span>
                    </div>
                    <div className="mt-1 text-[12px] text-[hsl(var(--ink-hi))]">{p.value}</div>
                    <div className="mt-0.5 text-[11px] text-[hsl(var(--ink-mid))]">{p.note}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* 追问对话 */}
        <div className="flex min-h-0 flex-col rounded-md border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-1))]">
          <div ref={messagesRef} className="min-h-0 flex-1 overflow-y-auto p-4">
            <div className="flex flex-col gap-3">
              {msgs.map((m, i) =>
                m.role === 'system' ? (
                  <div key={i} className="my-1 flex items-center gap-3">
                    <div className="h-px flex-1 bg-[hsl(var(--metal-line))]" />
                    <span className="micro-label">{m.text}</span>
                    <div className="h-px flex-1 bg-[hsl(var(--metal-line))]" />
                  </div>
                ) : (
                  <div key={i} className={`flex ${m.role === 'me' ? 'justify-end' : 'justify-start'}`}>
                    <div
                      className="max-w-[85%] rounded-md px-3.5 py-2.5 text-[13px] leading-relaxed"
                      style={{
                        backgroundColor: m.role === 'me' ? 'hsl(var(--quench) / 0.14)' : 'hsl(var(--graphite-2))',
                        border: `1px solid ${m.role === 'me' ? 'hsl(var(--quench) / 0.35)' : 'hsl(var(--metal-line))'}`,
                        color: 'hsl(var(--ink-hi))',
                      }}
                    >
                      {m.role === 'coach' && <div className="micro-label mb-1" style={{ color: 'hsl(var(--quench))' }}>Coach</div>}
                      {m.text}
                    </div>
                  </div>
                )
              )}
              {typing && (
                <div className="flex justify-start">
                  <div className="rounded-md border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-2))] px-3.5 py-2.5 text-[13px] text-[hsl(var(--ink-mid))]">
                    教练正在审阅你的论证
                    <span className="caret-blink ml-1" />
                  </div>
                </div>
              )}
            </div>
          </div>
          <div className="border-t border-[hsl(var(--metal-line))] p-3">
            {unlocked ? (
              <div className="text-center text-[12px] text-[hsl(var(--ink-mid))]">
                论证已完成。去「鹰眼评片」上传按方案制备的新试样，完成 Audit 闭环。
              </div>
            ) : (
              <div className="flex gap-2">
                <input
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && submit()}
                  placeholder="写下你的理由，教练会继续追问…"
                  className="flex-1 rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-3 py-2 text-[13px] text-[hsl(var(--ink-hi))] outline-none placeholder:text-[hsl(var(--ink-low))] focus:border-[hsl(var(--quench))]"
                />
                <button
                  onClick={submit}
                  className="rounded px-4 py-2 text-[13px] font-medium transition-transform active:scale-[0.98]"
                  style={{ backgroundColor: 'hsl(var(--quench))', color: 'hsl(var(--graphite-0))' }}
                >
                  提交论证
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
