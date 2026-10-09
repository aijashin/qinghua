import { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, Eye, MessageSquare, Plus, Save, Send, Users } from 'lucide-react'
import { Badge, Button, Empty, Field, Loading, Message, PageTitle, Panel, RecordDate } from './WorkbenchUI'
import { BasisBadge, BackToList, ReportDetail, SampleImage } from './LearningWorkspace'
import { Trend } from './StudentRecords'
import { errorMessage, list, request, type Attempt, type ClassRecord, type Insights, type LearningSession, type PracticeTask, type Report, type Review, type Sample } from '../api'

export function TeacherWorkspace() {
  const [tab, setTab] = useState('records')
  const [reports, setReports] = useState<Report[]>([])
  const [classes, setClasses] = useState<ClassRecord[]>([])
  const [classId, setClassId] = useState('')
  const [selected, setSelected] = useState<Report | null>(null)
  const [insights, setInsights] = useState<Insights | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [generating, setGenerating] = useState(false)
  const [generatedDraft, setGeneratedDraft] = useState<PracticeTask | null>(null)
  const reload = useCallback(async () => {
    setError('')
    try {
      const [items, groups, summary] = await Promise.all([
        list<Report>(`/teacher/submissions${classId ? '?class_id=' + encodeURIComponent(classId) : ''}`),
        list<ClassRecord>('/classes'),
        request<Insights>(`/insights?include_demo=false${classId ? '&class_id=' + encodeURIComponent(classId) : ''}`),
      ])
      setReports(items); setClasses(groups); setInsights(summary)
    } catch (err) { setError(errorMessage(err)) } finally { setLoading(false) }
  }, [classId])
  useEffect(() => { void Promise.resolve().then(reload) }, [reload])
  async function open(report: Report) {
    setSelected(report)
    try { await request(`/reports/${report.id}/viewed`, { method: 'POST' }) } catch (err) { setError(errorMessage(err)) }
  }
  async function generateDraft() {
    if (!classId || generating) return
    setGenerating(true); setError('')
    try {
      const current = await request<Insights>(`/insights?class_id=${encodeURIComponent(classId)}&include_demo=false`)
      const issues = current.defects.filter(item => item.count > 0).sort((a, b) => b.count - a.count)
      setInsights(current)
      if (!issues.length) throw new Error('当前班级尚无已记录的共性问题，暂不能生成干预草稿。')
      const name = classes.find(item => item.id === classId)?.name || '当前班级'
      const focus = issues[0]
      const instruction = [
        `草稿依据：${name}已保存的真实样本记录（不含合成示意图）。`,
        `已记录问题：${issues.slice(0, 3).map(item => `${item.name}（${item.count} 条记录）`).join('；')}。`,
        `本次仅聚焦“${focus.name}”，以下是基于记录整理的练习模板，需教师核对后发布。`,
        '',
        '1. 回看自己已提交的原图，指出该问题的可见证据；没有观察到时明确说明。',
        '2. 选择一项准备调整的制样或观察步骤，写清调整理由和保持不变的条件。',
        '3. 制备并上传下一轮样本，关联本任务，保留前后图像对照。',
        '4. 提交实际操作、观察结果和反思；说明问题是否改善及依据，证据不足时保留不确定。',
      ].join('\n')
      const draft = await request<PracticeTask>('/practice-tasks', { method: 'POST', body: { title: `${focus.name} · 对照验证练习`, instruction, class_id: classId, assignee_ids: [], status: 'draft' } })
      setGeneratedDraft(draft); setTab('tasks')
    } catch (err) { setError(errorMessage(err)) } finally { setGenerating(false) }
  }
  if (selected) return <TeacherRecord report={selected} onBack={() => { setSelected(null); void reload() }} onUpdate={setSelected} />
  const period = insights?.period
  return <>
    <PageTitle label="TEACHER WORKSPACE" title="看见每一次理解的变化" description="从学生的观察、修正与练习记录出发，提供具体反馈。查看、指导和复核分别记录。" action={<Badge tone="cyan"><Users size={12} />教师工作台</Badge>} />
    <div className="tabs">{[['records', '学习记录'], ['insights', '班级学情'], ['tasks', '干预与训练']].map(([id, label]) => <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)}>{label}</button>)}</div>
    {error && <Message>{error}</Message>}
    {tab !== 'tasks' && <div className="toolbar"><select aria-label="筛选班级" disabled={generating} value={classId} onChange={event => setClassId(event.target.value)}><option value="">全部授权班级</option>{classes.map(item => <option value={item.id} key={item.id}>{item.name}</option>)}</select><Button variant="secondary" onClick={reload}>刷新</Button></div>}
    {loading ? <Loading /> : tab === 'records' ? <Panel title="学生学习记录" note="仅显示你有权限查看的班级记录。">
      {!reports.length ? <Empty title="暂时没有学习记录">学生完成 AI 分析后，可在这里查看原稿、学生修正和证据。</Empty> : <div className="table-wrap"><table className="table"><thead><tr><th>学生 / 样本</th><th>模块</th><th>提交时间</th><th>复核状态</th><th>操作</th></tr></thead><tbody>{reports.map(item => <tr key={item.id}><td><strong>{item.owner_name || item.owner_id}</strong><small>{item.sample_id}</small></td><td>{item.kind === 'assessment' ? '鹰眼评图' : item.kind === 'understanding' ? '识相悟道' : item.kind === 'coaching' ? '训练建议' : '智能识图'}</td><td><RecordDate value={item.created_at} /></td><td><Badge tone={item.status === 'reviewed' ? 'green' : 'amber'}>{item.status === 'reviewed' ? '已复核' : '待复核'}</Badge></td><td><Button variant="secondary" className="compact" onClick={() => open(item)}><Eye size={13} />查看记录</Button></td></tr>)}</tbody></table></div>}
    </Panel> : tab === 'insights' ? insights && <>
      <div className="notice-strip">统计范围：{period?.label || '全部已授权记录'}{period?.from && <> · <RecordDate value={period.from} /> 至 <RecordDate value={period.to} /></>}。仅统计真实样本，合成示意图不计入本页学情。</div>
      <div className="metrics"><div className="stat"><span>样本总数</span><strong>{insights.sample_count}</strong><small>待复核记录 {insights.pending_review_count} 条</small></div><div className="stat"><span>学生修正记录</span><strong>{insights.revision_count ?? '—'}</strong><small>涉及 {insights.revised_report_count ?? '—'} 份报告</small></div><div className="stat"><span>训练分配人次</span><strong>{insights.assignment_count ?? '—'}</strong><small>{insights.practice_count} 项任务的实际分配</small></div><div className="stat"><span>已完成训练人次</span><strong>{insights.completed_assignment_count ?? '—'}</strong><small>按每位学生的完成记录计数</small></div></div>
      <div className="two-col"><Trend insights={insights} /><Panel title="共性问题" note="按已记录问题汇总，不推断未提交学生的情况。"><div className="panel-body">{insights.defects.length ? insights.defects.map(item => <div className="finding row row-between" key={item.name}><h3>{item.name}</h3><Badge tone="amber">{item.count} 条记录</Badge></div>) : <Empty title="暂时没有共性问题数据" />}</div></Panel></div>
      <div className="notice-strip row row-between"><span>{!classId ? '选择一个具体班级后，可根据已记录共性问题生成干预草稿。' : !insights.defects.length ? '当前班级暂无共性问题，积累学习记录后再生成。' : '根据本班已记录问题整理任务模板。先编辑核对，再显式发布；此操作不调用 AI。'}</span><Button variant="secondary" disabled={generating || !classId || !insights.defects.some(item => item.count > 0)} onClick={generateDraft}><Plus size={14} />{generating ? '正在保存模板草稿…' : '根据共性问题生成草稿'}</Button></div>
    </> : <TeacherTasks classes={classes} initialDraft={generatedDraft} onDraftHandled={() => setGeneratedDraft(null)} />}
  </>
}

