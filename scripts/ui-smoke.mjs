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
import { jobEvents } from './src/api'
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
const steps = render(<StepPath current="atlas" sample={null} reports={[]} onChange={()=>{}}/> )
assert.equal((steps.match(/disabled=""/g)||[]).length,3,'later steps require a saved sample')
const title = render(<PageTitle label="DECORATIVE_LABEL" title="组织图谱"/> )
assert(!title.includes('DECORATIVE_LABEL'),'decorative English label must not appear')
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
