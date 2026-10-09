import { AnalysisProgress } from './AnalysisProgress'
import { useCallback, useEffect, useRef, useState } from 'react'
import { FlaskConical, LoaderCircle, RefreshCw } from 'lucide-react'
import { errorMessage, jobError, jobEvents, list, request, type Job, type JobProgress, type LearningSession, type Report, type Sample } from '../api'
import { Badge, Button, Disclosure, Empty, Loading, Message, Panel } from './WorkbenchUI'
import { elapsedText, mergeProgress } from './reportPresentation'

type Props = { sample: Sample; session?: LearningSession | null; kind: Report['kind']; configured: boolean; onComplete: () => Promise<void>; label: string; input?: Record<string, unknown>; awaitingReport?: boolean }

export function JobRunner(props: Props) {
  return <AnalysisJob key={`${props.sample.id}:${props.session?.id || 'standalone'}:${props.kind}`} {...props} />
}

function AnalysisJob({ sample, session, kind, configured, onComplete, label, input, awaitingReport = false }: Props) {
  const [job, setJob] = useState<Job | null>(null), [progress, setProgress] = useState<JobProgress[]>([])
  const [restoring, setRestoring] = useState(true), [restoreAttempt, setRestoreAttempt] = useState(0), [restoreFailed, setRestoreFailed] = useState(false)
  const [busy, setBusy] = useState(false), [cancelling, setCancelling] = useState(false), [error, setError] = useState(''), [connection, setConnection] = useState('')
  const [resultError, setResultError] = useState(''), [resultLoading, setResultLoading] = useState(false), [legacyMessage, setLegacyMessage] = useState('')
  const [now, setNow] = useState(() => Date.now())
  const latestJob = useRef<Job | null>(null)
  const callback = useRef(onComplete), loadedResult = useRef(''), mounted = useRef(true), resultRequest = useRef(false)
  useEffect(() => { callback.current = onComplete }, [onComplete])
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])

  const loadResult = useCallback(async (record: Job) => {
    if (!record.result_id || resultRequest.current || loadedResult.current === record.result_id) return
    resultRequest.current = true
    setResultLoading(true); setResultError('')
    try {
      await request<Report>(`/reports/${record.result_id}`)
      if (!mounted.current) return
      await callback.current()
      if (mounted.current) loadedResult.current = record.result_id
    } catch (err) { if (mounted.current) setResultError(errorMessage(err)) }
    finally { resultRequest.current = false; if (mounted.current) setResultLoading(false) }
  }, [])

  const receive = useCallback((record: Job) => {
    if (!mounted.current) return
    if (latestJob.current?.id === record.id && ['succeeded', 'failed', 'cancelled'].includes(latestJob.current.status) && ['queued', 'running'].includes(record.status)) return
    latestJob.current = record
    setJob(current => current?.id === record.id && ['succeeded', 'failed', 'cancelled'].includes(current.status) && ['queued', 'running'].includes(record.status) ? current : record)
    setProgress(items => mergeProgress(items, [...(record.progress_history || []), ...(record.progress ? [record.progress] : [])]))
    if (record.status === 'failed') setError(jobError(record))
    if (record.status === 'succeeded') void loadResult(record)
  }, [loadResult])

  useEffect(() => {
    const controller = new AbortController()
    void list<Job>('/jobs', controller.signal).then(items => {
      if (controller.signal.aborted) return
      const latest = items.filter(item => item.sample_id === sample.id && item.kind === kind && (session?.id ? item.learning_session_id === session.id : !item.learning_session_id)).sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0]
      setRestoreFailed(false); setError('')
      if (latest) receive(latest)
    }).catch(err => { if (!controller.signal.aborted) { setError(errorMessage(err)); setRestoreFailed(true) } }).finally(() => { if (!controller.signal.aborted) setRestoring(false) })
    return () => controller.abort()
  }, [sample.id, session?.id, kind, receive, restoreAttempt])

  const active = job?.status === 'queued' || job?.status === 'running'
  const jobId = job?.id
  useEffect(() => {
    if (!jobId || !active) return
    let live = true
    const refresh = () => request<Job>(`/jobs/${jobId}`).then(record => { if (live) { receive(record); setConnection('') } }).catch(err => { if (live) setConnection(`暂时无法更新任务状态：${errorMessage(err)}`) })
    const unsubscribe = jobEvents(jobId, (type, data) => {
      if (!live) return
      if (type !== 'connection') setConnection('')
      if (type === 'status') receive(data as Job)
      if (type === 'progress') setProgress(items => mergeProgress(items, [data as JobProgress]))
      if (type === 'delta') setLegacyMessage((data as { text: string }).text)
      if (type === 'completed' || type === 'error') void refresh()
      if (type === 'connection') setConnection((data as { message: string }).message)
    })
    const poll = window.setInterval(() => { void refresh() }, 5000)
    const clock = window.setInterval(() => setNow(Date.now()), 1000)
    return () => { live = false; unsubscribe(); window.clearInterval(poll); window.clearInterval(clock) }
  }, [jobId, active, receive])

  async function start() {
    setBusy(true); setError(''); setResultError(''); setConnection(''); setLegacyMessage('')
    try {
      const record = await request<Job>('/jobs', { method: 'POST', body: { kind, sample_id: sample.id, learning_session_id: session?.id, input: input || {}, idempotency_key: crypto.randomUUID() } })
      if (!mounted.current) return
      setProgress([]); loadedResult.current = ''; receive(record); setNow(Date.now())
    } catch (err) { if (mounted.current) setError(errorMessage(err)) } finally { if (mounted.current) setBusy(false) }
  }
  async function cancel() {
    if (!job) return
    setCancelling(true)
    try { receive(await request<Job>(`/jobs/${job.id}/cancel`, { method: 'POST' })) } catch (err) { if (mounted.current) setError(errorMessage(err)) } finally { if (mounted.current) setCancelling(false) }
  }
  const latest = progress.at(-1)
  const elapsed = active && job ? Math.max(0, now - Date.parse(job.created_at)) : typeof job?.timings?.total_ms === 'number' ? job.timings.total_ms + (job.timings.queue_ms || 0) : undefined
  const timeline = job ? <AnalysisProgress progress={progress} job={job} now={now} /> : null
  const stageElapsed = latest ? Math.max(0, now - Date.parse(latest.at)) : 0

  return <div className="analysis-job">
    {!configured && <Message kind="info">AI 服务待配置，当前可保存样本与学习记录。</Message>}
    {error && <Message>{error}</Message>}
    {restoring ? <div className="analysis-restoring"><Loading text="正在恢复分析状态…" /></div> : restoreFailed ? <Button variant="secondary" onClick={() => { setRestoring(true); setRestoreAttempt(value => value + 1) }}><RefreshCw size={15} />重新读取任务状态</Button> : active ? <section className="analysis-progress" aria-label="分析进度" aria-busy="true"><div className="analysis-progress-head"><div><h3 aria-live="polite"><LoaderCircle className="spin" size={17} />{latest?.label || job.stage || '正在处理'}</h3><div className="analysis-progress-meta"><span>本轮已用 {elapsedText(elapsed || 0)}</span><span>当前阶段 {elapsedText(stageElapsed)}</span></div></div><Button variant="secondary" disabled={cancelling} onClick={cancel}>{cancelling ? '正在取消…' : '取消分析'}</Button></div>{progress.length ? timeline : <p>{legacyMessage || '等待服务更新任务进度。'}</p>}{connection && <p className="progress-connection" role="status">{connection}</p>}</section> : <div className="row space-bottom"><Button variant={job?.status === 'succeeded' ? 'secondary' : 'primary'} onClick={start} disabled={busy || resultLoading || !configured}>{busy ? <LoaderCircle size={15} className="spin" /> : <FlaskConical size={16} />}{job?.status === 'failed' || job?.status === 'cancelled' ? '重新分析' : label}</Button>{job?.status === 'succeeded' && <Badge tone="green">分析完成</Badge>}{job?.status === 'cancelled' && <Badge tone="amber">已取消</Badge>}</div>}
    {!active && progress.length > 0 && <Disclosure title="本次分析过程" meta={typeof elapsed === 'number' ? elapsedText(elapsed) : undefined}>{timeline}</Disclosure>}
    {active && awaitingReport && <div className="analysis-skeleton" role="status">完整结果将在分析与校验完成后显示。</div>}
    {!restoring && !restoreFailed && !active && !resultLoading && !resultError && awaitingReport && job?.status !== 'succeeded' && <Panel><Empty title="尚未生成分析结果">开始分析后，可查看实时进度。</Empty></Panel>}
    {resultLoading && <Loading text="分析已完成，正在读取结果…" />}
    {resultError && <Message>分析已完成，读取结果失败：{resultError}<div className="form-actions"><Button variant="secondary" disabled={resultLoading} onClick={() => job && void loadResult(job)}><RefreshCw size={15} />重新读取结果</Button></div></Message>}
  </div>
}
