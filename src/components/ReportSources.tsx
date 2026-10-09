import { useEffect, useState } from 'react'
import { BookOpen, ChevronRight } from 'lucide-react'
import { errorMessage, request, type Citation, type Report } from '../api'
import { Badge, Button, Dialog, Disclosure, Loading, Message } from './WorkbenchUI'

export function Sources({ report }: { report: Report }) {
  const [selected, setSelected] = useState<Citation | null>(null)
  return <div className="report-sources">
    {(report.demo || report.sample_snapshot?.demo) && <div className="report-demo-note"><Badge tone="amber">合成示意图</Badge><span>仅用于演示，与真实样本分开统计。</span></div>}
    <Disclosure title="引用资料" meta={`${report.sources?.length || 0} 条`}><div className="source-list">{report.sources?.length ? report.sources.map(source => <button type="button" className="source-item" key={source.id} onClick={() => setSelected(source)}><BookOpen size={16} /><span><strong>{source.title}</strong><small>{source.document_version ? `资料第 ${source.document_version} 版` : '资料版本未记录'}</small></span><ChevronRight size={16} /></button>) : <p className="muted">本次没有匹配的知识库片段，请结合教材或教师意见核对。</p>}</div></Disclosure>
    {!!report.data.limitations?.length && <Disclosure title="判断边界" meta={`${report.data.limitations.length} 项`}><ul className="report-limitations">{report.data.limitations.map((text, index) => <li key={index}>{text}</li>)}</ul></Disclosure>}
    <Dialog open={!!selected} onOpenChange={open => { if (!open) setSelected(null) }} title={selected?.title || '引用资料'} description={selected?.document_version ? `报告引用的资料第 ${selected.document_version} 版` : '报告引用的资料片段'} wide>{selected && <SourcePreview key={selected.id} source={selected} />}</Dialog>
  </div>
}

function SourcePreview({ source }: { source: Citation }) {
  const [data, setData] = useState<Citation | null>(null), [loading, setLoading] = useState(true), [error, setError] = useState(''), [retry, setRetry] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    void request<Citation>(`/knowledge/chunks/${encodeURIComponent(source.id)}`, { signal: controller.signal }).then(value => { if (!controller.signal.aborted) { setData(value); setError('') } }).catch(err => { if (!controller.signal.aborted) setError(errorMessage(err)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [source.id, retry])
  if (loading) return <Loading text="正在读取引用资料…" />
  if (error) return <><Message>{error}</Message><Button variant="secondary" onClick={() => { setLoading(true); setRetry(value => value + 1) }}>重新读取引用</Button></>
  return <div className="source-preview wrap-text">{data?.content || '此片段没有可展示的正文。'}</div>
}
