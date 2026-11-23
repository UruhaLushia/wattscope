package kit

import (
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	themeRevealDuration = 600 * time.Millisecond
	revealTileSize      = float32(7)
	revealNoiseRadius   = 3 * revealTileSize
	revealTileFade      = 0.65 * revealTileSize
)

var (
	clickOriginMu  sync.Mutex
	lastClickPt    ui.Point
	lastClickValid bool
)

func recordClickOrigin(el ui.Element) {
	b := el.LayoutBounds()
	if b.W <= 0 || b.H <= 0 {
		return
	}
	clickOriginMu.Lock()
	lastClickPt = ui.Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
	lastClickValid = true
	clickOriginMu.Unlock()
}

// ThemeRevealOrigin records the viewport point from which a theme reveal wave expands.
type ThemeRevealOrigin struct {
	point ui.Point
	valid bool
}

// CaptureThemeRevealOrigin returns el's center as a reveal origin and clears any previously recorded click origin so menu/toggle reveals do not clash.
func CaptureThemeRevealOrigin(el ui.Element) ThemeRevealOrigin {
	TakeThemeRevealOrigin()
	b := el.LayoutBounds()
	return ThemeRevealOrigin{point: ui.Point{X: b.X + b.W/2, Y: b.Y + b.H/2}, valid: b.W > 0 && b.H > 0}
}

// TakeThemeRevealOrigin returns and clears the last recorded click origin for theme transitions triggered from menus.
func TakeThemeRevealOrigin() ThemeRevealOrigin {
	clickOriginMu.Lock()
	defer clickOriginMu.Unlock()
	origin := ThemeRevealOrigin{point: lastClickPt, valid: lastClickValid}
	lastClickValid = false
	return origin
}

// ThemeReveal manages concentric tile reveal waves across theme transitions.
type ThemeReveal struct {
	waves []themeRevealWave
}

// Start begins a new theme reveal wave toward dark from origin.
func (r *ThemeReveal) Start(ctx *ui.Context, dark bool, origin ThemeRevealOrigin) {
	if ctx.Preferences().ReduceMotion {
		r.waves = nil
		return
	}
	now := ctx.Now()
	r.waves = activeRevealWaves(r.waves, now)
	if len(r.waves) != 0 && r.waves[len(r.waves)-1].dark == dark {
		r.waves = nil
	}
	r.waves = append(r.waves, themeRevealWave{origin: origin, dark: dark, startedAt: now, seed: rand.Uint32()})
	ctx.DisableAnimations()
}

// Sync returns the effects for the alternate and current palettes, in paint order.
func (r *ThemeReveal) Sync(ctx *ui.Context, dark bool) (alternate, current ui.Effect) {
	now := ctx.Now()
	r.waves = activeRevealWaves(r.waves, now)
	if len(r.waves) == 0 {
		return nil, nil
	}
	latest := r.waves[len(r.waves)-1]
	if ctx.Preferences().ReduceMotion || latest.dark != dark {
		r.waves = nil
		return nil, nil
	}
	ctx.After(themeRevealDuration - now.Sub(latest.startedAt))
	composite := &themeRevealComposite{waves: r.waves, dark: dark}
	return themeRevealAlternate{composite: composite}, composite
}

func activeRevealWaves(waves []themeRevealWave, now time.Time) []themeRevealWave {
	for index, wave := range waves {
		if now.Sub(wave.startedAt) < themeRevealDuration {
			return waves[index:]
		}
	}
	return nil
}

type themeRevealWave struct {
	origin    ThemeRevealOrigin
	dark      bool
	startedAt time.Time
	seed      uint32
}

type themeRevealComposite struct {
	waves     []themeRevealWave
	dark      bool
	alternate revealLayer
}

type themeRevealAlternate struct {
	composite *themeRevealComposite
}

type revealLayer struct {
	bitmap *ui.Bitmap
	origin ui.Point
}

func (layer revealLayer) draw(painter *ui.Painter, rect ui.Rect, alpha float32) {
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	var options ui.ImageOptions
	if alpha < 1 {
		tint := ui.RGBA(255, 255, 255, alpha)
		if tint.A == 0 {
			return
		}
		if tint.A < 255 {
			options.Tint = tint
		}
	}
	pixels := revealSourceRect(rect, painter.Scale(), layer.origin.X, layer.origin.Y)
	if pixels.W <= 0 || pixels.H <= 0 {
		return
	}
	painter.ImageRegion(layer.bitmap, pixels, rect, options)
}

func revealLayerOrigin(box ui.Rect, scale float32, inset ui.Point) ui.Point {
	return ui.Point{
		X: float32(math.Round(float64(box.X*scale - inset.X))),
		Y: float32(math.Round(float64(box.Y*scale - inset.Y))),
	}
}

