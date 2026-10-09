import { cloneElement, isValidElement, useEffect, useId, useRef, useState, type ReactNode, type ReactElement, type ButtonHTMLAttributes } from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as Collapsible from '@radix-ui/react-collapsible'
import * as Popover from '@radix-ui/react-popover'
import { Command } from 'cmdk'
import type { JobTimings } from '../api'
import { AlertCircle, ArrowRight, Check, CheckCircle2, ChevronDown, ChevronsUpDown, Clock3, LoaderCircle, Microscope, Search, X } from 'lucide-react'

export function Button({ children, variant = 'primary', className = '', type = 'button', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'quiet' | 'danger' }) {
  return <button type={type} className={`btn btn-${variant} ${className}`} {...props}>{children}</button>
}

export function Badge({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'cyan' | 'green' | 'amber' | 'red' }) {
  return <span className={`badge badge-${tone}`}><i aria-hidden="true" />{children}</span>
}

export function Panel({ title, note, action, children, className = '' }: { title?: string; note?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return <section className={`panel ${className}`}>{(title || action) && <div className="panel-head"><div>{title && <h2>{title}</h2>}{note && <p>{note}</p>}</div>{action}</div>}{children}</section>
}

export function PageTitle({ title, description, action }: { label?: string; title: string; description?: string; action?: ReactNode }) {
  return <header className="page-title"><div><h1>{title}</h1>{description && <p>{description}</p>}</div>{action}</header>
}

export function Empty({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return <div className="empty"><span className="empty-icon"><Microscope size={30} strokeWidth={1.3} /></span><h3>{title}</h3>{children && <p>{children}</p>}{action}</div>
}

export function Loading({ text = '正在读取记录…' }: { text?: string }) {
  return <div className="loading" role="status"><LoaderCircle className="spin" size={22} /><span>{text}</span></div>
}

export function Message({ children, kind = 'error' }: { children: ReactNode; kind?: 'error' | 'success' | 'info' }) {
  return <div className={`message message-${kind}`} role={kind === 'error' ? 'alert' : 'status'}>{kind === 'success' ? <CheckCircle2 size={18} /> : <AlertCircle size={18} />}<div>{children}</div></div>
}

/** Keep labels outside interactive children (dialogs and comboboxes contain buttons). */
export function Field({ label, children, hint, htmlFor }: { label: string; children: ReactNode; hint?: string; htmlFor?: string }) {
  const generated = useId()
  const control = isValidElement(children) ? children as ReactElement<{ id?: string; 'aria-describedby'?: string }> : null
  const id = htmlFor || control?.props.id || generated
  const hintId = `${id}-hint`
  const content = control && typeof control.type === 'string' && ['input', 'textarea', 'select'].includes(control.type)
    ? cloneElement(control, { id, 'aria-describedby': [control.props['aria-describedby'], hint ? hintId : ''].filter(Boolean).join(' ') || undefined }) : children
  return <div className="field"><label className="field-label" htmlFor={id}>{label}</label>{content}{hint && <small className="field-hint" id={hintId}>{hint}</small>}</div>
}

export function BackgroundField({ label, value, options, onChange, disabled = false }: { label: string; value: string; options: string[]; onChange: (value: string) => void; disabled?: boolean }) {
  const id = useId(), [open, setOpen] = useState(false), [query, setQuery] = useState('')
  const choices = [...new Set([...options, '未知'])]
  const normalized = query.trim()
  const filtered = choices.filter(item => item.toLocaleLowerCase().includes(normalized.toLocaleLowerCase()))
  const custom = normalized && !choices.includes(normalized)
  function select(next: string) { onChange(next); setOpen(false); setQuery('') }
  return <div className="field"><label className="field-label" id={`${id}-label`} htmlFor={id}>{label}</label><Popover.Root open={open} onOpenChange={next => { setOpen(next); if (next) setQuery('') }}><Popover.Trigger asChild><button type="button" id={id} role="combobox" aria-expanded={open} aria-haspopup="dialog" aria-controls={`${id}-options`} aria-labelledby={`${id}-label ${id}-value`} className={`combobox-trigger ${value ? '' : 'placeholder'}`} disabled={disabled} onKeyDown={event => { if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); setQuery(''); setOpen(true) } }}><span id={`${id}-value`}>{value || '选择或输入'}</span><ChevronsUpDown size={16} /></button></Popover.Trigger><Popover.Portal><Popover.Content id={`${id}-options`} aria-labelledby={`${id}-label`} className="combobox-popover" align="start" sideOffset={5} collisionPadding={12}><Command shouldFilter={false} label={`${label}选项`}><div className="combobox-search"><Search size={15} /><Command.Input autoFocus value={query} onValueChange={setQuery} placeholder={`搜索或输入${label}`} maxLength={500} aria-label={`搜索或输入${label}`} /></div><Command.List className="combobox-options">{filtered.map(item => <Command.Item key={item} value={item} className="combobox-option" onSelect={() => select(item)}><span>{item}</span>{value === item && <Check size={15} />}</Command.Item>)}{custom && <Command.Item value={`custom:${normalized}`} className="combobox-option combobox-custom" onSelect={() => select(normalized)}>使用“{normalized}”</Command.Item>}{!filtered.length && !custom && <Command.Empty className="combobox-empty">输入自定义内容，或选择未知。</Command.Empty>}</Command.List></Command></Popover.Content></Popover.Portal></Popover.Root></div>
}

