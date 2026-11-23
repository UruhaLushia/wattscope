package kit

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestComputeRevealInnerRect(t *testing.T) {
	box := ui.Rect{X: 3, Y: 5, W: 96, H: 64}
	for _, scale := range []float32{1.0, 1.5, 2.0} {
		bx0, by0 := int(math.Round(float64(box.X*scale))), int(math.Round(float64(box.Y*scale)))
		bx1, by1 := int(math.Round(float64((box.X+box.W)*scale))), int(math.Round(float64((box.Y+box.H)*scale)))

		if got := computeRevealInnerRect(box, revealWaveGeometry{centerX: 48, centerY: 32, core: 1}, bx0, by0, bx1, by1, scale); got.hasInner {
			t.Fatalf("scale=%.2f: small radius expected !hasInner, got %+v", scale, got)
		}

		mid := computeRevealInnerRect(box, revealWaveGeometry{centerX: 30, centerY: 24, core: 42}, bx0, by0, bx1, by1, scale)
		if !mid.hasInner || mid.rx0 < bx0 || mid.ry0 < by0 || mid.rx1 > bx1 || mid.ry1 > by1 || mid.rx0 >= mid.rx1 || mid.ry0 >= mid.ry1 {
			t.Fatalf("scale=%.2f: invalid inner rect %+v in [%d,%d)×[%d,%d)", scale, mid, bx0, bx1, by0, by1)
		}
		limit := float64(42 * scale)
		for _, pt := range [4][2]int{{mid.rx0, mid.ry0}, {mid.rx1, mid.ry0}, {mid.rx0, mid.ry1}, {mid.rx1, mid.ry1}} {
			if dist := math.Hypot(float64(pt[0])-float64(mid.cxPx), float64(pt[1])-float64(mid.cyPx)); dist >= limit {
				t.Fatalf("scale=%.2f: corner %v outside core circle (dist=%.3f >= %.3f)", scale, pt, dist, limit)
			}
		}

		fullCore := float32(math.Hypot(float64(box.W), float64(box.H))) + 8
		full := computeRevealInnerRect(box, revealWaveGeometry{centerX: 0, centerY: 0, core: fullCore}, bx0, by0, bx1, by1, scale)
		if !full.hasInner || full.rx0 != bx0 || full.ry0 != by0 || full.rx1 != bx1 || full.ry1 != by1 {
			t.Fatalf("scale=%.2f: expected full box [%d,%d)×[%d,%d), got %+v", scale, bx0, bx1, by0, by1, full)
		}
	}

	for _, tc := range []struct {
		alpha float32
		want  uint8
	}{{0, 0}, {0.001, 0}, {0.999, 255}, {1, 255}} {
		if got := ui.RGBA(255, 255, 255, tc.alpha).A; got != tc.want {
			t.Fatalf("ui.RGBA alpha(%g) = %d, want %d", tc.alpha, got, tc.want)
		}
	}
}
