import { useState } from 'react'
import { INTERVENTION_TASKS } from '../../data/teacher'

export default function TeacherCalibrate() {
  const [tasks, setTasks] = useState(INTERVENTION_TASKS)

  const sendTask = (id: string) => {
    setTasks((ts) => ts.map((t) => (t.id === id ? { ...t, sent: true } : t)))
  }

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="border-b border-[hsl(var(--metal-line))] pb-4">
        <div className="micro-label">Intervention / 训练干预</div>
        <div className="mt-1 text-[15px] font-semibold">根据学生薄弱点推送专项训练</div>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1.2fr_0.8fr]">
        <div>
          <div className="micro-label mb-3" style={{ color: 'hsl(var(--quench))' }}>定向训练任务</div>
          <div className="flex flex-col gap-2">
            {tasks.map((t) => (
              <div key={t.id} className="hairline rounded-md bg-[hsl(var(--graphite-1))] p-4">
                <div className="text-[13px] font-medium leading-relaxed text-[hsl(var(--ink-hi))]">{t.title}</div>
                <div className="mt-1.5 flex items-center justify-between gap-3">
                  <span className="text-[11.5px] text-[hsl(var(--ink-mid))]">推送对象：{t.target}</span>
                  {t.sent ? (
                    <span className="shrink-0 font-mono-tech text-[11px] text-[hsl(var(--bronze))]">已推送 ✓</span>
                  ) : (
                    <button
                      onClick={() => sendTask(t.id)}
                      className="shrink-0 rounded border border-[hsl(var(--quench))] px-3 py-1 text-[12px] text-[hsl(var(--quench))] transition-transform active:scale-[0.98] hover:bg-[hsl(var(--quench)/0.1)]"
                      style={{ transitionProperty: 'background-color, transform' }}
                    >
                      定向推送
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div>
          <div className="micro-label mb-3">新建专项任务</div>
          <div className="hairline rounded-md border-l-2 bg-[hsl(var(--graphite-1))] p-4" style={{ borderLeftColor: 'hsl(var(--quench))' }}>
            <div className="micro-label mb-2">生成逆向综合题</div>
            <div className="rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-2))] px-3 py-2.5 text-[12.5px] leading-relaxed text-[hsl(var(--ink-hi))]">
              「设计一种工艺，使 45 钢表面硬度 ≥ HRC 58 且心部冲击韧性 ≥ 25 J/cm²」
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-[hsl(var(--ink-mid))]">
              <span className="rounded border border-[hsl(var(--metal-line))] px-2 py-0.5">解锁条件：3 轮追问论证</span>
              <span className="rounded border border-[hsl(var(--metal-line))] px-2 py-0.5">限时：2 周</span>
              <span className="rounded border border-[hsl(var(--metal-line))] px-2 py-0.5">关联缺陷本：腐蚀过深</span>
            </div>
          </div>
          <p className="mt-3 rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-1))] px-3 py-2.5 text-[11.5px] leading-relaxed text-[hsl(var(--ink-mid))]">
            AI 评分的确认与纠错已统一放在「班级评卷台」，这里只负责给学生安排后续训练。
          </p>
        </div>
      </div>
    </div>
  )
}