export function Dialog({ open, onOpenChange, title, description, children, footer, wide = false, busy = false }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; children: ReactNode; footer?: ReactNode; wide?: boolean; busy?: boolean }) {
  const descriptionId = useId()
  const returnFocus = useRef<HTMLElement | null>(null)
  useEffect(() => {
    if (open) return
    const remember = (event?: Event) => {
      const target = event?.target instanceof Element ? event.target.closest<HTMLElement>('button, input, select, textarea, a[href], [tabindex]') : document.activeElement
      if (target instanceof HTMLElement && target !== document.body && !target.closest('.workbench-dialog')) returnFocus.current = target
    }
    remember()
    document.addEventListener('focusin', remember)
    document.addEventListener('pointerdown', remember, true)
    return () => { document.removeEventListener('focusin', remember); document.removeEventListener('pointerdown', remember, true) }
  }, [open])
  return <DialogPrimitive.Root open={open} onOpenChange={next => { if (!busy) onOpenChange(next) }}><DialogPrimitive.Portal><DialogPrimitive.Overlay className="workbench-overlay" /><DialogPrimitive.Content className={`workbench-dialog ${wide ? 'wide' : ''}`} aria-describedby={description ? descriptionId : undefined} onCloseAutoFocus={event => { event.preventDefault(); if (returnFocus.current?.isConnected) returnFocus.current.focus() }} onEscapeKeyDown={event => { if (busy) event.preventDefault() }} onPointerDownOutside={event => { if (busy) event.preventDefault() }}><div className="dialog-head"><div><DialogPrimitive.Title>{title}</DialogPrimitive.Title>{description && <DialogPrimitive.Description id={descriptionId} className="dialog-description">{description}</DialogPrimitive.Description>}</div><DialogPrimitive.Close asChild><Button className="dialog-close" variant="quiet" aria-label="关闭窗口" disabled={busy}><X size={18} /></Button></DialogPrimitive.Close></div><div className="dialog-body">{children}</div>{footer && <div className="dialog-footer">{footer}</div>}</DialogPrimitive.Content></DialogPrimitive.Portal></DialogPrimitive.Root>
}

export function Disclosure({ title, meta, children, defaultOpen = false, className = '' }: { title: string; meta?: ReactNode; children: ReactNode; defaultOpen?: boolean; className?: string }) {
  return <Collapsible.Root className={`disclosure ${className}`} defaultOpen={defaultOpen}><Collapsible.Trigger className="disclosure-trigger"><span className="disclosure-heading">{title}</span>{meta && <span className="disclosure-meta">{meta}</span>}<ChevronDown size={16} aria-hidden="true" /></Collapsible.Trigger><Collapsible.Content className="disclosure-body">{children}</Collapsible.Content></Collapsible.Root>
}

export function ResultMeta({ report }: { report: { created_at: string; timings?: JobTimings } }) {
  const milliseconds = report.timings?.total_ms
  const duration = typeof milliseconds === 'number' && Number.isFinite(milliseconds) && milliseconds >= 0 ? milliseconds < 1000 ? '不足 1 秒' : milliseconds < 60000 ? `${(milliseconds / 1000).toFixed(1)} 秒` : `${Math.floor(milliseconds / 60000)} 分 ${Math.floor(milliseconds % 60000 / 1000)} 秒` : '—'
  return <div className="result-meta"><span>生成于 <RecordDate value={report.created_at} /></span><span><Clock3 size={13} aria-hidden="true" />分析耗时 {duration}</span></div>
}

export function NextButton({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) { return <Button {...props}>{children}<ArrowRight size={16} /></Button> }

export function RecordDate({ value }: { value?: string }) {
  if (!value) return <span>—</span>
  const date = new Date(value)
  return <time dateTime={value} title={Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { hour12: false })}>{Number.isNaN(date.valueOf()) ? value : date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })}</time>
}
