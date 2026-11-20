package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	MotionFastDur        = 120 * time.Millisecond
	MotionNavDur         = 175 * time.Millisecond
	MotionMoveDur        = 220 * time.Millisecond
	MotionPopupOpenDur   = 180 * time.Millisecond
	MotionPopupCloseDur  = 100 * time.Millisecond
	MotionOptionDur      = 220 * time.Millisecond
	MotionOptionCloseDur = 125 * time.Millisecond
	MotionPopupDur       = 230 * time.Millisecond
	MotionSheetDur       = 260 * time.Millisecond
	MotionPageDur        = 180 * time.Millisecond
	MotionUnfoldDur      = 280 * time.Millisecond
)

var (
	EaseStandard   = ui.CubicBezier(0.2, 0, 0, 1)
	EaseAccelerate = ui.CubicBezier(0.4, 0, 1, 1)

	MotionFast         = ui.Animation{Duration: MotionFastDur, Ease: ui.EaseOut}
	MotionNav          = ui.Animation{Duration: MotionNavDur, Ease: EaseStandard}
	MotionChipOut      = ui.Animation{Duration: time.Nanosecond}
	MotionChipIn       = ui.Animation{Delay: MotionOptionCloseDur, Duration: 75 * time.Millisecond, Ease: EaseStandard}
	MotionCapsuleIn    = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut}
	MotionCapsuleDrag  = ui.Animation{Duration: 22 * time.Millisecond, Ease: ui.EaseOut}
	MotionCapsulePill  = ui.Animation{Duration: MotionNavDur, Ease: EaseStandard}
	MotionMove         = ui.Animation{Duration: MotionMoveDur, Ease: ui.EaseOut}
	MotionLocale       = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.04}}
	MotionPage         = ui.Animation{Duration: MotionPageDur, Ease: EaseStandard}
	MotionRail         = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.14}}
	MotionGlassPress   = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond, Bounce: 0.24}}
	MotionGlassRelease = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.28}}
	MotionGlassScrub   = ui.Animation{Spring: ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.24}}
	MotionNavPanel     = ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.14}}
	MotionNavToggle    = ui.Animation{Spring: ui.SpringOptions{Duration: 310 * time.Millisecond, Bounce: 0.16}}
	MotionSpring       = ui.Animation{Spring: ui.SpringOptions{Duration: MotionMoveDur, Bounce: 0.06}}
	MotionOptionSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 230 * time.Millisecond, Bounce: 0.06}}
	MotionPopup        = ui.Animation{Duration: MotionPopupOpenDur, Ease: EaseStandard}
	MotionOption       = ui.Animation{Duration: MotionOptionDur, Ease: EaseStandard}
	MotionOptionClose  = ui.Animation{Duration: MotionOptionCloseDur, Ease: EaseStandard}
	MotionDialogPop    = ui.Animation{Spring: ui.SpringOptions{Duration: MotionPopupDur, Bounce: 0.06}}
	MotionSheet        = ui.Animation{Spring: ui.SpringOptions{Duration: MotionSheetDur, Bounce: 0.04}}

	ValueMotion          = ui.ContentTransition{Animation: &MotionMove}
	LocaleTextTransition = ui.ContentTransition{Morph: true, Animation: &MotionLocale}
	LocaleSizeTransition = ui.ElementTransition{Position: true, Size: true, Animation: &MotionLocale}

	LayoutTransition      = ui.ElementTransition{Position: true, Size: true, Animation: &MotionMove}
	SideContentTransition = ui.ElementTransition{
		Position:  true,
		Size:      true,
		Animation: &MotionNavPanel,
	}
	PopupTransition = ui.ElementTransition{
		Duration:     MotionPopupOpenDur,
		Ease:         EaseStandard,
		ExitDuration: MotionPopupCloseDur,
		ExitEase:     EaseAccelerate,
		Position:     true,
		Enter:        &ui.Motion{Y: -2, Scale: 0.88, Flip: true},
		Exit:         &ui.Motion{Y: -1, Scale: 0.92, Flip: true},
	}
	OptionPopupTransition = ui.ElementTransition{
		Animation:    &MotionOptionSpring,
		ExitDuration: MotionOptionCloseDur,
		ExitEase:     EaseStandard,
		Enter:        &ui.Motion{Scale: 0.78, Opacity: 1},
		Exit:         &ui.Motion{Scale: 0.88, Opacity: 0},
	}
	ModalTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &MotionDialogPop,
		Enter: &ui.Motion{Y: 10, Scale: 0.94},
		Exit:  &ui.Motion{Y: 6, Scale: 0.96},
	}
	SheetTransition = ui.ElementTransition{
		Position: true, Colors: true, Animation: &MotionSheet,
		Enter: &ui.Motion{Y: 56},
		Exit:  &ui.Motion{Y: 40},
	}
	BackdropTransition = ui.ElementTransition{
		Animation: &MotionMove,
		Colors:    true,
		Enter:     &ui.Motion{},
		Exit:      &ui.Motion{},
	}

	ScaleReflow    bool
	StaticSnapshot bool
)

// BeginStaticSnapshot suppresses shared Match keys and entry/layout transitions
// while painting an off-frame snapshot (such as the previous theme reveal layer).
func BeginStaticSnapshot() func() {
	prev := StaticSnapshot
	StaticSnapshot = true
	return func() { StaticSnapshot = prev }
}

