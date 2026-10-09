import { useEffect, useState } from 'react'
import { errorMessage, list, request, type Report, type ReportData, type Revision } from '../api'
import { originalData } from './reportPresentation'

/** An open editor owns its draft. A server refresh never changes that draft. */
export function useReportRevision(report: Report, onSaved: () => Promise<void>) {
  const [revision, setRevision] = useState<Revision | null>(report.latest_revision || null)
  const [loading, setLoading] = useState(true), [saving, setSaving] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState('')
  const [generation, setGeneration] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    void list<Revision>(`/reports/${report.id}/revisions`, controller.signal).then(items => {
      if (controller.signal.aborted) return
      setRevision(items.sort((a, b) => (b.revision_no || 0) - (a.revision_no || 0))[0] || null)
      setError('')
    }).catch(err => { if (!controller.signal.aborted) setError(errorMessage(err)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [report.id, generation])
  const data = revision?.data || originalData(report)
  const baseVersion = revision?.revision_no || 0
  async function save(next: ReportData, reason: string, base: number): Promise<boolean> {
    setSaving(true); setError(''); setNotice('')
    try {
      const item = await request<Revision>(`/reports/${report.id}/revisions`, { method: 'POST', body: { data: next, reason, comment: reason, base_version: base } })
      setRevision(item)
      try { await onSaved() } catch { setNotice('记录已保存，页面同步暂未完成；可刷新记录。') }
      return true
    } catch (err) { setError(errorMessage(err)); return false }
    finally { setSaving(false) }
  }
  function refresh() { setLoading(true); setGeneration(value => value + 1) }
  return { revision, data, baseVersion, loading, saving, error, notice, save, refresh, clearError: () => setError('') }
}
