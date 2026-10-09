import { useMemo, useState } from 'react'
import { REVIEW_QUEUE, TEACHER, type ReviewItem } from '../../data/teacher'

const CONF_COLOR = (c: number) =>
  c >= 0.85 ? 'hsl(var(--bronze))' : c >= 0.7 ? '#d59a3d' : 'hsl(var(--rust))'

export default function TeacherReview() {
  const [queue, setQueue] = useState<ReviewItem[]>(REVIEW_QUEUE)
  const [sel, setSel] = useState<string | null>(REVIEW_QUEUE[1].id) // 默认选中一个争议样本
  const [scoreDraft, setScoreDraft] = useState('')
  const [commentDraft, setCommentDraft] = useState('')

  const pending = queue.filter((q) => q.status === 'pending')
  const done = queue.length - pending.length
  const disputes = queue.filter((q) => q.dispute && q.status === 'pending')
  const current = useMemo(() => queue.find((q) => q.id === sel) ?? null, [queue, sel])

  const act = (id: string, action: 'confirm' | 'adjust') => {
    setQueue((qs) =>
      qs.map((q) => {
        if (q.id !== id) return q
        if (action === 'confirm') return { ...q, status: 'confirmed', finalScore: q.aiScore }
        const s = parseFloat(scoreDraft)
        return { ...q, status: 'adjusted', finalScore: Number.isFinite(s) ? s : q.aiScore, comment: commentDraft || undefined }
      })
    )
    setScoreDraft(''); setCommentDraft('')
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-hidden p-6">
      {/* 头部统计 */}
      <div className="flex flex-wrap items-end justify-between gap-4 border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Class Review / 班级评卷台</div>
          <div className="mt-1 text-[15px] font-semibold">{TEACHER.className} · 第 7 次练习批量复核</div>
        </div>
        <div className="flex gap-5 font-mono-tech text-[12px]">
          <div><span className="text-[hsl(var(--ink-low))]">待复核 </span><span className="text-[18px] font-bold" style={{ color: 'hsl(var(--rust))' }}>{pending.length}</span></div>
          <div><span className="text-[hsl(var(--ink-low))]">争议样本 </span><span className="text-[18px] font-bold" style={{ color: '#d59a3d' }}>{disputes.length}</span></div>
          <div><span className="text-[hsl(var(--ink-low))]">已处理 </span><span className="text-[18px] font-bold text-[hsl(var(--bronze))]">{done}</span></div>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 xl:grid-cols-[1fr_360px]">
        {/* 复核队列 */}
        <div className="min-h-0 overflow-y-auto pr-1">
          <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
            {queue.map((q) => {
              const active = sel === q.id
              return (
                <button
                  key={q.id}
                  onClick={() => setSel(q.id)}
                  className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors"
                  style={{ backgroundColor: active ? 'hsl(var(--quench) / 0.07)' : undefined }}
                >
                  {q.dispute && q.status === 'pending' && (
                    <span className="shrink-0 rounded border border-[hsl(var(--rust)/0.6)] px-1.5 py-0.5 font-mono-tech text-[9px] tracking-wider" style={{ color: 'hsl(var(--rust))' }}>争议</span>
                  )}
                  <span className="w-14 text-[13px] font-medium text-[hsl(var(--ink-hi))]">{q.student}</span>
                  <span className="w-36 truncate text-[12px] text-[hsl(var(--ink-mid))]">{q.sample}</span>
                  <span className="font-mono-tech text-[13px] text-[hsl(var(--quench))]">AI {q.aiScore}</span>
                  <span className="font-mono-tech w-12 text-[11px]" style={{ color: CONF_COLOR(q.aiConf) }}>{Math.round(q.aiConf * 100)}%</span>
                  <span className="flex-1 truncate text-[11.5px] text-[hsl(var(--ink-low))]">{q.topIssue}</span>
                  {q.status === 'pending' ? (
                    <span className="micro-label" style={{ color: 'hsl(var(--rust))' }}>待复核</span>
                  ) : (
                    <span className="font-mono-tech text-[11px] text-[hsl(var(--bronze))]">
                      ✓ {q.finalScore}{q.status === 'adjusted' ? ' 改' : ''}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>

        {/* 复核操作面板 */}
        <div className="min-h-0 overflow-y-auto">
          {current ? (
            <div className="hairline flex flex-col gap-4 rounded-md bg-[hsl(var(--graphite-1))] p-4">
              <div>
                <div className="micro-label">Review / 复核面板</div>
                <div className="mt-1.5 text-[15px] font-semibold">{current.student} · {current.sample}</div>
                <div className="mt-1 font-mono-tech text-[11px] text-[hsl(var(--ink-low))]">{current.id}</div>
              </div>

              <div className="flex items-center gap-4 rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-2))] p-3">
                <div>
                  <div className="micro-label">AI 预评分</div>
                  <div className="font-mono-tech text-[30px] font-bold leading-tight text-[hsl(var(--quench))]">{current.aiScore}</div>
                </div>
                <div className="flex-1">
                  <div className="micro-label mb-1">置信度</div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                    <div className="h-full rounded-full" style={{ width: `${current.aiConf * 100}%`, backgroundColor: CONF_COLOR(current.aiConf) }} />
                  </div>
                  <div className="mt-1 text-[10.5px] text-[hsl(var(--ink-low))]">{Math.round(current.aiConf * 100)}% · 主要问题：{current.topIssue}</div>
                </div>
              </div>

              {current.dispute && (
                <div className="rounded border-l-2 bg-[hsl(var(--graphite-2))] px-3 py-2 text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]" style={{ borderLeftColor: 'hsl(var(--rust))' }}>
                  <span className="font-semibold" style={{ color: 'hsl(var(--rust))' }}>疑似争议：</span>{current.disputeReason}
                </div>
              )}

              {current.status === 'pending' ? (
                <>
                  <div className="flex flex-col gap-2">
                    <label className="micro-label">改分（留空 = 维持 AI 分）</label>
                    <input
                      value={scoreDraft}
                      onChange={(e) => setScoreDraft(e.target.value.replace(/[^\d.]/g, ''))}
                      placeholder={`如 ${Math.min(100, current.aiScore + 3)}`}
                      className="rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-3 py-2 font-mono-tech text-[14px] text-[hsl(var(--ink-hi))] outline-none focus:border-[hsl(var(--quench))]"
                    />
                    <label className="micro-label mt-1">评语</label>
                    <textarea
                      value={commentDraft}
                      onChange={(e) => setCommentDraft(e.target.value)}
                      rows={2}
                      placeholder="写给学生的针对性反馈…"
                      className="rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-3 py-2 text-[12.5px] text-[hsl(var(--ink-hi))] outline-none placeholder:text-[hsl(var(--ink-low))] focus:border-[hsl(var(--quench))]"
                    />
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={() => act(current.id, 'confirm')}
                      className="flex-1 rounded px-3 py-2 text-[13px] font-medium transition-transform active:scale-[0.98]"
                      style={{ backgroundColor: 'hsl(var(--bronze))', color: 'hsl(var(--graphite-0))' }}
                    >
                      ✓ 确认 AI 评分
                    </button>
                    <button
                      onClick={() => act(current.id, 'adjust')}
                      className="flex-1 rounded border px-3 py-2 text-[13px] font-medium transition-transform active:scale-[0.98]"
                      style={{ borderColor: 'hsl(var(--quench))', color: 'hsl(var(--quench))' }}
                    >
                      修改并完成复核
                    </button>
                  </div>
                  <p className="text-[10.5px] leading-relaxed text-[hsl(var(--ink-low))]">
                    教师在这里一次完成确认或纠错。修改后会自动记录为 AI 纠错样本，不需要再去其他页面重复标定。
                  </p>
                </>
              ) : (
                <div className="rounded border border-[hsl(var(--bronze)/0.4)] bg-[hsl(var(--bronze)/0.06)] px-3 py-2.5 text-[12.5px] text-[hsl(var(--bronze))]">
                  已处理：终评 {current.finalScore} 分{current.comment ? ` · 评语「${current.comment}」` : ''}
                </div>
              )}
            </div>
          ) : (
            <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-6 text-center text-[12px] text-[hsl(var(--ink-low))]">
              从左侧队列选择一份样本
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