function TeacherRecord({ report, onBack, onUpdate }: { report: Report; onBack: () => void; onUpdate: (report: Report) => void }) {
  const [sample, setSample] = useState<Sample | null>(null), [feedback, setFeedback] = useState<Review[]>([]), [reviews, setReviews] = useState<Review[]>([]), [comment, setComment] = useState(''), [score, setScore] = useState(''), [decision, setDecision] = useState('confirmed'), [error, setError] = useState(''), [message, setMessage] = useState(''), [busy, setBusy] = useState(false)
  const load = useCallback(async () => { try { const [s, f, r] = await Promise.all([report.sample_snapshot ? Promise.resolve(report.sample_snapshot) : request<Sample>(`/samples/${report.sample_id}`), list<Review>(`/reports/${report.id}/feedback`), list<Review>(`/reports/${report.id}/reviews`)]); setSample(report.sample_snapshot || s); setFeedback(f); setReviews(r) } catch (err) { setError(errorMessage(err)) } }, [report.id, report.sample_id, report.sample_snapshot])
  useEffect(() => { void Promise.resolve().then(load) }, [load])
  async function save(review: boolean) { setBusy(true); setError(''); setMessage(''); try { await request(`/reports/${report.id}/${review ? 'review' : 'feedback'}`, { method: 'POST', body: review ? { decision, comment, ...(score !== '' ? { corrected_score: Number(score) } : {}) } : { comment } }); setMessage(review ? '复核决定已保存，原始 AI 报告仍保留。' : '指导意见已保存，复核状态未改变。'); setComment(''); await load(); onUpdate(await request<Report>(`/reports/${report.id}`)) } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) } }
  const revision = report.latest_revision
  return <><BackToList onClick={onBack} /><PageTitle label="LEARNING EVIDENCE" title="学习记录与教师反馈" description="对照原始图像、AI 依据与学生修正，再做教学判断。" /><div className="two-col"><div>{sample && <Panel title={sample.title} note={`${sample.material} · ${sample.process}`}><div className="panel-body"><SampleImage sample={sample} label={sample.title} /></div></Panel>}<ReportDetail report={report} /></div><div><Panel title="学生修正" note={revision ? revision.reason || revision.comment : '当前尚未提交修正。'}><div className="panel-body">{revision?.data ? <>{revision.data.final_phase && <p>组织判断：{revision.data.final_phase}</p>}{revision.data.criteria?.map(item => <div className="finding" key={item.id}><div className="row row-between"><h3>{item.label}</h3><strong>{item.score == null ? '无法判断' : `${item.score}/${item.max_score}`}</strong></div><p>{item.rule_text}</p><p>{item.evidence}</p></div>)}{revision.data.dimensions?.map(item => <div className="finding" key={item.name}><div className="row"><h3>{item.name}</h3><BasisBadge basis={item.basis} /></div><p>{item.content}</p></div>)}</> : <p className="muted">可以针对 AI 原稿给出阅读提示，再请学生补充判断依据。</p>}</div></Panel><Panel title="指导与复核" note="发送指导不会自动确认结果；请明确保存复核决定。"><div className="panel-body stack"><Field label="具体反馈 / 复核理由"><textarea value={comment} onChange={event => setComment(event.target.value)} placeholder="指出需要补充的证据，或解释你的确认与修订。" /></Field><Button variant="secondary" disabled={busy || !comment.trim()} onClick={() => save(false)}><MessageSquare size={15} />仅保存指导意见</Button><div className="divider" /><div className="fields"><Field label="复核决定"><select value={decision} onChange={event => setDecision(event.target.value)}><option value="confirmed">确认当前结论</option><option value="needs_revision">请学生补充修订</option></select></Field>{report.kind === 'assessment' && <Field label="教师修正总分（选填）"><input type="number" min={0} max={100} value={score} onChange={event => setScore(event.target.value)} placeholder="0–100" /></Field>}</div><Button disabled={busy || !comment.trim() || (score !== '' && (Number(score) < 0 || Number(score) > 100))} onClick={() => save(true)}><CheckCircle2 size={15} />保存复核决定</Button>{error && <Message>{error}</Message>}{message && <Message kind="success">{message}</Message>}</div></Panel><Panel title="反馈与复核记录"><div className="panel-body">{[...reviews.map(item => ({ ...item, kind: 'review' })), ...feedback.map(item => ({ ...item, kind: 'feedback' }))].sort((a, b) => b.created_at.localeCompare(a.created_at)).map(item => <div className="finding" key={item.id}><div className="row row-between"><Badge tone={item.kind === 'review' ? 'green' : 'cyan'}>{item.kind === 'review' ? '明确复核' : '教学指导'}</Badge><span className="muted" style={{ fontSize: 12 }}><RecordDate value={item.created_at} /></span></div><p className="space-top">{item.comment}</p></div>)}{!feedback.length && !reviews.length && <p className="muted">还没有教师反馈。</p>}</div></Panel></div></div></>
}

