import { useId, useState, type FormEvent } from 'react'
import { LoaderCircle, Pencil, Save } from 'lucide-react'
import type { Criterion, Dimension, Report, ReportData } from '../api'
import { useDraftGuard } from '../unsaved'
import { Badge, Button, Dialog, Disclosure, Field, Loading, Message, Panel, RecordDate } from './WorkbenchUI'
import { originalData, scoreTotal } from './reportPresentation'
import { useReportRevision } from './useReportRevision'

type Draft = { data: ReportData; original: string; reason: string; baseVersion: number; mode: string }
function makeDraft(data: ReportData, baseVersion: number, mode: string): Draft { return { data: structuredClone(data), original: JSON.stringify(data), reason: '', baseVersion, mode } }
function draftChanged(draft: Draft | null) { return !!draft && (JSON.stringify(draft.data) !== draft.original || !!draft.reason.trim()) }
function scoreLabel(criterion?: Criterion) { return criterion?.score == null ? '无法判断' : `${criterion.score}/${criterion.max_score}` }

export function AssessmentEditor({ report, onSaved }: { report: Report; onSaved: () => Promise<void> }) {
  const state = useReportRevision(report, onSaved), original = originalData(report)
  const [draft, setDraft] = useState<Draft | null>(null), [version, setVersion] = useState<'original' | 'mine'>('mine')
  const formId = useId(), dirty = draftChanged(draft)
  useDraftGuard(dirty)
  const selected = draft?.mode === 'adopt' ? undefined : draft?.data.criteria?.find(item => item.id === draft.mode)
  const selectedOriginal = original.criteria?.find(item => item.id === selected?.id)
  const displaying = version === 'original' || !state.revision ? original : state.data
  const rows = original.criteria || [], total = scoreTotal(displaying)
  const observed = displaying.criteria?.filter(item => typeof item.score === 'number').length || 0
  function open(mode: string) { state.clearError(); setDraft(makeDraft(state.data, state.baseVersion, mode)) }
  function close() { if (!state.saving && (!dirty || window.confirm('有未保存的修正，确定放弃吗？'))) { setDraft(null); if (state.error) state.refresh() } }
  function update(patch: Partial<Criterion>) { if (draft && selected) setDraft({ ...draft, data: { ...draft.data, criteria: draft.data.criteria?.map(item => item.id === selected.id ? { ...item, ...patch } : item) } }) }
  async function submit(event: FormEvent) { event.preventDefault(); if (!draft || !draft.reason.trim()) return; const data = { ...draft.data, total_score: scoreTotal(draft.data) }; if (await state.save(data, draft.reason.trim(), draft.baseVersion)) { setDraft(null); setVersion('mine') } }
  const invalid = !!draft?.data.criteria?.some(item => item.score !== null && (!Number.isFinite(item.score) || item.score < 0 || item.score > item.max_score))
  return <>
    <Panel title="四项评分与依据" note="观察依据与评分规则默认展开，便于逐项核对。" action={<Badge tone={state.revision ? 'green' : 'neutral'}>{state.revision ? `我的第 ${state.revision.revision_no || 1} 版` : '待核对'}</Badge>}>
      <div className="score-summary"><div className="score-summary-copy"><span>综合得分</span><strong className="score-total">{total ?? '—'}<small>/100</small></strong><p>{observed}/4 项可判断 · 每项 25 分</p></div><div className="report-version-tabs" role="group" aria-label="查看评分版本"><button type="button" className={version === 'original' || !state.revision ? 'active' : ''} aria-pressed={version === 'original' || !state.revision} onClick={() => setVersion('original')}>AI 原稿</button><button type="button" className={version === 'mine' && state.revision ? 'active' : ''} aria-pressed={version === 'mine' && !!state.revision} disabled={!state.revision} onClick={() => setVersion('mine')}>我的版本</button></div></div>
      <div className="criteria-list">{rows.map((item, index) => { const current = state.data.criteria?.find(row => row.id === item.id); const visible = displaying.criteria?.find(row => row.id === item.id) || item; const isMine = version === 'mine' && !!state.revision; return <article className="criterion-card" key={item.id}><div className="criterion-card-head"><div className="criterion-title"><span>{String(index + 1).padStart(2, '0')}</span><div><h3>{item.label}</h3><small>本项满分 {item.max_score} 分</small></div></div><div className="criterion-score-group"><div><span>AI 原稿</span><strong>{scoreLabel(item)}</strong></div><div className={isMine ? 'active' : ''}><span>我的版本</span><strong>{state.revision ? scoreLabel(current) : '未记录'}</strong></div></div><Button variant="quiet" disabled={state.loading} onClick={() => open(item.id)} aria-label={`修正${item.label}`}><Pencil size={14} />修正</Button></div><div className="criterion-rationale"><section><h4>{isMine ? '我的观察依据' : 'AI 观察依据'}</h4><p>{visible.evidence || '未记录具体依据'}</p></section><section><h4>评分规则</h4><p>{visible.rule_text || visible.description || '规则文字未记录'}</p></section></div></article> })}</div>
      <div className="panel-body">{observed < 4 && <p className="muted">缺项保留“无法判断”，不合成总分。</p>}{state.loading && <Loading text="正在读取我的修正…" />}{state.error && !draft && <Message>{state.error}<Button variant="quiet" onClick={state.refresh}>重新读取</Button></Message>}{state.notice && <Message kind="info">{state.notice}</Message>}<div className="form-actions"><Button disabled={state.loading} onClick={() => open('adopt')}>{state.revision ? '补充核对依据' : '采用并记录依据'}</Button></div></div>
    </Panel>
    <Dialog open={!!draft} onOpenChange={open => { if (!open) close() }} title={draft?.mode === 'adopt' ? '记录评分依据' : `修正 · ${selected?.label || '评分'}`} description={draft?.mode === 'adopt' ? '保留当前四项得分，记录你核对图像后采用的依据。' : 'AI 原稿保留；修正将保存为你的新版本。'} busy={state.saving} footer={<><Button variant="secondary" disabled={state.saving} onClick={close}>取消</Button><Button type="submit" form={formId} disabled={state.saving || !draft?.reason.trim() || invalid}>{state.saving ? <LoaderCircle size={15} className="spin" /> : <Save size={15} />}保存记录</Button></>}>
      {draft && <form id={formId} onSubmit={submit} className="stack">{selected && <><div className="editor-original"><strong>AI 原稿 · {scoreLabel(selectedOriginal)}</strong><p>{selectedOriginal?.evidence}</p><Disclosure title="原始评分规则" defaultOpen><p>{selectedOriginal?.rule_text || selectedOriginal?.description || '规则未记录'}</p></Disclosure></div><Field label="我的得分" hint="留空表示无法判断。"><input type="number" min={0} max={selected.max_score} step="any" value={selected.score ?? ''} onChange={event => update({ score: event.target.value === '' ? null : Number(event.target.value) })} /></Field><Field label="我的观察依据"><textarea required value={selected.evidence} onChange={event => update({ evidence: event.target.value })} /></Field><Disclosure title="调整评分规则文字" meta="选填"><Field label="规则文字" hint="修改规则后，历史趋势将按不同评分口径分别统计。"><textarea value={selected.rule_text || selected.description || ''} onChange={event => update({ rule_text: event.target.value })} /></Field></Disclosure></>}<Field label={draft.mode === 'adopt' ? '采用依据' : '修正理由'}><textarea required value={draft.reason} onChange={event => setDraft({ ...draft, reason: event.target.value })} placeholder="说明图像或资料中的依据，以及仍需求证的地方。" /></Field>{state.error && <Message>{state.error}草稿已保留。</Message>}</form>}
    </Dialog>
  </>
}

