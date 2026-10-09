import { useEffect, useRef } from 'react'

// 程序化生成显微组织示意（演示用合成图，不代表真实试样或测量标尺）
// overlay: none 原图 | annotate AI 标注 | segment 分割热力

interface Props {
  overlay?: 'none' | 'annotate' | 'segment'
  anchors?: { id: number; x: number; y: number; severity: 'high' | 'mid' | 'low' }[]
  activeAnchor?: number | null
  phases?: { name: string; color: string }[]
  scanning?: boolean
  className?: string
}

// 可复现伪随机
function mulberry32(seed: number) {
  return function () {
    seed |= 0; seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const W = 960, H = 640

function drawMicrostructure(ctx: CanvasRenderingContext2D) {
  const rnd = mulberry32(20260920)

  // 铁素体基体：浅灰底 + 晶粒团块
  ctx.fillStyle = '#b8bdc2'
  ctx.fillRect(0, 0, W, H)

  // 晶粒：不规则多边形拼接感
  const grains: { cx: number; cy: number; r: number }[] = []
  for (let i = 0; i < 90; i++) {
    grains.push({ cx: rnd() * W, cy: rnd() * H, r: 34 + rnd() * 56 })
  }
  grains.forEach((g) => {
    const tone = 168 + Math.floor(rnd() * 40)
    ctx.beginPath()
    const n = 7
    for (let i = 0; i <= n; i++) {
      const a = (i / n) * Math.PI * 2
      const rr = g.r * (0.75 + rnd() * 0.5)
      const x = g.cx + Math.cos(a) * rr
      const y = g.cy + Math.sin(a) * rr
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    }
    ctx.closePath()
    ctx.fillStyle = `rgb(${tone},${tone + 3},${tone + 6})`
    ctx.fill()
  })

  // 晶界：描绘多边形边缘（再描一遍 stroke）
  const rnd2 = mulberry32(777)
  ctx.strokeStyle = 'rgba(70,76,82,0.55)'
  ctx.lineWidth = 1.4
  grains.forEach((g) => {
    ctx.beginPath()
    const n = 7
    for (let i = 0; i <= n; i++) {
      const a = (i / n) * Math.PI * 2
      const rr = g.r * (0.75 + rnd2() * 0.5)
      const x = g.cx + Math.cos(a) * rr
      const y = g.cy + Math.sin(a) * rr
      if (i === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    }
    ctx.closePath()
    ctx.stroke()
  })

  // 球状碳化物：弥散深色小颗粒
  const rnd3 = mulberry32(45111)
  for (let i = 0; i < 620; i++) {
    const x = rnd3() * W, y = rnd3() * H
    const r = 1.4 + rnd3() * 3.6
    const g = rnd3()
    ctx.beginPath()
    ctx.arc(x, y, r, 0, Math.PI * 2)
    ctx.fillStyle = g > 0.85 ? 'rgba(28,30,34,0.95)' : `rgba(45,48,54,${0.75 + rnd3() * 0.2})`
    ctx.fill()
    // 颗粒亮边
    if (r > 3) {
      ctx.beginPath()
      ctx.arc(x - r * 0.3, y - r * 0.3, r * 0.4, 0, Math.PI * 2)
      ctx.fillStyle = 'rgba(220,224,228,0.28)'
      ctx.fill()
    }
  }

  // 残留珠光体片层团（视场右下、左中两块）
  const lamellar = (cx: number, cy: number, w: number, h: number, angle: number) => {
    ctx.save()
    ctx.translate(cx, cy)
    ctx.rotate(angle)
    ctx.beginPath()
    ctx.ellipse(0, 0, w / 2, h / 2, 0, 0, Math.PI * 2)
    ctx.clip()
    ctx.fillStyle = 'rgba(90,96,104,0.5)'
    ctx.fillRect(-w / 2, -h / 2, w, h)
    ctx.strokeStyle = 'rgba(35,38,44,0.85)'
    ctx.lineWidth = 1.1
    for (let y = -h / 2; y < h / 2; y += 4.5) {
      ctx.beginPath()
      ctx.moveTo(-w / 2, y)
      ctx.lineTo(w / 2, y)
      ctx.stroke()
    }
    ctx.restore()
  }
  lamellar(720, 480, 150, 110, 0.5)
  lamellar(210, 420, 120, 90, -0.35)

  // 缺陷 1：两条贯穿粗划痕（斜向）
  const scratch = (x0: number, y0: number, x1: number, y1: number, w: number, alpha: number) => {
    ctx.save()
    ctx.strokeStyle = `rgba(52,55,60,${alpha})`
    ctx.lineWidth = w
    ctx.lineCap = 'round'
    ctx.beginPath(); ctx.moveTo(x0, y0); ctx.lineTo(x1, y1); ctx.stroke()
    ctx.strokeStyle = `rgba(230,233,236,${alpha * 0.5})`
    ctx.lineWidth = w * 0.35
    ctx.beginPath(); ctx.moveTo(x0, y0 - w * 0.28); ctx.lineTo(x1, y1 - w * 0.28); ctx.stroke()
    ctx.restore()
  }
  scratch(60, 320, 620, 130, 5.5, 0.85)
  scratch(150, 500, 830, 250, 4, 0.7)

  // 缺陷 2：抛光曳尾带（中部发亮单向条纹）
  ctx.save()
  ctx.globalAlpha = 0.16
  ctx.fillStyle = '#e6e9ec'
  for (let i = 0; i < 26; i++) {
    ctx.fillRect(480 + i * 4.2, 330 + i * 1.1, 2.4, 90)
  }
  ctx.restore()

  // 缺陷 3：右下局部腐蚀过深（发黑区）
  const g1 = ctx.createRadialGradient(760, 520, 20, 760, 520, 170)
  g1.addColorStop(0, 'rgba(24,26,30,0.55)')
  g1.addColorStop(1, 'rgba(24,26,30,0)')
  ctx.fillStyle = g1
  ctx.fillRect(560, 330, 400, 310)

  // 缺陷 4：左上零星细抛痕
  ctx.strokeStyle = 'rgba(238,240,242,0.35)'
  ctx.lineWidth = 1.1
  for (let i = 0; i < 8; i++) {
    ctx.beginPath()
    ctx.moveTo(90 + i * 14, 60)
    ctx.lineTo(150 + i * 14, 190)
    ctx.stroke()
  }

  // 轻微暗角（显微视场感）
  const vg = ctx.createRadialGradient(W / 2, H / 2, H * 0.35, W / 2, H / 2, H * 0.85)
  vg.addColorStop(0, 'rgba(0,0,0,0)')
  vg.addColorStop(1, 'rgba(0,0,0,0.38)')
  ctx.fillStyle = vg
  ctx.fillRect(0, 0, W, H)
}

const SEV_COLOR = { high: '#c1573a', mid: '#d59a3d', low: '#8b959e' }

export default function MicroCanvas({
  overlay = 'none', anchors = [], activeAnchor = null, phases = [], scanning = false, className = '',
}: Props) {
  const ref = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const cv = ref.current
    if (!cv) return
    const ctx = cv.getContext('2d')
    if (!ctx) return
    drawMicrostructure(ctx)

    if (overlay === 'segment' && phases.length > 0) {
      // 分割热力：按相叠加半透明染色（碳化物颗粒描蓝、基体灰、残留珠光体红）
      const [carb, ferr, pear] = phases
      ctx.save()
      ctx.globalCompositeOperation = 'source-atop'
      ctx.fillStyle = `${ferr.color}22`
      ctx.fillRect(0, 0, W, H)
      ctx.restore()
      // 珠光体团红晕
      const heat = (cx: number, cy: number, r: number) => {
        const g = ctx.createRadialGradient(cx, cy, 8, cx, cy, r)
        g.addColorStop(0, `${pear.color}55`)
        g.addColorStop(1, `${pear.color}00`)
        ctx.fillStyle = g
        ctx.fillRect(cx - r, cy - r, r * 2, r * 2)
      }
      heat(720, 480, 120); heat(210, 420, 95)
      // 碳化物颗粒蓝点描边（复画一层小颗粒）
      const rnd = mulberry32(45111)
      for (let i = 0; i < 620; i++) {
        const x = rnd() * W, y = rnd() * H
        const r = 1.4 + rnd() * 3.6
        rnd() // 保持序列一致
        ctx.beginPath()
        ctx.arc(x, y, r + 1.2, 0, Math.PI * 2)
        ctx.strokeStyle = `${carb.color}66`
        ctx.lineWidth = 1
        ctx.stroke()
      }
    }
  }, [overlay, phases])

  return (
    <div className={`relative overflow-hidden ${className}`}>
      <canvas ref={ref} width={W} height={H} className="block h-auto w-full" />
      {/* 视场框四角 */}
      <div className="pointer-events-none absolute inset-3 border border-white/25" />
      <div className="pointer-events-none absolute left-3 top-3 h-4 w-4 border-l-2 border-t-2 border-white/60" />
      <div className="pointer-events-none absolute right-3 top-3 h-4 w-4 border-r-2 border-t-2 border-white/60" />
      <div className="pointer-events-none absolute bottom-3 left-3 h-4 w-4 border-b-2 border-l-2 border-white/60" />
      <div className="pointer-events-none absolute bottom-3 right-3 h-4 w-4 border-b-2 border-r-2 border-white/60" />


      {/* 缺陷编号锚点 */}
      {overlay === 'annotate' &&
        anchors.map((a) => (
          <button
            key={a.id}
            className="absolute flex h-6 w-6 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border text-[11px] font-semibold transition-transform hover:scale-110"
            style={{
              left: `${a.x}%`, top: `${a.y}%`,
              borderColor: SEV_COLOR[a.severity],
              backgroundColor: 'rgba(13,17,20,0.85)',
              color: SEV_COLOR[a.severity],
              boxShadow: activeAnchor === a.id ? `0 0 0 3px ${SEV_COLOR[a.severity]}55` : undefined,
            }}
          >
            {a.id}
          </button>
        ))}

      {scanning && <div className="scan-line" />}
    </div>
  )
}
