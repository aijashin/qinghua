import { useEffect, useState } from 'react'
import { ChevronLeft, ChevronRight, Eye, Search } from 'lucide-react'
import { errorMessage, request, type ClassRecord, type Page, type Report } from '../api'
import { Badge, Button, Empty, Loading, Message, Panel, RecordDate } from './WorkbenchUI'

export function TeacherSubmissions({ classId, classes, refreshKey, onOpen }: { classId: string; classes: ClassRecord[]; refreshKey: number; onOpen: (report: Report) => void }) {
  const [studentId, setStudentId] = useState(''), [query, setQuery] = useState(''), [search, setSearch] = useState('')
  const [sort, setSort] = useState('score_desc')
  const [kind, setKind] = useState('assessment'), [reviewStatus, setReviewStatus] = useState('')
  const [page, setPage] = useState(0), [limit, setLimit] = useState(20), [retry, setRetry] = useState(0)
  const [result, setResult] = useState<Page<Report> | null>(null), [loading, setLoading] = useState(true), [error, setError] = useState('')
  const students = [...new Map(classes.filter(group => !classId || group.id === classId).flatMap(group => group.members).filter(member => member.role === 'student').map(member => [member.id, member])).values()]
  useEffect(() => {
    const controller = new AbortController()
    const params = new URLSearchParams({ limit: String(limit), offset: String(page * limit), sort })
    if (classId) params.set('class_id', classId)
    if (studentId) params.set('student_id', studentId)
    if (search) params.set('q', search)
    if (kind) params.set('kind', kind)
    if (reviewStatus) params.set('review_status', reviewStatus)
    void Promise.resolve().then(() => {
      if (controller.signal.aborted) return null
      setLoading(true); setError('')
      return request<Page<Report>>(`/teacher/submissions?${params}`, { signal: controller.signal })
    }).then(data => {
      if (!data || controller.signal.aborted) return
      if (page > 0 && page * limit >= data.total) { setPage(Math.max(0, Math.ceil(data.total / limit) - 1)); return }
      setResult(data); setError('')
    }).catch(err => { if (!controller.signal.aborted) setError(errorMessage(err)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [classId, studentId, search, kind, reviewStatus, sort, page, limit, refreshKey, retry])
  function changePage(next: number) { setLoading(true); setPage(next) }
  return <Panel title="学生学习记录" note="默认聚焦鹰眼评图，可按学生、模块和复核状态查看真实图片组、AI结果与学生核对记录。">
    <form className="teacher-record-filters" onSubmit={event => { event.preventDefault(); setLoading(true); setPage(0); setSearch(query.trim()); setRetry(value => value + 1) }}>
      <select aria-label="筛选学生" value={studentId} onChange={event => { setLoading(true); setPage(0); setStudentId(event.target.value) }}><option value="">全部学生</option>{students.map(student => <option key={student.id} value={student.id}>{student.display_name || student.username}（{student.username}）</option>)}</select>
      <input aria-label="检索学生姓名或账号" placeholder="学生姓名或账号" maxLength={150} value={query} onChange={event => setQuery(event.target.value)} />
      <select aria-label="筛选模块" value={kind} onChange={event => { setLoading(true); setPage(0); setKind(event.target.value) }}><option value="assessment">鹰眼评图</option><option value="understanding">识相悟道</option><option value="coaching">炉火纯青相关建议</option><option value="">全部模块</option></select>
      <select aria-label="筛选复核状态" value={reviewStatus} onChange={event => { setLoading(true); setPage(0); setReviewStatus(event.target.value) }}><option value="">全部复核状态</option><option value="pending">待复核</option><option value="reviewed">已复核</option></select>
      <select aria-label="记录排序" value={sort} onChange={event => { setLoading(true); setPage(0); setSort(event.target.value) }}><option value="score_desc">得分从高到低</option><option value="score_asc">得分从低到高</option><option value="latest">最新记录优先</option></select>
      <Button variant="secondary" type="submit"><Search size={15} />检索</Button>
    </form>
    {error && <div className="panel-body"><Message>{error}<Button variant="quiet" onClick={() => { setLoading(true); setRetry(value => value + 1) }}>重新读取</Button></Message></div>}
    {loading ? <Loading text="正在读取学习记录…" /> : error ? null : !result?.items.length ? <Empty title={studentId || search || kind || reviewStatus ? '未找到匹配的学习记录' : '暂时没有学习记录'}>学生完成分析后，记录会按实际身份显示在这里。</Empty> : <div className="table-wrap"><table className="table"><thead><tr><th>学生</th><th>班级 / 图片组</th><th>模块</th><th>学生核对</th><th>得分</th><th>生成时间</th><th>复核状态</th><th>操作</th></tr></thead><tbody>{result.items.map(item => <tr key={item.id}><td><strong>{item.owner_name || '未填写姓名'}</strong><small>{item.owner_username}</small></td><td><strong>{item.class_name || '未关联班级'}</strong><small>{item.sample_title || item.sample_snapshot?.title || item.sample_snapshot?.material || '金相学习记录'} · {item.image_count || item.sample_snapshot?.file_ids?.length || 1} 张</small></td><td>{{ assessment: '鹰眼评图', understanding: '识相悟道', coaching: '炉火纯青', identify: '智能识图' }[item.kind]}</td><td><Badge tone={item.student_checked || item.latest_revision ? 'cyan' : 'neutral'}>{item.student_checked || item.latest_revision ? '已核对' : '待核对'}</Badge></td><td><strong>{typeof item.score === 'number' ? `${item.score} 分` : '—'}</strong></td><td><RecordDate value={item.created_at} /></td><td><Badge tone={item.status === 'reviewed' ? 'green' : 'amber'}>{item.status === 'reviewed' ? '已复核' : '待复核'}</Badge></td><td><Button variant="secondary" className="compact" onClick={() => onOpen(item)}><Eye size={14} />查看记录</Button></td></tr>)}</tbody></table></div>}
    <div className="panel-footer record-pagination"><span aria-live="polite">{result && !error ? `共 ${result.total} 条 · 第 ${page + 1} / ${Math.max(1, Math.ceil(result.total / limit))} 页` : '—'}</span><div className="row"><select aria-label="每页记录数" value={limit} onChange={event => { setLoading(true); setPage(0); setLimit(Number(event.target.value)) }}>{[10, 20, 50].map(size => <option key={size} value={size}>每页 {size} 条</option>)}</select><Button variant="secondary" disabled={loading || !!error || page === 0} onClick={() => changePage(page - 1)}><ChevronLeft size={15} />上一页</Button><Button variant="secondary" disabled={loading || !!error || !result || (page + 1) * limit >= result.total} onClick={() => changePage(page + 1)}>下一页<ChevronRight size={15} /></Button></div></div>
  </Panel>
}
