import { useEffect, useState, type FormEvent } from 'react'
import { ArrowLeft, ArrowRight, CheckCircle2, Eye, EyeOff, FileImage, LoaderCircle, RefreshCw, Save, Upload } from 'lucide-react'
import { Badge, BackgroundField, Button, Disclosure, Empty, Field, Loading, Message, Panel, ResultMeta } from './WorkbenchUI'
import { errorMessage, imageUrls, list, request, upload, type ClassRecord, type LearningSession, type LearningStep, type Report, type ReportData, type Sample } from '../api'
import { steps } from '../learning'
import { ReportHistory } from './ReportHistory'
import { useDraftGuard } from '../unsaved'
import { JobRunner } from './AnalysisJob'
import { AssessmentEditor, BasisBadge, UnderstandingEditor } from './LearningEditors'
import { Sources } from './ReportSources'
import { originalData, validObservationBox } from './reportPresentation'
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
  const assessment = reports.some(r => r.kind === 'assessment' && r.latest_revision), understanding = reports.some(r => r.kind === 'understanding' && r.latest_revision)
  return <nav className="step-path" aria-label="四步学习路径">{steps.map((step, index) => <button type="button" key={step.id} aria-current={current === step.id ? 'step' : undefined} className={current === step.id ? 'active' : ''} disabled={index > 0 && (!sample || (index > 1 && !assessment) || (index > 2 && !understanding))} onClick={() => onChange(step.id)}><b>{String(index + 1).padStart(2, '0')}</b><span>{step.name}</span></button>)}</nav>
}

