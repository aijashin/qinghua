export type Role = 'student' | 'teacher' | 'admin'
export type User = { id: string; username: string; display_name: string; role: Role; disabled: boolean }
export type AuthSession = { user: User | null; csrf_token: string; ai_configured: boolean }
export type FileAsset = { id: string; name: string; mime: string; size: number; url: string }
export type Sample = { id: string; owner_id: string; owner_name?: string; class_id?: string; file_id: string; file_ids?: string[]; title: string; material: string; process: string; magnification: string; etchant: string; image_url?: string; image_urls?: string[]; status: string; version: number; created_at: string; updated_at: string; demo?: boolean }
export type LearningStep = 'atlas' | 'assessment' | 'understanding' | 'practice'
export type LearningSession = { id: string; owner_id?: string; sample_id: string; title: string; step: LearningStep; completed: boolean; parent_session_id?: string; task_id?: string; sample_version?: number; sample_snapshot?: Sample; created_at: string; updated_at: string }
export type Criterion = { id: string; label: string; score: number | null; max_score: number; evidence: string; rule_text?: string; description?: string }
export type Dimension = { name: string; content: string; basis?: 'known' | 'observed' | 'inferred' }
export type ReportData = { citations?: string[]; summary: string; criteria?: Criterion[]; defects?: { name: string; severity: string; evidence: string; suggestion: string; bbox?: { x: number; y: number; width: number; height: number } }[]; total_score?: number | null; coverage?: number; max_score?: number; rubric_id?: string; candidates?: { name: string; evidence: string; uncertainty: string }[]; dimensions?: Dimension[]; questions?: string[]; suggestions?: string[]; verification_plan?: string[]; limitations?: string[]; phase?: string; final_phase?: string; phase_reason?: string }
export type Citation = { id: string; title: string; content: string; document_id?: string; document_version?: number; source_kind?: 'document' | 'atlas' | 'review_case'; source_version?: number; image_url?: string }
export type Page<T> = { items: T[]; total: number; limit: number; offset: number }
export type Report = { score?: number | null; rubric?: Rubric | null; owner_username?: string; class_name?: string; sample_title?: string; started_at?: string; finished_at?: string; timings?: JobTimings; id: string; kind: 'assessment' | 'understanding' | 'identify' | 'coaching'; sample_id: string; job_id: string; learning_session_id?: string; owner_id: string; owner_name?: string; status: string; data: ReportData; sources: Citation[]; model: string; prompt_version: string; created_at: string; version?: number; sample_version?: number; demo?: boolean; original_data?: ReportData; latest_revision?: Revision; revision_no?: number; sample_snapshot?: Sample; latest_review?: Review }
export type Revision = { id: string; report_id: string; comment: string; reason?: string; data?: ReportData; base_version?: number; revision_no?: number; created_at: string }
export type Review = { id: string; decision: string; comment: string; corrected_score?: number; created_at: string; reviewer_name?: string; revision_no?: number; sample_version?: number }
export type JobProgress = { code: 'queued' | 'preparing' | 'retrieving' | 'model_waiting' | 'model_receiving' | 'validating' | 'repairing' | 'persisting' | 'extracting' | 'chunking' | 'succeeded' | 'failed' | 'cancelled'; stage_id?: string; updated_at?: string; details?: ProgressDetails; label: string; at: string; elapsed_ms: number; attempt?: number; completed?: number; total?: number }
export type ProgressDetails = { images?: { role: string; mime: string; bytes: number; width: number; height: number }[]; sources?: { title: string; source_kind: string; source_version: number }[]; document_count?: number; reference_count?: number; unavailable_images?: number; sent_at?: string; response_at?: string; first_content_ms?: number; received_bytes?: number; received_chunks?: number; timeout_ms?: number; checks?: { name: string; status: string }[]; call?: { attempt: number; duration_ms: number; input_tokens?: number; output_tokens?: number; total_tokens?: number }; saved?: boolean }
export type ReferenceCase = { id: string; title: string; status: string; scope: 'class' | 'public'; version: number; case_version: number; class_id: string; created_at: string; updated_at: string; published_at?: string; material: string; phase: string; process: string; magnification: string; etchant: string; conclusion: string; observation: string; description: string; source: string; image_url: string }
export type JobTimings = { queue_ms?: number; total_ms?: number; model_ms?: number; stages?: { code: string; attempt?: number; duration_ms: number }[]; model_calls?: { attempt: number; duration_ms: number; first_content_ms?: number; input_tokens?: number; output_tokens?: number; total_tokens?: number }[] }
export type Job = { started_at?: string; finished_at?: string; error_code?: string; origin_request_id?: string; progress?: JobProgress; progress_history?: JobProgress[]; timings?: JobTimings; id: string; kind: string; sample_id?: string; learning_session_id?: string; status: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'; stage: string; error?: string; result_id?: string; created_at: string; updated_at: string }
export type ClassRecord = { id: string; name: string; members: User[] }
export type PracticeTask = { id: string; title: string; instruction: string; sample_id?: string; report_id?: string; class_id?: string; due_at?: string; status: string; reflection?: string; assignee_ids?: string[]; assignees?: User[]; assignments?: { user_id: string; status: string; completed_at?: string; reflection?: string }[]; completions?: { user_id: string; reflection?: string; completed_at?: string; status?: string }[]; created_at: string }
export type Attempt = { id: string; task_id?: string; answer: string; owner_id?: string; created_at: string }
export type AtlasEntry = { id: string; title: string; material: string; phase: string; process: string; magnification: string; etchant: string; description: string; observation: string; file_id?: string; image_url?: string; source: string; status: string; demo: boolean; sample_id?: string; learning_session_id?: string; created_at?: string; reports?: Report[] | Record<string, Report>; sample?: Sample; sample_snapshot?: Sample; revisions?: Revision[] }
export type Insights = { period?: { from?: string; to?: string; label: string }; revision_count?: number; revised_report_count?: number; assignment_count?: number; completed_assignment_count?: number; sample_count: number; report_count: number; pending_review_count: number; practice_count: number; completed_practice_count: number; average_score: number | null; score_history: { date: string; score: number; sample_id: string }[]; defects: { name: string; count: number }[]; score_groups?: { score_group_id?: string; demo?: boolean; rubric_id: string; rubric_title: string; score_history: { date: string; score: number; sample_id: string }[]; average_score: number }[] }
export type Rubric = { is_builtin?: boolean; published_at?: string; version?: number; id: string; title: string; description: string; criteria: { id: string; label: string; max_score: number; rule_text?: string; description?: string }[]; demo: boolean; status: string; created_at: string }
export type KnowledgeDocument = { material_tags?: string[]; phase_tags?: string[]; process_tags?: string[]; document_version?: number; series_id?: string; version: number; id: string; title: string; file_id: string; status: string; content?: string; text?: string; error?: string; chunk_count?: number; created_at: string }
export type Capabilities = { ai_configured?: boolean; model?: string; base_url?: string; provider?: string; [key: string]: unknown }

let csrfToken = ''
const base = '/api/v1'
export class ApiError extends Error {
  status: number
  code: string
  constructor(message: string, status: number, code = 'request_failed') { super(message); this.name = 'ApiError'; this.status = status; this.code = code }
}
export function setCsrfToken(token: string) { csrfToken = token }
export async function request<T>(path: string, options: { method?: string; body?: unknown; signal?: AbortSignal } = {}): Promise<T> {
  const form = options.body instanceof FormData
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (options.body !== undefined && !form) headers['Content-Type'] = 'application/json'
  if (options.method && options.method !== 'GET') headers['X-CSRF-Token'] = csrfToken
  const response = await fetch(base + path, { method: options.method || 'GET', credentials: 'include', headers, body: options.body === undefined ? undefined : form ? options.body as FormData : JSON.stringify(options.body), signal: options.signal })
  const data = await response.json().catch(() => null)
  if (!response.ok) {
    if (response.status === 401) window.dispatchEvent(new Event('session-expired'))
    throw new ApiError(data?.error?.message || `请求未完成（${response.status}），请稍后重试。`, response.status, data?.error?.code)
  }
  return data as T
}
export async function list<T>(path: string, signal?: AbortSignal): Promise<T[]> {
  const data = await request<{ items: T[]; total?: number }>(path, { signal })
  const items = data.items || []
  while (typeof data.total === 'number' && items.length < data.total) {
    const next = await request<{ items: T[] }>(`${path}${path.includes('?') ? '&' : '?'}limit=100&offset=${items.length}`, { signal })
    if (!next.items?.length) break
    items.push(...next.items)
  }
  return items
}
export async function upload(file: File) { const data = new FormData(); data.append('file', file); return request<FileAsset>('/files', { method: 'POST', body: data }) }
export function fileUrl(id?: string) { return id ? `${base}/files/${encodeURIComponent(id)}` : '' }
export function imageUrl(sample: { image_url?: string; file_id?: string }) { return sample.image_url || fileUrl(sample.file_id) }
export function imageUrls(sample: { image_url?: string; image_urls?: string[]; file_id?: string; file_ids?: string[] }) {
  const urls = sample.image_urls?.length ? sample.image_urls : sample.file_ids?.map(fileUrl)
  return urls?.filter(Boolean) || (imageUrl(sample) ? [imageUrl(sample)] : [])
}
export function errorMessage(error: unknown) { return error instanceof Error ? error.message : '操作未完成，请重试。' }
export function jobEvents(id: string, onEvent: (type: string, data: unknown) => void) {
  const stream = new EventSource(`${base}/jobs/${encodeURIComponent(id)}/events`, { withCredentials: true })
  let lastSeen = -1
  for (const type of ['status', 'progress', 'delta', 'completed', 'error']) stream.addEventListener(type, (event) => {
    if (!(event instanceof MessageEvent)) return
    const sequence = Number(event.lastEventId)
    if (event.lastEventId && Number.isSafeInteger(sequence)) {
      if (sequence <= lastSeen) return
      lastSeen = sequence
    }
    try { onEvent(type, JSON.parse(event.data)) } catch { onEvent('connection', { message: '进度格式异常，正在等待任务状态。' }) }
    if (type === 'completed') stream.close()
  })
  stream.onerror = () => onEvent('connection', { message: '进度连接暂时中断，将自动恢复。任务仍在后台继续。' })
  return () => stream.close()
}

export function jobError(job: Job): string {
  if (job.error?.includes("context deadline exceeded")) return "上次模型分析超时，未生成新报告。可重新分析，或联系管理员检查模型响应速度。"
  return job.error || "分析未完成，请重试。"
}