function TeacherTasks({ classes, initialDraft, onDraftHandled }: { classes: ClassRecord[]; initialDraft: PracticeTask | null; onDraftHandled: () => void }) {
  const [tasks, setTasks] = useState<PracticeTask[]>([])
  const [selected, setSelected] = useState<PracticeTask | null>(initialDraft)
  const [editing, setEditing] = useState(!!initialDraft)
  const [title, setTitle] = useState(initialDraft?.title || '')
  const [instruction, setInstruction] = useState(initialDraft?.instruction || '')
  const [classId, setClassId] = useState(initialDraft?.class_id || '')
  const [assignees, setAssignees] = useState<string[]>(initialDraft?.assignee_ids || [])
  const [attempts, setAttempts] = useState<Attempt[]>([])
  const [rounds, setRounds] = useState<LearningSession[]>([])
  const [detailLoading, setDetailLoading] = useState(false)
  const [activeRound, setActiveRound] = useState<LearningSession | null>(null)
  const [roundReports, setRoundReports] = useState<Report[]>([])
  const [roundLoading, setRoundLoading] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState(initialDraft ? '已根据班级记录保存模板草稿。请编辑任务内容和目标学生，确认后再发布。' : '')
  const [busy, setBusy] = useState(false)
  const reload = useCallback(async () => { try { setTasks(await list<PracticeTask>('/practice-tasks')) } catch (err) { setError(errorMessage(err)) } }, [])
  useEffect(() => { void Promise.resolve().then(reload) }, [reload])
  function edit(task?: PracticeTask) {
    setSelected(task || null); setTitle(task?.title || ''); setInstruction(task?.instruction || ''); setClassId(task?.class_id || classes[0]?.id || ''); setAssignees(task?.assignee_ids || []); setEditing(true); setActiveRound(null); setError(''); setMessage('')
  }
  async function save() {
    setBusy(true); setError('')
    try {
      const result = await request<PracticeTask>(selected ? `/practice-tasks/${selected.id}` : '/practice-tasks', { method: selected ? 'PATCH' : 'POST', body: { title, instruction, class_id: classId, assignee_ids: assignees, status: selected?.status || 'draft' } })
      setSelected(result); setEditing(false); setAttempts([]); setRounds([]); onDraftHandled(); setMessage('任务草稿已保存，发布后学生才会收到。'); await reload()
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  async function publish(task: PracticeTask) {
    setBusy(true); setError('')
    try {
      const result = await request<PracticeTask>(`/practice-tasks/${task.id}/publish`, { method: 'POST' })
      if (selected?.id === task.id) setSelected(result)
      onDraftHandled(); setMessage('训练任务已发布。'); await reload()
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  async function detail(task: PracticeTask) {
    setDetailLoading(true); setError(''); setSelected(task); setEditing(false); setAttempts([]); setRounds([]); setActiveRound(null); setRoundReports([]); onDraftHandled()
    try {
      const [record, submissions, sessions] = await Promise.all([
        request<PracticeTask>(`/practice-tasks/${task.id}`),
        list<Attempt>(`/practice-tasks/${task.id}/attempts`),
        list<LearningSession>('/learning-sessions'),
      ])
      setSelected(record); setAttempts(submissions); setRounds(sessions.filter(item => item.task_id === task.id))
    } catch (err) { setError(errorMessage(err)) } finally { setDetailLoading(false) }
  }
  async function openRound(round: LearningSession) {
    setActiveRound(round); setRoundReports([]); setRoundLoading(true); setError('')
    try { setRoundReports((await list<Report>(`/reports?sample_id=${encodeURIComponent(round.sample_id)}`)).filter(item => item.learning_session_id === round.id)) } catch (err) { setError(errorMessage(err)) } finally { setRoundLoading(false) }
  }
  const members = classes.find(item => item.id === classId)?.members?.filter(item => item.role === 'student') || []
  const nameOf = (id: string) => classes.flatMap(group => group.members || []).find(user => user.id === id)?.display_name || id
  const studentIds = [...new Set([...(selected?.assignee_ids || []), ...attempts.map(item => item.owner_id).filter((id): id is string => !!id), ...rounds.map(item => item.owner_id).filter((id): id is string => !!id)])]
  return <>
    {error && <Message>{error}</Message>}{message && <Message kind="success">{message}</Message>}
    <div className="toolbar"><Button onClick={() => edit()}><Plus size={15} />编写干预任务</Button><p className="muted">先保存草稿，再明确发布到班级或指定学生。</p></div>
    {editing && <Panel title={selected ? '编辑任务草稿' : '新建任务草稿'}><div className="panel-body stack">
      <Field label="任务名称"><input value={title} onChange={event => setTitle(event.target.value)} /></Field>
      <Field label="任务说明与验证方法"><textarea rows={10} value={instruction} onChange={event => setInstruction(event.target.value)} /></Field>
      <Field label="目标班级" hint={selected ? '已保存草稿的班级固定。如需更换班级，请新建任务。' : undefined}><select disabled={!!selected} value={classId} onChange={event => { setClassId(event.target.value); setAssignees([]) }}><option value="">请选择班级</option>{classes.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field>
      <div><p className="rule-text">不勾选则发布给该班级全部学生；勾选后仅分配给指定学生。</p><div className="row">{members.map(item => <label key={item.id} className="check-row"><input type="checkbox" checked={assignees.includes(item.id)} onChange={event => setAssignees(event.target.checked ? [...assignees, item.id] : assignees.filter(id => id !== item.id))} />{item.display_name}</label>)}</div></div>
      <div className="form-actions"><Button variant="secondary" onClick={() => { setEditing(false); onDraftHandled() }}>取消编辑</Button><Button disabled={busy || !title.trim() || !instruction.trim() || !classId} onClick={save}><Save size={15} />保存草稿</Button></div>
    </div></Panel>}
    <Panel title="训练任务与完成追踪">{tasks.length ? <div className="table-wrap"><table className="table"><thead><tr><th>任务</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead><tbody>{tasks.map(item => <tr key={item.id}><td><strong>{item.title}</strong><small>{item.instruction.slice(0, 60)}</small></td><td><Badge tone={item.status === 'draft' ? 'amber' : 'cyan'}>{item.status === 'draft' ? '草稿' : '已发布'}</Badge></td><td><RecordDate value={item.created_at} /></td><td><div className="row"><Button className="compact" variant="secondary" disabled={detailLoading} onClick={() => detail(item)}>查看提交与追踪</Button>{item.status === 'draft' && <><Button className="compact" variant="secondary" onClick={() => edit(item)}>编辑</Button><Button className="compact" disabled={busy || (editing && selected?.id === item.id)} onClick={() => publish(item)}><Send size={12} />发布</Button></>}</div></td></tr>)}</tbody></table></div> : <Empty title="还没有训练任务" />}</Panel>
    {selected && !editing && <Panel title={selected.title} note={selected.instruction} action={<Button variant="secondary" disabled={detailLoading} onClick={() => detail(selected)}>刷新提交</Button>}>
      <div className="panel-body">{detailLoading ? <Loading text="正在读取学生提交、反思与关联轮次…" /> : studentIds.length ? studentIds.map(studentId => {
        const assignment = selected.assignments?.find(item => item.user_id === studentId)
        const submitted = attempts.filter(item => item.owner_id === studentId).sort((a, b) => b.created_at.localeCompare(a.created_at))
        const linked = rounds.filter(item => item.owner_id === studentId)
        return <section className="finding" key={studentId}>
          <div className="row row-between"><h3>{nameOf(studentId)}</h3><div className="row"><Badge tone={assignment?.status === 'completed' ? 'green' : 'amber'}>{assignment?.status === 'completed' ? '已完成' : '待完成'}</Badge>{assignment?.completed_at && <span className="muted">完成于 <RecordDate value={assignment.completed_at} /></span>}</div></div>
          <h3 className="space-top">实际提交 · {submitted.length} 次</h3>
          {submitted.length ? submitted.map(attempt => <div className="citation" key={attempt.id}><strong>提交于 <RecordDate value={attempt.created_at} /></strong><p className="wrap-text">{attempt.answer}</p></div>) : <p className="rule-text">该学生尚未提交回答。</p>}
          <h3 className="space-top">完成反思</h3><p className="rule-text wrap-text">{assignment?.reflection || (assignment?.status === 'completed' ? '该次完成记录未填写反思。' : '完成任务时提交的反思会显示在这里。')}</p>
          <h3 className="space-top">关联学习轮次 · {linked.length} 轮</h3>
          {linked.length ? linked.map(round => <div className="citation" key={round.id}><div className="row row-between"><div><strong>{round.title || round.sample_snapshot?.title || '关联练习'}</strong><span className="muted">{round.sample_snapshot?.material || '材料未记录'} · <RecordDate value={round.created_at} /> · {round.completed ? '已归档' : '学习中'}</span></div><Button className="compact" variant="secondary" disabled={roundLoading} onClick={() => openRound(round)}>查看轮次证据</Button></div></div>) : <p className="rule-text">尚未有明确关联本任务的学习轮次，不根据提交时间推测关联。</p>}
        </section>
      }) : <Empty title={selected.status === 'draft' ? '草稿尚未发布给学生' : '暂无任务分配记录'} />}</div>
    </Panel>}
    {activeRound && <Panel title={`关联轮次 · ${activeRound.title || activeRound.sample_snapshot?.title || activeRound.id}`} note={`${nameOf(activeRound.owner_id || '')} · 本轮背景与图片按创建时快照保留。`} action={<Button variant="quiet" onClick={() => setActiveRound(null)}>关闭</Button>}>
      <div className="panel-body">{roundLoading ? <Loading text="正在读取该轮次的证据…" /> : <>{activeRound.sample_snapshot && <div className="space-bottom"><SampleImage sample={activeRound.sample_snapshot} label={activeRound.sample_snapshot.title} /></div>}{roundReports.length ? roundReports.map(report => <div key={report.id}><ReportDetail report={report} />{report.latest_revision?.data && <Panel title="该轮次的学生修正" note={report.latest_revision.reason}><div className="panel-body">{report.latest_revision.data.final_phase && <p>组织判断：{report.latest_revision.data.final_phase}</p>}{report.latest_revision.data.criteria?.map(item => <div className="finding" key={item.id}><h3>{item.label} · {item.score == null ? '无法判断' : `${item.score}/${item.max_score}`}</h3><p>{item.rule_text}</p><p>{item.evidence}</p></div>)}{report.latest_revision.data.dimensions?.map(item => <div className="finding" key={item.name}><div className="row"><h3>{item.name}</h3><BasisBadge basis={item.basis} /></div><p>{item.content}</p></div>)}</div></Panel>}</div>) : <Empty title="本轮尚未生成分析报告">已关联轮次不表示已经完成分析或实验验证。</Empty>}</>}</div>
    </Panel>}
  </>
}