// GlassPressAnimation returns the spring for interactive Liquid Glass press or release.
func GlassPressAnimation(pressed bool) ui.Animation {
	if pressed {
		return MotionGlassPress
	}
	return MotionGlassRelease
}

func PillMatchTransition(matchKey any) ui.ElementTransition {
	return pillMatchTransition(matchKey, &MotionRail)
}

func pillMatchTransition(matchKey any, anim *ui.Animation) ui.ElementTransition {
	if StaticSnapshot {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position: true, Size: true, Colors: true,
		Animation: anim, Match: matchKey,
	}
}

func NavPanelTransition(matchKey any) ui.ElementTransition {
	if StaticSnapshot {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position:  true,
		Size:      true,
		Colors:    true,
		Shadows:   true,
		Animation: &MotionNavPanel,
		Match:     matchKey,
		Fly:       true,
	}
}

func NavToggleTransition(matchKey any) ui.ElementTransition {
	if StaticSnapshot {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{
		Position:  true,
		Size:      true,
		Colors:    true,
		Animation: &MotionNavToggle,
		Match:     matchKey,
		Fly:       true,
	}
}

// NavTabTransition staggers each row's spring slightly so later tabs follow
// their leader across top-bar and floating-sidebar morphs.
func NavTabTransition(index int, matchKey any) ui.ElementTransition {
	if StaticSnapshot {
		return ui.ElementTransition{}
	}
	anim := ui.Animation{
		Spring: ui.SpringOptions{
			Duration: time.Duration(300+index*26) * time.Millisecond,
			Bounce:   0.18,
		},
	}
	return ui.ElementTransition{
		Position:  true,
		Size:      true,
		Colors:    true,
		Animation: &anim,
		Match:     matchKey,
		Fly:       true,
	}
}

func LayoutAnimation(animation ui.Animation) ui.Animation {
	if ScaleReflow {
		return ui.Animation{Duration: time.Nanosecond}
	}
	return animation
}

// BeginFrame prepares the overlay tree and suppresses positional transitions
// while the window or viewport is actively resizing so layouts stay crisp.
func BeginFrame(ctx *ui.Context) func() {
	ui.Overlay(ctx, func() {})
	type frameState struct {
		w, h  float32
		until time.Time
	}
	w, h := ctx.Size()
	state := ui.Local(ui.Box(ctx).Absolute().Size(0, 0).PassThrough(), "frame-layout", func() frameState {
		return frameState{w: w, h: h}
	})
	if state.w != w || state.h != h {
		state.w, state.h = w, h
		state.until = ctx.Now().Add(MotionUnfoldDur)
	}
	ScaleReflow = ctx.Now().Before(state.until)
	if ScaleReflow {
		ctx.DisableAnimations()
		ctx.After(state.until.Sub(ctx.Now()))
	}
	return func() { ScaleReflow = false }
}

func NavigationPageMotion(el ui.Element, horizontal bool, direction int) ui.Element {
	if direction == 0 {
		direction = 1
	}
	enter := &ui.Motion{Opacity: 0.72}
	if horizontal {
		enter.X = float32(direction * 12)
	} else {
		enter.Y = float32(direction * 10)
	}
	return el.Transition(ui.ElementTransition{
		Position: true, Animation: &MotionPage, Enter: enter,
	})
}

func AnimatedLabel(ctx *ui.Context, text string) ui.Element {
	return ui.Text(ctx, text).ContentTransition(LocaleTextTransition)
}

func ControlMotion(el ui.Element) ui.Element {
	// Avoid Pose{Scale} on text-bearing controls so mygo glyph rasterization does not re-bucket fonts mid-press.
	return el.Transition(LayoutTransition)
}

func CardMotion(el ui.Element) ui.Element {
	pose := ui.Pose{}
	if el.Hovered() && !el.IsDisabled() && !el.Pressed() {
		pose.Y = -1
	}
	return el.TransformOrigin(0.5, 0.5).AnimatePose(pose, MotionSpring)
}

func PopupMotion(el ui.Element) ui.Element {
	return el.AttachedOrigin().Transition(PopupTransition)
}

func PopupMotionAt(el ui.Element, originX, originY float32) ui.Element {
	return el.TransformOrigin(originX, originY).Transition(PopupTransition)
}

func OptionMenuMotion(el ui.Element) ui.Element {
	return el.AttachedOrigin().Transition(OptionPopupTransition)
}

func ModalMotion(el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 0.5).Transition(ModalTransition)
}

func SheetMotion(el ui.Element) ui.Element {
	return el.TransformOrigin(0.5, 1).Transition(SheetTransition)
}

func BackdropMotion(el ui.Element, p Palette) ui.Element {
	bg := p.Backdrop
	if p.LiquidGlass {
		bg = ui.RGBA(8, 10, 14, .24)
	}
	return el.Background(bg).Transition(BackdropTransition)
}

func ScrolledEdge(el ui.Element, p Palette) ui.Element {
	color := p.LineSoft
	return el.DrawOver(func(painter *ui.Painter, rect ui.Rect) {
		painter.Fill(ui.Rect{X: rect.X, Y: rect.Y, W: rect.W, H: 1}, color, 0)
	})
}
