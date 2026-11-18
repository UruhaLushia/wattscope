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

func RecordClickOrigin(el ui.Element) {
	b := el.LayoutBounds()
	if b.W <= 0 || b.H <= 0 {
		return
	}
	clickOriginMu.Lock()
	lastClickPt = ui.Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
	lastClickValid = true
	clickOriginMu.Unlock()
}

type ThemeRevealOrigin struct {
	point ui.Point
	valid bool
}

func CaptureThemeRevealOrigin(el ui.Element) ThemeRevealOrigin {
	TakeThemeRevealOrigin()
	b := el.LayoutBounds()
	return ThemeRevealOrigin{point: ui.Point{X: b.X + b.W/2, Y: b.Y + b.H/2}, valid: b.W > 0 && b.H > 0}
}

func TakeThemeRevealOrigin() ThemeRevealOrigin {
	clickOriginMu.Lock()
	defer clickOriginMu.Unlock()
	origin := ThemeRevealOrigin{point: lastClickPt, valid: lastClickValid}
	lastClickValid = false
	return origin
}

type ThemeReveal struct {
	waves []themeRevealWave
}

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
	pixels := revealSourceRect(rect, painter.Scale(), layer.origin.X, layer.origin.Y)
	options := ui.ImageOptions{}
	if alpha < 1 {
		options.Tint = ui.RGBA(255, 255, 255, alpha)
	}
	painter.ImageRegion(layer.bitmap, pixels, rect, options)
}

func (themeRevealAlternate) EffectMargin() ui.Point { return ui.Point{} }

func (effect themeRevealAlternate) PaintEffect(painter *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	effect.composite.alternate = revealLayer{
		bitmap: content,
		origin: ui.Point{X: box.X*painter.Scale() - inset.X, Y: box.Y*painter.Scale() - inset.Y},
	}
}

func (*themeRevealComposite) EffectMargin() ui.Point { return ui.Point{} }

func (effect *themeRevealComposite) PaintEffect(painter *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	current := revealLayer{
		bitmap: content,
		origin: ui.Point{X: box.X*painter.Scale() - inset.X, Y: box.Y*painter.Scale() - inset.Y},
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
		wave.paint(painter, box, now.Sub(wave.startedAt), func(rect ui.Rect, alpha float32) {
			layer.draw(painter, rect, alpha)
		})
	}
}

func (wave themeRevealWave) paint(painter *ui.Painter, box ui.Rect, elapsed time.Duration, draw func(ui.Rect, float32)) {
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
	core := radius - margin
	if core > 0 {
		circle := ui.Rect{X: box.X + centerX - core, Y: box.Y + centerY - core, W: 2 * core, H: 2 * core}
		painter.Clip(circle, core, func() { draw(box, 1) })
	}
	wave.paintTiles(box, centerX, centerY, radius, max(0, core-revealTileSize), draw)
}

func (wave themeRevealWave) paintTiles(box ui.Rect, centerX, centerY, radius, inner float32, draw func(ui.Rect, float32)) {
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
			draw(ui.Rect{X: box.X + x, Y: box.Y + y, W: right - x, H: height}, alpha)
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
