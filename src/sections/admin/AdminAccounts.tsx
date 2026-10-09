import { useState } from 'react'
import { ACCOUNTS, ROLE_LABEL, type AccountRow } from '../../data/admin'

export default function AdminAccounts() {
  const [rows, setRows] = useState<AccountRow[]>(ACCOUNTS)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: '', role: 'student' as AccountRow['role'], classBind: '' })

  const toggle = (id: string) => {
    setRows((rs) => rs.map((r) => (r.id === id ? { ...r, status: r.status === 'active' ? 'disabled' : 'active' } : r)))
  }

  const create = () => {
    if (!form.name.trim()) return
    const seq = String(rows.length + 1).padStart(4, '0')
    setRows((rs) => [
      { id: `NEW-26-${seq}`, name: form.name.trim(), role: form.role, classBind: form.classBind || '未绑定', status: 'active', lastLogin: '—' },
      ...rs,
    ])
    setForm({ name: '', role: 'student', classBind: '' })
    setShowForm(false)
  }

  const roleColor = (r: AccountRow['role']) =>
    r === 'teacher' ? 'hsl(var(--bronze))' : r === 'admin' ? 'hsl(var(--rust))' : 'hsl(var(--quench))'

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto p-6">
      <div className="flex items-end justify-between border-b border-[hsl(var(--metal-line))] pb-4">
        <div>
          <div className="micro-label">Governance / 权限治理</div>
          <div className="mt-1 text-[15px] font-semibold">账号 · 角色 · 班级绑定</div>
        </div>
        <button
          onClick={() => setShowForm(!showForm)}
          className="rounded px-3.5 py-1.5 text-[12.5px] font-medium transition-transform active:scale-[0.98]"
          style={{ backgroundColor: 'hsl(var(--quench))', color: 'hsl(var(--graphite-0))' }}
        >
          {showForm ? '取消' : '+ 创建账号'}
        </button>
      </div>

      {showForm && (
        <div className="stage-in hairline flex flex-wrap items-end gap-3 rounded-md bg-[hsl(var(--graphite-1))] p-4">
          <div>
            <label className="micro-label block">姓名/名称</label>
            <input
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              className="mt-1 w-36 rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-3 py-1.5 text-[13px] text-[hsl(var(--ink-hi))] outline-none focus:border-[hsl(var(--quench))]"
            />
          </div>
          <div>
            <label className="micro-label block">角色分配</label>
            <div className="mt-1 flex gap-1 rounded border border-[hsl(var(--metal-line))] p-0.5">
              {(Object.keys(ROLE_LABEL) as AccountRow['role'][]).map((r) => (
                <button
                  key={r}
                  onClick={() => setForm((f) => ({ ...f, role: r }))}
                  className="rounded px-2.5 py-1 text-[12px]"
                  style={{
                    backgroundColor: form.role === r ? 'hsl(var(--graphite-3))' : 'transparent',
                    color: form.role === r ? roleColor(r) : 'hsl(var(--ink-low))',
                  }}
                >
                  {ROLE_LABEL[r]}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="micro-label block">班级/课程绑定</label>
            <input
              value={form.classBind}
              onChange={(e) => setForm((f) => ({ ...f, classBind: e.target.value }))}
              placeholder="如 材料 2301"
              className="mt-1 w-36 rounded border border-[hsl(var(--metal-line))] bg-[hsl(var(--graphite-0))] px-3 py-1.5 text-[13px] text-[hsl(var(--ink-hi))] outline-none placeholder:text-[hsl(var(--ink-low))] focus:border-[hsl(var(--quench))]"
            />
          </div>
          <button onClick={create} className="rounded px-4 py-1.5 text-[12.5px] font-medium" style={{ backgroundColor: 'hsl(var(--bronze))', color: 'hsl(var(--graphite-0))' }}>
            确认创建
          </button>
        </div>
      )}

      <div className="hairline divide-y divide-[hsl(var(--metal-line))] rounded-md bg-[hsl(var(--graphite-1))]">
        <div className="flex items-center gap-3 px-4 py-2 text-[10.5px] text-[hsl(var(--ink-low))]">
          <span className="w-28">账号 ID</span>
          <span className="w-16">名称</span>
          <span className="w-14">角色</span>
          <span className="w-32">班级绑定</span>
          <span className="w-24">最近登录</span>
          <span className="ml-auto">操作</span>
        </div>
        {rows.map((r) => (
          <div key={r.id} className="flex items-center gap-3 px-4 py-2.5" style={{ opacity: r.status === 'disabled' ? 0.5 : 1 }}>
            <span className="font-mono-tech w-28 text-[11.5px] text-[hsl(var(--ink-low))]">{r.id}</span>
            <span className="w-16 text-[13px] text-[hsl(var(--ink-hi))]">{r.name}</span>
            <span className="w-14">
              <span className="rounded border px-1.5 py-0.5 text-[11px]" style={{ borderColor: `${roleColor(r.role)}66`, color: roleColor(r.role) }}>
                {ROLE_LABEL[r.role]}
              </span>
            </span>
            <span className="w-32 text-[12px] text-[hsl(var(--ink-mid))]">{r.classBind}</span>
            <span className="font-mono-tech w-24 text-[11px] text-[hsl(var(--ink-low))]">{r.lastLogin}</span>
            <button
              onClick={() => toggle(r.id)}
              className="ml-auto rounded border px-3 py-1 text-[11.5px] transition-colors"
              style={
                r.status === 'active'
                  ? { borderColor: 'hsl(var(--rust)/0.6)', color: 'hsl(var(--rust))' }
                  : { borderColor: 'hsl(var(--bronze)/0.6)', color: 'hsl(var(--bronze))' }
              }
            >
              {r.status === 'active' ? '停用' : '启用'}
            </button>
          </div>
        ))}
      </div>

      <p className="border-l-2 border-[hsl(var(--rust))] pl-3 text-[11.5px] leading-relaxed text-[hsl(var(--ink-mid))]">
        权限红线：账号操作全部写入审计日志；管理端不可查看任何单个学生的评片与分数明细——此处仅呈现账号级元信息。
      </p>
    </div>
  )
}
