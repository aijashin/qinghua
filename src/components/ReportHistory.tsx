import { useCallback, useEffect, useState } from 'react'
import { list, errorMessage, type Report, type Revision, type Review } from '../api'
import { Badge, Button, Disclosure, Loading, Message, RecordDate } from './WorkbenchUI'

type TeacherRecord = Review & { recordType: 'feedback' | 'review' }

/** Fetch live feedback separately from immutable report / atlas snapshots. */
export function ReportHistory({ report }: { report: Report }) { return <LiveReportHistory key={report.id} report={report} /> }

function LiveReportHistory({ report }: { report: Report }) {
  const [revisions, setRevisions] = useState<Revision[]>([]), [feedback, setFeedback] = useState<Review[]>([]), [reviews, setReviews] = useState<Review[]>([])
  const [loading, setLoading] = useState(true), [error, setError] = useState(''), [refresh, setRefresh] = useState(0)
  const retry = useCallback(() => { setLoading(true); setRefresh(value => value + 1) }, [])
  useEffect(() => {
    const controller = new AbortController()
    void Promise.all([list<Revision>(`/reports/${report.id}/revisions`, controller.signal), list<Review>(`/reports/${report.id}/feedback`, controller.signal), list<Review>(`/reports/${report.id}/reviews`, controller.signal)]).then(([revisionItems, feedbackItems, reviewItems]) => {
      if (controller.signal.aborted) return
      setRevisions(revisionItems.sort((a, b) => (b.revision_no || 0) - (a.revision_no || 0))); setFeedback(feedbackItems); setReviews(reviewItems); setError('')
    }).catch(err => { if (!controller.signal.aborted) setError(errorMessage(err)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [report.id, report.revision_no, refresh])
  const teacherRecords: TeacherRecord[] = [...feedback.map(item => ({ ...item, recordType: 'feedback' as const })), ...reviews.map(item => ({ ...item, recordType: 'review' as const }))].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
  return <div className="report-history">
    {teacherRecords[0] && <section className="teacher-feedback-latest"><h3>最新教师反馈</h3><TeacherFeedback item={teacherRecords[0]} /></section>}
    {error && <Message>{error}<div className="form-actions"><Button variant="secondary" onClick={retry} disabled={loading}>重新读取修正与反馈</Button></div></Message>}
    <Disclosure title="修正历史与教师反馈" meta={loading ? '正在读取…' : error ? '读取失败' : `${revisions.length} 次修正 · ${teacherRecords.length} 条反馈`}>
      <div className="row row-between space-bottom"><span className="muted">已保存记录</span><Button variant="quiet" onClick={retry} disabled={loading}>刷新记录</Button></div>
      {loading ? <Loading text="正在读取修正与反馈…" /> : error && !revisions.length && !teacherRecords.length ? <p className="muted">记录暂不可用，请重新读取。</p> : <>
        <section className="history-section"><h3>{teacherRecords.length > 0 ? '更早的教师反馈' : '教师反馈'}</h3>{teacherRecords.length > 1 ? teacherRecords.slice(1).map(item => <TeacherFeedback key={item.id} item={item} />) : <p className="muted">{teacherRecords.length ? '暂无更早记录。' : '尚无教师反馈。'}</p>}</section>
        <section className="history-section"><h3>我的修正</h3>{revisions.length ? revisions.map(item => <details className="finding" key={item.id}><summary>第 {item.revision_no || '—'} 版 · <RecordDate value={item.created_at} /></summary><p className="space-top">依据：{item.reason || item.comment || '未记录'}</p>{item.data?.final_phase && <p>组织判断：{item.data.final_phase}</p>}{item.data?.phase_reason && <p>判断理由：{item.data.phase_reason}</p>}{item.data?.criteria?.map(criterion => <div className="finding" key={criterion.id}><h3>{criterion.label} · {criterion.score == null ? '无法判断' : `${criterion.score}/${criterion.max_score}`}</h3><p>{criterion.rule_text || criterion.description}</p><p>{criterion.evidence}</p></div>)}{item.data?.dimensions?.map(dimension => <div className="finding" key={dimension.name}><div className="row"><h3>{dimension.name}</h3><Badge>{dimension.basis === 'known' ? '背景信息' : dimension.basis === 'observed' ? '图像观察' : dimension.basis === 'inferred' ? '资料推断' : '依据待补充'}</Badge></div><p>{dimension.content}</p></div>)}</details>) : <p className="muted">尚未保存采用或修正稿。</p>}</section>
      </>}
    </Disclosure>
  </div>
}

function TeacherFeedback({ item }: { item: TeacherRecord }) {
  return <div className="finding"><div className="row row-between"><Badge tone={item.recordType === 'feedback' ? 'cyan' : item.decision === 'confirmed' ? 'green' : 'amber'}>{item.recordType === 'feedback' ? '指导意见' : item.decision === 'confirmed' ? '教师确认' : '需要补充修订'}</Badge><small className="muted"><RecordDate value={item.created_at} /></small></div><p className="space-top">{item.comment || '未附文字说明'}</p>{typeof item.corrected_score === 'number' && <p className="muted">教师建议总分：{item.corrected_score} / 100</p>}</div>
}
