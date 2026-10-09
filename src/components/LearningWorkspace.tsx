import { useEffect, useState, type FormEvent } from 'react'
import { ArrowLeft, ArrowRight, CheckCircle2, Eye, EyeOff, FileImage, LoaderCircle, RefreshCw, Save, Upload } from 'lucide-react'
import { Badge, BackgroundField, Button, Disclosure, Empty, Field, Loading, Message, Panel, ResultMeta } from './WorkbenchUI'
import { errorMessage, imageUrl, list, request, upload, type ClassRecord, type LearningSession, type LearningStep, type Report, type ReportData, type Sample } from '../api'
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

export function SampleImage({ sample, label, defects = [], showRegions = true, activeDefect = null }: { sample?: { image_url?: string; file_id?: string; demo?: boolean }; label?: string; defects?: ReportData['defects']; showRegions?: boolean; activeDefect?: number | null }) {
  const url = sample ? imageUrl(sample) : ''
  const [failedUrl, setFailedUrl] = useState('')
  const hasRegions = defects.some(item => validObservationBox(item.bbox))
  return <><div className="image-stage">{url && failedUrl !== url ? <div className="image-with-overlay"><img src={url} alt={label || '金相样本原图'} onError={() => setFailedUrl(url)} />{showRegions && defects.map((item, index) => validObservationBox(item.bbox) && item.bbox && <span className={`observation-region ${activeDefect === index ? 'active' : ''}`} key={`${item.name}-${index}`} style={{ left: `${item.bbox.x * 100}%`, top: `${item.bbox.y * 100}%`, width: `${item.bbox.width * 100}%`, height: `${item.bbox.height * 100}%` }} aria-label={`观察定位 ${index + 1}：${item.name}`}><span style={{ ...(item.bbox.x >= 0.5 ? { left: 'auto', right: 0 } : { left: 0 }), ...(item.bbox.y >= 0.5 ? { top: 'auto', bottom: 0 } : { top: 0 }) }}>{index + 1}</span></span>)}</div> : <Empty title={failedUrl === url && url ? '照片暂时无法显示' : '上传金相照片'}>{url ? '请确认登录状态和文件权限，再刷新记录。' : '请选择本地金相照片。'}</Empty>}</div>{label && <div className="image-caption"><span><FileImage size={13} aria-hidden="true" />{label}{sample?.demo && <Badge tone="amber">合成示意图</Badge>}</span><span>{hasRegions && showRegions ? '参考定位 · 非像素分割' : '原始视场'}</span></div>}</>
}

export function StepPath({ current, sample, reports, onChange }: { current: LearningStep; sample: Sample | null; reports: Report[]; onChange: (step: LearningStep) => void }) {
  const assessment = reports.some(r => r.kind === 'assessment' && r.latest_revision), understanding = reports.some(r => r.kind === 'understanding' && r.latest_revision)
  return <nav className="step-path" aria-label="四步学习路径">{steps.map((step, index) => <button type="button" key={step.id} aria-current={current === step.id ? 'step' : undefined} className={current === step.id ? 'active' : ''} disabled={index > 0 && (!sample || (index > 1 && !assessment) || (index > 2 && !understanding))} onClick={() => onChange(step.id)}><b>{String(index + 1).padStart(2, '0')}</b><span>{step.name}</span></button>)}</nav>
}