export function UnderstandingEditor({ report, onSaved }: { report: Report; onSaved: () => Promise<void> }) {
  const state = useReportRevision(report, onSaved), original = originalData(report)
  const [draft, setDraft] = useState<Draft | null>(null), [version, setVersion] = useState<'original' | 'mine'>('mine')
  const formId = useId(), dirty = draftChanged(draft)
  useDraftGuard(dirty)
  const displaying = version === 'mine' && state.revision ? state.data : original
  const phase = displaying.final_phase || displaying.phase || displaying.candidates?.[0]?.name || '证据不足，暂不判断'
  const selected = draft?.data.dimensions?.find(item => item.name === draft.mode)
  const originalDimension = original.dimensions?.find(item => item.name === selected?.name)
  function open(mode: string) {
    state.clearError()
    const data = { ...state.data, final_phase: state.data.final_phase || state.data.phase || state.data.candidates?.[0]?.name || '证据不足，暂不判断' }
    setDraft(makeDraft(data, state.baseVersion, mode))
  }
  function close() { if (!state.saving && (!dirty || window.confirm('有未保存的理解，确定放弃吗？'))) { setDraft(null); if (state.error) state.refresh() } }
  function update(patch: Partial<Dimension>) { if (draft && selected) setDraft({ ...draft, data: { ...draft.data, dimensions: draft.data.dimensions?.map(item => item.name === selected.name ? { ...item, ...patch } : item) } }) }
  async function submit(event: FormEvent) { event.preventDefault(); if (!draft || !draft.reason.trim()) return; if (await state.save(draft.data, draft.reason.trim(), draft.baseVersion)) { setDraft(null); setVersion('mine') } }
  return <>
    <Panel title="组织判断与五维理解" action={<Badge tone={state.revision ? 'green' : 'neutral'}>{state.revision ? `我的第 ${state.revision.revision_no || 1} 版` : '待核对'}</Badge>}>
      <div className="panel-body"><div className="report-version-tabs" role="group" aria-label="查看理解版本"><button type="button" className={version === 'original' || !state.revision ? 'active' : ''} aria-pressed={version === 'original' || !state.revision} onClick={() => setVersion('original')}>AI 原稿</button><button type="button" className={version === 'mine' && state.revision ? 'active' : ''} aria-pressed={version === 'mine' && !!state.revision} disabled={!state.revision} onClick={() => setVersion('mine')}>我的版本</button></div></div>
      <div className="understanding-judgment">
        <div className="judgment-heading"><div><span className="muted">{version === 'mine' && state.revision ? '我的组织判断' : 'AI 原始候选'}</span><h3>{phase}</h3></div><Button variant="secondary" disabled={state.loading} onClick={() => open('phase')}><Pencil size={14} />修正判断</Button></div>
        {version === 'mine' && state.revision ? <><div className="judgment-status"><Badge tone={phase !== (original.candidates?.[0]?.name || '') ? 'cyan' : 'green'}>{phase !== (original.candidates?.[0]?.name || '') ? '已修正判断' : '已记录依据'}</Badge><span>第 {state.revision.revision_no} 版 · <RecordDate value={state.revision.created_at} /></span>{report.latest_review?.decision === 'confirmed' && report.latest_review.revision_no === state.baseVersion ? <Badge tone="green">教师已确认本版</Badge> : <Badge>待教师复核</Badge>}</div><div className="judgment-evidence"><h4>我的判断依据</h4><p>{displaying.phase_reason || state.revision.reason || '未单独记录组织判断依据。'}</p>{state.revision.reason && <div className="judgment-reason"><strong>记录理由</strong><p>{state.revision.reason}</p></div>}</div><p className="judgment-followup">组织判断修改后，请核对下方五维解释；保存不会自动改写其他维度。</p></> : <p className="judgment-evidence">{original.candidates?.[0]?.evidence || original.summary}</p>}
        <Disclosure title="对照 AI 原始候选" meta={`${original.candidates?.length || 0} 项`}><div className="candidate-list">{original.candidates?.map((candidate, index) => <section className="candidate-card" key={index}><div className="row row-between"><h4>{candidate.name}</h4>{candidate.name === phase && <Badge tone="cyan">当前判断</Badge>}</div><p>{candidate.evidence}</p>{candidate.uncertainty && <p className="muted">待求证：{candidate.uncertainty}</p>}</section>)}</div></Disclosure>
      </div>
      <div className="understanding-dimensions">{(displaying.dimensions || []).map((item, index) => { const prior = original.dimensions?.find(row => row.name === item.name); const changed = version === 'mine' && !!state.revision && (prior?.content !== item.content || prior?.basis !== item.basis); return <section className="understanding-dimension" key={item.name}><div className="dimension-card-heading"><div className="row"><span className="dimension-number">0{index + 1}</span><h3>{item.name}</h3></div><Button variant="quiet" disabled={state.loading} onClick={() => open(item.name)} aria-label={`修正${item.name}理解`}><Pencil size={14} />修正</Button></div><div className="row dimension-badges"><BasisBadge basis={item.basis} />{changed && <Badge tone="cyan">已修正</Badge>}</div><p className="dimension-card-text">{item.content}</p><Disclosure title={changed ? '查看原稿对照与完整内容' : '查看完整理解'}><p className="source-text">{item.content}</p>{changed && <div className="editor-original space-top"><strong>AI 原稿</strong><p>{prior?.content}</p></div>}</Disclosure></section> })}</div>
      <div className="panel-body">{state.loading && <Loading text="正在读取我的理解…" />}{state.error && !draft && <Message>{state.error}<Button variant="quiet" onClick={state.refresh}>重新读取</Button></Message>}{state.notice && <Message kind="info">{state.notice}</Message>}<div className="form-actions"><Button disabled={state.loading} onClick={() => open('adopt')}>{state.revision ? '补充核对依据' : '采用并记录依据'}</Button></div></div>
    </Panel>
    <Dialog open={!!draft} onOpenChange={open => { if (!open) close() }} wide title={draft?.mode === 'phase' ? '修正组织判断' : selected ? `修正 · ${selected.name}理解` : '记录采用依据'} description="保留 AI 原稿，将当前理解与依据保存为你的新版本。" busy={state.saving} footer={<><Button variant="secondary" disabled={state.saving} onClick={close}>取消</Button><Button type="submit" form={formId} disabled={state.saving || !draft?.reason.trim()}>{state.saving ? <LoaderCircle size={15} className="spin" /> : <Save size={15} />}保存我的理解</Button></>}>
      {draft && <form id={formId} onSubmit={submit} className="stack">{draft.mode === 'phase' && <div className="judgment-editor-grid"><div className="editor-original"><strong>AI 原始候选</strong>{original.candidates?.map((item, index) => <p key={index}>{item.name}：{item.evidence}</p>)}</div><div className="stack"><Field label="组织判断"><input required value={draft.data.final_phase || ''} onChange={event => setDraft({ ...draft, data: { ...draft.data, final_phase: event.target.value } })} placeholder="证据不足时可明确暂不判断" /></Field><Field label="判断依据"><textarea required value={draft.data.phase_reason || ''} onChange={event => setDraft({ ...draft, data: { ...draft.data, phase_reason: event.target.value } })} /></Field></div></div>}{selected && <><div className="editor-original"><strong>AI 原稿</strong><p>{originalDimension?.content}</p><BasisBadge basis={originalDimension?.basis} /></div><Field label="依据类型"><select value={selected.basis || 'inferred'} onChange={event => update({ basis: event.target.value as Dimension['basis'] })}><option value="known">背景信息</option><option value="observed">图像观察</option><option value="inferred">资料推断</option></select></Field><Field label="我的理解"><textarea required value={selected.content} onChange={event => update({ content: event.target.value })} /></Field></>}<Field label={draft.mode === 'adopt' ? '采用依据' : '修正理由'}><textarea required value={draft.reason} onChange={event => setDraft({ ...draft, reason: event.target.value })} placeholder="记录形貌、背景或资料中的依据；保留待求证之处。" /></Field>{state.error && <Message>{state.error}草稿已保留。</Message>}</form>}
    </Dialog>
  </>
}

export function BasisBadge({ basis }: { basis?: Dimension['basis'] }) {
  return <Badge tone={basis === 'observed' ? 'cyan' : basis === 'inferred' ? 'amber' : 'neutral'}>{basis === 'known' ? '背景信息' : basis === 'observed' ? '图像观察' : basis === 'inferred' ? '资料推断' : '依据待补充'}</Badge>
}