export function SampleEditor({ sample, onSaved, onStart, nextParent, clearParent }: { sample: Sample | null; onSaved: (sample: Sample) => Promise<void>; onStart: () => void; nextParent?: LearningSession | null; clearParent: () => void }) {
  const [fields, setFields] = useState({ title: sample?.title || '', material: sample?.material || '', process: sample?.process || '', magnification: sample?.magnification || '', etchant: sample?.etchant || '', class_id: sample?.class_id || '' })
  const [files, setFiles] = useState<File[]>([]), [previews, setPreviews] = useState<string[]>([])
  const [classes, setClasses] = useState<ClassRecord[]>([])
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [saved, setSaved] = useState(false)
  useEffect(() => { let alive = true; void list<ClassRecord>('/classes').then(items => { if (alive) { setClasses(items); if (!sample && items.length === 1) setFields(value => ({ ...value, class_id: items[0].id })) } }).catch(err => { if (alive) setError(errorMessage(err)) }); return () => { alive = false } }, [sample])
  const dirty = files.length > 0 || Object.entries(fields).some(([key, value]) => (sample || key !== 'class_id') && value !== (sample?.[key as keyof typeof fields] || ''))
  const complete = files.length > 0 || !!sample?.file_id
  useEffect(() => () => { previews.forEach(URL.revokeObjectURL) }, [previews])
  useDraftGuard(dirty)
  function chooseFiles(selected?: FileList | null) {
    setError(''); if (!selected?.length) return
    const next = Array.from(selected)
    if (next.length > 8) { setError('每组最多上传 8 张图片。'); return }
    if (next.some(file => !['image/jpeg', 'image/png', 'image/webp'].includes(file.type))) { setError('请选择 JPG、PNG 或 WebP 图片。'); return }
    if (next.some(file => file.size > 20 * 1024 * 1024)) { setError('单张图片最大 20 MiB，请压缩后重试。'); return }
    previews.forEach(URL.revokeObjectURL)
    setFiles(next); setPreviews(next.map(URL.createObjectURL)); setSaved(false)
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
      await onSaved(result); setSaved(true); setFiles([]); setPreviews([])
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  return <>
    {nextParent && <div className="notice-strip row row-between"><span>下一轮练习 · 已关联上一轮记录</span><Button variant="quiet" onClick={clearParent}>取消关联</Button></div>}
    <Panel title="样本与背景" action={<Badge tone={sample ? 'cyan' : 'neutral'}>{sample ? `已保存 · 第 ${sample.version} 版` : '待保存'}</Badge>}>
      <form onSubmit={submit}><div className="sample-workbench"><div className="sample-visual"><SampleImage sample={previews.length ? { image_urls: previews } : sample || undefined} label={files.length ? `${files.length} 张待上传照片` : sample?.title} /><div className="sample-upload-actions"><label className="upload-box"><Upload size={20} /><strong>{files.length ? `已选择 ${files.length} 张照片` : '上传一组照片'}</strong><small>1–8 张 · JPG / PNG / WebP · 单张最大 20 MiB</small><input multiple disabled={busy} type="file" accept="image/jpeg,image/png,image/webp" onChange={event => chooseFiles(event.target.files)} /></label></div></div>
      <div className="sample-form sample-background"><div className="fields">{([{ id: 'material', name: '材料牌号（选填）' }, { id: 'process', name: '热处理工艺（选填）' }, { id: 'magnification', name: '放大倍数（选填）' }, { id: 'etchant', name: '腐蚀剂（选填）' }] as const).map(item => <BackgroundField label={item.name} key={item.id} value={fields[item.id]} options={backgroundOptions[item.id]} disabled={busy} onChange={value => { setFields(current => ({ ...current, [item.id]: value })); setSaved(false) }} />)}</div><p className="sample-context muted">材料详情不是必填项；未提供的信息将由 AI 标记为未知，不会强行推断。</p><Field label="提交到班级" hint={sample ? '本轮班级已固定；更换班级请新建样本。' : undefined}><select disabled={!!sample || busy} value={fields.class_id} onChange={event => setFields({ ...fields, class_id: event.target.value })}><option value="">个人练习</option>{classes.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field><Field label="练习名称（选填）"><input disabled={busy} maxLength={150} value={fields.title} onChange={event => setFields({ ...fields, title: event.target.value })} placeholder="例如：低倍与高倍视场综合评估" /></Field></div></div>
      {(error || saved) && <div className="panel-body">{error && <Message>{error}</Message>}{saved && <Message kind="success">图片组已保存，可生成综合评估。</Message>}</div>}<div className="panel-footer"><p>{dirty ? '图片或选填信息尚未保存' : sample ? '图片组已保存，可开始综合评图' : '至少上传一张图片后保存'}</p><div className="row"><Button variant="secondary" disabled={busy || !complete || (!dirty && !!sample)} type="submit">{busy ? <LoaderCircle className="spin" size={15} /> : <Save size={15} />}{sample && files.length ? '另存为新图片组' : '保存图片组'}</Button><Button onClick={onStart} disabled={!sample || dirty || busy}>生成综合评估 <ArrowRight size={15} /></Button></div></div></form>
    </Panel>

  </>
}

export function AssessmentView({ sample, report, session, configured, reload, onNext }: { sample: Sample; report?: Report; session: LearningSession | null; configured: boolean; reload: () => Promise<void>; onNext: () => void }) {
  return <><JobRunner sample={sample} session={session} kind="assessment" configured={configured} onComplete={reload} label={report ? '重新评图' : '开始评图'} awaitingReport={!report} />{report && <AssessmentResult key={report.id} sample={sample} report={report} reload={reload} onNext={onNext} />}</>
}

function AssessmentResult({ sample, report, reload, onNext }: { sample: Sample; report: Report; reload: () => Promise<void>; onNext: () => void }) {
  const [showRegions, setShowRegions] = useState(true), [activeDefect, setActiveDefect] = useState<number | null>(null)
  const snapshot = report.sample_snapshot || sample
  const data = originalData(report), hasRegions = data.defects?.some(item => validObservationBox(item.bbox))
  return <><div className="report-heading"><ResultMeta report={report} /><Badge tone="cyan">学习参考</Badge></div><div className="assessment-layout"><div className="assessment-visual"><Panel title="原始视场" note={`${snapshot.material} · ${snapshot.magnification}`}><div className="panel-body"><SampleImage sample={snapshot} label={snapshot.title} defects={data.defects} showRegions={showRegions} activeDefect={activeDefect} />{hasRegions && <div className="image-tools"><Button variant="quiet" aria-pressed={showRegions} onClick={() => setShowRegions(value => !value)}>{showRegions ? <EyeOff size={15} /> : <Eye size={15} />}{showRegions ? '隐藏定位框' : '显示定位框'}</Button><span className="muted">红框为观察参考</span></div>}<div className="assessment-findings"><div className="row row-between"><h3>可见问题</h3>{data.defects?.length ? <span className="muted">{data.defects.length} 项 · 详情按需展开</span> : null}</div>{data.defects?.length ? data.defects.map((defect, index) => {
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
    }) : <p className="muted">本次未记录具体缺陷。</p>}</div></div></Panel><Disclosure title="AI 评图摘要"><p>{data.summary}</p></Disclosure><Sources report={report} /></div><div><AssessmentEditor key={report.id} report={report} onSaved={reload} /><ReportHistory report={report} /><div className="form-actions"><Button onClick={onNext} disabled={!report.latest_revision}>进入识相悟道 <ArrowRight size={16} /></Button></div></div></div></>
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
