import { ReferenceCases } from './ReferenceCases'
import { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, MessageSquare, Users } from 'lucide-react'
import { Badge, Button, Empty, Field, Loading, Message, PageTitle, Panel, RecordDate } from './WorkbenchUI'
import { BasisBadge, BackToList, ReportDetail, SampleImage } from './LearningWorkspace'
import { Trend } from './StudentRecords'
import { TeacherSubmissions } from './TeacherSubmissions'
import { errorMessage, list, request, type ClassRecord, type Insights, type Report, type Review, type Sample } from '../api'

export function TeacherWorkspace() {
  const [tab, setTab] = useState('records')
  const [refreshKey, setRefreshKey] = useState(0)
  const [classes, setClasses] = useState<ClassRecord[]>([])
  const [classId, setClassId] = useState('')
  const [selected, setSelected] = useState<Report | null>(null)
  const [insights, setInsights] = useState<Insights | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showAllIssues, setShowAllIssues] = useState(false)
  const reload = useCallback(async () => {
    setError('')
    try {
      const [groups, summary] = await Promise.all([
        list<ClassRecord>('/classes'),
        request<Insights>(`/insights?include_demo=false${classId ? '&class_id=' + encodeURIComponent(classId) : ''}`),
      ])
      setClasses(groups); setInsights(summary); setRefreshKey(value => value + 1)
    } catch (err) { setError(errorMessage(err)) } finally { setLoading(false) }
  }, [classId])
  useEffect(() => { void Promise.resolve().then(reload) }, [reload])
  async function open(report: Report) {
    setSelected(report)
    try { await request(`/reports/${report.id}/viewed`, { method: 'POST' }) } catch (err) { setError(errorMessage(err)) }
  }
  const period = insights?.period
  const commonIssues = [...(insights?.defects || [])].sort((a, b) => b.count - a.count)
  const visibleIssues = showAllIssues ? commonIssues : commonIssues.slice(0, 5)
  return <>
    {selected && <TeacherRecord report={selected} onBack={() => { setSelected(null); void reload() }} onUpdate={setSelected} />}
    <div hidden={!!selected}>
    <PageTitle label="TEACHER WORKSPACE" title="学生评图与复核" description="集中查看学生上传的图片组、AI评判、学生核对版本与教师复核结果。" action={<Badge tone="cyan"><Users size={12} />教师工作台</Badge>} />
    <div className="tabs">{[['records', '学习记录'], ['insights', '班级学情'], ['cases', '教学案例']].map(([id, label]) => <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)}>{label}</button>)}</div>
    {error && <Message>{error}</Message>}
    <div className="toolbar"><select aria-label="筛选班级" value={classId} onChange={event => { setClassId(event.target.value); setShowAllIssues(false) }}><option value="">全部授权班级</option>{classes.map(item => <option value={item.id} key={item.id}>{item.name}</option>)}</select><Button variant="secondary" onClick={reload}>刷新</Button></div>
    {loading ? <Loading /> : tab === 'records' ? <TeacherSubmissions key={classId} classId={classId} classes={classes} refreshKey={refreshKey} onOpen={open} /> : tab === 'insights' ? insights && <>
      <div className="notice-strip">统计范围：{period?.label || '全部已授权记录'}{period?.from && <> · <RecordDate value={period.from} /> 至 <RecordDate value={period.to} /></>}。仅统计真实上传样本。</div>
      <div className="metrics"><div className="stat"><span>样本总数</span><strong>{insights.sample_count}</strong><small>仅统计真实上传样本</small></div><div className="stat"><span>AI 报告</span><strong>{insights.report_count}</strong><small>学生已生成的分析记录</small></div><div className="stat"><span>学生修正记录</span><strong>{insights.revision_count ?? '—'}</strong><small>涉及 {insights.revised_report_count ?? '—'} 份报告</small></div><div className="stat"><span>待复核记录</span><strong>{insights.pending_review_count}</strong><small>等待教师查看与确认</small></div></div>
      <div className="two-col"><Trend insights={insights} /><Panel className="common-issues-panel" title="共性问题" note={commonIssues.length ? `已归纳 ${commonIssues.length} 项，按出现次数排序。` : '按已记录问题汇总，不推断未提交学生的情况。'} action={commonIssues.length > 5 ? <Button className="compact" variant="quiet" aria-expanded={showAllIssues} onClick={() => setShowAllIssues(value => !value)}>{showAllIssues ? '收起列表' : `查看全部 ${commonIssues.length} 项`}</Button> : undefined}><div className={`panel-body common-issue-list ${showAllIssues ? 'is-expanded' : ''}`}>{commonIssues.length ? <>{visibleIssues.map((item, index) => <div className="common-issue-item" key={item.name} title={item.name}><span className="common-issue-rank" aria-hidden="true">{String(index + 1).padStart(2, '0')}</span><span className="common-issue-name">{item.name}</span><Badge tone="amber">{item.count} 条</Badge></div>)}{!showAllIssues && commonIssues.length > visibleIssues.length && <p className="common-issues-hint">仅展示高频前 5 项，其余 {commonIssues.length - visibleIssues.length} 项已收起</p>}</> : <Empty title="暂时没有共性问题数据" />}</div></Panel></div>
    </> : <ReferenceCases classId={classId} />}
    </div>
  </>
}

