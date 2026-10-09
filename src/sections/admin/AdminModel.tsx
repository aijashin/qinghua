import { useState } from 'react'
import { APPEAL_QUEUE, CALIBRATION_AUDIT, SCORE_RULES, type ScoreRule } from '../../data/admin'

export default function AdminModel() {
  const [appeals, setAppeals] = useState(APPEAL_QUEUE)
  const [rules, setRules] = useState<ScoreRule[]>(SCORE_RULES)
  const [draftRules, setDraftRules] = useState<ScoreRule[]>(SCORE_RULES)
  const [editing, setEditing] = useState(false)
  const [saved, setSaved] = useState(false)

  const resolve = (id: string) => {
    setAppeals((as) => as.map((a) => (a.id === id ? { ...a, status: 'resolved', days: 0 } : a)))
  }

  const queued = appeals.filter((a) => a.status === 'queued')
  const total = draftRules.reduce((sum, rule) => sum + rule.full, 0)

  const startEditing = () => {
    setDraftRules(rules.map((rule) => ({ ...rule })))
    setSaved(false)
    setEditing(true)
  }

  const updateRule = (code: string, patch: Partial<ScoreRule>) => {
    setDraftRules((items) => items.map((rule) => (rule.code === code ? { ...rule, ...patch } : rule)))
  }

  const saveRules = () => {
    if (total !== 100 || draftRules.some((rule) => rule.full <= 0 || !rule.description.trim())) return
    setRules(draftRules.map((rule) => ({ ...rule })))
    setEditing(false)
    setSaved(true)
  }

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="border-b border-[hsl(var(--metal-line))] pb-4">
        <div className="micro-label">Rules & Review / 规则与审核</div>
        <div className="mt-1 text-[15px] font-semibold">评分规则设置 · AI 纠错审核</div>
      </div>

      {/* 当前评分规则 */}
      <div>
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="micro-label">Scoring Rules / 当前评分规则</div>
            <div className="mt-1 text-[11.5px] text-[hsl(var(--ink-mid))]">新作业都使用下面的规则，总分必须为 100 分。</div>
          </div>
          <div className="flex items-center gap-3">
            <span className="font-mono-tech text-[12px]" style={{ color: total === 100 ? 'hsl(var(--bronze))' : 'hsl(var(--rust))' }}>
              当前总分 {total}
            </span>
            {!editing && (
              <button
                onClick={startEditing}
                className="rounded border border-[hsl(var(--quench))] px-3 py-1.5 text-[12px] text-[hsl(var(--quench))]"
              >
                修改规则
              </button>
            )}
          </div>
        </div>
        <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
          {(editing ? draftRules : rules).map((rule) => (
            <div key={rule.code} className="grid grid-cols-1 gap-3 px-4 py-3 md:grid-cols-[56px_180px_90px_1fr] md:items-center">
              <span className="font-mono-tech text-[11px] text-[hsl(var(--ink-low))]">{rule.code}</span>
              <span className="text-[12.5px] font-medium text-[hsl(var(--ink-hi))]">{rule.name}</span>
              {editing ? (
                <label className="flex items-center gap-2 text-[11px] text-[hsl(var(--ink-mid))]">
                  <input
                    type="number"
                    min={1}
                    max={100}
                    value={rule.full}
                    onChange={(e) => updateRule(rule.code, { full: Number(e.target.value) })}
                    className="w-16 rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-2 py-1.5 font-mono-tech text-[12px] text-[hsl(var(--ink-hi))] outline-none focus:border-[hsl(var(--quench))]"
                  />
                  分
                </label>
              ) : (
                <span className="font-mono-tech text-[12px] text-[hsl(var(--quench))]">{rule.full} 分</span>
              )}
              {editing ? (
                <input
                  value={rule.description}
                  onChange={(e) => updateRule(rule.code, { description: e.target.value })}
                  className="rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-3 py-1.5 text-[12px] text-[hsl(var(--ink-hi))] outline-none focus:border-[hsl(var(--quench))]"
                />
              ) : (
                <span className="text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]">{rule.description}</span>
              )}
            </div>
          ))}
        </div>
        {editing && (
          <div className="mt-3 flex items-center justify-end gap-2">
            {total !== 100 && <span className="mr-auto text-[11.5px] text-[hsl(var(--rust))]">请将总分调整为 100 分后再保存</span>}
            <button
              onClick={() => setEditing(false)}
              className="rounded border border-[hsl(var(--metal-line))] px-3 py-1.5 text-[12px] text-[hsl(var(--ink-mid))]"
            >
              取消
            </button>
            <button
              onClick={saveRules}
              disabled={total !== 100}
              className="rounded px-3 py-1.5 text-[12px] font-medium disabled:cursor-not-allowed disabled:opacity-40"
              style={{ backgroundColor: 'hsl(var(--quench))', color: 'hsl(var(--graphite-0))' }}
            >
              保存修改
            </button>
          </div>
        )}
        {saved && !editing && (
          <div className="mt-3 text-[11.5px] text-[hsl(var(--bronze))]">✓ 评分规则已保存，后续新作业将使用新规则。</div>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {/* 申诉队列 */}
        <div>
          <div className="micro-label mb-3" style={{ color: 'hsl(var(--rust))' }}>AI 纠错审核 · {queued.length} 件待处理</div>
          <div className="flex flex-col gap-2">
            {appeals.map((a) => {
              const pColor = a.priority === 'high' ? 'hsl(var(--rust))' : a.priority === 'mid' ? '#d59a3d' : 'hsl(var(--ink-low))'
              return (
                <div key={a.id} className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-3.5" style={{ opacity: a.status === 'resolved' ? 0.5 : 1 }}>
                  <div className="flex items-center gap-2">
                    <span className="rounded border px-1.5 py-0.5 font-mono-tech text-[9px] tracking-wider" style={{ borderColor: pColor, color: pColor }}>
                      {a.priority.toUpperCase()}
                    </span>
                    <span className="font-mono-tech text-[10px] text-[hsl(var(--ink-low))]">{a.id}</span>
                    <span className="text-[11.5px] text-[hsl(var(--ink-mid))]">{a.from}</span>
                    {a.days > 0 && <span className="ml-auto font-mono-tech text-[10.5px]" style={{ color: a.days >= 3 ? 'hsl(var(--rust))' : 'hsl(var(--ink-low))' }}>排队 {a.days} 天</span>}
                  </div>
                  <p className="mt-2 text-[12px] leading-relaxed text-[hsl(var(--ink-hi))]">{a.reason}</p>
                  {a.status === 'queued' ? (
                    <button
                      onClick={() => resolve(a.id)}
                      className="mt-2.5 rounded border border-[hsl(var(--bronze)/0.6)] px-3 py-1 text-[11.5px] text-[hsl(var(--bronze))] transition-transform active:scale-[0.98]"
                    >
                      标记已处理
                    </button>
                  ) : (
                    <div className="mt-2 text-[11px] text-[hsl(var(--bronze))]">已处理 ✓</div>
                  )}
                </div>
              )
            })}
          </div>
        </div>

        {/* 纠错样本检查 */}
        <div>
          <div className="micro-label mb-3">Correction Samples / 纠错样本检查</div>
          <div className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
            <div className="grid grid-cols-3 gap-3">
              {[
                { label: '教师纠错样本', value: CALIBRATION_AUDIT.total, color: 'hsl(var(--ink-hi))' },
                { label: '合格', value: CALIBRATION_AUDIT.qualified, color: 'hsl(var(--bronze))' },
                { label: '退回（重复+冲突）', value: CALIBRATION_AUDIT.dup + CALIBRATION_AUDIT.conflict, color: 'hsl(var(--rust))' },
              ].map((s) => (
                <div key={s.label} className="rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-2))] p-3 text-center">
                  <div className="font-mono-tech text-[24px] font-bold" style={{ color: s.color }}>{s.value}</div>
                  <div className="mt-1 text-[10.5px] text-[hsl(var(--ink-low))]">{s.label}</div>
                </div>
              ))}
            </div>
            {/* 合格率条 */}
            <div className="mt-4">
              <div className="mb-1 flex justify-between text-[11px]">
                <span className="text-[hsl(var(--ink-mid))]">合格率</span>
                <span className="font-mono-tech text-[hsl(var(--bronze))]">93.4%</span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-[hsl(var(--graphite-3))]">
                <div className="h-full rounded-full bg-[hsl(var(--bronze))]" style={{ width: '93.4%' }} />
              </div>
            </div>
            <p className="mt-3 text-[11.5px] leading-relaxed text-[hsl(var(--ink-mid))]">{CALIBRATION_AUDIT.note}</p>
          </div>

          <div className="mt-3 hairline rounded-md border-l-2 bg-[hsl(var(--graphite-1))] p-4" style={{ borderLeftColor: 'hsl(var(--rust))' }}>
            <div className="micro-label mb-1.5" style={{ color: 'hsl(var(--rust))' }}>风险提示</div>
            <p className="text-[12px] leading-relaxed text-[hsl(var(--ink-mid))]">
              回火索氏体误判连续出现 3 例。建议先处理这批纠错样本，再修改上方的组织判别规则。
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