export function SampleEditor({ sample, onSaved, onStart, nextParent, clearParent }: { sample: Sample | null; onSaved: (sample: Sample) => Promise<void>; onStart: () => void; nextParent?: LearningSession | null; clearParent: () => void }) {
  const [fields, setFields] = useState({ title: sample?.title || '', material: sample?.material || '', process: sample?.process || '', magnification: sample?.magnification || '', etchant: sample?.etchant || '', class_id: sample?.class_id || '' })
  const [file, setFile] = useState<File | null>(null), [preview, setPreview] = useState('')
  const [classes, setClasses] = useState<ClassRecord[]>([])
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [saved, setSaved] = useState(false)
  useEffect(() => { let alive = true; void list<ClassRecord>('/classes').then(items => { if (alive) { setClasses(items); if (!sample && items.length === 1) setFields(value => ({ ...value, class_id: items[0].id })) } }).catch(err => { if (alive) setError(errorMessage(err)) }); return () => { alive = false } }, [sample])
  const dirty = !!file || Object.entries(fields).some(([key, value]) => (sample || key !== 'class_id') && value !== (sample?.[key as keyof typeof fields] || ''))
  const complete = ['material', 'process', 'magnification', 'etchant'].every(key => fields[key as keyof typeof fields].trim())
  useEffect(() => () => { if (preview) URL.revokeObjectURL(preview) }, [preview])
  useDraftGuard(dirty)
  function chooseFile(selected?: File) {
    setError(''); if (!selected) return
    if (!['image/jpeg', 'image/png', 'image/webp'].includes(selected.type)) { setError('请选择 JPG、PNG 或 WebP 图片。'); return }
    if (selected.size > 20 * 1024 * 1024) { setError('图片超过 20 MiB，请压缩后重试。'); return }
    setFile(selected); setPreview(URL.createObjectURL(selected)); setSaved(false)
  }
  async function submit(event: FormEvent) {
    event.preventDefault(); if (!complete) { setError('请填写四项背景，无法确认的内容可选择未知。'); return }
    setBusy(true); setError('')
    try {
      const asset = file ? await upload(file) : null
      const file_id = asset?.id || sample?.file_id
      if (!file_id) throw new Error('请先上传图片。')
      const body = { ...fields, title: fields.title.trim() || `${fields.material} · 金相练习`, file_id, ...(sample ? { base_version: sample.version } : {}) }
      const editExisting = sample && !file
      const result = await request<Sample>(editExisting ? `/samples/${sample.id}` : '/samples', { method: editExisting ? 'PATCH' : 'POST', body })
      await onSaved(result); setSaved(true); setFile(null)
    } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) }
  }
  return <>
    {nextParent && <div className="notice-strip row row-between"><span>下一轮练习 · 已关联上一轮记录</span><Button variant="quiet" onClick={clearParent}>取消关联</Button></div>}
    <Panel title="样本与背景" action={<Badge tone={sample ? 'cyan' : 'neutral'}>{sample ? `已保存 · 第 ${sample.version} 版` : '待保存'}</Badge>}>
      <form onSubmit={submit}><div className="sample-workbench"><div className="sample-visual"><SampleImage sample={preview ? { image_url: preview } : sample || undefined} label={file?.name || sample?.title} /><div className="sample-upload-actions"><label className="upload-box"><Upload size={20} /><strong>{file ? '更换已选照片' : '上传照片'}</strong><small>JPG / PNG / WebP · 最大 20 MiB</small><input disabled={busy} type="file" accept="image/jpeg,image/png,image/webp" onChange={event => chooseFile(event.target.files?.[0])} /></label></div></div>
      <div className="sample-form sample-background"><div className="fields">{([{ id: 'material', name: '材料牌号' }, { id: 'process', name: '热处理工艺' }, { id: 'magnification', name: '放大倍数' }, { id: 'etchant', name: '腐蚀剂' }] as const).map(item => <BackgroundField label={item.name} key={item.id} value={fields[item.id]} options={backgroundOptions[item.id]} disabled={busy} onChange={value => { setFields(current => ({ ...current, [item.id]: value })); setSaved(false) }} />)}</div><p className="sample-context muted">可选择常用项或输入自定义内容，不确定时选择“未知”。</p><Field label="提交到班级" hint={sample ? '本轮班级已固定；更换班级请新建样本。' : undefined}><select disabled={!!sample || busy} value={fields.class_id} onChange={event => setFields({ ...fields, class_id: event.target.value })}><option value="">个人练习</option>{classes.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></Field><Field label="练习名称（选填）"><input disabled={busy} maxLength={150} value={fields.title} onChange={event => setFields({ ...fields, title: event.target.value })} placeholder="例如：换砂纸后的划痕对照" /></Field></div></div>
      {(error || saved) && <div className="panel-body">{error && <Message>{error}</Message>}{saved && <Message kind="success">样本已保存。</Message>}</div>}<div className="panel-footer"><p>{dirty ? '背景信息尚未保存' : sample ? '样本已保存，可开始评图' : '保存后进入评图'}</p><div className="row"><Button variant="secondary" disabled={busy || !complete || (!dirty && !!sample)} type="submit">{busy ? <LoaderCircle className="spin" size={15} /> : <Save size={15} />}{sample && file ? '另存为新样本' : '保存样本'}</Button><Button onClick={onStart} disabled={!sample || dirty || busy}>进入鹰眼评图 <ArrowRight size={15} /></Button></div></div></form>
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
  return <><div className="report-heading"><ResultMeta report={report} /><Badge tone="cyan">学习参考</Badge></div><div className="assessment-layout"><div className="assessment-visual"><Panel title="原始视场" note={`${snapshot.material} · ${snapshot.magnification}`}><div className="panel-body"><SampleImage sample={snapshot} label={snapshot.title} defects={data.defects} showRegions={showRegions} activeDefect={activeDefect} />{hasRegions && <div className="image-tools"><Button variant="quiet" aria-pressed={showRegions} onClick={() => setShowRegions(value => !value)}>{showRegions ? <EyeOff size={15} /> : <Eye size={15} />}{showRegions ? '隐藏定位框' : '显示定位框'}</Button><span className="muted">红框为观察参考</span></div>}<div className="assessment-findings"><h3>可见问题</h3>{data.defects?.length ? data.defects.map((defect, index) => <button type="button" className={`finding-button ${activeDefect === index ? 'active' : ''}`} key={`${defect.name}-${index}`} aria-pressed={activeDefect === index} onClick={() => { setActiveDefect(value => value === index ? null : index); if (validObservationBox(defect.bbox)) setShowRegions(true) }}><span className="row"><span className="finding-number">{index + 1}</span><strong>{defect.name}</strong><Badge tone="amber">{{ high: '明显', medium: '中等', low: '轻微', uncertain: '待核对' }[defect.severity] || defect.severity}</Badge>{!validObservationBox(defect.bbox) && <small className="muted">无定位坐标</small>}</span><span>{defect.evidence}</span><span className="muted">验证建议：{defect.suggestion}</span></button>) : <p className="muted">本次未记录具体缺陷。</p>}</div></div></Panel><Disclosure title="AI 评图摘要"><p>{data.summary}</p></Disclosure><Sources report={report} /></div><div><AssessmentEditor key={report.id} report={report} onSaved={reload} /><ReportHistory report={report} /><div className="form-actions"><Button onClick={onNext} disabled={!report.latest_revision}>进入识相悟道 <ArrowRight size={16} /></Button></div></div></div></>
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
