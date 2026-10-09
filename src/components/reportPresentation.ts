import type { JobProgress, Report, ReportData } from '../api'

export function originalData(report: Report): ReportData { return report.original_data || report.data }
export function currentData(report: Report): ReportData { return report.latest_revision?.data || originalData(report) }
export function scoreTotal(data: ReportData): number | null {
  const rows = data.criteria
  return rows?.length === 4 && new Set(rows.map(item => item.id)).size === 4 && rows.every(item => typeof item.score === 'number' && Number.isFinite(item.score) && Number.isFinite(item.max_score) && item.max_score > 0 && item.score >= 0 && item.score <= item.max_score)
    ? rows.reduce((sum, item) => sum + (item.score ?? 0), 0) : null
}
export function mergeProgress(records: JobProgress[], incoming: JobProgress[]): JobProgress[] {
  const seen = new Map<string, JobProgress>()
  for (const item of [...records, ...incoming]) seen.set(`${item.code}:${item.at}:${item.attempt ?? 0}`, item)
  return [...seen.values()].sort((a, b) => (Date.parse(a.at) - Date.parse(b.at)) || a.elapsed_ms - b.elapsed_ms)
}
export function elapsedText(milliseconds: number): string {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000))
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`
}

export function validObservationBox(box: { x: number; y: number; width: number; height: number } | undefined): boolean {
  return !!box && [box.x, box.y, box.width, box.height].every(Number.isFinite) && box.x >= 0 && box.y >= 0 && box.width > 0 && box.height > 0 && box.x + box.width <= 1 && box.y + box.height <= 1
}
