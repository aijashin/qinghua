import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowRight, BookOpen, FileImage, Plus, Search } from 'lucide-react'
import { Badge, Button, Empty, Field, Loading, Message, PageTitle, Panel, RecordDate } from './WorkbenchUI'
import { ReportHistory } from './ReportHistory'
import { useDraftGuard } from '../unsaved'
import { BasisBadge, BackToList, JobRunner, ReportDetail, SampleImage } from './LearningWorkspace'
import { errorMessage, imageUrl, list, request, type AtlasEntry, type Attempt, type AuthSession, type Criterion, type Insights, type LearningSession, type PracticeTask, type Report, type Sample } from '../api'

export function Trend({ insights }: { insights: Insights }) {
  const [groupId, setGroupId] = useState('')
  const groups = insights.score_groups || []
  const group = groups.find(item => (item.score_group_id || item.rubric_id) === groupId) || groups[0]
  const history = [...(group ? group.score_history : insights.score_history)].sort((a, b) => a.date.localeCompare(b.date)).slice(-10)
  return <Panel title="练习趋势" note="只呈现已有学习记录；不同评分规则分别查看。" action={groups.length > 1 ? <select style={{ width: 160 }} value={group?.score_group_id || group?.rubric_id} onChange={event => setGroupId(event.target.value)}>{groups.map(item => <option key={item.score_group_id || item.rubric_id} value={item.score_group_id || item.rubric_id}>{item.rubric_title}{item.demo ? ' · 合成示意图' : ' · 真实样本'}</option>)}</select> : undefined}>{!history.length ? <Empty title="还没有可比较的评分">完成真实评图后，这里会逐次记录变化。</Empty> : <><div className="trend">{history.map((item, index) => <div className="trend-item" key={`${item.sample_id}-${index}`}><strong>{item.score}</strong><i style={{ height: `${Math.max(2, Math.min(100, item.score))}%` }} /><small>{new Date(item.date).toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric' })}</small></div>)}</div><div className="panel-footer"><p>{group?.rubric_title || '当前教学规则'}{group?.demo ? ' · 合成示意图' : ' · 真实样本'} · {history.length} 条已保存评分记录</p><Badge>仅用于学习反馈</Badge></div></>}</Panel>
}

export function PracticeView({ sample, session, reports, configured, reloadReports, onNext, onCompleted }: { sample: Sample; session: LearningSession | null; reports: Report[]; configured: boolean; reloadReports: () => Promise<void>; onNext: (session: LearningSession, taskId?: string) => void; onCompleted: (session: LearningSession) => void }) {
  const [insights, setInsights] = useState<Insights | null>(null)
  const [tasks, setTasks] = useState<PracticeTask[]>([])
  const [currentUserId, setCurrentUserId] = useState('')
  const [previousRound, setPreviousRound] = useState<{ session: LearningSession; sample: Sample; reports: Report[] } | null>(null)
  const [parentError, setParentError] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [nextTaskId, setNextTaskId] = useState('')
  const coaching = reports.find(item => item.kind === 'coaching')
  const reload = useCallback(async () => {
    try {
      const [summary, taskList, identity] = await Promise.all([request<Insights>('/insights'), list<PracticeTask>('/practice-tasks'), request<AuthSession>('/auth/session')])
      setInsights(summary); setCurrentUserId(identity.user?.id || '')
      setTasks(taskList.filter(task => task.assignee_ids?.includes(identity.user?.id || '')))
    } catch (err) { setError(errorMessage(err)) }
  }, [])
  useEffect(() => { void Promise.resolve().then(reload) }, [reload])
  useEffect(() => {
    if (!session?.parent_session_id) return
    let alive = true
    void request<LearningSession>(`/learning-sessions/${session.parent_session_id}`).then(async parent => {
      const [record, results] = await Promise.all([
        parent.sample_snapshot ? Promise.resolve(parent.sample_snapshot) : request<Sample>(`/samples/${parent.sample_id}`),
        list<Report>(`/reports?sample_id=${encodeURIComponent(parent.sample_id)}`),
      ])
      if (alive) { setPreviousRound({ session: parent, sample: record, reports: results.filter(item => item.learning_session_id === parent.id) }); setParentError('') }
    }).catch(err => { if (alive) setParentError(errorMessage(err)) })
    return () => { alive = false }
  }, [session?.parent_session_id])
  async function complete() {
    if (!session) return
    setBusy(true); setError('')
    try {
      const updated = await request<LearningSession>(`/learning-sessions/${session.id}`, { method: 'PATCH', body: { step: 'practice', completed: true } })
      onCompleted(updated); await reload()
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  return <>
    {error && <Message>{error}</Message>}
    {insights && <>
      <div className="notice-strip">统计范围：{insights.period?.label || '全部已授权记录'}{insights.period?.from && <> · <RecordDate value={insights.period.from} /> 至 <RecordDate value={insights.period.to} /></>}。已保存 {insights.revision_count ?? '—'} 次修正，涉及 {insights.revised_report_count ?? '—'} 份报告。</div>
      <div className="metrics"><div className="stat"><span>已保存样本</span><strong>{insights.sample_count}</strong><small>包括上传样本与选用示例</small></div><div className="stat"><span>本轮状态</span><strong style={{ fontSize: 20 }}>{session?.completed ? '已归档' : '待归档'}</strong><small>完成后进入个人图谱</small></div><div className="stat"><span>待完成训练</span><strong>{insights.assignment_count == null || insights.completed_assignment_count == null ? '—' : Math.max(0, insights.assignment_count - insights.completed_assignment_count)}</strong><small>只统计分配给我的已发布任务</small></div><div className="stat"><span>已完成训练</span><strong>{insights.completed_assignment_count ?? '—'}</strong><small>记录反思与验证</small></div></div>
      <div className="two-col"><Trend insights={insights} /><Panel title="重复出现的问题" ><div className="panel-body">{insights.defects.length ? insights.defects.map(item => <div className="finding row row-between" key={item.name}><h3>{item.name}</h3><Badge tone="amber">{item.count} 次</Badge></div>) : <Empty title="暂未形成重复问题记录" />}</div></Panel></div>
    </>}
    {session?.parent_session_id && <Panel title="上一轮与本轮对照" note="对照原图、判断与评分规则。">{parentError ? <div className="panel-body"><Message>{parentError}</Message></div> : previousRound?.session.id === session.parent_session_id ? <div className="panel-body"><RoundComparison previous={previousRound} current={{ sample, reports }} /></div> : <Loading text="正在读取上一轮授权记录…" />}</Panel>}
    <Panel title="下一轮训练"><div className="panel-body"><JobRunner sample={sample} session={session} kind="coaching" configured={configured} onComplete={reloadReports} label="生成针对本次样本的训练建议" />{coaching && <><p>{coaching.data.summary}</p>{coaching.data.verification_plan?.map((item, index) => <div className="finding" key={index}><h3>验证 {index + 1}</h3><p>{item}</p></div>)}{coaching.data.questions?.map((item, index) => <div className="citation" key={index}>{item}</div>)}</>}<PracticeTaskForm sampleId={sample.id} reportId={coaching?.id} onSaved={reload} /></div></Panel>
    <Panel title="我的训练任务" note="保存回答后，可记录反思并完成任务。"><div className="panel-body">{tasks.length ? tasks.map(task => <StudentTask key={task.id} task={task} userId={currentUserId} reload={reload} />) : <Empty title="还没有训练任务">创建一个本次要验证的小任务，或等待教师发布。</Empty>}</div></Panel>
    <div className="panel-body"><Field label="下一轮关联的验证任务（选填）"><select value={nextTaskId} onChange={event => setNextTaskId(event.target.value)}><option value="">不关联任务</option>{tasks.filter(task => task.status === 'published').map(task => <option key={task.id} value={task.id}>{task.title}</option>)}</select></Field></div>
    <div className="panel-footer"><div><h3>{session?.completed ? '本轮已保存到个人图谱' : '为本轮学习留下一份完整记录'}</h3><p>保留样本、AI 原稿、你的修正与后续训练。</p></div><div className="row">{!session?.completed && <Button onClick={complete} disabled={busy || !session}>完成本轮并归档</Button>}<Button variant={session?.completed ? 'primary' : 'secondary'} onClick={() => session && onNext(session, nextTaskId || undefined)} disabled={!session?.completed}>准备下一轮样本 <ArrowRight size={15} /></Button></div></div>
  </>
}

function PracticeTaskForm({ sampleId, reportId, onSaved }: { sampleId: string; reportId?: string; onSaved: () => Promise<void> }) {
  const [open, setOpen] = useState(false), [title, setTitle] = useState(''), [instruction, setInstruction] = useState(''), [error, setError] = useState(''), [busy, setBusy] = useState(false)
  async function save() { setBusy(true); setError(''); try { await request('/practice-tasks', { method: 'POST', body: { title, instruction, sample_id: sampleId, report_id: reportId, status: 'published' } }); setOpen(false); setTitle(''); setInstruction(''); await onSaved() } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) } }
  if (!open) return <Button variant="secondary" onClick={() => setOpen(true)}><Plus size={15} />记录下一次验证任务</Button>
  return <div className="stack space-top"><Field label="任务名称"><input value={title} onChange={event => setTitle(event.target.value)} placeholder="例如：换砂纸前确认旧划痕" /></Field><Field label="如何做、用什么证据验证"><textarea value={instruction} onChange={event => setInstruction(event.target.value)} /></Field>{error && <Message>{error}</Message>}<div className="form-actions"><Button variant="secondary" onClick={() => setOpen(false)}>取消</Button><Button onClick={save} disabled={busy || !title.trim() || !instruction.trim()}>保存我的任务</Button></div></div>
}

function StudentTask({ task, userId, reload }: { task: PracticeTask; userId: string; reload: () => Promise<void> }) {
  const [answer, setAnswer] = useState('')
  const [savedAnswer, setSavedAnswer] = useState('')
  const [history, setHistory] = useState<Attempt[]>([])
  const [historyLoading, setHistoryLoading] = useState(true)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const assignment = task.assignments?.find(item => item.user_id === userId)
  const complete = assignment?.status === 'completed' || task.status === 'completed'
  const loadHistory = useCallback(async () => {
    try { setHistory((await list<Attempt>(`/practice-tasks/${task.id}/attempts`)).filter(item => item.owner_id === userId).sort((a, b) => b.created_at.localeCompare(a.created_at))) }
    catch (err) { setError(errorMessage(err)) } finally { setHistoryLoading(false) }
  }, [task.id, userId])
  useEffect(() => { void Promise.resolve().then(loadHistory) }, [loadHistory])
  useDraftGuard(!complete && !!answer.trim() && answer !== savedAnswer)
  async function act(finish: boolean) {
    setBusy(true); setError(''); setMessage('')
    const submittedAnswer = answer
    try {
      await request(`/practice-tasks/${task.id}/${finish ? 'complete' : 'attempts'}`, { method: 'POST', body: finish ? { reflection: submittedAnswer } : { answer: submittedAnswer } })
      setSavedAnswer(submittedAnswer)
      setMessage(finish ? '已记录任务完成与反思。' : '回答已保存，尚未作自动评分。')
      await Promise.all([reload(), loadHistory()])
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  return <div className="finding">
    <div className="row row-between"><h3>{task.title}</h3><Badge tone={complete ? 'green' : 'amber'}>{complete ? '已完成' : task.status === 'draft' ? '草稿' : '待验证'}</Badge></div>
    <p className="wrap-text">{task.instruction}</p>
    {!complete && task.status !== 'draft' && <><textarea className="space-top" disabled={busy} value={answer} onChange={event => setAnswer(event.target.value)} placeholder="记录实际操作、实验结果和仍有疑问的地方。读取历史不会覆盖这里的草稿。" /><div className="form-actions"><Button variant="secondary" onClick={() => act(false)} disabled={busy || !answer.trim()}>保存回答</Button><Button onClick={() => act(true)} disabled={busy || !answer.trim()}>完成任务并保存反思</Button></div></>}
    {message && <Message kind="success">{message}</Message>}{error && <Message>{error}</Message>}
    {complete && <div className="citation"><strong>我的完成反思 · <RecordDate value={assignment?.completed_at} /></strong><p className="wrap-text">{assignment?.reflection || '该次完成记录未填写反思。'}</p></div>}
    <div className="row row-between space-top"><h3>我的提交历史</h3><Button variant="quiet" disabled={busy || historyLoading} onClick={loadHistory}>刷新提交</Button></div>
    {historyLoading ? <Loading text="正在读取我的已保存回答…" /> : history.length ? history.map(item => <div className="citation" key={item.id}><div className="row row-between"><strong>提交于 <RecordDate value={item.created_at} /></strong>{!complete && !answer && <Button variant="quiet" className="compact" onClick={() => { setAnswer(item.answer); setSavedAnswer(item.answer) }}>以这次回答继续编辑</Button>}</div><p className="wrap-text">{item.answer}</p></div>) : <p className="rule-text">还没有单独保存的回答。完成反思与回答历史分别记录。</p>}
  </div>
}

type ScoredReport = Report & { rubric?: { id?: string; title?: string; version?: number; criteria?: Array<Partial<Criterion>> } }
function latestReport(reports: Report[], kind: Report['kind']) {
  return reports.filter(item => item.kind === kind).sort((a, b) => b.created_at.localeCompare(a.created_at))[0] as ScoredReport | undefined
}
// Match effective rule text and immutable rubric identity, excluding score values.
function comparisonKey(report?: ScoredReport) {
  if (!report) return null
  const data = report.latest_revision?.data || report.data
  const rubric = report.rubric
  const id = rubric?.id || data.rubric_id
  if (!id || !data.criteria?.length) return null
  const criteria = data.criteria.map(item => {
    const base = rubric?.criteria?.find(rule => rule.id === item.id)
    return { id: item.id, max_score: item.max_score, rule_text: (item.rule_text ?? item.description ?? base?.rule_text ?? base?.description ?? '').trim() }
  }).sort((a, b) => a.id.localeCompare(b.id))
  return JSON.stringify({ rubric_id: id, version: rubric?.version ?? null, demo: report.demo || report.sample_snapshot?.demo || false, criteria })
}
function RoundComparison({ previous, current }: { previous: { sample: Sample; reports: Report[] }; current: { sample: Sample; reports: Report[] } }) {
  const before = latestReport(previous.reports, 'assessment'), after = latestReport(current.reports, 'assessment')
  const beforeScore = (before?.latest_revision?.data || before?.data)?.total_score
  const afterScore = (after?.latest_revision?.data || after?.data)?.total_score
  const sameRules = comparisonKey(before) !== null && comparisonKey(before) === comparisonKey(after)
  const comparable = sameRules && typeof beforeScore === 'number' && typeof afterScore === 'number'
  return <><div className="comparison"><RoundEvidence title="上一轮" sample={previous.sample} reports={previous.reports} /><RoundEvidence title="本轮" sample={current.sample} reports={current.reports} /></div><Message kind="info">{comparable ? '两轮评分规则版本、有效规则文字与来源类型一致，可并列参照分值。这里不据此生成改善幅度、能力提升或性能变化结论。' : !before || !after ? '有一轮尚无完整评图报告，暂不比较分值。' : !sameRules ? '两轮规则版本、修正规则文字或来源类型不同，分值不能直接比较；请分别核对依据。' : '两轮评分口径一致，但存在无法判断的评分项；保留缺失值，不比较总分。'}</Message></>
}
function RoundEvidence({ title, sample, reports }: { title: string; sample: Sample; reports: Report[] }) {
  const assessment = latestReport(reports, 'assessment'), understanding = latestReport(reports, 'understanding')
  const score = assessment?.latest_revision?.data || assessment?.data
  const interpretation = understanding?.latest_revision?.data || understanding?.data
  const phase = interpretation?.final_phase || interpretation?.phase || interpretation?.candidates?.[0]?.name
  return <div>
    <h3>{title} · {sample.material}</h3><SampleImage sample={sample} label={sample.title} />
    <dl className="definition-list space-top">
      <div><dt>组织判断</dt><dd>{phase || '尚未形成组织判断'}{understanding && <div className="space-top"><Badge tone={understanding.latest_revision ? 'cyan' : 'amber'}>{understanding.latest_revision ? '已保存个人理解' : 'AI 候选，尚未采用或修正'}</Badge></div>}</dd></div>
      <div><dt>判断理由</dt><dd>{interpretation?.phase_reason || understanding?.latest_revision?.reason || '尚未填写个人判断理由'}</dd></div>
      <div><dt>关键证据</dt><dd>{interpretation?.candidates?.[0]?.evidence || interpretation?.summary || '尚无组织判断证据'}</dd></div>
      <div><dt>评分</dt><dd>{typeof score?.total_score === 'number' ? `${score.total_score}/${score.max_score || 100}` : '无法形成完整总分'}{assessment && <div className="space-top"><Badge>{assessment.latest_revision ? '学生采用 / 修正稿' : 'AI 原始评分'}</Badge></div>}</dd></div>
      <div><dt>规则版本</dt><dd>{assessment?.rubric?.title || score?.rubric_id || '尚未记录规则'}{assessment?.rubric?.version != null && <> · v{assessment.rubric.version}</>}<small className="muted" style={{ display: 'block' }}>{assessment?.rubric?.id || score?.rubric_id || ''}</small></dd></div>
    </dl>
    {score?.criteria?.length ? <details className="space-top"><summary className="link-button">展开逐项分值、规则与依据</summary>{score.criteria.map(item => <div className="citation" key={item.id}><strong>{item.label} · {item.score == null ? '无法判断' : `${item.score}/${item.max_score}`}</strong>{item.rule_text && <p>规则：{item.rule_text}</p>}<p>{item.evidence}</p></div>)}</details> : null}
  </div>
}

export function LibraryView({ onResume }: { onResume: (sampleId: string, sessionId?: string) => void }) {
  const [tab, setTab] = useState<'personal' | 'reference'>('personal'), [entries, setEntries] = useState<AtlasEntry[]>([]), [selected, setSelected] = useState<AtlasEntry | null>(null), [query, setQuery] = useState(''), [busy, setBusy] = useState(true), [error, setError] = useState(''), [details, setDetails] = useState<Report[]>([])
  useEffect(() => { let alive = true; void list<AtlasEntry>(tab === 'personal' ? '/personal-atlas' : '/atlas').then(items => { if (alive) setEntries(items) }).catch(err => { if (alive) setError(errorMessage(err)) }).finally(() => { if (alive) setBusy(false) }); return () => { alive = false } }, [tab])
  const filtered = entries.filter(item => `${item.title} ${item.material} ${item.phase} ${item.process}`.toLowerCase().includes(query.toLowerCase()))
  async function open(entry: AtlasEntry) { setSelected(entry); setDetails(Array.isArray(entry.reports) ? entry.reports : Object.values(entry.reports || {})); setError(''); if (!entry.reports && (entry.sample_id || entry.sample?.id)) try { setDetails(await list<Report>(`/reports?sample_id=${entry.sample_id || entry.sample?.id}`)) } catch (err) { setError(errorMessage(err)) } }
  if (selected) return <><div className="detail-banner"><BackToList onClick={() => setSelected(null)} />{selected.sample_id && <Button onClick={() => onResume(selected.sample_id!, selected.learning_session_id)}>打开完整学习记录 <ArrowRight size={14} /></Button>}</div><PageTitle label={tab === 'personal' ? 'PERSONAL ATLAS' : 'REFERENCE ATLAS'} title={selected.title} description={`${selected.material} · ${selected.phase || '组织待确认'} · ${selected.process}`} /><Panel><div className="sample-workbench"><SampleImage sample={selected.sample_snapshot || selected.sample || selected} label={selected.title} /><div><dl className="definition-list">{[['材料', selected.material], ['工艺', selected.process], ['倍率', selected.magnification || selected.sample_snapshot?.magnification], ['腐蚀剂', selected.etchant || selected.sample_snapshot?.etchant], ['来源', selected.source || '本人学习记录']].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value || '未记录'}</dd></div>)}</dl><p className="space-top">{selected.observation || selected.description}</p>{selected.demo && <div className="space-top"><Badge tone="amber">合成示意图 · 不作为实验结果</Badge></div>}</div></div></Panel>{error && <Message>{error}</Message>}{details.map(report => <div key={report.id}><ReportDetail report={report} /><ReportHistory key={`history-${report.id}`} report={report} />{report.latest_revision && <Panel title="归档时的修正" note={report.latest_revision.reason}><div className="panel-body"><p>{report.latest_revision.data?.summary}</p>{report.latest_revision.data?.criteria?.map(item => <div className="finding" key={item.id}><h3>{item.label} · {item.score == null ? '无法判断' : `${item.score}/${item.max_score}`}</h3><p>{item.evidence}</p></div>)}{report.latest_revision.data?.dimensions?.map(item => <div className="finding" key={item.name}><div className="row"><h3>{item.name}</h3><BasisBadge basis={item.basis} /></div><p>{item.content}</p></div>)}</div></Panel>}</div>)}</>
  return <><PageTitle label="OBSERVE · COLLECT · REVISIT" title="我的图谱与参考库" description="查阅学习记录与已发布图谱。" /><div className="tabs"><button className={tab === 'personal' ? 'active' : ''} onClick={() => setTab('personal')}>个人图谱</button><button className={tab === 'reference' ? 'active' : ''} onClick={() => setTab('reference')}>参考图谱</button></div><div className="toolbar"><Search size={17} className="muted" /><input aria-label="搜索图谱" value={query} onChange={event => setQuery(event.target.value)} placeholder="按材料、组织、工艺搜索" /><span className="muted">{filtered.length} 条记录</span></div>{error && <Message>{error}</Message>}{busy ? <Loading /> : !filtered.length ? <Panel><Empty title={tab === 'personal' ? '你的图谱，从一次完整练习开始' : '还没有匹配的参考图谱'}>{tab === 'personal' ? '在炉火纯青完成本轮学习，样本和理解将自动收进个人图谱。' : '试试其他关键词，或请教师发布带来源的图谱。'}</Empty></Panel> : <div className="records">{filtered.map(item => <button className="record-card" key={item.id} onClick={() => open(item)}><div className="record-thumb">{imageUrl(item.sample_snapshot || item.sample || item) ? <img src={imageUrl(item.sample_snapshot || item.sample || item)} alt={item.title} /> : <FileImage size={35} />}</div><div className="record-info"><h3>{item.title}</h3><p>{item.material} · {item.phase || '组织待确认'}</p><p>{item.process}</p><div className="row row-between"><Badge tone={item.demo ? 'amber' : 'cyan'}>{item.demo ? '合成示意图' : tab === 'personal' ? '本人记录' : '参考图谱'}</Badge><span style={{ fontSize: 12 }} className="muted"><RecordDate value={item.created_at} /></span></div></div></button>)}</div>}</>
}

export function IdentifyTool({ samples, configured, onPrepare }: { samples: Sample[]; configured: boolean; onPrepare: () => void }) {
  const [selected, setSelected] = useState(samples[0]?.id || ''), [reports, setReports] = useState<Report[]>([]), [error, setError] = useState('')
  const sample = samples.find(item => item.id === selected)
  const loadGeneration = useRef(0)
  const reload = useCallback(async () => {
    if (!selected) return
    const generation = ++loadGeneration.current
    try {
      const items = await list<Report>(`/reports?sample_id=${encodeURIComponent(selected)}`)
      if (generation === loadGeneration.current) { setReports(items.filter(item => item.kind === 'identify')); setError('') }
    } catch (err) {
      if (generation === loadGeneration.current) { setError(errorMessage(err)); throw err }
    }
  }, [selected])
  const invalidate = useCallback(() => { loadGeneration.current++ }, [])
  useEffect(() => { void Promise.resolve().then(reload).catch(() => {}); return invalidate }, [reload, invalidate])
  return <><PageTitle label="IMAGE READING" title="智能识图" description="选择已保存的图像，查看候选组织与证据。" action={<Button variant="secondary" onClick={onPrepare}><Plus size={15} />准备新样本</Button>} />{!samples.length ? <Panel><Empty title="先保存一张样本照片" action={<Button onClick={onPrepare}>前往组织图谱</Button>}>记录材料和工艺背景，有助于区分相似形貌。</Empty></Panel> : <><div className="toolbar"><BookOpen size={18} className="muted" /><select aria-label="选择识图样本" value={selected} onChange={event => { loadGeneration.current++; setReports([]); setError(''); setSelected(event.target.value) }}>{samples.map(item => <option key={item.id} value={item.id}>{item.title || item.material}</option>)}</select></div>{error && <Message>{error}</Message>}{sample && <div className="two-col"><Panel title="当前样本"><div className="panel-body"><SampleImage sample={sample} label={sample.title} /><dl className="definition-list space-top"><div><dt>材料</dt><dd>{sample.material}</dd></div><div><dt>工艺</dt><dd>{sample.process}</dd></div></dl></div></Panel><div><JobRunner sample={sample} kind="identify" configured={configured} onComplete={reload} label="识别候选组织" />{reports[0] ? <ReportDetail report={reports[0]} /> : <Panel><Empty title="尚未生成识图记录">证据不足时会明确保留不确定项。</Empty></Panel>}</div></div>}</>}</>
}
