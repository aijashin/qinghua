import { randomUUID } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'
import { basename, extname } from 'node:path'

const configPath = process.argv[2]
if (!configPath) throw new Error('usage: node scripts/import-photo-submissions.mjs <config.json>')

const config = JSON.parse(await readFile(configPath, 'utf8'))
const baseURL = String(config.base_url || 'http://127.0.0.1:8080/api/v1').replace(/\/$/, '')

function mimeFor(file) {
  const extension = extname(file).toLowerCase()
  if (extension === '.png') return 'image/png'
  if (extension === '.webp') return 'image/webp'
  return 'image/jpeg'
}

async function responseJSON(response) {
  const text = await response.text()
  let value
  try {
    value = text ? JSON.parse(text) : {}
  } catch {
    value = { raw: text }
  }
  if (!response.ok) {
    const message = value?.error?.message || value?.raw || `HTTP ${response.status}`
    throw new Error(message)
  }
  return value
}

async function login(username, password) {
  const response = await fetch(`${baseURL}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const session = await responseJSON(response)
  const setCookies = typeof response.headers.getSetCookie === 'function'
    ? response.headers.getSetCookie()
    : [response.headers.get('set-cookie')].filter(Boolean)
  const cookie = setCookies.map(value => value.split(';', 1)[0]).join('; ')
  if (!cookie || !session.csrf_token) throw new Error(`${username} 登录未返回会话`)
  return { cookie, csrf: session.csrf_token }
}

async function api(path, auth, init = {}) {
  const headers = new Headers(init.headers || {})
  headers.set('Cookie', auth.cookie)
  if (init.method && init.method !== 'GET') headers.set('X-CSRF-Token', auth.csrf)
  const response = await fetch(`${baseURL}${path}`, { ...init, headers })
  return responseJSON(response)
}

async function uploadImage(file, auth) {
  const form = new FormData()
  form.append('file', new Blob([await readFile(file)], { type: mimeFor(file) }), basename(file))
  return api('/files', auth, { method: 'POST', body: form })
}

async function waitForJob(jobID, auth) {
  const deadline = Date.now() + 20 * 60 * 1000
  let previous = ''
  while (Date.now() < deadline) {
    const job = await api(`/jobs/${encodeURIComponent(jobID)}`, auth)
    const state = `${job.status}:${job.stage || ''}`
    if (state !== previous) {
      console.log(`[${jobID.slice(0, 8)}] ${state}`)
      previous = state
    }
    if (job.status === 'succeeded') return job
    if (job.status === 'failed' || job.status === 'cancelled') {
      throw new Error(job.error || `任务${job.status}`)
    }
    await new Promise(resolve => setTimeout(resolve, 2000))
  }
  throw new Error('等待AI评图超过20分钟')
}

async function submitGroup(group) {
  const auth = await login(group.username, group.password)
  let sample
  let learningSession
  let imageCount = Number(group.image_count || 0)
  if (group.sample_id && group.learning_session_id) {
    sample = { id: group.sample_id }
    learningSession = { id: group.learning_session_id }
    console.log(`${group.username}：复用已上传的${group.material}图片组重新评图`)
  } else {
    console.log(`${group.username}：开始上传${group.files.length}张${group.material}照片`)
    const assets = []
    for (const file of group.files) assets.push(await uploadImage(file, auth))
    const fileIDs = assets.map(asset => asset.id)
    imageCount = fileIDs.length
    sample = await api('/samples', auth, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        class_id: group.class_id,
        title: group.title,
        material: group.material,
        file_id: fileIDs[0],
        file_ids: fileIDs,
      }),
    })
    learningSession = await api('/learning-sessions', auth, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sample_id: sample.id, title: `${group.material}多视场综合评图` }),
    })
  }
  const job = await api('/jobs', auth, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      kind: 'assessment',
      sample_id: sample.id,
      learning_session_id: learningSession.id,
      input: {},
      idempotency_key: `real-photo-${randomUUID()}`,
    }),
  })
  console.log(`${group.username}：已提交评图任务 ${job.id}`)
  const finished = await waitForJob(job.id, auth)
  const report = await api(`/reports/${encodeURIComponent(finished.result_id)}`, auth)
  return {
    username: group.username,
    material: group.material,
    image_count: imageCount,
    sample_id: sample.id,
    job_id: job.id,
    report_id: report.id,
    score: report.data?.total_score ?? null,
    max_score: report.data?.max_score ?? 100,
    summary: report.data?.summary || '',
    criteria: report.data?.criteria || [],
  }
}

const results = await Promise.all(config.groups.map(submitGroup))
if (config.output) await writeFile(config.output, `${JSON.stringify(results, null, 2)}\n`, 'utf8')
console.log(JSON.stringify(results.map(({ username, material, image_count, score, max_score }) => ({
  username,
  material,
  image_count,
  score,
  max_score,
})), null, 2))