function TeacherRecord({ report, onBack, onUpdate }: { report: Report; onBack: () => void; onUpdate: (report: Report) => void }) {
  const [sample, setSample] = useState<Sample | null>(null), [feedback, setFeedback] = useState<Review[]>([]), [reviews, setReviews] = useState<Review[]>([]), [comment, setComment] = useState(''), [score, setScore] = useState(''), [decision, setDecision] = useState('confirmed'), [error, setError] = useState(''), [message, setMessage] = useState(''), [busy, setBusy] = useState(false)
  const load = useCallback(async () => { try { const [s, f, r] = await Promise.all([report.sample_snapshot ? Promise.resolve(report.sample_snapshot) : request<Sample>(`/samples/${report.sample_id}`), list<Review>(`/reports/${report.id}/feedback`), list<Review>(`/reports/${report.id}/reviews`)]); setSample(report.sample_snapshot || s); setFeedback(f); setReviews(r) } catch (err) { setError(errorMessage(err)) } }, [report.id, report.sample_id, report.sample_snapshot])
  useEffect(() => { void Promise.resolve().then(load) }, [load])
  async function save(review: boolean) { setBusy(true); setError(''); setMessage(''); try { await request(`/reports/${report.id}/${review ? 'review' : 'feedback'}`, { method: 'POST', body: review ? { decision, comment, base_revision: report.revision_no || 0, ...(score !== '' ? { corrected_score: Number(score) } : {}) } : { comment } }); setMessage(review ? '复核决定已保存，原始 AI 报告仍保留。' : '指导意见已保存，复核状态未改变。'); setComment(''); await load(); onUpdate({ ...report, ...await request<Report>(`/reports/${report.id}`) }) } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) } }
  const revision = report.latest_revision
  const effective = revision?.data || report.data
  const effectiveScore = effective.total_score
  const imageCount = sample?.file_ids?.length || report.image_count || 1
  return <><BackToList onClick={onBack} /><PageTitle label="LEARNING EVIDENCE" title="学生评图记录" description={`${report.owner_name || "学生"} · ${report.owner_username || ""} · ${report.class_name || "未关联班级"}`} /><section className="teacher-record-summary"><div><span>图片组</span><strong>{imageCount} 张</strong><small>{sample?.title || report.sample_title || '金相学习记录'}</small></div><div><span>学生核对</span><strong>{revision ? '已保存' : '待完成'}</strong><small>{revision ? `第 ${revision.revision_no || 1} 版` : '当前仅有 AI 原稿'}</small></div><div><span>综合得分</span><strong>{typeof effectiveScore === 'number' ? `${effectiveScore} 分` : '无法判断'}</strong><small>按当前学生版本显示</small></div><div><span>教师复核</span><strong>{report.status === 'reviewed' ? '已复核' : '待复核'}</strong><small>查看与复核分别留痕</small></div></section><div className="two-col teacher-record-layout"><div>{sample && <Panel title={sample.title} note={[sample.material, sample.process].filter(Boolean).join(' · ') || '材料背景未记录'}><div className="panel-body"><SampleImage sample={sample} label={sample.title} /></div></Panel>}<ReportDetail report={report} /></div><div><Panel title="学生修正" note={revision ? revision.reason || revision.comment : '当前尚未提交修正。'}><div className="panel-body">{revision?.data ? <>{revision.data.final_phase && <p>组织判断：{revision.data.final_phase}</p>}{revision.data.criteria?.map(item => <div className="finding" key={item.id}><div className="row row-between"><h3>{item.label}</h3><strong>{item.score == null ? '无法判断' : `${item.score}/${item.max_score}`}</strong></div><p>{item.rule_text}</p><p>{item.evidence}</p></div>)}{revision.data.dimensions?.map(item => <div className="finding" key={item.name}><div className="row"><h3>{item.name}</h3><BasisBadge basis={item.basis} /></div><p>{item.content}</p></div>)}</> : <p className="muted">可以针对 AI 原稿给出阅读提示，再请学生补充判断依据。</p>}</div></Panel><Panel title="指导与复核" note="发送指导不会自动确认结果；请明确保存复核决定。"><div className="panel-body stack"><Field label="具体反馈 / 复核理由"><textarea value={comment} onChange={event => setComment(event.target.value)} placeholder="指出需要补充的证据，或解释你的确认与修订。" /></Field><Button variant="secondary" disabled={busy || !comment.trim()} onClick={() => save(false)}><MessageSquare size={15} />仅保存指导意见</Button><div className="divider" /><div className="fields"><Field label="复核决定"><select value={decision} onChange={event => setDecision(event.target.value)}><option value="confirmed">确认当前结论</option><option value="needs_revision">请学生补充修订</option></select></Field>{report.kind === 'assessment' && <Field label="教师修正总分（选填）"><input type="number" min={0} max={100} value={score} onChange={event => setScore(event.target.value)} placeholder="0–100" /></Field>}</div><Button disabled={busy || !comment.trim() || (score !== '' && (Number(score) < 0 || Number(score) > 100))} onClick={() => save(true)}><CheckCircle2 size={15} />保存复核决定</Button>{error && <Message>{error}</Message>}{message && <Message kind="success">{message}</Message>}</div></Panel><ReferenceCases report={report} /><Panel title="反馈与复核记录"><div className="panel-body">{[...reviews.map(item => ({ ...item, kind: 'review' })), ...feedback.map(item => ({ ...item, kind: 'feedback' }))].sort((a, b) => b.created_at.localeCompare(a.created_at)).map(item => <div className="finding" key={item.id}><div className="row row-between"><Badge tone={item.kind === 'review' ? 'green' : 'cyan'}>{item.kind === 'review' ? '明确复核' : '教学指导'}</Badge><span className="muted" style={{ fontSize: 12 }}><RecordDate value={item.created_at} /></span></div><p className="space-top">{item.comment}</p></div>)}{!feedback.length && !reviews.length && <p className="muted">还没有教师反馈。</p>}</div></Panel></div></div></>
}
