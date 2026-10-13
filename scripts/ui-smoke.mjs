#!/usr/bin/env node
import { mkdtemp, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const require = createRequire(import.meta.url)
const { build } = createRequire(require.resolve('vite'))('esbuild')
const root = fileURLToPath(new URL('../', import.meta.url))
const directory = await mkdtemp(join(tmpdir(), 'zhiwei-ui-check-'))
const source = `
import assert from 'node:assert/strict'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { BackgroundField, ResultMeta, PageTitle } from './src/components/WorkbenchUI'
import { AssessmentView, SampleImage, SampleEditor, StepPath, ReportDetail } from './src/components/LearningWorkspace'
import { TeacherSubmissions } from './src/components/TeacherSubmissions'
import { IdentifyTool, LibraryView } from './src/components/StudentRecords'
import { UnderstandingEditor } from './src/components/LearningEditors'
import { AnalysisProgress } from './src/components/AnalysisProgress'
import { mergeProgress } from './src/components/reportPresentation'
import { jobEvents, jobError } from './src/api'
const render = element => renderToStaticMarkup(element)
const sample = { id: 's1', owner_id: 'u1', file_id: 'image1', title: '界面测试样本', material: '自定义合金 123', process: '未知', magnification: '123×', etchant: '自定义腐蚀剂', status: 'draft', version: 1, created_at: '2026-10-08T02:00:00Z', updated_at: '2026-10-08T02:00:00Z' }
const criteria = ['划痕','假象','侵蚀程度','组织清晰与正确度'].map((label,index) => ({ id: 'c'+index, label, score: index === 0 ? null : 20, max_score: 25, evidence: index === 0 ? '此项证据不足' : '测试观察依据', rule_text: '测试规则' }))
const report = { id:'r1', kind:'assessment', sample_id:'s1', job_id:'j1', owner_id:'u1', status:'draft', model:'DO_NOT_SHOW_MODEL_123', prompt_version:'test', created_at:'2026-10-08T02:01:00Z', sources:[], data:{summary:'测试总结', criteria, total_score:null, max_score:100, coverage:.75, defects:[{name:'疑似问题', severity:'uncertain', evidence:'待核对', suggestion:'补充观察', bbox:{x:.1,y:.2,width:.3,height:.2}}], limitations:[]} }
const opts = { sample, report, session:null, configured:true, reload:async()=>{}, onNext:()=>{} }
const image = render(<SampleImage sample={sample} defects={report.data.defects} label={sample.title}/> )
assert(image.includes('left:10%') && image.includes('top:20%') && image.includes('width:30%') && image.includes('height:20%'), 'normalized bbox must map to displayed image proportions')
assert(image.includes('observation-region'), 'known bbox must have an overlay')
const noBox = render(<SampleImage sample={sample} defects={[{name:'无明确位置',severity:'uncertain',evidence:'不确定',suggestion:'复核'}]}/> )
assert(!noBox.includes('class="observation-region'), 'missing bbox must not produce a fabricated box')
const assessment = render(<AssessmentView {...opts}/>)
assert(!assessment.includes(report.model), 'assessment must not display model name')
assert(assessment.includes('无法判断'), 'null score must remain unjudgeable')
assert(!assessment.includes('60 / 100'), 'partial scores must not produce a total')
assert(assessment.includes('此项证据不足'), 'scoring evidence remains readable')
assert(!render(<ReportDetail report={report}/>).includes(report.model), 'shared report view must hide model')
assert(render(<ResultMeta report={{...report,timings:{total_ms:12300}}}/>).includes('12.3'), 'real server duration should render')
assert(render(<ResultMeta report={report}/>).includes('分析耗时 —'), 'legacy duration must not be invented')
const field = render(<BackgroundField label="材料" value="自定义合金 123" options={['45 钢','未知']} onChange={()=>{}}/> )
assert(field.includes('自定义合金 123'), 'custom historical value must survive preset control rendering')
assert(field.includes('aria-labelledby'), 'background field requires accessible label relationship')
const editor = render(<SampleEditor sample={sample} onSaved={async()=>{}} onStart={()=>{}} clearParent={()=>{}}/> )
assert(editor.includes('自定义合金 123') && editor.includes('123×') && editor.includes('自定义腐蚀剂'), 'saved arbitrary background values must be preserved')
assert(!editor.includes('选择示例') && !editor.includes('示例图谱'), 'sample editor must only offer actual uploads')
const teacher = render(<TeacherSubmissions classId="a" classes={[{id:'a',name:'材料一班',members:[{id:'student-uuid',username:'s01',display_name:'王同学',role:'student'}]}]} refreshKey={0} onOpen={()=>{}}/> )
assert(teacher.includes('王同学（s01）') && teacher.includes('学生姓名或账号'), 'teacher filters must expose real identity and name search')
assert(render(<LibraryView onResume={()=>{}}/>).includes('见微知著'), 'atlas library must display the requested new name')
const identify = render(<IdentifyTool samples={[]} configured={true} onSampleCreated={()=>{}} onStartLearning={()=>{}}/>)
assert(identify.includes('上传识图图片') && identify.includes('不创建学习轮次'), 'smart identification must offer independent upload without creating a practice')
assert(!identify.includes('前往组织图谱') && !identify.includes('准备新样本'), 'smart identification must not route upload through the learning workspace')
assert(!identify.includes('当前识图样本') && !identify.includes('选择识图样本'), 'upload mode must not render the existing-sample analysis workspace at the same time')
assert(!jobError({error:'context deadline exceeded',status:'failed'}).includes('context deadline'), 'legacy timeout errors must be readable')
const steps = render(<StepPath current="atlas" sample={null} reports={[]} onChange={()=>{}}/> )
assert.equal((steps.match(/disabled=""/g)||[]).length,3,'later steps require a saved sample')
const title = render(<PageTitle label="DECORATIVE_LABEL" title="组织图谱"/> )
assert(!title.includes('DECORATIVE_LABEL'),'decorative English label must not appear')
const understanding = {...report,kind:'understanding',sample_version:1,revision_no:2,data:{summary:'原稿',candidates:[{name:'珠光体',evidence:'片层候选',uncertainty:'待核实'}],dimensions:['成分','工艺','组织','性能','应用'].map(name=>({name,content:'五维原稿'+name,basis:'inferred'})),limitations:[]},latest_revision:{id:'rev2',revision_no:2,created_at:'2026-10-09T02:00:00Z',reason:'根据新的形貌依据修正',data:{final_phase:'贝氏体候选',phase_reason:'按实际形貌仍待验证',dimensions:['成分','工艺','组织','性能','应用'].map(name=>({name,content:'个人理解'+name,basis:'observed'}))}}}
const understood = render(<UnderstandingEditor report={understanding} onSaved={async()=>{}}/> )
assert(understood.includes('贝氏体候选') && understood.includes('按实际形貌仍待验证') && understood.includes('根据新的形貌依据修正'),'saved phase, evidence and revision reason must be visible')
assert(understood.includes('understanding-dimensions') && understood.includes('待教师复核'),'five dimensions use cards and do not inherit an unrelated review')
const initialProgress = {stage_id:'stage1',code:'model_receiving',label:'接收结果',at:'2026-10-09T02:00:00Z',elapsed_ms:10,details:{received_bytes:20}}
const updatedProgress = {...initialProgress,updated_at:'2026-10-09T02:00:02Z',details:{received_bytes:40,received_chunks:3}}
const merged = mergeProgress([updatedProgress],[initialProgress])
assert(merged.length===1 && merged[0].details.received_bytes===40,'stale polling must not replace newer stage details')
const processView=render(<AnalysisProgress progress={[updatedProgress]} job={{id:'j',status:'running',created_at:initialProgress.at}} now={Date.parse(updatedProgress.updated_at)}/> )
assert(processView.includes('analysis-steps') && processView.includes('准备图片') && !processView.includes('50%'),'progress shows pending steps without a fabricated percentage')
class FakeEventSource {
  constructor(){this.handlers={};FakeEventSource.last=this}
  addEventListener(type,fn){this.handlers[type]=fn}
  close(){this.closed=true}
}
class FakeMessageEvent { constructor(id,data){this.lastEventId=String(id);this.data=JSON.stringify(data)} }
global.EventSource=FakeEventSource
global.MessageEvent=FakeMessageEvent
const received=[]
const unsubscribe=jobEvents('j1',(type,data)=>received.push({type,data}))
const stream=FakeEventSource.last
assert(stream.handlers.progress,'structured progress listener must be registered')
stream.handlers.progress(new FakeMessageEvent(2,{code:'model_waiting',label:'等待模型响应'}))
stream.handlers.progress(new FakeMessageEvent(2,{code:'model_waiting',label:'重复事件'}))
stream.handlers.status(new FakeMessageEvent(1,{status:'running'}))
assert.equal(received.length,1,'replayed and older events must be deduplicated')
stream.onerror()
assert.equal(received.at(-1).type,'connection','disconnection must be an explicit UI state')
stream.handlers.completed(new FakeMessageEvent(3,{status:'succeeded'}))
assert(stream.closed,'completed stream should close')
unsubscribe()
console.log('UI semantic rendering and SSE replay checks passed (not a browser visual test).')
`
try {
  const compiled = await build({ stdin: { contents: source, resolveDir: root, sourcefile: 'ui-check.tsx', loader: 'tsx' }, bundle: true, platform: 'node', format: 'cjs', jsx: 'automatic', write: false, logLevel: 'silent' })
  const path = join(directory, 'check.cjs')
  await writeFile(path, compiled.outputFiles[0].text)
  createRequire(import.meta.url)(path)
} finally {
  await rm(directory, { recursive: true, force: true })
}
