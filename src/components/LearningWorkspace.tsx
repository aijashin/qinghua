import { useEffect, useState, type FormEvent } from 'react'
import { ArrowLeft, ArrowRight, CheckCircle2, ChevronLeft, ChevronRight, Eye, EyeOff, FileImage, Images, LoaderCircle, RefreshCw, Save, Trash2, Upload } from 'lucide-react'
import { Badge, BackgroundField, Button, Disclosure, Empty, Field, Loading, Message, Panel, ResultMeta } from './WorkbenchUI'
import { errorMessage, imageUrls, list, request, upload, type ClassRecord, type LearningSession, type LearningStep, type Report, type ReportData, type Sample } from '../api'
import { steps } from '../learning'
import { ReportHistory } from './ReportHistory'
import { useDraftGuard } from '../unsaved'
import { JobRunner } from './AnalysisJob'
import { AssessmentEditor, BasisBadge, UnderstandingEditor } from './LearningEditors'
import { Sources } from './ReportSources'
import { originalData, scoreTotal, validObservationBox } from './reportPresentation'
export { JobRunner } from './AnalysisJob'
export { BasisBadge } from './LearningEditors'
export { Sources } from './ReportSources'

const backgroundOptions = {
  material: ['45 钢', 'Q235', 'T8', 'HT200', '铝合金'],
  process: ['铸态', '退火', '正火', '淬火', '淬火＋回火'],
  magnification: ['50×', '100×', '200×', '500×', '1000×'],
  etchant: ['硝酸酒精溶液', '苦味酸酒精溶液', '氯化铁溶液', '未腐蚀'],
}

export function SampleImage({ sample, label, defects = [], showRegions = true, activeDefect = null }: { sample?: { image_url?: string; image_urls?: string[]; file_id?: string; file_ids?: string[]; demo?: boolean }; label?: string; defects?: ReportData['defects']; showRegions?: boolean; activeDefect?: number | null }) {
  const urls = sample ? imageUrls(sample) : []
  const [failedUrls, setFailedUrls] = useState<string[]>([])
  const hasRegions = defects.some(item => validObservationBox(item.bbox))
  const available = urls.filter(url => !failedUrls.includes(url))
  return <><div className={`image-stage ${available.length > 1 ? 'multi-image-stage' : ''}`}>{available.length ? available.map((url, imageIndex) => <div className="image-with-overlay" key={url}><img src={url} alt={`${label || '金相样本原图'} ${imageIndex + 1}`} onError={() => setFailedUrls(current => [...current, url])} />{imageIndex === 0 && showRegions && defects.map((item, index) => validObservationBox(item.bbox) && item.bbox && <span className={`observation-region ${activeDefect === index ? 'active' : ''}`} key={`${item.name}-${index}`} style={{ left: `${item.bbox.x * 100}%`, top: `${item.bbox.y * 100}%`, width: `${item.bbox.width * 100}%`, height: `${item.bbox.height * 100}%` }} aria-label={`观察定位 ${index + 1}：${item.name}`}><span style={{ ...(item.bbox.x >= 0.5 ? { left: 'auto', right: 0 } : { left: 0 }), ...(item.bbox.y >= 0.5 ? { top: 'auto', bottom: 0 } : { top: 0 }) }}>{index + 1}</span></span>)}</div>) : <Empty title={urls.length ? '照片暂时无法显示' : '上传金相照片'}>{urls.length ? '请确认登录状态和文件权限，再刷新记录。' : '请选择本地金相照片。'}</Empty>}</div>{label && <div className="image-caption"><span><FileImage size={13} aria-hidden="true" />{label}{sample?.demo && <Badge tone="amber">合成示意图</Badge>}</span><span>{urls.length > 1 ? `${urls.length} 张视场 · 综合评估` : hasRegions && showRegions ? '参考定位 · 非像素分割' : '原始视场'}</span></div>}</>
}

export function StepPath({ current, sample, reports, onChange }: { current: LearningStep; sample: Sample | null; reports: Report[]; onChange: (step: LearningStep) => void }) {
  const assessment = reports.some(r => r.kind === 'assessment' && r.latest_revision)
  return <nav className="step-path" aria-label="四步学习路径">{steps.map((step, index) => <button type="button" key={step.id} aria-current={current === step.id ? 'step' : undefined} className={current === step.id ? 'active' : ''} disabled={index > 0 && (!sample || (index > 1 && !assessment))} onClick={() => onChange(step.id)}><b>{String(index + 1).padStart(2, '0')}</b><span>{step.name}</span></button>)}</nav>
}

