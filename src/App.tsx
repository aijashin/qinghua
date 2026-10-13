import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { BookOpen, ChevronRight, FlaskConical, Image, Library, LogOut, Microscope, Plus, ShieldCheck, Users } from 'lucide-react'
import { Badge, Button, Field, Loading, Message, PageTitle } from './components/WorkbenchUI'
import { AssessmentView, SampleEditor, StepPath, UnderstandingView } from './components/LearningWorkspace'
import { IdentifyTool, LibraryView, PracticeView } from './components/StudentRecords'
import { TeacherWorkspace } from './components/TeacherWorkspace'
import { AdminWorkspace } from './components/AdminWorkspace'
import { errorMessage, list, request, setCsrfToken, type AuthSession, type LearningSession, type LearningStep, type Report, type Sample } from './api'
import { steps } from './learning'
import { confirmNavigation } from './unsaved'
import './App.css'

type View = 'learning' | 'identify' | 'library' | 'teacher' | 'admin'
const storageKey = 'zhiwei-current-learning-session'

export default function App() {
  const [auth, setAuth] = useState<AuthSession | null>(null), [authLoading, setAuthLoading] = useState(true), [error, setError] = useState(''), [view, setView] = useState<View>('learning'), [step, setStep] = useState<LearningStep>('atlas')
  const [samples, setSamples] = useState<Sample[]>([]), [sample, setSample] = useState<Sample | null>(null), [session, setSession] = useState<LearningSession | null>(null), [reports, setReports] = useState<Report[]>([]), [loading, setLoading] = useState(false), [parent, setParent] = useState<LearningSession | null>(null)
  const [sessions, setSessions] = useState<LearningSession[]>([])
  const [nextTaskId, setNextTaskId] = useState<string | undefined>()
  const loadGeneration = useRef(0)
  const activeUserId = useRef('')
  const resetWorkspace = useCallback(() => {
    loadGeneration.current++; activeUserId.current = ''
    setSample(null); setSession(null); setReports([]); setSamples([]); setSessions([]); setParent(null); setNextTaskId(undefined); setView('learning'); setStep('atlas'); setLoading(false)
  }, [])
  function navigate(next: View) { if (confirmNavigation()) setView(next) }
  useEffect(() => { let live = true; void request<AuthSession>('/auth/session').then(value => { if (live) { setCsrfToken(value.csrf_token); activeUserId.current = value.user?.id || ''; setAuth(value) } }).catch(err => { if (live) setError(errorMessage(err)) }).finally(() => { if (live) setAuthLoading(false) }); return () => { live = false } }, [])
  useEffect(() => { const expire = () => { setAuth(null); setCsrfToken(''); resetWorkspace(); setError('登录已过期，请重新登录。已保存的记录不会丢失。') }; window.addEventListener('session-expired', expire); return () => window.removeEventListener('session-expired', expire) }, [resetWorkspace])
  const user = auth?.user
  const userId = user?.id
  const sampleId = sample?.id
  const loadSample = useCallback(async (sampleId: string, learning?: LearningSession) => {
    const generation = ++loadGeneration.current
    setLoading(true); setError('')
    try {
      const [record, results] = await Promise.all([request<Sample>(`/samples/${sampleId}`), list<Report>(`/reports?sample_id=${encodeURIComponent(sampleId)}`)])
      if (generation !== loadGeneration.current) return
      setSample(learning?.sample_snapshot || record); setReports(results); setSession(learning || null)
      if (learning) { setStep(learning.step || 'atlas'); localStorage.setItem(storageKey, learning.id) } else setStep('atlas')
    } catch (err) { if (generation === loadGeneration.current) setError(errorMessage(err)) }
    finally { if (generation === loadGeneration.current) setLoading(false) }
  }, [])
  useEffect(() => {
    if (!userId) return
    let alive = true
    void Promise.all([list<Sample>('/samples'), list<LearningSession>('/learning-sessions')]).then(async ([items, rounds]) => {
      if (!alive) return
      const owned = items.filter(item => item.owner_id === userId)
      setSamples(owned); setSessions(rounds)
      const savedId = localStorage.getItem(storageKey)
      const current = rounds.find(item => !item.completed && item.id === savedId && owned.some(sample => sample.id === item.sample_id))
        || rounds.find(item => !item.completed && owned.some(sample => sample.id === item.sample_id))
      if (savedId && !current) localStorage.removeItem(storageKey)
      if (current) await loadSample(current.sample_id, current)
    }).catch(err => { if (alive) setError(errorMessage(err)) }).finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [userId, loadSample])
  const reloadReports = useCallback(async () => { if (!sampleId) return; const generation = loadGeneration.current; try { const items = await list<Report>(`/reports?sample_id=${encodeURIComponent(sampleId)}`); if (generation === loadGeneration.current) setReports(items) } catch (err) { if (generation === loadGeneration.current) { setError(errorMessage(err)); throw err } } }, [sampleId])
  async function saved(record: Sample) {
    if (record.owner_id !== activeUserId.current) return
    const generation = ++loadGeneration.current
    setSample(record)
    setSamples(items => [record, ...items.filter(item => item.id !== record.id)])
    if (!session || session.sample_id !== record.id || record.version !== sample?.version) {
      const next = await request<LearningSession>('/learning-sessions', { method: 'POST', body: { sample_id: record.id, title: record.title, parent_session_id: parent?.id || (sample?.id !== record.id ? session?.id : undefined), task_id: nextTaskId } })
      if (generation !== loadGeneration.current) return
      setSession(next); setSessions(items => [next, ...items]); localStorage.setItem(storageKey, next.id); setParent(null); setNextTaskId(undefined); setReports([])
    }
  }
  async function changeStep(next: LearningStep) {
    if (next !== step && !confirmNavigation()) return
    setError('')
    const generation = loadGeneration.current
    if (session) try { const result = await request<LearningSession>(`/learning-sessions/${session.id}`, { method: 'PATCH', body: { step: next } }); if (generation !== loadGeneration.current) return; setSession(result) } catch (err) { setError(errorMessage(err)); return }
    setStep(next); setView('learning')
  }
  async function logout() { if (!confirmNavigation()) return; try { await request('/auth/logout', { method: 'POST' }); setAuth(null); setCsrfToken(''); resetWorkspace(); setError('') } catch (err) { setError(errorMessage(err)) } }
  function newSample(parentSession?: LearningSession, taskId?: string) { if (!confirmNavigation()) return; loadGeneration.current++; setLoading(false); setNextTaskId(taskId); setSample(null); setSession(null); setReports([]); setStep('atlas'); setView('learning'); setParent(parentSession || null); localStorage.removeItem(storageKey) }
  async function startLearningFromSample(record: Sample) {
    if (!confirmNavigation()) return
    const existing = sessions.find(item => !item.completed && item.sample_id === record.id)
    if (existing) {
      await loadSample(record.id, existing)
      setView('learning')
      return
    }
    const generation = ++loadGeneration.current
    setLoading(true); setError('')
    try {
      const next = await request<LearningSession>('/learning-sessions', { method: 'POST', body: { sample_id: record.id, title: record.title } })
      if (generation !== loadGeneration.current) return
      setSample(record); setSession(next); setReports([]); setSessions(items => [next, ...items]); setStep('atlas'); setView('learning'); localStorage.setItem(storageKey, next.id)
    } catch (err) { if (generation === loadGeneration.current) setError(errorMessage(err)) }
    finally { if (generation === loadGeneration.current) setLoading(false) }
  }
  async function resume(sampleId: string, sessionId?: string) { if (!confirmNavigation()) return; const learning = sessions.find(item => item.id === sessionId) || sessions.find(item => item.sample_id === sampleId); await loadSample(sampleId, learning); setView('learning') }
  if (authLoading) return <div className="app-connecting"><Loading text="正在连接学习工作台…" /></div>
  if (!user) return <Login error={error} onLogin={value => { resetWorkspace(); activeUserId.current = value.user?.id || ''; setAuth(value); setCsrfToken(value.csrf_token); setError('') }} />
  const activeReports = reports.filter(item => !session || item.learning_session_id === session.id || (!item.learning_session_id && (!item.sample_version || item.sample_version === sample?.version)))
  const assessment = activeReports.find(item => item.kind === 'assessment'), understanding = activeReports.find(item => item.kind === 'understanding')
  const activeStep = steps.find(item => item.id === step)!
  const resumableSessions = sessions.filter(item => !item.completed && samples.some(record => record.id === item.sample_id))
  const pageName = view === 'learning' ? activeStep.name : view === 'library' ? '见微知著' : view === 'identify' ? '智能识图' : view === 'teacher' ? '教师工作台' : '平台治理'
  const descriptions: Record<LearningStep, string> = {
    atlas: '上传图像，记录本次实验的背景。',
    assessment: '核对图像质量，保存你的判断与依据。',
    understanding: '联系成分、工艺、组织、性能与应用。',
    practice: '回顾本轮记录，安排下一次验证。',
  }
  return <div className="app">
    <aside className="sidebar">
      <button className="brand" onClick={() => navigate('learning')} aria-label="返回学习工作台">
        <span className="brand-icon"><Microscope size={25} strokeWidth={1.5} /></span>
        <span><strong>见微知著</strong><small>金相组织制备与观察评测分析AI优化系统</small></span>
      </button>
      <nav aria-label="工作台导航">
        <div className="nav-group">
          <div className="nav-label">学习</div>
          <button className={'nav-item ' + (view === 'learning' ? 'active' : '')} aria-current={view === 'learning' ? 'page' : undefined} onClick={() => navigate('learning')}><BookOpen size={18} /><span>学习工作台</span></button>
          <button className={'nav-item ' + (view === 'identify' ? 'active' : '')} aria-current={view === 'identify' ? 'page' : undefined} onClick={() => navigate('identify')}><Image size={18} /><span>智能识图</span></button>
          <button className={'nav-item ' + (view === 'library' ? 'active' : '')} aria-current={view === 'library' ? 'page' : undefined} onClick={() => navigate('library')}><Library size={18} /><span>见微知著</span></button>
        </div>
        {(user.role === 'teacher' || user.role === 'admin') && <div className="nav-group">
          <div className="nav-label">教学与管理</div>
          <button className={'nav-item ' + (view === 'teacher' ? 'active' : '')} aria-current={view === 'teacher' ? 'page' : undefined} onClick={() => navigate('teacher')}><Users size={18} /><span>教师工作台</span></button>
          {user.role === 'admin' && <button className={'nav-item ' + (view === 'admin' ? 'active' : '')} aria-current={view === 'admin' ? 'page' : undefined} onClick={() => navigate('admin')}><ShieldCheck size={18} /><span>平台治理</span></button>}
        </div>}
      </nav>
      <div className="sidebar-bottom"><Microscope size={16} /><span>观察 · 理解 · 求证</span></div>
    </aside>
    <div className="app-body">
      <header className="topbar">
        <div className="topbar-context"><span>学习空间</span><ChevronRight size={13} /><strong>{pageName}</strong></div>
        <div className="topbar-right">
          <Badge tone={auth.ai_configured ? 'green' : 'amber'}>{auth.ai_configured ? 'AI 已配置' : 'AI 待配置'}</Badge>
          <div className="user-badge"><span className="avatar">{user.display_name.slice(0, 1)}</span><span>{user.display_name}</span><span className="user-role">{{ student: '学生', teacher: '教师', admin: '管理员' }[user.role]}</span></div>
          <Button variant="quiet" onClick={logout} aria-label="退出登录"><LogOut size={17} /></Button>
        </div>
      </header>
      <main className="main">
        {error && <Message>{error}</Message>}
        {view === 'learning' ? <>
          <PageTitle title={activeStep.name} description={descriptions[step]} action={<Button variant="secondary" onClick={() => newSample()}><Plus size={16} />新建练习</Button>} />
          <StepPath current={step} sample={sample} reports={activeReports} onChange={next => void changeStep(next)} />
          {step === 'atlas' && samples.length > 0 && <div className="toolbar sample-switcher">
            <div><label htmlFor="resume-session">继续未完成练习</label><small>已归档记录请前往“见微知著”查看</small></div>
            {resumableSessions.length ? <select id="resume-session" value={session && !session.completed ? session.id : ''} onChange={event => {
              const learning = sessions.find(item => item.id === event.target.value)
              if (learning) void resume(learning.sample_id, learning.id)
            }}>
              <option value="">选择未完成练习</option>
              {resumableSessions.map(item => {
                const record = samples.find(sample => sample.id === item.sample_id)
                const title = item.title || item.sample_snapshot?.title || record?.title || record?.material || '未命名练习'
                const currentStep = steps.find(option => option.id === item.step)?.name || '组织图谱'
                const updated = new Date(item.updated_at || item.created_at).toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
                return <option key={item.id} value={item.id}>{title} · {currentStep} · {updated}</option>
              })}
            </select> : <span className="muted">当前没有未完成练习</span>}
          </div>}
          {loading ? <Loading /> : step === 'atlas' ?
            <SampleEditor key={`${sample?.id || 'new'}-${sample?.version || 0}`} sample={sample} onSaved={saved} onStart={() => void changeStep('assessment')} nextParent={parent} clearParent={() => setParent(null)} /> : sample && step === 'assessment' ?
            <AssessmentView sample={sample} session={session} report={assessment} configured={auth.ai_configured} reload={reloadReports} onNext={() => void changeStep('understanding')} onCompleted={value => { setSession(value); setSessions(items => items.map(item => item.id === value.id ? value : item)); localStorage.removeItem(storageKey) }} /> : sample && step === 'understanding' ?
            <UnderstandingView sample={sample} session={session} report={understanding} configured={auth.ai_configured} reload={reloadReports} onNext={() => void changeStep('practice')} /> : sample &&
            <PracticeView sample={sample} session={session} reports={activeReports} configured={auth.ai_configured} reloadReports={reloadReports} onNext={newSample} onCompleted={value => { setSession(value); setSessions(items => items.map(item => item.id === value.id ? value : item)) }} />}
        </> : view === 'library' ? <LibraryView onResume={(sampleId, sessionId) => void resume(sampleId, sessionId)} /> : view === 'identify' ? <IdentifyTool samples={samples} configured={auth.ai_configured} onSampleCreated={record => setSamples(items => [record, ...items.filter(item => item.id !== record.id)])} onStartLearning={record => void startLearningFromSample(record)} /> : view === 'teacher' ? <TeacherWorkspace /> : <AdminWorkspace />}
        <footer className="footer-note">见微知著 · 学习参考，保留观察依据与修正记录</footer>
      </main>
    </div>
  </div>
}

function Login({ onLogin, error: outerError }: { onLogin: (session: AuthSession) => void; error: string }) {
  const [username, setUsername] = useState(''), [password, setPassword] = useState(''), [error, setError] = useState(''), [busy, setBusy] = useState(false)
  async function submit(event: FormEvent) { event.preventDefault(); setBusy(true); setError(''); try { onLogin(await request<AuthSession>('/auth/login', { method: 'POST', body: { username, password } })) } catch (err) { setError(errorMessage(err)) } finally { setBusy(false) } }
  return <main className="login-page">
    <section className="login-story">
      <div className="brand"><span className="brand-icon"><Microscope size={26} strokeWidth={1.5} /></span><span><strong>见微知著</strong><small>金相组织制备与观察评测分析AI优化系统</small></span></div>
      <div className="login-intro"><h1>从图像出发，<br />让判断<em>有据可循。</em></h1><p>记录样本，核对观察，把本次理解带回下一次实验。</p></div>
      <div className="login-trace">{steps.map((item, index) => <div key={item.id}><span>{String(index + 1).padStart(2, '0')}</span><strong>{item.name}</strong><small>{item.subtitle}</small></div>)}</div>
      <footer>观察 · 理解 · 求证</footer>
    </section>
    <section className="login-form-wrap"><form className="login-form" onSubmit={submit}>
      <h2>登录工作台</h2><p>使用管理员分配的账号。</p>
      <Field label="账号"><input autoComplete="username" required value={username} onChange={event => setUsername(event.target.value)} placeholder="请输入账号" /></Field>
      <Field label="密码"><input type="password" autoComplete="current-password" required value={password} onChange={event => setPassword(event.target.value)} placeholder="请输入密码" /></Field>
      {(error || outerError) && <Message>{error || outerError}</Message>}
      <Button className="full" type="submit" disabled={busy || !username.trim() || !password}>{busy ? '正在验证账号…' : '登录'}<FlaskConical size={17} /></Button>
      <p className="login-note">首次使用或忘记密码，请联系管理员。</p>
    </form></section>
  </main>
}
