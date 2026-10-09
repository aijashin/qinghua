import { useEffect, useId } from 'react'

const drafts = new Set<string>()
export function useDraftGuard(dirty: boolean) {
  const id = useId()
  useEffect(() => {
    if (dirty) drafts.add(id)
    else drafts.delete(id)
    const beforeUnload = (event: BeforeUnloadEvent) => { if (dirty) { event.preventDefault(); event.returnValue = '' } }
    window.addEventListener('beforeunload', beforeUnload)
    return () => { drafts.delete(id); window.removeEventListener('beforeunload', beforeUnload) }
  }, [dirty, id])
}
export function confirmNavigation() {
  return !drafts.size || window.confirm('当前有未保存的编辑内容。确定离开并放弃这些编辑吗？')
}