export function SampleEditor({ sample, onSaved, onStart, nextParent, clearParent }: { sample: Sample | null; onSaved: (sample: Sample) => Promise<void>; onStart: () => void; nextParent?: LearningSession | null; clearParent: () => void }) {
  const [fields, setFields] = useState({ title: sample?.title || '', material: sample?.material || '', process: sample?.process || '', magnification: sample?.magnification || '', etchant: sample?.etchant || '', class_id: sample?.class_id || '' })
  const [files, setFiles] = useState<File[]>([]), [previews, setPreviews] = useState<string[]>([])
  const [classes, setClasses] = useState<ClassRecord[]>([])
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [saved, setSaved] = useState(false)
  useEffect(() => { let alive = true; void list<ClassRecord>('/classes').then(items => { if (alive) { setClasses(items); if (!sample && items.length === 1) setFields(value => ({ ...value, class_id: items[0].id })) } }).catch(err => { if (alive) setError(errorMessage(err)) }); return () => { alive = false } }, [sample])
  const dirty = files.length > 0 || Object.entries(fields).some(([key, value]) => (sample || key !== 'class_id') && value !== (sample?.[key as keyof typeof fields] || ''))
  const complete = files.length > 0 || !!sample?.file_id
  useEffect(() => {
    const urls = files.map(URL.createObjectURL)
    setPreviews(urls)
    return () => { urls.forEach(URL.revokeObjectURL) }
  }, [files])
  useDraftGuard(dirty)
  function chooseFiles(selected?: FileList | null) {
    setError(''); if (!selected?.length) return
    const incoming = Array.from(selected)
    const existing = new Set(files.map(file => `${file.name}-${file.size}-${file.lastModified}`))
    const next = incoming.filter(file => !existing.has(`${file.name}-${file.size}-${file.lastModified}`))
    if (files.length + next.length > 8) { setError(`每组最多上传 8 张图片，当前已选 ${files.length} 张。`); return }
    if (next.some(file => !['image/jpeg', 'image/png', 'image/webp'].includes(file.type))) { setError('请选择 JPG、PNG 或 WebP 图片。'); return }
    if (next.some(file => file.size > 20 * 1024 * 1024)) { setError('单张图片最大 20 MiB，请压缩后重试。'); return }
    setFiles(current => [...current, ...next]); setSaved(false)
  }
  function removeFile(index: number) { setFiles(current => current.filter((_, fileIndex) => fileIndex !== index)); setSaved(false) }
  function moveFile(index: number, direction: -1 | 1) {
    const target = index + direction
    if (target < 0 || target >= files.length) return
    setFiles(current => {
      const next = [...current]
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })
    setSaved(false)
  }
  async function submit(event: FormEvent) {
    event.preventDefault(); if (!complete) { setError('请至少上传一张图片。'); return }
    setBusy(true); setError('')
    try {
      const assets = files.length ? await Promise.all(files.map(upload)) : []
      const file_ids = assets.length ? assets.map(asset => asset.id) : sample?.file_ids?.length ? sample.file_ids : sample?.file_id ? [sample.file_id] : []
      if (!file_ids.length) throw new Error('请先上传图片。')
      const body = { ...fields, title: fields.title.trim() || `${fields.material.trim() || '未填写材料'} · 金相练习`, file_id: file_ids[0], file_ids, ...(sample ? { base_version: sample.version } : {}) }
      const editExisting = sample && !files.length
      const result = await request<Sample>(editExisting ? `/samples/${sample.id}` : '/samples', { method: editExisting ? 'PATCH' : 'POST', body })
      await onSaved(result); setSaved(true); setFiles([])
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  return <>
    {nextParent && <div className="notice-strip row row-between"><span>下一轮练习 · 已关联上一轮记录</span><Button variant="quiet" onClick={clearParent}>取消关联</Button></div>}
    <Panel title="样本与背景" action={<Badge tone={sample ? 'cyan' : 'neutral'}>{sample ? `已保存 · 第 ${sample.version} 版` : '待保存'}</Badge>}>
      <form onSubmit={submit}><div className="sample-workbench"><div className="sample-visual">{files.length ? <div className="upload-selection"><div className="upload-selection-head"><div><span><Images size={16} />待上传图片组</span><small>AI 将综合分析全部视场，不按单张分别计分。</small></div><Badge tone="cyan">{files.length}/8 张</Badge></div><div className="upload-preview-grid">{previews.map((url, index) => <article className="upload-preview-card" key={`${files[index]?.name}-${files[index]?.lastModified}`}><img src={url} alt={`待上传金相照片 ${index + 1}`} /><div className="upload-preview-meta"><span><b>{String(index + 1).padStart(2, '0')}</b><span title={files[index]?.name}>{files[index]?.name}</span></span><div><button type="button" disabled={busy || index === 0} onClick={() => moveFile(index, -1)} aria-label={`将第 ${index + 1} 张前移`}><ChevronLeft size={14} /></button><button type="button" disabled={busy || index === files.length - 1} onClick={() => moveFile(index, 1)} aria-label={`将第 ${index + 1} 张后移`}><ChevronRight size={14} /></button><button type="button" disabled={busy} onClick={() => removeFile(index)} aria-label={`移除第 ${index + 1} 张`}><Trash2 size={14} /></button></div></div></article>)}</div></div> : <SampleImage sample={sample || undefined} label={sample?.title} />}<div className="sample-upload-actions"><label className="upload-box"><Upload size={20} /><strong>{files.length ? '继续添加照片' : sample ? '上传新图片组' : '上传一组照片'}</strong><small>支持一次或分次选择，最多 8 张 · JPG / PNG / WebP</small><input multiple disabled={busy || files.length >= 8} type="file" accept="image/jpeg,image/png,image/webp" onChange={event => { chooseFiles(event.target.files); event.currentTarget.value = '' }} /></label>{files.length > 0 && <Button type="button" variant="quiet" disabled={busy} onClick={() => setFiles([])}><Trash2 size={14} />清空本次选择</Button>}</div></div>
      <div className="sample-form sample-background"><div className="fields">{([{ id: 'material', name: '材料牌号（选填）' }, { id: 'process', name: '热处理工艺（选填）' }, { id: 'magnification', name: '放大倍数（选填）' }, { id: 'etchant', name: '腐蚀剂（选填）' }] as const).map(item => <BackgroundField label={item.name} key={item.id} value={fields[item.id]} options={backgroundOptions[item.id]} disabled={busy} onChange={value => { setFields(current => ({ ...current, [item.id]: value })); setSaved(false) }} />)}</div><p className="sample-context muted">材料详情不是必填项；未提供的信息将由 AI 标记为未知，不会强行推断。</p><Field label="提交到班级" hint={sample ? '本轮班级已固定；更换班级请新建样本。' : undefined}><select disabled={!!sample || busy} value={fields.class_id} onChange={event => setFields({ ...fields, class_id: event.target.value })}><option value="">个人练习</option>{classes.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field><Field label="练习名称（选填）"><input disabled={busy} maxLength={150} value={fields.title} onChange={event => setFields({ ...fields, title: event.target.value })} placeholder="例如：低倍与高倍视场综合评估" /></Field></div></div>
      {(error || saved) && <div className="panel-body">{error && <Message>{error}</Message>}{saved && <Message kind="success">图片组已保存，可生成综合评估。</Message>}</div>}<div className="panel-footer"><p>{dirty ? '图片或选填信息尚未保存' : sample ? '图片组已保存，可开始综合评图' : '至少上传一张图片后保存'}</p><div className="row"><Button variant="secondary" disabled={busy || !complete || (!dirty && !!sample)} type="submit">{busy ? <LoaderCircle className="spin" size={15} /> : <Save size={15} />}{sample && files.length ? '另存为新图片组' : '保存图片组'}</Button><Button onClick={onStart} disabled={!sample || dirty || busy}>生成综合评估 <ArrowRight size={15} /></Button></div></div></form>
    </Panel>

  </>
}

export function AssessmentView({ sample, report, session, configured, reload, onNext, onCompleted }: { sample: Sample; report?: Report; session: LearningSession | null; configured: boolean; reload: () => Promise<void>; onNext: () => void; onCompleted: (session: LearningSession) => void }) {
  return <><JobRunner sample={sample} session={session} kind="assessment" configured={configured} onComplete={reload} label={report ? '重新评图' : '开始评图'} awaitingReport={!report} />{report && <AssessmentResult key={report.id} sample={sample} session={session} report={report} reload={reload} onNext={onNext} onCompleted={onCompleted} />}</>
}

function AssessmentResult({ sample, session, report, reload, onNext, onCompleted }: { sample: Sample; session: LearningSession | null; report: Report; reload: () => Promise<void>; onNext: () => void; onCompleted: (session: LearningSession) => void }) {
  const [showRegions, setShowRegions] = useState(true), [activeDefect, setActiveDefect] = useState<number | null>(null)
  const [archiving, setArchiving] = useState(false), [archiveError, setArchiveError] = useState('')
  const snapshot = report.sample_snapshot || sample
  const data = originalData(report), hasRegions = data.defects?.some(item => validObservationBox(item.bbox))
  const total = scoreTotal(report.latest_revision?.data || data)
  const imageCount = imageUrls(snapshot).length
  async function complete() {
    if (!session || session.completed || !report.latest_revision) return
    setArchiving(true); setArchiveError('')
    try {
      const updated = await request<LearningSession>(`/learning-sessions/${session.id}`, { method: 'PATCH', body: { completed: true } })
      onCompleted(updated)
    } catch (err) { setArchiveError(errorMessage(err)) } finally { setArchiving(false) }
  }
  return <><div className="report-heading"><ResultMeta report={report} /><Badge tone="cyan">学习参考</Badge></div><section className="assessment-overview"><div className="assessment-overview-copy"><div className="row"><Badge tone="cyan">AI 综合评判报告</Badge><span>{imageCount} 张视场 · 四项规则</span></div><h2>{total == null ? '存在无法判断项' : `综合得分 ${total} 分`}</h2><p>{data.summary}</p></div><div className="assessment-overview-metrics"><div><span>可判断项</span><strong>{data.criteria?.filter(item => typeof item.score === 'number').length || 0}/4</strong></div><div><span>可见问题</span><strong>{data.defects?.length || 0}</strong></div><div><span>学生核对</span><strong>{report.latest_revision ? '已保存' : '待完成'}</strong></div></div></section><div className="assessment-layout"><div className="assessment-visual"><Panel title="原始视场" note={[snapshot.material, snapshot.magnification].filter(Boolean).join(' · ') || `${imageCount} 张图像`}><div className="panel-body"><SampleImage sample={snapshot} label={snapshot.title} defects={data.defects} showRegions={showRegions} activeDefect={activeDefect} />{hasRegions && <div className="image-tools"><Button variant="quiet" aria-pressed={showRegions} onClick={() => setShowRegions(value => !value)}>{showRegions ? <EyeOff size={15} /> : <Eye size={15} />}{showRegions ? '隐藏定位框' : '显示定位框'}</Button><span className="muted">红框为观察参考</span></div>}<div className="assessment-findings"><div className="row row-between"><h3>图像问题定位</h3>{data.defects?.length ? <span className="muted">{data.defects.length} 项 · 详情按需展开</span> : null}</div>{data.defects?.length ? data.defects.map((defect, index) => {
      const hasLocation = validObservationBox(defect.bbox)
      const severity = { high: '明显', medium: '中等', low: '轻微', uncertain: '待核对' }[defect.severity] || defect.severity
      return <article className={`finding-card ${activeDefect === index ? 'active' : ''}`} key={`${defect.name}-${index}`}>
        <div className="finding-card-head">
          <span className="finding-number">{index + 1}</span>
          <strong className="finding-card-title" title={defect.name}>{defect.name}</strong>
          <Badge tone="amber">{severity}</Badge>
          {hasLocation ? <button type="button" className="finding-locate" aria-pressed={activeDefect === index} onClick={() => { setActiveDefect(value => value === index ? null : index); setShowRegions(true) }}>{activeDefect === index ? '取消定位' : '定位'}</button> : <span className="finding-location-muted">无定位</span>}
        </div>
        <details className="finding-card-details">
          <summary>查看依据与验证建议</summary>
          <div className="finding-card-body"><p>{defect.evidence || '未记录具体观察依据。'}</p><p><b>验证建议</b>{defect.suggestion || '未记录验证建议。'}</p></div>
        </details>
      </article>
    }) : <p className="muted">本次未记录具体缺陷。</p>}</div></div></Panel><Sources report={report} /></div><div><AssessmentEditor key={report.id} report={report} onSaved={reload} /><ReportHistory report={report} /><section className="assessment-completion"><div><span className="eyebrow">本轮完成条件</span><h3>{session?.completed ? '本轮已归档' : report.latest_revision ? '学生核对已保存，可以归档' : '请先采用或修正 AI 评判'}</h3><p>识相悟道与炉火纯青继续保留，可按学习需要进入，不作为本轮归档的强制条件。</p></div>{archiveError && <Message>{archiveError}</Message>}<div className="form-actions">{!session?.completed && <Button onClick={complete} disabled={archiving || !session || !report.latest_revision}>{archiving ? <LoaderCircle className="spin" size={15} /> : <CheckCircle2 size={15} />}核对完成并归档</Button>}<Button variant="secondary" onClick={onNext} disabled={!report.latest_revision}>进入识相悟道（可选） <ArrowRight size={16} /></Button></div></section></div></div></>
}

export function UnderstandingView({ sample, report, session, configured, reload, onNext }: { sample: Sample; report?: Report; session: LearningSession | null; configured: boolean; reload: () => Promise<void>; onNext: () => void }) {
  return <><JobRunner sample={sample} session={session} kind="understanding" configured={configured} onComplete={reload} label={report ? '重新生成理解' : '生成五维理解'} awaitingReport={!report} />{report && <><div className="report-heading"><ResultMeta report={report} /><Badge tone="cyan">学习参考</Badge></div><UnderstandingEditor key={report.id} report={report} onSaved={reload} /><Sources report={report} />{!!report.data.questions?.length && <Disclosure title="待求证问题" meta={`${report.data.questions.length} 项`}><ol className="report-limitations">{report.data.questions.map((question, index) => <li key={index}>{question}</li>)}</ol></Disclosure>}<ReportHistory report={report} /><div className="form-actions"><Button onClick={onNext} disabled={!report.latest_revision}>进入炉火纯青 <ArrowRight size={16} /></Button></div></>}</>
}

export function ReportDetail({ report }: { report: Report }) {
  const data = originalData(report)
  return <Panel title={report.kind === 'assessment' ? 'AI 评图原稿' : report.kind === 'coaching' ? '训练建议原稿' : 'AI 组织理解原稿'} action={<Badge tone={report.status === 'reviewed' ? 'green' : 'amber'}>{report.status === 'reviewed' ? '已复核' : '待复核'}</Badge>}><div className="panel-body"><ResultMeta report={report} /><p className="space-top">{data.summary}</p>{data.criteria?.map(item => <Disclosure key={item.id} title={item.label} meta={item.score == null ? '无法判断' : `${item.score}/${item.max_score}`}><p>{item.evidence}</p><p className="muted">规则：{item.rule_text || item.description || '未记录'}</p></Disclosure>)}{data.dimensions?.map(item => <Disclosure key={item.name} title={item.name} meta={<BasisBadge basis={item.basis} />}><p>{item.content}</p></Disclosure>)}{data.candidates?.length ? <Disclosure title="候选组织与依据" meta={`${data.candidates.length} 项`}>{data.candidates.map((item, index) => <div className="finding" key={index}><h3>{item.name}</h3><p>{item.evidence}</p><p>{item.uncertainty}</p></div>)}</Disclosure> : null}{data.suggestions?.map((item, index) => <p key={index} className="rule-text">{item}</p>)}<Sources report={report} /></div></Panel>
}

export function BackToList({ onClick }: { onClick: () => void }) { return <Button variant="quiet" onClick={onClick}><ArrowLeft size={15} />返回列表</Button> }
export function ReloadButton({ onClick }: { onClick: () => void }) { return <Button variant="secondary" onClick={onClick}><RefreshCw size={14} />刷新记录</Button> }
export function Placeholder({ loading, message }: { loading: boolean; message: string }) { return loading ? <Loading /> : <Empty title={message} /> }
export function SavedMark() { return <Badge tone="green"><CheckCircle2 size={13} />已保存</Badge> }