func revealPixelRect(px0, py0, px1, py1 int, scale float32) ui.Rect {
	x0, y0 := float32(px0)/scale, float32(py0)/scale
	x1, y1 := float32(px1)/scale, float32(py1)/scale
	return ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func (themeRevealAlternate) EffectMargin() ui.Point { return ui.Point{} }

func (effect themeRevealAlternate) PaintEffect(painter *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	effect.composite.alternate = revealLayer{
		bitmap: content,
		origin: revealLayerOrigin(box, painter.Scale(), inset),
	}
}

func (*themeRevealComposite) EffectMargin() ui.Point { return ui.Point{} }

func (effect *themeRevealComposite) PaintEffect(painter *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	scale := painter.Scale()
	current := revealLayer{
		bitmap: content,
		origin: revealLayerOrigin(box, scale, inset),
	}
	alternate := effect.alternate
	effect.alternate = revealLayer{}
	now := painter.Now()
	waves := activeRevealWaves(effect.waves, now)
	if len(waves) == 0 || alternate.bitmap == nil {
		current.draw(painter, box, 1)
		return
	}
	painter.AnimationFrame()
	bx0 := int(math.Round(float64(box.X * scale)))
	by0 := int(math.Round(float64(box.Y * scale)))
	bx1 := int(math.Round(float64((box.X + box.W) * scale)))
	by1 := int(math.Round(float64((box.Y + box.H) * scale)))
	if len(waves) == 1 {
		wave := waves[0]
		fgLayer, bgLayer := alternate, current
		if wave.dark == effect.dark {
			fgLayer, bgLayer = current, alternate
		}
		geom := wave.geometry(box, now.Sub(wave.startedAt))
		inner := computeRevealInnerRect(box, geom, bx0, by0, bx1, by1, scale)
		if !inner.hasInner {
			bgLayer.draw(painter, box, 1)
		} else {
			if by0 < inner.ry0 {
				bgLayer.draw(painter, revealPixelRect(bx0, by0, bx1, inner.ry0, scale), 1)
			}
			if inner.ry1 < by1 {
				bgLayer.draw(painter, revealPixelRect(bx0, inner.ry1, bx1, by1, scale), 1)
			}
			if bx0 < inner.rx0 {
				bgLayer.draw(painter, revealPixelRect(bx0, inner.ry0, inner.rx0, inner.ry1, scale), 1)
			}
			if inner.rx1 < bx1 {
				bgLayer.draw(painter, revealPixelRect(inner.rx1, inner.ry0, bx1, inner.ry1, scale), 1)
			}
		}
		wave.paintCore(painter, geom, inner, bx0, by0, bx1, by1, scale, fgLayer)
		wave.paintTiles(painter, box, geom.centerX, geom.centerY, geom.radius, max(0, geom.core-revealTileSize), fgLayer)
		return
	}
	if waves[0].dark == effect.dark {
		alternate.draw(painter, box, 1)
	} else {
		current.draw(painter, box, 1)
	}
	for _, wave := range waves {
		layer := alternate
		if wave.dark == effect.dark {
			layer = current
		}
		geom := wave.geometry(box, now.Sub(wave.startedAt))
		inner := computeRevealInnerRect(box, geom, bx0, by0, bx1, by1, scale)
		wave.paintCore(painter, geom, inner, bx0, by0, bx1, by1, scale, layer)
		wave.paintTiles(painter, box, geom.centerX, geom.centerY, geom.radius, max(0, geom.core-revealTileSize), layer)
	}
}

type revealWaveGeometry struct {
	centerX float32
	centerY float32
	radius  float32
	core    float32
}

type revealInnerRect struct {
	circle     ui.Rect
	scX0, scY0 float32
	scX1, scY1 float32
	cxPx, cyPx float32
	rx0, ry0   int
	rx1, ry1   int
	hasInner   bool
}

func (wave themeRevealWave) geometry(box ui.Rect, elapsed time.Duration) revealWaveGeometry {
	centerX, centerY := box.W/2, box.H/2
	if wave.origin.valid {
		centerX = min(max(wave.origin.point.X-box.X, 0), box.W)
		centerY = min(max(wave.origin.point.Y-box.Y, 0), box.H)
	}
	farthestX := max(centerX, box.W-centerX)
	farthestY := max(centerY, box.H-centerY)
	limit := float32(math.Hypot(float64(farthestX), float64(farthestY)))
	tail := 1 - max(0, float32(elapsed)/float32(themeRevealDuration))
	progress := 1 - tail*(0.18+0.82*tail)
	margin := revealNoiseRadius + revealTileFade + revealTileSize
	radius := -margin + progress*(limit+2*margin)
	return revealWaveGeometry{
		centerX: centerX,
		centerY: centerY,
		radius:  radius,
		core:    radius - margin,
	}
}

func computeRevealInnerRect(box ui.Rect, geom revealWaveGeometry, bx0, by0, bx1, by1 int, scale float32) revealInnerRect {
	if geom.core <= 0 || bx0 >= bx1 || by0 >= by1 {
		return revealInnerRect{}
	}
	circle := ui.Rect{
		X: box.X + geom.centerX - geom.core,
		Y: box.Y + geom.centerY - geom.core,
		W: 2 * geom.core,
		H: 2 * geom.core,
	}
	scX0 := float32(math.Round(float64(circle.X * scale)))
	scY0 := float32(math.Round(float64(circle.Y * scale)))
	scX1 := float32(math.Round(float64((circle.X + circle.W) * scale)))
	scY1 := float32(math.Round(float64((circle.Y + circle.H) * scale)))
	cxPx := 0.5 * (scX0 + scX1)
	cyPx := 0.5 * (scY0 + scY1)
	out := revealInnerRect{
		circle: circle,
		scX0:   scX0,
		scY0:   scY0,
		scX1:   scX1,
		scY1:   scY1,
		cxPx:   cxPx,
		cyPx:   cyPx,
	}
	rPx := min(geom.core*scale, min(0.5*(scX1-scX0), 0.5*(scY1-scY0)))
	rSafe := float64(rPx - 2.5)
	if rSafe <= 0 {
		return out
	}
	rSafeSq := rSafe * rSafe
	sqHalf := rSafe * (math.Sqrt2 / 2)
	cx64, cy64 := float64(cxPx), float64(cyPx)
	bestArea := 0

	// Candidate 1: clamp Y to viewport first, then expand X via Pythagoras.
	y0 := max(by0, int(math.Ceil(cy64-sqHalf)))
	y1 := min(by1, int(math.Floor(cy64+sqHalf)))
	if y0 < y1 {
		maxDY := max(math.Abs(float64(y0)-cy64), math.Abs(float64(y1)-cy64))
		if rem := rSafeSq - maxDY*maxDY; rem > 0 {
			halfX := math.Sqrt(rem)
			x0 := max(bx0, int(math.Ceil(cx64-halfX)))
			x1 := min(bx1, int(math.Floor(cx64+halfX)))
			if x0 < x1 {
				if area := (x1 - x0) * (y1 - y0); area > bestArea {
					bestArea = area
					out.rx0, out.ry0, out.rx1, out.ry1 = x0, y0, x1, y1
					out.hasInner = true
				}
			}
		}
	}

	// Candidate 2: clamp X to viewport first, then expand Y via Pythagoras.
	x0 := max(bx0, int(math.Ceil(cx64-sqHalf)))
	x1 := min(bx1, int(math.Floor(cx64+sqHalf)))
	if x0 < x1 {
		maxDX := max(math.Abs(float64(x0)-cx64), math.Abs(float64(x1)-cx64))
		if rem := rSafeSq - maxDX*maxDX; rem > 0 {
			halfY := math.Sqrt(rem)
			y0 := max(by0, int(math.Ceil(cy64-halfY)))
			y1 := min(by1, int(math.Floor(cy64+halfY)))
			if y0 < y1 {
				if area := (x1 - x0) * (y1 - y0); area > bestArea {
					out.rx0, out.ry0, out.rx1, out.ry1 = x0, y0, x1, y1
					out.hasInner = true
				}
			}
		}
	}

	return out
}

func (wave themeRevealWave) paintCore(painter *ui.Painter, geom revealWaveGeometry, inner revealInnerRect, bx0, by0, bx1, by1 int, scale float32, layer revealLayer) {
	if geom.core <= 0 {
		return
	}
	cx0 := max(bx0, int(inner.scX0))
	cy0 := max(by0, int(inner.scY0))
	cx1 := min(bx1, int(inner.scX1))
	cy1 := min(by1, int(inner.scY1))
	if cx0 >= cx1 || cy0 >= cy1 {
		return
	}
	if !inner.hasInner {
		rect := revealPixelRect(cx0, cy0, cx1, cy1, scale)
		painter.Clip(inner.circle, geom.core, func() {
			layer.draw(painter, rect, 1)
		})
		return
	}

	layer.draw(painter, revealPixelRect(inner.rx0, inner.ry0, inner.rx1, inner.ry1, scale), 1)

	var crescents [4]ui.Rect
	count := 0
	rOut := float64(max(geom.core*scale, 0.5*max(inner.scX1-inner.scX0, inner.scY1-inner.scY0)) + 3)
	rOutSq := rOut * rOut
	cx64, cy64 := float64(inner.cxPx), float64(inner.cyPx)

	if cy0 < inner.ry0 {
		dyMin := max(0, cy64-float64(inner.ry0))
		if rem := rOutSq - dyMin*dyMin; rem > 0 {
			hx := math.Sqrt(rem)
			tx0 := max(cx0, int(math.Floor(cx64-hx)))
			tx1 := min(cx1, int(math.Ceil(cx64+hx)))
			if tx0 < tx1 {
				crescents[count] = revealPixelRect(tx0, cy0, tx1, inner.ry0, scale)
				count++
			}
		}
	}
	if inner.ry1 < cy1 {
		dyMin := max(0, float64(inner.ry1)-cy64)
		if rem := rOutSq - dyMin*dyMin; rem > 0 {
			hx := math.Sqrt(rem)
			tx0 := max(cx0, int(math.Floor(cx64-hx)))
			tx1 := min(cx1, int(math.Ceil(cx64+hx)))
			if tx0 < tx1 {
				crescents[count] = revealPixelRect(tx0, inner.ry1, tx1, cy1, scale)
				count++
			}
		}
	}
	if cx0 < inner.rx0 {
		crescents[count] = revealPixelRect(cx0, inner.ry0, inner.rx0, inner.ry1, scale)
		count++
	}
	if inner.rx1 < cx1 {
		crescents[count] = revealPixelRect(inner.rx1, inner.ry0, cx1, inner.ry1, scale)
		count++
	}
	if count == 0 {
		return
	}
	painter.Clip(inner.circle, geom.core, func() {
		for i := range count {
			layer.draw(painter, crescents[i], 1)
		}
	})
}

func (wave themeRevealWave) paintTiles(painter *ui.Painter, box ui.Rect, centerX, centerY, radius, inner float32, layer revealLayer) {
	cell := revealTileSize
	outer := radius + revealNoiseRadius
	if outer <= 0 {
		return
	}
	cols := int(math.Ceil(float64(box.W / cell)))
	rows := int(math.Ceil(float64(box.H / cell)))
	rowStart, rowEnd := revealTileSpan(centerY, outer, rows)
	for row := rowStart; row < rowEnd; row++ {
		y := float32(row) * cell
		dy := y + cell/2 - centerY
		vertical := dy * dy
		if vertical >= outer*outer {
			continue
		}
		edge := float32(math.Sqrt(float64(outer*outer - vertical)))
		start, end := revealTileSpan(centerX, edge, cols)
		height := min(cell, box.H-y)
		tileRun := func(first, last int, alpha float32) {
			x := float32(first) * cell
			right := min(float32(last)*cell, box.W)
			layer.draw(painter, ui.Rect{X: box.X + x, Y: box.Y + y, W: right - x, H: height}, alpha)
		}
		paintSpan := func(first, last int) {
			run := -1
			for column := first; column < last; column++ {
				dx := (float32(column)+0.5)*cell - centerX
				distance := float32(math.Sqrt(float64(dx*dx + vertical)))
				fine := revealNoise(wave.seed, column, row)
				coarse := revealNoise(wave.seed^0xa511e9b3, column/3, row/3)
				offset := revealNoiseRadius * (0.62*fine + 0.38*coarse)
				alpha := (radius + offset - distance) / revealTileFade
				if alpha >= 1 {
					if run < 0 {
						run = column
					}
					continue
				}
				if run >= 0 {
					tileRun(run, column, 1)
					run = -1
				}
				if alpha > 0 {
					tileRun(column, column+1, alpha*alpha*(3-2*alpha))
				}
			}
			if run >= 0 {
				tileRun(run, last, 1)
			}
		}
		if vertical >= inner*inner {
			paintSpan(start, end)
			continue
		}
		inside := float32(math.Sqrt(float64(inner*inner - vertical)))
		skipStart, skipEnd := revealTileSpan(centerX, inside, cols)
		paintSpan(start, min(end, skipStart))
		paintSpan(max(start, skipEnd), end)
	}
}

func revealTileSpan(center, radius float32, count int) (int, int) {
	left := int(math.Ceil(float64((center-radius)/revealTileSize - 0.5)))
	right := int(math.Floor(float64((center+radius)/revealTileSize-0.5))) + 1
	return max(0, left), min(count, right)
}

func revealNoise(seed uint32, column, row int) float32 {
	value := seed + uint32(column)*0x9e3779b9 + uint32(row)*0x85ebca6b
	value = (value ^ (value >> 16)) * 0x7feb352d
	value = (value ^ (value >> 15)) * 0x846ca68b
	return float32((value^(value>>16))>>16)/32767.5 - 1
}

func revealSourceRect(rect ui.Rect, scale, bitmapX, bitmapY float32) ui.Rect {
	left := float32(math.Round(float64(rect.X * scale)))
	top := float32(math.Round(float64(rect.Y * scale)))
	right := float32(math.Round(float64((rect.X + rect.W) * scale)))
	bottom := float32(math.Round(float64((rect.Y + rect.H) * scale)))
	return ui.Rect{X: left - bitmapX, Y: top - bitmapY, W: right - left, H: bottom - top}
}
